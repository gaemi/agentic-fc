// agenticfc is the core daemon: one process per world, hosting the Simulation
// Core, the MCP gateway, and the Console API.
//
// World bootstrap is currently CLI-flag based. A future Admin Mode can layer a
// Console workflow over the same world-generation path.
// World state persists as an atomic JSON snapshot (FR-28): the daemon
// resumes where it left off, and a crash mid-save leaves the previous
// snapshot intact. Rolls are audited to audit.jsonl (FR-29).
package main

import (
	"context"
	"crypto/rand"
	"encoding/binary"
	"encoding/csv"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"text/tabwriter"
	"time"

	"github.com/modelcontextprotocol/go-sdk/auth"
	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/gaemi/agentic-fc/internal/buildinfo"
	"github.com/gaemi/agentic-fc/internal/consoleapi"
	"github.com/gaemi/agentic-fc/internal/engine"
	"github.com/gaemi/agentic-fc/internal/mcpserver"
	"github.com/gaemi/agentic-fc/internal/narrative"
	"github.com/gaemi/agentic-fc/internal/sim"
	"github.com/gaemi/agentic-fc/internal/store"
	"github.com/gaemi/agentic-fc/internal/worldgen"
)

func main() {
	dataFlag := flag.String("data", "", "world data directory (default: ./data if it already holds a world, otherwise the OS user data directory)")
	consoleAddr := flag.String("console-addr", "127.0.0.1:7420", "Console API listen address")
	mcpAddr := flag.String("mcp-addr", "127.0.0.1:7421", "MCP listen address (HTTP transport)")
	preset := flag.String("preset", "classic", "world preset: compact|classic|deep|sprawling (new worlds only)")
	seed := flag.Uint64("seed", 0, "world seed, 0 = random (new worlds only)")
	worldName := flag.String("world-name", "", "world display name, empty = generated (new worlds only)")
	clubNames := flag.String("club-names", "", "comma-separated custom club names in generation order; appended before -club-names-file entries (new worlds only)")
	clubNamesFile := flag.String("club-names-file", "", "line-separated custom club names appended after -club-names entries (new worlds only)")
	managerNames := flag.String("manager-names", "", "comma-separated custom manager names in generation order; appended before -manager-names-file entries (new worlds only)")
	managerNamesFile := flag.String("manager-names-file", "", "line-separated custom manager names appended after -manager-names entries (new worlds only)")
	profile := flag.String("profile", "default", "run profile: default|fast|slow|custom (new worlds only)")
	speed := flag.Int("speed", 0, "override match speed: 5|15|30|60 (new worlds only)")
	idleAccel := flag.Int("idle-accel", 0, "override in-season idle acceleration: 2..64 × game speed (new worlds only)")
	offseasonAccel := flag.Int("offseason-accel", 0, "override off-season acceleration: 2..240 × game speed (new worlds only)")
	start := flag.Bool("start", false, "begin running immediately")
	snapshotEvery := flag.Duration("snapshot-interval", time.Minute, "periodic snapshot cadence (real time)")
	widgetMode := flag.String("widget-mode", "apps", "MCP UI mode: apps (official MCP Apps resource) | meta/content (compatibility fallbacks)")
	widgetLocale := flag.String("widget-locale", "", "MCP UI locale override: supported language tag (en/ko, e.g. ko-KR); empty = client/system language")
	mcpConfig := flag.Bool("mcp-config", false, "print ready-to-paste MCP client setup for this world and exit")
	mcpManager := flag.Int64("mcp-manager", 0, "manager id whose token -mcp-config embeds (default: first manifest entry)")
	versionFlag := flag.Bool("version", false, "print version and exit")
	flag.Parse()
	if *versionFlag {
		fmt.Println(buildinfo.String("agenticfc"))
		return
	}
	explicitFlags := map[string]bool{}
	flag.Visit(func(f *flag.Flag) { explicitFlags[f.Name] = true })

	if *mcpConfig {
		dataDir, err := resolveDataDir(*dataFlag)
		if err != nil {
			log.Fatal(err)
		}
		out, err := mcpConfigText(filepath.Join(dataDir, "manifest.json"), "http://"+dialableAddr(*mcpAddr), *mcpManager)
		if err != nil {
			log.Fatal(err)
		}
		fmt.Print(out)
		return
	}

	runProfile, err := resolveRunProfile(*profile, sim.Speed(*speed), explicitFlags["speed"],
		*idleAccel, explicitFlags["idle-accel"], *offseasonAccel, explicitFlags["offseason-accel"])
	if err != nil {
		log.Fatal(err)
	}

	// Bind both listen addresses before anything touches the data directory:
	// a busy port must fail the launch while the generation flags are still
	// unconsumed. Binding after generation would persist a fresh world during
	// the failed launch, and the corrected relaunch would silently resume it
	// with the creation flags ignored. Data-dir resolution comes after the
	// bind for the same reason: the common second-daemon launch must surface
	// the port hint, not whatever filesystem state that daemon happens to see.
	consoleLn, err := listenTCP("console api", "-console-addr", *consoleAddr)
	if err != nil {
		log.Fatal(err)
	}
	mcpLn, err := listenTCP("mcp gateway", "-mcp-addr", *mcpAddr)
	if err != nil {
		log.Fatal(err)
	}

	dataDir, err := resolveDataDir(*dataFlag)
	if err != nil {
		log.Fatal(err)
	}
	// Serve immediately so a client that connects while the world is still
	// loading gets a truthful 503 instead of hanging in the accept backlog;
	// the real handlers are installed below once the world is up.
	consoleHandler := &startupHandler{}
	mcpStartup := &startupHandler{}
	srv := &http.Server{Handler: consoleHandler}
	mcpHTTP := &http.Server{Handler: mcpStartup}
	go func() {
		if err := srv.Serve(consoleLn); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("console api: %v", err)
		}
	}()
	go func() {
		if err := mcpHTTP.Serve(mcpLn); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("mcp gateway: %v", err)
		}
	}()

	adminToken, firstLaunch, err := ensureAdminToken(dataDir)
	if err != nil {
		log.Fatalf("admin token: %v", err)
	}
	if firstLaunch {
		// Printed once at daemon first launch — this is how the operator
		// obtains Admin Mode before any world exists (docs/05 A7, FR-34).
		fmt.Printf("Admin Token (first launch — store it safely): %s\n", adminToken)
	} else {
		fmt.Println("Admin token loaded.")
	}

	fstore := &store.FileStore{Dir: dataDir}
	manifestPath := filepath.Join(dataDir, "manifest.json")

	loaded, err := loadOrGenerate(fstore, manifestPath, *preset, *seed, *worldName,
		*clubNames, *clubNamesFile, *managerNames, *managerNamesFile, runProfile, *start)
	if err != nil {
		log.Fatal(err)
	}
	world := loaded.world

	hub := consoleapi.NewHub(narrative.Default)
	eng := engine.New(world, loaded.queue, store.NewFileAuditLog(dataDir))
	eng.SetSink(hub)
	eng.ResumeAt(loaded.now)
	inputLog, err := store.NewFileInputLog(dataDir)
	if err != nil {
		log.Fatalf("input log: %v", err)
	}
	host := newWorldHost(world, eng, fstore, inputLog, loaded.creds, hub, world.Config)
	if loaded.resumed && inputLog.Seq() > loaded.lastIngressSeq {
		lost := inputLog.Seq() - loaded.lastIngressSeq
		// Inputs logged after the last snapshot are not reflected in the
		// resumed state. Full WAL replay is future recovery work; until then
		// this must be loud, never silent.
		log.Printf("WARNING: %d logged input(s) newer than the snapshot watermark "+
			"(seq %d > %d) are NOT reflected in the resumed world", lost,
			inputLog.Seq(), loaded.lastIngressSeq)
	}
	gateway := mcpserver.New(host, inputLog, narrative.Default, loaded.creds)
	gateway.SetWidgetMode(*widgetMode) // human-facing UI cards; locale follows the system language (FR-35c)
	if *widgetLocale != "" {
		loc, ok := narrative.TryResolveTag(*widgetLocale)
		if !ok {
			log.Fatalf("invalid -widget-locale %q: supported language tags resolve to en or ko", *widgetLocale)
		}
		gateway.Locale = loc
	}
	host.gateway = gateway // single owner of the cred set (admin listing + auth)
	eng.SetAlertSink(gateway)

	// The run intent (a resumed world returns to its persisted state; --start forces).
	// Used both to start the world below and as the manifest's start_state — computing
	// it here, before host.Start(), keeps a startup manifest rewrite from stamping
	// "ready" onto a world that is about to resume running. After host.Start()
	// this equals host.started.Load(), so the reconciler reads it safely
	// on the snapshot cadence too.
	willRun := *start || loaded.started || world.Config.StartRunning

	// Runtime token minting (FR-34): managers spawned mid-run — caretakers now, and
	// newgen backfills later — enter the world without a credential. The reconciler
	// diffs the world against the cred set, mints a token for each tokenless ACTIVE
	// manager, persists it to the manifest, THEN registers it for auth (persist
	// before expose — a token is never usable before it is durable). Tokens are
	// crypto-random and off-hash, so this never perturbs the world (NFR-2); it runs
	// outside the deterministic core, driven only from this single goroutine (the
	// startup backfill below and the snapshot cadence — never concurrently).
	reconcileTokens := func() {
		var pending []worldgen.ManagerCredential
		host.Locked(func() { pending = gateway.TokenlessManagers(host.World()) })
		if len(pending) == 0 {
			return
		}
		for i := range pending {
			token, err := worldgen.MintManagerToken(rand.Reader)
			if err != nil {
				log.Printf("token mint: %v", err) // retried next reconcile
				return
			}
			pending[i].Token = token
		}
		manifest := manifestFrom(world, willRun, append(gateway.Credentials(), pending...))
		if err := writeManifest(manifestPath, manifest); err != nil {
			log.Printf("token reconcile: manifest write failed, deferring: %v", err)
			return // not exposed — stays tokenless, retried next reconcile
		}
		gateway.AddCredentials(pending)
		for _, c := range pending {
			log.Printf("minted Manager Token for %s (manager %d, %s)", c.ManagerName, c.ManagerID, c.ClubName)
		}
	}
	// If loading pruned orphan credentials (a crash left the manifest ahead of the
	// resumed snapshot — Gateway.New drops creds whose manager is gone), purge them
	// from the durable manifest now so disk agrees with the in-memory cred set.
	// Startup, pre-serving — no race.
	if len(gateway.Credentials()) < len(loaded.creds) {
		if err := writeManifest(manifestPath, manifestFrom(world, willRun, gateway.Credentials())); err != nil {
			log.Printf("manifest orphan purge: %v", err)
		}
	}
	// Backfill any tokenless managers a resumed snapshot already carries (a prior run
	// may have installed caretakers after its last manifest write, or crashed between
	// the snapshot and the manifest rewrite).
	reconcileTokens()

	// Snapshot promptly after accepted agent inputs — shrinks the
	// logged-but-unsnapshotted crash window to roughly the debounce.
	dirty := make(chan struct{}, 1)
	gateway.OnAccepted = func() {
		select {
		case dirty <- struct{}{}:
		default:
		}
	}

	hash, _ := world.Hash()
	fmt.Printf("world: %s · seed %d · %d clubs · %s · hash %s…\n",
		world.Config.Name, world.Config.Seed, len(world.Clubs),
		eng.Now(), hash[:12])
	fmt.Printf("pacing: profile %s · match %dx · idle %dx · offseason %dx\n",
		configRunProfile(world.Config), world.Config.GameSpeed,
		world.Config.IdleAcceleration, world.Config.OffseasonAccel)
	fmt.Printf("data: %s\n", dataDir)
	fmt.Printf("manager tokens: %s (0600)\n", manifestPath)
	// The listeners' addresses, not the flag values: with a ":0" flag this is
	// where the picked port becomes visible.
	consoleURL := "http://" + dialableAddr(consoleLn.Addr().String())
	mcpURL := "http://" + dialableAddr(mcpLn.Addr().String())
	fmt.Printf("Console API: %s  ·  MCP: %s\n", consoleURL, mcpURL)
	// Repeat only the flags that change what -mcp-config would print, so the
	// suggested command works verbatim for this exact launch.
	connectHint := "agenticfc -mcp-config"
	if explicitFlags["data"] {
		connectHint += " -data " + *dataFlag
	}
	if mcpURL != "http://"+dialableAddr(*mcpAddr) {
		// A ":0" flag landed on an OS-picked port -mcp-config cannot re-derive.
		connectHint += " -mcp-addr " + dialableAddr(mcpLn.Addr().String())
	} else if explicitFlags["mcp-addr"] {
		connectHint += " -mcp-addr " + *mcpAddr
	}
	fmt.Printf("connect an AI agent: %s\n", connectHint)

	if willRun {
		if err := host.Start(); err != nil {
			log.Fatalf("start: %v", err)
		}
	}
	fmt.Printf("state: %s\n", host.State())
	if host.State() == "ready" {
		fmt.Println("the world clock is stopped until the world is started.")
		fmt.Println("start it by relaunching with -start, or through the Console API:")
		fmt.Printf("  curl -X POST %s/v1/admin/start -H \"Authorization: Bearer <token from %s>\"\n",
			consoleURL, filepath.Join(dataDir, "admin.token"))
	}

	api := &consoleapi.Server{
		AdminToken: adminToken,
		Host:       host,
		Feed:       hub,
		Catalogs:   narrative.Default,
	}
	consoleHandler.Set(api.Routes())

	// MCP gateway: Manager-Token bearer auth in front of the streamable
	// transport (docs/04 §0; INVALID_TOKEN surfaces as 401 here).
	mcpSrv := gateway.MCPServer()
	mcpStartup.Set(auth.RequireBearerToken(gateway.VerifyToken, nil)(
		mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server { return mcpSrv }, nil)))

	// Periodic snapshots: cheap insurance between the pause/shutdown saves.
	snapCtx, snapCancel := context.WithCancel(context.Background())
	go func() {
		ticker := time.NewTicker(*snapshotEvery)
		defer ticker.Stop()
		for {
			select {
			case <-snapCtx.Done():
				return
			case <-ticker.C:
			case <-dirty:
				// Debounce: absorb the burst, then persist once.
				time.Sleep(2 * time.Second)
				for {
					select {
					case <-dirty:
						continue
					default:
					}
					break
				}
			}
			// Mint tokens for any managers the runner spawned since the last pass
			// (caretakers on a sacking), then persist — same cadence, one goroutine.
			reconcileTokens()
			if err := host.SaveSnapshot(); err != nil {
				log.Printf("snapshot: %v", err)
			}
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	<-stop
	fmt.Println("shutting down")
	snapCancel()
	host.Shutdown()
	if err := host.SaveSnapshot(); err != nil {
		log.Printf("final snapshot: %v", err)
	}
	_ = mcpHTTP.Close()
	_ = srv.Close()
}

type loadResult struct {
	world          *worldgen.World
	queue          *sim.Queue
	creds          []worldgen.ManagerCredential
	now            sim.GameTime
	started        bool
	resumed        bool
	lastIngressSeq uint64
}

type runProfile struct {
	Name                  string
	Speed                 sim.Speed
	IdleAcceleration      int
	OffseasonAcceleration int
}

func resolveRunProfile(name string, speed sim.Speed, speedSet bool, idleAccel int, idleSet bool,
	offseasonAccel int, offseasonSet bool) (runProfile, error) {

	p, err := baseRunProfile(name)
	if err != nil {
		return runProfile{}, err
	}
	if speedSet {
		p.Speed = speed
	}
	if idleSet {
		p.IdleAcceleration = idleAccel
	}
	if offseasonSet {
		p.OffseasonAcceleration = offseasonAccel
	}
	if speedSet || idleSet || offseasonSet {
		p.Name = "custom"
	}
	cfg := worldgen.DefaultConfig(1)
	cfg.RunProfile = p.Name
	cfg.GameSpeed = p.Speed
	cfg.IdleAcceleration = p.IdleAcceleration
	cfg.OffseasonAccel = p.OffseasonAcceleration
	if err := cfg.Validate(); err != nil {
		return runProfile{}, err
	}
	return p, nil
}

func baseRunProfile(name string) (runProfile, error) {
	switch name {
	case "default":
		return runProfile{
			Name:                  "default",
			Speed:                 sim.Speed15,
			IdleAcceleration:      sim.DefaultIdleAcceleration,
			OffseasonAcceleration: sim.DefaultOffseasonAcceleration,
		}, nil
	case "fast":
		return runProfile{
			Name:                  "fast",
			Speed:                 sim.Speed30,
			IdleAcceleration:      32,
			OffseasonAcceleration: 192,
		}, nil
	case "slow":
		return runProfile{
			Name:                  "slow",
			Speed:                 sim.Speed15,
			IdleAcceleration:      6,
			OffseasonAcceleration: 36,
		}, nil
	case "custom":
		return runProfile{
			Name:                  "custom",
			Speed:                 sim.Speed15,
			IdleAcceleration:      sim.DefaultIdleAcceleration,
			OffseasonAcceleration: sim.DefaultOffseasonAcceleration,
		}, nil
	default:
		return runProfile{}, fmt.Errorf("unknown run profile %q", name)
	}
}

func configRunProfile(cfg worldgen.WorldConfig) string {
	if cfg.RunProfile == "" {
		return "custom"
	}
	return cfg.RunProfile
}

// loadOrGenerate resumes the persisted world if one exists (FR-28) —
// generation flags are ignored then — or generates a fresh one.
//
// Creation order is snapshot FIRST, manifest second: if the process dies
// between the two, the next launch resumes the world (missing manifest is
// a loud warning, and tokens are recoverable via admin regeneration,
// FR-34a). The reverse order would let a crash silently regenerate a
// different world — an FR-28a violation.
func loadOrGenerate(fstore *store.FileStore, manifestPath, preset string,
	seed uint64, worldName, clubNames, clubNamesFile, managerNames, managerNamesFile string,
	profile runProfile, start bool) (*loadResult, error) {

	snap, err := fstore.LoadSnapshot()
	if err != nil {
		return nil, fmt.Errorf("loading snapshot: %w", err)
	}
	if snap != nil {
		creds, err := readManifestCredentials(manifestPath)
		if err != nil {
			log.Printf("manifest: %v (admin manager listing will be empty)", err)
		}
		fmt.Println("world: resumed from snapshot (generation flags ignored)")
		return &loadResult{
			world:          snap.World,
			queue:          sim.RestoreQueue(snap.Queue, snap.QueueNextSeq),
			creds:          creds,
			now:            snap.Now,
			started:        snap.Started,
			resumed:        true,
			lastIngressSeq: snap.LastIngressSeq,
		}, nil
	}

	if seed == 0 {
		var b [8]byte
		if _, err := rand.Read(b[:]); err != nil {
			return nil, fmt.Errorf("rolling seed: %w", err)
		}
		seed = binary.LittleEndian.Uint64(b[:])
	}
	cfg, err := presetConfig(preset, seed)
	if err != nil {
		return nil, err
	}
	overrides, err := parseNameOverrides(clubNames, clubNamesFile, managerNames, managerNamesFile)
	if err != nil {
		return nil, err
	}
	cfg.Name = worldName
	cfg.NameOverrides = overrides
	cfg.RunProfile = profile.Name
	cfg.GameSpeed = profile.Speed
	cfg.IdleAcceleration = profile.IdleAcceleration
	cfg.OffseasonAccel = profile.OffseasonAcceleration
	cfg.StartRunning = start

	res, err := worldgen.Generate(cfg)
	if err != nil {
		return nil, fmt.Errorf("world generation: %w", err)
	}
	events, nextSeq := res.Queue.Snapshot()
	if err := fstore.SaveSnapshot(&store.Snapshot{
		Now: 0, World: res.World, Queue: events, QueueNextSeq: nextSeq,
	}); err != nil {
		return nil, fmt.Errorf("initial snapshot: %w", err)
	}
	if err := writeManifest(manifestPath, res.Manifest); err != nil {
		return nil, fmt.Errorf("manifest: %w", err)
	}
	return &loadResult{
		world: res.World,
		queue: res.Queue,
		creds: res.Manifest.Managers,
	}, nil
}

func parseNameOverrides(clubInline, clubFile, managerInline, managerFile string) (worldgen.NameOverrides, error) {
	clubNames, err := parseNameListFlag(clubInline, clubFile)
	if err != nil {
		return worldgen.NameOverrides{}, fmt.Errorf("club names: %w", err)
	}
	managerNames, err := parseNameListFlag(managerInline, managerFile)
	if err != nil {
		return worldgen.NameOverrides{}, fmt.Errorf("manager names: %w", err)
	}
	return worldgen.NameOverrides{ClubNames: clubNames, ManagerNames: managerNames}, nil
}

func parseNameListFlag(inline, file string) ([]string, error) {
	var names []string
	if strings.TrimSpace(inline) != "" {
		r := csv.NewReader(strings.NewReader(inline))
		r.TrimLeadingSpace = true
		fields, err := r.Read()
		if err != nil {
			return nil, err
		}
		more, err := r.Read()
		switch {
		case err == nil:
			return nil, fmt.Errorf("expected one CSV record, got extra record %q", strings.Join(more, ","))
		case err == io.EOF:
		default:
			return nil, err
		}
		names = append(names, fields...)
	}
	file = strings.TrimSpace(file)
	if file != "" {
		b, err := os.ReadFile(file)
		if err != nil {
			return nil, err
		}
		for _, line := range strings.Split(string(b), "\n") {
			line = strings.TrimSpace(line)
			if line == "" || strings.HasPrefix(line, "#") {
				continue
			}
			names = append(names, line)
		}
	}
	return names, nil
}

func presetConfig(name string, seed uint64) (worldgen.WorldConfig, error) {
	switch name {
	case "compact":
		return worldgen.PresetCompact(seed), nil
	case "classic":
		return worldgen.PresetClassic(seed), nil
	case "deep":
		return worldgen.PresetDeep(seed), nil
	case "sprawling":
		return worldgen.PresetSprawling(seed), nil
	default:
		return worldgen.WorldConfig{}, fmt.Errorf("unknown preset %q", name)
	}
}

// manifestFrom assembles the credential handover for the current world + cred set.
// The metadata is derivable from the world, so a manifest rewrite (runtime token
// mint, orphan purge) never needs to re-read the prior file — the credential set is
// the only moving part.
func manifestFrom(w *worldgen.World, started bool, creds []worldgen.ManagerCredential) *worldgen.Manifest {
	startState := "ready"
	if started {
		startState = "running"
	}
	return &worldgen.Manifest{
		WorldName:  w.Config.Name,
		Seed:       w.Config.Seed,
		StartState: startState,
		Managers:   creds,
	}
}

// writeManifest persists the credential handover (0600 — it holds tokens).
// The tokens go into a brand-new random-named 0600 temp file (CreateTemp
// guarantees both) which is renamed over the target: no pre-existing file
// of any permission ever holds the credentials for any window.
func writeManifest(path string, m *worldgen.Manifest) error {
	b, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}
	f, err := os.CreateTemp(filepath.Dir(path), ".manifest-*.tmp")
	if err != nil {
		return err
	}
	tmp := f.Name()
	if _, err := f.Write(b); err != nil {
		f.Close()
		os.Remove(tmp)
		return err
	}
	if err := f.Sync(); err != nil { // durable before the rename
		f.Close()
		os.Remove(tmp)
		return err
	}
	if err := f.Close(); err != nil {
		os.Remove(tmp)
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		os.Remove(tmp)
		return err
	}
	d, err := os.Open(filepath.Dir(path))
	if err != nil {
		return err
	}
	defer d.Close()
	return d.Sync()
}

