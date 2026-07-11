package main

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/gaemi/agentic-fc/internal/sim"
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

	if err := os.WriteFile(filepath.Join(dir, "data", "world.json"), []byte("{}"), 0o600); err != nil {
		t.Fatal(err)
	}
	if got, err := resolveDataDir(""); err != nil || got != "./data" {
		t.Errorf("./data with world state: got %q, %v; want ./data", got, err)
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
	for _, marker := range []string{"world.json", "manifest.json", "admin.token"} {
		sub := filepath.Join(dir, marker+"-case")
		if err := os.Mkdir(sub, 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(sub, marker), []byte("x"), 0o600); err != nil {
			t.Fatal(err)
		}
		if got, err := isWorldDataDir(sub); err != nil || !got {
			t.Errorf("dir with %s: got %v, %v; want true", marker, got, err)
		}
	}
	// A marker that is a directory does not count.
	sub := filepath.Join(dir, "dir-marker")
	if err := os.MkdirAll(filepath.Join(sub, "world.json"), 0o700); err != nil {
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
	xdg := t.TempDir() // host-absolute, so filepath.IsAbs holds on every platform
	env := map[string]string{}
	getenv := func(k string) string { return env[k] }
	home := func() (string, error) { return "/home/gaemi", nil }

	tests := []struct {
		name string
		goos string
		env  map[string]string
		want string
	}{
		{"darwin", "darwin", nil,
			filepath.FromSlash("/home/gaemi/Library/Application Support/agenticfc")},
		{"windows", "windows", map[string]string{"LocalAppData": `C:\Users\g\AppData\Local`},
			filepath.Join(`C:\Users\g\AppData\Local`, "agenticfc")},
		{"linux xdg", "linux", map[string]string{"XDG_DATA_HOME": xdg},
			filepath.Join(xdg, "agenticfc")},
		{"linux relative xdg ignored", "linux", map[string]string{"XDG_DATA_HOME": ".xdg"},
			filepath.FromSlash("/home/gaemi/.local/share/agenticfc")},
		{"linux fallback", "linux", nil,
			filepath.FromSlash("/home/gaemi/.local/share/agenticfc")},
	}
	for _, tt := range tests {
		env = tt.env
		got, err := userDataDir(tt.goos, getenv, home)
		if err != nil {
			t.Errorf("%s: %v", tt.name, err)
			continue
		}
		if got != tt.want {
			t.Errorf("%s: got %q, want %q", tt.name, got, tt.want)
		}
	}

	env = nil
	if _, err := userDataDir("windows", getenv, home); err == nil {
		t.Error("windows without LocalAppData: want error, got nil")
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
