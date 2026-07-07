.PHONY: build test fmt fmt-check vet lint-docs lint-actions vulncheck secret-scan security verify ci run-daemon run-console

build:
	go build ./...
	go build -o bin/agenticfc ./cmd/agenticfc
	go build -o bin/agenticfc-console ./cmd/agenticfc-console
	go build -o bin/agenticfc-calibrate ./cmd/agenticfc-calibrate

test:
	go test ./...

fmt:
	gofmt -w .

fmt-check:
	test -z "$$(gofmt -l .)"

vet:
	go vet ./...

lint-docs:
	ruby -e 'Dir["**/*.md"].reject { |f| f.start_with?("data/") || f.start_with?("data-fast/") }.each do |f|; File.read(f).scan(/\]\(([^)#]+\.md)(#[^)]+)?\)/).each do |m|; pth=m[0]; next if pth.start_with?("http://", "https://"); target=File.expand_path(pth, File.dirname(f)); abort("#{f}: missing #{pth}") unless File.exist?(target); end; end'

lint-actions:
	go install github.com/rhysd/actionlint/cmd/actionlint@v1.7.12
	"$$(go env GOPATH)/bin/actionlint"

vulncheck:
	go install golang.org/x/vuln/cmd/govulncheck@v1.5.0
	"$$(go env GOPATH)/bin/govulncheck" ./...

secret-scan:
	go install github.com/zricethezav/gitleaks/v8@v8.30.1
	"$$(go env GOPATH)/bin/gitleaks" detect --source . --redact --verbose

security: vulncheck secret-scan

verify: fmt-check vet build test lint-docs lint-actions

ci: verify security

run-daemon: build
	./bin/agenticfc

run-console: build
	./bin/agenticfc-console
