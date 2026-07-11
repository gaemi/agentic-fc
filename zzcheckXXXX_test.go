package main

import (
	"fmt"
	"regexp"
	"sort"
	"testing"

	"github.com/gaemi/agentic-fc/internal/narrative"
	tuipkg "github.com/gaemi/agentic-fc/internal/tui"
)

func TestPrint(t *testing.T) {
	_ = tuipkg.Model{}
	params := map[string]any{"player": "P", "club": "C", "home_goals": 1, "away_goals": 0, "home": "H", "away": "A", "winner": "W", "home_pens": 4, "away_pens": 3, "on": "On", "off": "Off"}
	re := regexp.MustCompile(`^comment\.(goal|chance|save|card|injury|sub|quiet)(?:\.([a-z]+))?`)
	keys := make([]string, 0)
	for k := range narrative.Default["en"] {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		m := re.FindStringSubmatch(k)
		if m == nil {
			continue
		}
		line := narrative.Default.Render("en", k, params)
		_ = line
		fmt.Println(k)
	}
}
