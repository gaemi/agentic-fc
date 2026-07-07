## Summary

Describe the change and the user-facing behavior it affects.

## Verification

- [ ] `go test ./...`
- [ ] `go vet ./...`
- [ ] `go build ./...`
- [ ] `test -z "$(gofmt -l .)"`

## Contract Checks

- [ ] Docs updated when behavior or APIs changed
- [ ] Tunables registered in `docs/98-tunables.md`
- [ ] English and Korean catalog entries added for new human-facing text
- [ ] MCP/Console/TUI visibility boundaries preserved
- [ ] Determinism/replay impact considered and tested