func readManifestCredentials(path string) ([]worldgen.ManagerCredential, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var m worldgen.Manifest
	if err := json.Unmarshal(b, &m); err != nil {
		return nil, err
	}
	return m.Managers, nil
}

// mcpConfigText renders ready-to-paste MCP client setup for one Manager of
// the world whose manifest lives at manifestPath. The endpoint comes from
// the -mcp-addr flag value, so the daemon does not need to be running.
// managerID 0 picks the first manifest entry.
func mcpConfigText(manifestPath, mcpURL string, managerID int64) (string, error) {
	creds, err := readManifestCredentials(manifestPath)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return "", fmt.Errorf("no world manifest at %s — launch the daemon once to create a world (agenticfc -start)", manifestPath)
		}
		return "", err
	}
	if len(creds) == 0 {
		return "", fmt.Errorf("manifest %s lists no managers", manifestPath)
	}
	pick := creds[0]
	if managerID != 0 {
		found := false
		for _, c := range creds {
			if c.ManagerID == managerID {
				pick, found = c, true
				break
			}
		}
		if !found {
			return "", fmt.Errorf("manager %d is not in %s — run -mcp-config without -mcp-manager to list ids", managerID, manifestPath)
		}
	}

	var b strings.Builder
	fmt.Fprintf(&b, "manager tokens: %s\n\n", manifestPath)
	fmt.Fprintf(&b, "Managers in this world (the token binds the agent to one Manager):\n")
	tw := tabwriter.NewWriter(&b, 2, 0, 2, ' ', 0)
	fmt.Fprintf(tw, "\tID\tMANAGER\tCLUB\n")
	for _, c := range creds {
		mark := ""
		if c.ManagerID == pick.ManagerID {
			mark = "*"
		}
		club := c.ClubName
		if c.ClubID == 0 {
			club = "(unemployed)"
		}
		fmt.Fprintf(tw, "%s\t%d\t%s\t%s\n", mark, c.ManagerID, c.ManagerName, club)
	}
	tw.Flush()
	fmt.Fprintf(&b, "* = used below; choose another with -mcp-config -mcp-manager <id>\n\n")
	fmt.Fprintf(&b, "MCP endpoint (the daemon must be running): %s\n\n", mcpURL)
	fmt.Fprintf(&b, "Claude Code:\n")
	fmt.Fprintf(&b, "  claude mcp add --transport http agentic-fc %s --header \"Authorization: Bearer %s\"\n\n", mcpURL, pick.Token)
	type serverEntry struct {
		Type    string            `json:"type"`
		URL     string            `json:"url"`
		Headers map[string]string `json:"headers"`
	}
	cfgJSON, err := json.MarshalIndent(map[string]map[string]serverEntry{
		"mcpServers": {"agentic-fc": {
			Type:    "http",
			URL:     mcpURL,
			Headers: map[string]string{"Authorization": "Bearer " + pick.Token},
		}},
	}, "  ", "  ")
	if err != nil {
		return "", err
	}
	fmt.Fprintf(&b, "Any MCP client (JSON config):\n  %s\n\n", cfgJSON)
	fmt.Fprintf(&b, "First tool calls for a fresh agent: get_guide, get_settings, get_time, get_situation, get_mindset\n")
	return b.String(), nil
}

