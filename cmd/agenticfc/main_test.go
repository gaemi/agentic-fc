package main

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
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

func TestDialableAddr(t *testing.T) {
	tests := []struct{ in, want string }{
		{"0.0.0.0:7420", "127.0.0.1:7420"},
		{"[::]:7421", "127.0.0.1:7421"},
		{":7420", "127.0.0.1:7420"},
		{"127.0.0.1:7420", "127.0.0.1:7420"},
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
