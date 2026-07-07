package mcpserver

import (
	"context"
	"strconv"
	"testing"

	"github.com/gaemi/agentic-fc/internal/engine"
	"github.com/gaemi/agentic-fc/internal/narrative"
	"github.com/gaemi/agentic-fc/internal/rng"
	"github.com/gaemi/agentic-fc/internal/store"
	"github.com/gaemi/agentic-fc/internal/worldgen"
)

// TestRuntimeTokenMinting locks the B1 token seam (FR-34): a manager spawned at
// runtime (a caretaker here, as a sacking would) starts tokenless; the reconciler
// surfaces it via TokenlessManagers, the daemon mints + registers a token through
// AddCredentials, and the manager then authenticates through VerifyToken — while the
// world hash is untouched (tokens are off-hash, minted outside the deterministic
// core, NFR-2).
func TestRuntimeTokenMinting(t *testing.T) {
	g, host, _, manifest := newGateway(t)
	initialCreds := len(manifest.Managers)

	// Spawn a caretaker — it enters the world with no credential.
	var newID, clubID int64
	host.LockedWrite(func() {
		w := g.Host.World()
		clubID = w.Clubs[0].ID
		ct := worldgen.SpawnManager(w, rng.Stream(w.Config.Seed, "career/spawn/token-test"), clubID, 1, true)
		newID = ct.ID
	})

	// The reconciler sees exactly the caretaker, with its club carried over and no token.
	pending := g.TokenlessManagers(g.Host.World())
	if len(pending) != 1 || pending[0].ManagerID != newID {
		t.Fatalf("TokenlessManagers = %+v, want exactly the caretaker (id %d)", pending, newID)
	}
	if pending[0].ClubID != clubID || pending[0].Token != "" {
		t.Fatalf("pending cred malformed: %+v (token must be unset, club must carry over)", pending[0])
	}

	// The hash BEFORE minting — minting must not move it.
	beforeHash, err := g.Host.World().Hash()
	if err != nil {
		t.Fatal(err)
	}

	// Mint deterministically and register (the manifest persist is daemon glue).
	reader := &tokens{n: 9000}
	for i := range pending {
		tok, err := worldgen.MintManagerToken(reader)
		if err != nil {
			t.Fatal(err)
		}
		pending[i].Token = tok
	}
	g.AddCredentials(pending)

	// The caretaker now authenticates as itself.
	info, err := g.VerifyToken(context.Background(), pending[0].Token, nil)
	if err != nil {
		t.Fatalf("minted token rejected: %v", err)
	}
	if info.UserID != strconv.FormatInt(newID, 10) {
		t.Fatalf("token maps to UserID %q, want manager %d", info.UserID, newID)
	}

	// Minting is off-hash: the world is byte-identical before and after.
	afterHash, err := g.Host.World().Hash()
	if err != nil {
		t.Fatal(err)
	}
	if beforeHash != afterHash {
		t.Fatalf("world hash moved during token minting (tokens must be off-hash):\n%s\n%s", beforeHash, afterHash)
	}

	// The cred set grew by one, and a second pass is a no-op (idempotent reconcile).
	if got := len(g.Credentials()); got != initialCreds+1 {
		t.Fatalf("Credentials() = %d, want %d", got, initialCreds+1)
	}
	if again := g.TokenlessManagers(g.Host.World()); len(again) != 0 {
		t.Fatalf("second reconcile found %d tokenless managers, want 0 (idempotent)", len(again))
	}
}

// TestRuntimeTokenMintingSkipsRetired locks that the reconciler never mints a token
// for a RETIRED manager (a displaced caretaker that retired before the next pass) —
// a dead token would only be minted to sit unusable.
func TestRuntimeTokenMintingSkipsRetired(t *testing.T) {
	g, host, _, _ := newGateway(t)

	var retiredID int64
	host.LockedWrite(func() {
		w := g.Host.World()
		ct := worldgen.SpawnManager(w, rng.Stream(w.Config.Seed, "career/spawn/retired-test"), 0, 1, true)
		ct.Status = worldgen.ManagerRetired
		retiredID = ct.ID
	})

	for _, c := range g.TokenlessManagers(g.Host.World()) {
		if c.ManagerID == retiredID {
			t.Fatalf("reconciler surfaced a RETIRED manager (id %d) for token minting", retiredID)
		}
	}
}

// TestGatewayPrunesOrphanCredentials locks the crash-consistency fix:
// the runtime reconciler can leave the durable manifest one caretaker ahead of the
// resumed world snapshot, so a cred whose manager is absent from the loaded world is
// an orphan. New must drop it on load — else it would authenticate (bearer) yet
// resolve to no manager, and show in the admin listing.
func TestGatewayPrunesOrphanCredentials(t *testing.T) {
	res, err := worldgen.Generate(worldgen.PresetCompact(31), worldgen.WithTokenReader(&tokens{}))
	if err != nil {
		t.Fatal(err)
	}
	host := &testHost{world: res.World, eng: engine.New(res.World, res.Queue, &store.MemAuditLog{})}

	// An orphan: a manager_id no manager in the world carries.
	orphan := worldgen.ManagerCredential{ManagerID: 999999, ManagerName: "Ghost", Token: "mgr_orphan"}
	creds := append(append([]worldgen.ManagerCredential(nil), res.Manifest.Managers...), orphan)

	g := New(host, &store.MemInputLog{}, narrative.Default, creds)

	if _, err := g.VerifyToken(context.Background(), orphan.Token, nil); err == nil {
		t.Fatal("orphan token authenticated — it must be pruned on load")
	}
	for _, c := range g.Credentials() {
		if c.ManagerID == orphan.ManagerID {
			t.Fatal("orphan cred present in Credentials() — not pruned")
		}
	}
	// A real manager's token still authenticates — the prune didn't over-reach.
	if _, err := g.VerifyToken(context.Background(), res.Manifest.Managers[0].Token, nil); err != nil {
		t.Fatalf("real manager token rejected after prune: %v", err)
	}
}

// TestGatewayKeepsRetiredCredential guards the prune from over-reaching: a RETIRED
// manager stays IN world.Managers (FR-14e — the entry is never removed, the token is
// dead-but-present), so its cred must survive a re-load, not be pruned as an orphan.
func TestGatewayKeepsRetiredCredential(t *testing.T) {
	g, host, _, manifest := newGateway(t)
	retired := manifest.Managers[0]
	host.LockedWrite(func() { g.managers[retired.ManagerID].Status = worldgen.ManagerRetired })

	// Rebuild the gateway over the same world (manager now RETIRED but still present).
	g2 := New(host, &store.MemInputLog{}, narrative.Default, manifest.Managers)
	if _, err := g2.VerifyToken(context.Background(), retired.Token, nil); err != nil {
		t.Fatalf("RETIRED manager's cred was pruned — it must be kept (FR-14e): %v", err)
	}
}