// worldHost owns the running world: the runner goroutine is the single
// writer; the Console API reads under the shared RWMutex (docs/05 A1).
type worldHost struct {
	mu      sync.RWMutex
	snapMu  sync.Mutex // serializes snapshot writers
	world   *worldgen.World
	eng     *engine.Engine
	runner  *engine.Runner
	fstore  *store.FileStore
	inputs  store.InputLog
	creds   []worldgen.ManagerCredential
	gateway *mcpserver.Gateway // single owner of the runtime cred set (set post-construction)
	hub     *consoleapi.Hub

	started atomic.Bool
	cancel  context.CancelFunc
	done    chan struct{}
}

func newWorldHost(world *worldgen.World, eng *engine.Engine, fstore *store.FileStore,
	inputs store.InputLog, creds []worldgen.ManagerCredential, hub *consoleapi.Hub,
	cfg worldgen.WorldConfig) *worldHost {

	h := &worldHost{
		world:  world,
		eng:    eng,
		fstore: fstore,
		inputs: inputs,
		creds:  creds,
		hub:    hub,
		done:   make(chan struct{}),
	}
	h.runner = engine.NewRunner(
		eng,
		engine.Pacer{
			Speed:                 cfg.GameSpeed,
			IdleAcceleration:      cfg.IdleAcceleration,
			OffseasonAcceleration: cfg.OffseasonAccel,
		},
		engine.SleepReal,
		&h.mu,
	)
	return h
}

