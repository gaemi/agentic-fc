#!/usr/bin/env bash
set -euo pipefail

fail() {
  echo "version-check: $*" >&2
  exit 1
}

[[ -f VERSION ]] || fail "missing VERSION"
[[ -f CHANGELOG.md ]] || fail "missing CHANGELOG.md"
[[ -f docs/13-operations.md ]] || fail "missing docs/13-operations.md"

line_count="$(awk 'END { print NR }' VERSION)"
[[ "${line_count}" -eq 1 ]] ||
  fail "VERSION must contain exactly one line"

version="$(sed -n '1p' VERSION)"
version="${version%$'\r'}"
[[ -n "${version}" ]] ||
  fail "VERSION must not be empty"

[[ "${version}" != *[[:space:]]* ]] ||
  fail "VERSION must not contain whitespace"

semver_re='^(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)$'
[[ "${version}" =~ ${semver_re} ]] ||
  fail "VERSION must be bare SemVer MAJOR.MINOR.PATCH without v-prefix, prerelease, build metadata, or leading zeroes: ${version}"

IFS=. read -r major minor patch <<< "${version}"
# Agentic FC intentionally starts at 0.1.0; 0.0.x indicates an
# uninitialized or pre-release placeholder rather than a publishable version.
if (( major == 0 && minor == 0 )); then
  fail "VERSION must be 0.1.0 or newer, got ${version}"
fi

tag="v${version}"
build_example="${tag}+<commit_count>.g<short_sha>"
version_re="${version//./\\.}"
tag_re="v${version_re}"

grep -Eq "(^|[^0-9A-Za-z.+-])${tag_re}([^0-9A-Za-z.+-]|$)" docs/13-operations.md ||
  fail "docs/13-operations.md must mention release tag ${tag}"

grep -Fq "${build_example}" docs/13-operations.md ||
  fail "docs/13-operations.md must mention build metadata example ${build_example}"

grep -Eq "^## ${version_re} - [0-9]{4}-[0-9]{2}-[0-9]{2}$" CHANGELOG.md ||
  fail "CHANGELOG.md must contain a release section exactly like: ## ${version} - YYYY-MM-DD"

grep -Eq '^## Unreleased$' CHANGELOG.md ||
  fail "CHANGELOG.md must keep an Unreleased section"

grep -Eq '<[[:space:]]*VERSION([[:space:]]|$)' .github/workflows/draft-release.yml ||
  fail "draft-release workflow must read release version from VERSION"

verify_line="$(grep -nE '^[[:space:]]*run:[[:space:]]+make verify[[:space:]]*$' .github/workflows/draft-release.yml | head -n1 | cut -d: -f1 || true)"
package_line="$(grep -nE '^[[:space:]]*-[[:space:]]*name:[[:space:]]+Package binaries[[:space:]]*$' .github/workflows/draft-release.yml | head -n1 | cut -d: -f1 || true)"
[[ -n "${verify_line}" && -n "${package_line}" && "${verify_line}" -lt "${package_line}" ]] ||
  fail "draft-release workflow must run make verify before Package binaries"

echo "version-check: ${version}"
