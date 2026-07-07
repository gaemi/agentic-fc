package main

import (
	"testing"

	"github.com/gaemi/agentic-fc/internal/sim"
)

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