func (h *worldHost) Locked(read func()) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	read()
}

// LockedWrite serializes MCP intent writes with the runner (docs/05 A1:
// world systems have one writer; MCP touches only Mindset/Focus state).
func (h *worldHost) LockedWrite(fn func()) {
	h.mu.Lock()
	defer h.mu.Unlock()
	fn()
}

// Paused reports frozen game time: never-started worlds and the admin
// maintenance pause both halt the clock (and with it, Focus regen).
func (h *worldHost) Paused() bool {
	return !h.started.Load() || h.runner.Paused()
}

// RealUntil estimates wall-clock time until game time t at current pacing.
func (h *worldHost) RealUntil(t sim.GameTime) time.Duration {
	return h.eng.RealDuration(h.runner.PacerSnapshot(), h.eng.Now(), t)
}

func (h *worldHost) World() *worldgen.World { return h.world }
func (h *worldHost) Engine() *engine.Engine { return h.eng }

func (h *worldHost) State() string {
	switch {
	case !h.started.Load():
		return "ready"
	case h.runner.Paused():
		return "paused"
	default:
		return "running"
	}
}

func (h *worldHost) Tempo() sim.Tempo {
	if !h.started.Load() || h.runner.Paused() {
		return sim.TempoPaused
	}
	return h.eng.TempoAt(h.eng.Now())
}

