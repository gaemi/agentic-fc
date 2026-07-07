package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/gaemi/agentic-fc/internal/engine"
)

func main() {
	seedList := flag.String("seeds", "1,2,3,4,5", "comma-separated seeds")
	days := flag.Int("days", 365, "simulation horizon in game days")
	flag.Parse()

	seeds, err := parseSeeds(*seedList)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
	report, err := engine.RunCalibration(seeds, *days)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	if err := enc.Encode(report); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func parseSeeds(raw string) ([]uint64, error) {
	parts := strings.Split(raw, ",")
	seeds := make([]uint64, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		seed, err := strconv.ParseUint(part, 10, 64)
		if err != nil {
			return nil, fmt.Errorf("invalid seed %q", part)
		}
		seeds = append(seeds, seed)
	}
	if len(seeds) == 0 {
		return nil, fmt.Errorf("at least one seed is required")
	}
	return seeds, nil
}
