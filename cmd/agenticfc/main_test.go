package main

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/gaemi/agentic-fc/internal/sim"
	"github.com/gaemi/agentic-fc/internal/worldgen"
)

func TestListenTCP(t *testing.T) {
	ln, err := listenTCP("console api", "-console-addr", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listenTCP on a free port: %v", err)
	}
	defer ln.Close()

	_, err = listenTCP("console api", "-console-addr", ln.Addr().String())
	if err == nil {
		t.Fatalf("listenTCP on the busy port %s: want error, got nil", ln.Addr())
	}
	for _, want := range []string{"console api", ln.Addr().String(), "-console-addr"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("busy-port error %q missing %q", err, want)
		}
	}
}

func TestResolveDataDir(t *testing.T) {
	if got, err := resolveDataDir("./custom"); err != nil || got != "./custom" {
		t.Errorf("explicit flag: got %q, %v; want ./custom", got, err)
	}

	dir := t.TempDir()
	t.Chdir(dir)
	got, err := resolveDataDir("")
	if err != nil {
		t.Fatalf("no ./data: %v", err)
	}
	if got == "./data" {
		t.Errorf("no ./data present: resolved to ./data; want the user data directory")
	}

	// A bare data/ directory without world state (an unrelated project's
	// folder) must not be adopted.
	if err := os.Mkdir(filepath.Join(dir, "data"), 0o700); err != nil {
		t.Fatal(err)
	}
	if got, _ := resolveDataDir(""); got == "./data" {
		t.Errorf("empty ./data: resolved to ./data; want the user data directory")
	}

	for _, f := range []string{"world.json", "manifest.json"} {
		if err := os.WriteFile(filepath.Join(dir, "data", f), []byte("{}"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if got, err := resolveDataDir(""); err != nil || got != "./data" {
		t.Errorf("./data with world state: got %q, %v; want ./data", got, err)
	}
}

func TestResolveDataDirNoHomeFallsBack(t *testing.T) {
	t.Chdir(t.TempDir())
	t.Setenv("HOME", "")
	t.Setenv("XDG_DATA_HOME", "")
	t.Setenv("LocalAppData", "")
	got, err := resolveDataDir("")
	if err != nil || got != "./data" {
		t.Errorf("no home environment: got %q, %v; want ./data fallback", got, err)
	}
}

func TestIsWorldDataDir(t *testing.T) {
	dir := t.TempDir()
	if got, err := isWorldDataDir(dir); err != nil || got {
		t.Errorf("empty dir: got %v, %v; want false", got, err)
	}
	if got, err := isWorldDataDir(filepath.Join(dir, "missing")); err != nil || got {
		t.Errorf("missing dir: got %v, %v; want false", got, err)
	}
	// A snapshot must be accompanied by manifest.json or admin.token; any
	// single generically named file is not evidence of a world.
	cases := []struct {
		name  string
		files []string
		want  bool
	}{
		{"world only", []string{"world.json"}, false},
		{"manifest only", []string{"manifest.json"}, false},
		{"token only", []string{"admin.token"}, false},
		{"manifest and token, no snapshot", []string{"manifest.json", "admin.token"}, false},
		{"world and manifest", []string{"world.json", "manifest.json"}, true},
		{"world and token", []string{"world.json", "admin.token"}, true},
		{"full world dir", []string{"world.json", "manifest.json", "admin.token"}, true},
	}
	for i, tc := range cases {
		sub := filepath.Join(dir, fmt.Sprintf("case-%d", i))
		if err := os.Mkdir(sub, 0o700); err != nil {
			t.Fatal(err)
		}
		for _, f := range tc.files {
			if err := os.WriteFile(filepath.Join(sub, f), []byte("x"), 0o600); err != nil {
				t.Fatal(err)
			}
		}
		if got, err := isWorldDataDir(sub); err != nil || got != tc.want {
			t.Errorf("%s: got %v, %v; want %v", tc.name, got, err, tc.want)
		}
	}
	// A marker that is a directory does not count.
	sub := filepath.Join(dir, "dir-marker")
	if err := os.MkdirAll(filepath.Join(sub, "world.json"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sub, "manifest.json"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	if got, err := isWorldDataDir(sub); err != nil || got {
		t.Errorf("world.json as a directory: got %v, %v; want false", got, err)
	}
	// A plain file named data is not a data directory.
	file := filepath.Join(dir, "data-file")
	if err := os.WriteFile(file, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	if got, err := isWorldDataDir(file); err != nil || got {
		t.Errorf("plain file: got %v, %v; want false", got, err)
	}
}

func TestIsWorldDataDirUnreadable(t *testing.T) {
	if runtime.GOOS == "windows" || os.Getuid() == 0 {
		t.Skip("permission bits are not enforced here")
	}
	dir := t.TempDir()
	sub := filepath.Join(dir, "data")
	if err := os.Mkdir(sub, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sub, "world.json"), []byte("{}"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(sub, 0o000); err != nil {
		t.Fatal(err)
	}
	defer os.Chmod(sub, 0o700)
	if _, err := isWorldDataDir(sub); err == nil {
		t.Error("unreadable world dir: want error, got nil")
	}
}

func TestUserDataDir(t *testing.T) {
	// t.TempDir is host-absolute, so filepath.IsAbs holds on every platform.
	xdg := t.TempDir()
	homeDir := t.TempDir()
	localAppData := t.TempDir()
	env := map[string]string{}
	getenv := func(k string) string { return env[k] }
	home := func() (string, error) { return homeDir, nil }

	tests := []struct {
		name string
		goos string
		env  map[string]string
		want string // empty means an error is expected
	}{
		{"darwin", "darwin", nil,
			filepath.Join(homeDir, "Library", "Application Support", "agenticfc")},
		{"windows", "windows", map[string]string{"LocalAppData": localAppData},
			filepath.Join(localAppData, "agenticfc")},
		{"windows without LocalAppData", "windows", nil, ""},
		{"relative LocalAppData rejected", "windows",
			map[string]string{"LocalAppData": `AppData\Local`}, ""},
		{"linux xdg", "linux", map[string]string{"XDG_DATA_HOME": xdg},
			filepath.Join(xdg, "agenticfc")},
		{"linux relative xdg ignored", "linux", map[string]string{"XDG_DATA_HOME": ".xdg"},
			filepath.Join(homeDir, ".local", "share", "agenticfc")},
		{"linux fallback", "linux", nil,
			filepath.Join(homeDir, ".local", "share", "agenticfc")},
	}
	for _, tt := range tests {
		env = tt.env
		got, err := userDataDir(tt.goos, getenv, home)
		if tt.want == "" {
			if err == nil {
				t.Errorf("%s: want error, got %q", tt.name, got)
			}
			continue
		}
		if err != nil {
			t.Errorf("%s: %v", tt.name, err)
			continue
		}
		if got != tt.want {
			t.Errorf("%s: got %q, want %q", tt.name, got, tt.want)
		}
	}

	env = nil
	relHome := func() (string, error) { return "relative-home", nil }
	for _, goos := range []string{"darwin", "linux"} {
		if _, err := userDataDir(goos, getenv, relHome); err == nil {
			t.Errorf("%s with relative home: want error, got nil", goos)
		}
	}
}

func TestDialableAddr(t *testing.T) {
	tests := []struct{ in, want string }{
		{"0.0.0.0:7420", "127.0.0.1:7420"},
		{"[::]:7421", "[::1]:7421"},
		{":7420", "127.0.0.1:7420"},
		{"127.0.0.1:7420", "127.0.0.1:7420"},
		{"[::1]:7421", "[::1]:7421"},
		{"192.168.1.5:80", "192.168.1.5:80"},
		{"example.local:7420", "example.local:7420"},
		{"not-an-address", "not-an-address"},
	}
	for _, tt := range tests {
		if got := dialableAddr(tt.in); got != tt.want {
			t.Errorf("dialableAddr(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestStartupHandler(t *testing.T) {
	var h startupHandler

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("GET", "/healthz", nil))
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("before Set: status %d, want %d", rec.Code, http.StatusServiceUnavailable)
	}
	if rec.Header().Get("Retry-After") == "" {
		t.Error("before Set: missing Retry-After header")
	}

	h.Set(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusTeapot)
	}))
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("GET", "/healthz", nil))
	if rec.Code != http.StatusTeapot {
		t.Fatalf("after Set: status %d, want %d", rec.Code, http.StatusTeapot)
	}
}

func TestListenTCPMalformedAddress(t *testing.T) {
	_, err := listenTCP("mcp gateway", "-mcp-addr", "not-an-address")
	if err == nil {
		t.Fatal("listenTCP on a malformed address: want error, got nil")
	}
	if !strings.Contains(err.Error(), "-mcp-addr") {
		t.Errorf("malformed-address error %q missing flag name", err)
	}
}

func TestResolveRunProfile(t *testing.T) {
	tests := []struct {
		name          string
		profile       string
		speed         sim.Speed
		speedSet      bool
		idle          int
		idleSet       bool
		offseason     int
		offseasonSet  bool
		wantName      string
		wantSpeed     sim.Speed
		wantIdle      int
		wantOffseason int
	}{
		{
			name:          "default",
			profile:       "default",
			wantName:      "default",
			wantSpeed:     sim.Speed15,
			wantIdle:      sim.DefaultIdleAcceleration,
			wantOffseason: sim.DefaultOffseasonAcceleration,
		},
		{
			name:          "fast",
			profile:       "fast",
			wantName:      "fast",
			wantSpeed:     sim.Speed30,
			wantIdle:      32,
			wantOffseason: 192,
		},
		{
			name:          "slow",
			profile:       "slow",
			wantName:      "slow",
			wantSpeed:     sim.Speed15,
			wantIdle:      6,
			wantOffseason: 36,
		},
		{
			name:          "custom partial override",
			profile:       "custom",
			speed:         sim.Speed60,
			speedSet:      true,
			offseason:     120,
			offseasonSet:  true,
			wantName:      "custom",
			wantSpeed:     sim.Speed60,
			wantIdle:      sim.DefaultIdleAcceleration,
			wantOffseason: 120,
		},
		{
			name:          "profile with explicit idle override",
			profile:       "fast",
			idle:          24,
			idleSet:       true,
			wantName:      "custom",
			wantSpeed:     sim.Speed30,
			wantIdle:      24,
			wantOffseason: 192,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := resolveRunProfile(tt.profile, tt.speed, tt.speedSet, tt.idle, tt.idleSet, tt.offseason, tt.offseasonSet)
			if err != nil {
				t.Fatal(err)
			}
			if got.Name != tt.wantName || got.Speed != tt.wantSpeed ||
				got.IdleAcceleration != tt.wantIdle || got.OffseasonAcceleration != tt.wantOffseason {
				t.Fatalf("profile = %+v, want name=%s speed=%d idle=%d offseason=%d",
					got, tt.wantName, tt.wantSpeed, tt.wantIdle, tt.wantOffseason)
			}
		})
	}
}

func TestParseNameOverrides(t *testing.T) {
	dir := t.TempDir()
	clubFile := filepath.Join(dir, "clubs.txt")
	managerFile := filepath.Join(dir, "managers.txt")
	if err := os.WriteFile(clubFile, []byte("# comment\nCodex United\n\nAgentic Town\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(managerFile, []byte("Claude Lee\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	got, err := parseNameOverrides(`"AFC Gaemi",Rovers`, clubFile+" ", "Codex Park", managerFile)
	if err != nil {
		t.Fatal(err)
	}
	wantClubs := []string{"AFC Gaemi", "Rovers", "Codex United", "Agentic Town"}
	wantManagers := []string{"Codex Park", "Claude Lee"}
	if len(got.ClubNames) != len(wantClubs) || len(got.ManagerNames) != len(wantManagers) {
		t.Fatalf("override lengths = %+v", got)
	}
	for i, want := range wantClubs {
		if got.ClubNames[i] != want {
			t.Fatalf("club override %d = %q, want %q", i, got.ClubNames[i], want)
		}
	}
	for i, want := range wantManagers {
		if got.ManagerNames[i] != want {
			t.Fatalf("manager override %d = %q, want %q", i, got.ManagerNames[i], want)
		}
	}

	if _, err := parseNameListFlag("A\nB", ""); err == nil {
		t.Fatal("multi-record inline CSV accepted")
	}
}

func TestResolveRunProfileRejectsInvalidInput(t *testing.T) {
	if _, err := resolveRunProfile("turbo", 0, false, 0, false, 0, false); err == nil {
		t.Fatal("unknown profile accepted")
	}
	if _, err := resolveRunProfile("custom", sim.Speed15, true, 1, true, 0, false); err == nil {
		t.Fatal("invalid idle override accepted")
	}
	if _, err := resolveRunProfile("custom", 10, true, 0, false, 0, false); err == nil {
		t.Fatal("invalid speed override accepted")
	}
}

func TestMCPConfigText(t *testing.T) {
	dir := t.TempDir()
	manifest := filepath.Join(dir, "manifest.json")
	body := `{
  "world_name": "Test League",
  "seed": 7,
  "start_state": "running",
  "managers": [
    {"manager_id": 1001, "manager_name": "Ada One", "club_id": 1, "club_name": "Alpha FC", "archetype": "The Idealist", "reputation": 5000, "token": "mgr_alpha"},
    {"manager_id": 1002, "manager_name": "Bo Two", "club_id": 2, "club_name": "Beta United", "archetype": "The Professor", "reputation": 5100, "token": "mgr_beta"},
    {"manager_id": 1003, "manager_name": "Cy Three", "club_id": 0, "archetype": "The Firefighter", "reputation": 4200, "token": "mgr_free"},
    {"manager_id": 1004, "manager_name": "Del Four", "club_id": 4, "club_name": "Delta Town", "archetype": "The Idealist", "reputation": 3000, "token": "mgr_dead"},
    {"manager_id": 1005, "manager_name": "Eve Five", "club_id": 5, "club_name": "Echo City", "archetype": "The Professor", "reputation": 3100, "token": "mgr_ghost"}
  ]
}`
	world := &worldgen.World{
		Clubs: []worldgen.Club{
			{ID: 1, Name: "Alpha FC"},
			{ID: 9, Name: "Gamma Rovers"},
		},
		Managers: []worldgen.Manager{
			{ID: 1001, Name: "Ada One", ClubID: 1},
			// Moved clubs since the token was issued; the manifest still
			// says Beta United. The listing must show the snapshot's club.
			{ID: 1002, Name: "Bo Two", ClubID: 9},
			{ID: 1003, Name: "Cy Three"},
			{ID: 1004, Name: "Del Four", Status: worldgen.ManagerRetired},
			// 1005 is missing: an orphan credential the gateway prunes at load.
		},
	}
	if err := os.WriteFile(manifest, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}

	out, err := mcpConfigText(manifest, "http://127.0.0.1:7421", 0, world)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"claude mcp add --transport http agentic-fc http://127.0.0.1:7421 --header \"Authorization: Bearer mgr_alpha\"",
		"\"url\": \"http://127.0.0.1:7421\"",
		"\"Authorization\": \"Bearer mgr_alpha\"",
		"Ada One", "Bo Two", "Cy Three",
		"Gamma Rovers",
		"(unemployed)",
		"get_guide",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("default output missing %q\n%s", want, out)
		}
	}
	// Only the picked manager's token may appear: the other tokens stay in
	// the manifest so the listing never leaks every credential at once. The
	// retired manager (1004) and the orphan credential (1005) must vanish
	// entirely — their tokens are dead on arrival at the gateway.
	for _, leak := range []string{"mgr_beta", "mgr_free", "mgr_dead", "mgr_ghost", "Del Four", "Eve Five", "Beta United"} {
		if strings.Contains(out, leak) {
			t.Errorf("default output contains %q, want it omitted\n%s", leak, out)
		}
	}

	out, err = mcpConfigText(manifest, "http://127.0.0.1:7421", 1002, world)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "Bearer mgr_beta") {
		t.Errorf("-mcp-manager 1002 output missing beta token\n%s", out)
	}
	if strings.Contains(out, "mgr_alpha") {
		t.Errorf("-mcp-manager 1002 output leaks alpha token\n%s", out)
	}

	if _, err := mcpConfigText(manifest, "http://127.0.0.1:7421", 9999, world); err == nil {
		t.Fatal("unknown manager id accepted")
	}
	if _, err := mcpConfigText(manifest, "http://127.0.0.1:7421", 1004, world); err == nil ||
		!strings.Contains(err.Error(), "retired") {
		t.Fatalf("retired manager error = %v, want retirement hint", err)
	}
	if _, err := mcpConfigText(manifest, "http://127.0.0.1:7421", 1005, world); err == nil ||
		strings.Contains(err.Error(), "retired") {
		t.Fatalf("orphan credential error = %v, want plain not-found", err)
	}
	if _, err := mcpConfigText(filepath.Join(dir, "missing.json"), "http://127.0.0.1:7421", 0, world); err == nil ||
		!strings.Contains(err.Error(), "no world manifest") {
		t.Fatalf("missing manifest error = %v, want friendly hint", err)
	}

	empty := filepath.Join(dir, "empty.json")
	if err := os.WriteFile(empty, []byte(`{"world_name":"x","seed":1,"start_state":"ready","managers":[]}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := mcpConfigText(empty, "http://127.0.0.1:7421", 0, world); err == nil {
		t.Fatal("empty manager list accepted")
	}
	if _, err := mcpConfigText(manifest, "http://127.0.0.1:7421", 0,
		&worldgen.World{Managers: []worldgen.Manager{{ID: 1001, Status: worldgen.ManagerRetired}}}); err == nil ||
		!strings.Contains(err.Error(), "no playable managers") {
		t.Fatal("all-retired world accepted")
	}
}

func TestShellQuote(t *testing.T) {
	for in, want := range map[string]string{
		"/plain/path":           "'/plain/path'",
		"/with space/agenticfc": "'/with space/agenticfc'",
		"/dollar/$HOME/`cmd`":   "'/dollar/$HOME/`cmd`'",
		"/quote/it's here":      `'/quote/it'\''s here'`,
	} {
		if got := shellQuote(in); got != want {
			t.Errorf("shellQuote(%q) = %s, want %s", in, got, want)
		}
	}
}

func TestMCPEndpointURL(t *testing.T) {
	for in, want := range map[string]string{
		"127.0.0.1:7421": "http://127.0.0.1:7421",
		":7421":          "http://127.0.0.1:7421",
	} {
		got, err := mcpEndpointURL(in)
		if err != nil || got != want {
			t.Errorf("mcpEndpointURL(%q) = %q, %v; want %q", in, got, err, want)
		}
	}
	for _, bad := range []string{"not-an-address", "127.0.0.1:0", ":0", "127.0.0.1:http", "127.0.0.1:70000", ""} {
		if got, err := mcpEndpointURL(bad); err == nil {
			t.Errorf("mcpEndpointURL(%q) = %q, want error", bad, got)
		}
	}
}