func (h *worldHost) Start() error {
	if !h.started.CompareAndSwap(false, true) {
		return errors.New("world already started")
	}
	ctx, cancel := context.WithCancel(context.Background())
	h.cancel = cancel
	go func() {
		defer close(h.done)
		// Run to the far horizon; the daemon lives until signalled.
		if err := h.runner.Run(ctx, sim.GameTime(1)<<62); err != nil &&
			!errors.Is(err, context.Canceled) {
			log.Printf("runner stopped: %v", err)
		}
	}()
	return nil
}

func (h *worldHost) SetPaused(p bool) error {
	if !h.started.Load() {
		return errors.New("world not started")
	}
	h.runner.SetPaused(p)
	if p {
		// A paused world is a safe point — snapshot it (FR-28).
		if err := h.SaveSnapshot(); err != nil {
			log.Printf("pause snapshot: %v", err)
		}
	}
	return nil
}

func (h *worldHost) RuntimeSettings() consoleapi.RuntimeSettings {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.runtimeSettingsLocked()
}

func (h *worldHost) runtimeSettingsLocked() consoleapi.RuntimeSettings {
	return consoleapi.RuntimeSettings{
		GameSpeed:             h.world.Config.GameSpeed,
		IdleAcceleration:      h.world.Config.IdleAcceleration,
		OffseasonAcceleration: h.world.Config.OffseasonAccel,
	}
}

func (h *worldHost) UpdateRuntimeSettings(update consoleapi.RuntimeSettingsUpdater) (consoleapi.RuntimeSettings, error) {
	h.snapMu.Lock()
	defer h.snapMu.Unlock()

	h.mu.Lock()
	previous := h.runtimeSettingsLocked()
	settings, err := update(previous)
	if err != nil {
		h.mu.Unlock()
		return previous, err
	}
	h.world.Config.GameSpeed = settings.GameSpeed
	h.world.Config.IdleAcceleration = settings.IdleAcceleration
	h.world.Config.OffseasonAccel = settings.OffseasonAcceleration
	if err := h.saveSnapshotWithWorldLocked(); err != nil {
		log.Printf("runtime settings snapshot: %v", err)
		h.world.Config.GameSpeed = previous.GameSpeed
		h.world.Config.IdleAcceleration = previous.IdleAcceleration
		h.world.Config.OffseasonAccel = previous.OffseasonAcceleration
		h.mu.Unlock()
		return previous, err
	}
	h.runner.SetPacer(engine.Pacer{
		Speed:                 settings.GameSpeed,
		IdleAcceleration:      settings.IdleAcceleration,
		OffseasonAcceleration: settings.OffseasonAcceleration,
	})
	h.mu.Unlock()
	return settings, nil
}

func (h *worldHost) Seed() uint64 { return h.world.Config.Seed }

// Credentials delegates to the gateway once it is wired — the gateway is the
// single owner of the mutable cred set (runtime spawns append to it), so the admin
// listing and MCP auth never drift. Before wiring (a window with no serving), the
// generation-time creds are the source.
func (h *worldHost) Credentials() []worldgen.ManagerCredential {
	if h.gateway != nil {
		return h.gateway.Credentials()
	}
	return h.creds
}

// SaveSnapshot persists the current state under the read lock (the runner
// is the only writer; RLock excludes it mid-step). snapMu serializes
// concurrent savers (periodic timer vs pause vs shutdown) so a later
// state can never be overwritten by an earlier one still in flight.
func (h *worldHost) SaveSnapshot() error {
	h.snapMu.Lock()
	defer h.snapMu.Unlock()
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.saveSnapshotWithWorldLocked()
}

func (h *worldHost) saveSnapshotWithWorldLocked() error {
	events, nextSeq := h.eng.Queue().Snapshot()
	return h.fstore.SaveSnapshot(&store.Snapshot{
		Now:            h.eng.Now(),
		Started:        h.started.Load(),
		World:          h.world,
		Queue:          events,
		QueueNextSeq:   nextSeq,
		LastIngressSeq: h.inputs.Seq(),
	})
}

// Shutdown stops the runner (if running) and waits for it to exit.
func (h *worldHost) Shutdown() {
	if h.cancel != nil {
		h.cancel()
		<-h.done
	}
}

// startupHandler answers 503 until the real handler is installed, so the
// early-bound listeners never leave a client hanging while the world loads.
type startupHandler struct {
	h atomic.Pointer[http.Handler]
}

func (s *startupHandler) Set(h http.Handler) { s.h.Store(&h) }

func (s *startupHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	h := s.h.Load()
	if h == nil {
		w.Header().Set("Retry-After", "1")
		http.Error(w, "agenticfc is starting up", http.StatusServiceUnavailable)
		return
	}
	(*h).ServeHTTP(w, r)
}

// dialableAddr rewrites a wildcard bind address (":7420", "0.0.0.0:…",
// "[::]:…") to its loopback equivalent so banner URLs and copy-paste hints
// always point somewhere a local client can actually dial. The family is
// preserved: an IPv6 wildcard maps to ::1, because on IPV6_V6ONLY hosts the
// listener is not reachable via 127.0.0.1 (and ::1 also works dual-stack).
func dialableAddr(addr string) string {
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		return addr
	}
	ip := net.ParseIP(host)
	switch {
	case host == "" || (ip != nil && ip.IsUnspecified() && ip.To4() != nil):
		return net.JoinHostPort("127.0.0.1", port)
	case ip != nil && ip.IsUnspecified():
		return net.JoinHostPort("::1", port)
	}
	return addr
}

// resolveDataDir picks the world data directory when -data is not given.
// A ./data that already holds a world keeps working (the source-checkout
// layout used by the docs and Makefile); otherwise the per-user OS data
// directory is used, so a packaged binary run from any working directory
// always finds the same world.
func resolveDataDir(flagValue string) (string, error) {
	if flagValue != "" {
		return flagValue, nil
	}
	local, err := isWorldDataDir("data")
	if err != nil {
		// ./data may hold a world we cannot inspect (permissions broken by a
		// sudo run, another user's checkout). Falling back would silently
		// split the operator's state across two locations — fail instead.
		return "", fmt.Errorf("resolving data directory: %w (fix permissions or pass -data)", err)
	}
	if local {
		return "./data", nil
	}
	// Not adopted, but present: say so once, loudly. This covers both an
	// unrelated project's data/ folder and the corner case of a first launch
	// that died before its world snapshot (only admin.token written) — the
	// latter is deliberately not adopted, because adopting snapshot-less
	// directories would reopen generating a fresh world into a foreign dir.
	if fi, err := os.Stat("data"); err == nil && fi.IsDir() {
		log.Printf("note: ./data exists but holds no world snapshot; " +
			"using the per-user data directory (pass -data ./data to override)")
	}
	dir, err := userDataDir(runtime.GOOS, os.Getenv, os.UserHomeDir)
	if err != nil {
		// No resolvable per-user location (HOME/XDG/LocalAppData unset —
		// service-style launches under systemd, cron, or minimal containers).
		// Keep the historical ./data default there instead of refusing to
		// start; the environments without a home directory are exactly the
		// ones that ran from a fixed working directory before.
		log.Printf("data directory: %v — falling back to ./data", err)
		return "./data", nil
	}
	return dir, nil
}

// isWorldDataDir reports whether dir holds a resumable Agentic FC world: a
// world.json snapshot accompanied by manifest.json or admin.token. A bare
// directory named data is not evidence, and neither is a single generically
// named file — an installed binary launched from an unrelated project must
// not adopt (and chmod 0700, or worse, generate a fresh world over) that
// project's data/. Requiring the snapshot means an adopted ./data is only
// ever resumed, never written to from scratch.
func isWorldDataDir(dir string) (bool, error) {
	fi, err := os.Stat(dir)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		return false, nil
	case err != nil:
		return false, err
	case !fi.IsDir():
		return false, nil
	}
	world, err := hasFile(dir, "world.json")
	if err != nil || !world {
		return false, err
	}
	for _, companion := range []string{"manifest.json", "admin.token"} {
		ok, err := hasFile(dir, companion)
		if err != nil || ok {
			return ok, err
		}
	}
	return false, nil
}

func hasFile(dir, name string) (bool, error) {
	fi, err := os.Stat(filepath.Join(dir, name))
	switch {
	case err == nil:
		return !fi.IsDir(), nil
	case errors.Is(err, fs.ErrNotExist):
		return false, nil
	default:
		return false, err
	}
}

// userDataDir is the per-OS conventional location for game saves. The data
// directory holds world state, logs, and tokens — data, not configuration —
// hence XDG_DATA_HOME rather than the config directory on Linux.
func userDataDir(goos string, getenv func(string) string, home func() (string, error)) (string, error) {
	switch goos {
	case "darwin":
		h, err := absHome(home)
		if err != nil {
			return "", err
		}
		return filepath.Join(h, "Library", "Application Support", "agenticfc"), nil
	case "windows":
		if d := getenv("LocalAppData"); d != "" && filepath.IsAbs(d) {
			return filepath.Join(d, "agenticfc"), nil
		}
		return "", errors.New("LocalAppData is not set to an absolute path")
	default:
		// The XDG spec requires XDG_DATA_HOME to be absolute; a relative
		// value would make the world location depend on the working
		// directory again, so it is ignored like other spec violations.
		if d := getenv("XDG_DATA_HOME"); d != "" && filepath.IsAbs(d) {
			return filepath.Join(d, "agenticfc"), nil
		}
		h, err := absHome(home)
		if err != nil {
			return "", err
		}
		return filepath.Join(h, ".local", "share", "agenticfc"), nil
	}
}

// absHome resolves the user home and rejects relative values: on Unix,
// os.UserHomeDir returns $HOME verbatim, and a relative home would make the
// "per-user" path depend on the working directory after all.
func absHome(home func() (string, error)) (string, error) {
	h, err := home()
	if err != nil {
		return "", fmt.Errorf("user home: %w", err)
	}
	if !filepath.IsAbs(h) {
		return "", fmt.Errorf("user home %q is not an absolute path", h)
	}
	return h, nil
}

// listenTCP binds a daemon listen address, translating the launch failure a
// new operator actually hits — the port is taken, or the address is
// malformed — into a message that names the flag to change.
func listenTCP(name, flagName, addr string) (net.Listener, error) {
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return nil, fmt.Errorf("%s: cannot listen on %s: %w\n"+
			"another process may already be using this address (another agenticfc daemon?)\n"+
			"stop that process, or pick a different port with %s (%q picks a free one at random)",
			name, addr, err, flagName, flagName+" 127.0.0.1:0")
	}
	return ln, nil
}

// ensureAdminToken generates the Admin Token at daemon first launch and
// persists it (0600) in the data directory; later launches reuse it.
func ensureAdminToken(dataDir string) (token string, firstLaunch bool, err error) {
	if err := os.MkdirAll(dataDir, 0o700); err != nil {
		return "", false, err
	}
	// MkdirAll doesn't chmod an existing directory; the data dir holds
	// tokens and the full hidden-state snapshot, so re-tighten it.
	if err := os.Chmod(dataDir, 0o700); err != nil {
		return "", false, err
	}
	path := filepath.Join(dataDir, "admin.token")
	if b, err := os.ReadFile(path); err == nil && len(b) > 0 {
		// Re-tighten in case the mode drifted since creation.
		if err := os.Chmod(path, 0o600); err != nil {
			return "", false, err
		}
		return string(b), false, nil
	}
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", false, err
	}
	token = hex.EncodeToString(buf)
	if err := os.WriteFile(path, []byte(token), 0o600); err != nil {
		return "", false, err
	}
	if err := os.Chmod(path, 0o600); err != nil {
		return "", false, err
	}
	return token, true, nil
}
