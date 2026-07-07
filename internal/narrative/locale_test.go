package narrative

import "testing"

func TestResolveTag(t *testing.T) {
	cases := map[string]Locale{
		"ko":          LocaleKO,
		"ko-KR":       LocaleKO,
		"ko_KR.UTF-8": LocaleKO,
		"en":          LocaleEN,
		"en-US":       LocaleEN,
		"ja_JP.UTF-8": LocaleEN, // unsupported → English fallback
		"":            LocaleEN,
		"C":           LocaleEN,
	}
	for tag, want := range cases {
		if got := ResolveTag(tag); got != want {
			t.Errorf("ResolveTag(%q) = %s, want %s", tag, got, want)
		}
	}
}

func TestFromEnvPrecedence(t *testing.T) {
	env := map[string]string{"LANG": "en_US.UTF-8", "LC_ALL": "ko_KR.UTF-8"}
	got := FromEnv(func(k string) string { return env[k] })
	if got != LocaleKO {
		t.Fatalf("LC_ALL must beat LANG: got %s", got)
	}
	env = map[string]string{"LANG": "ko_KR.UTF-8"}
	if got := FromEnv(func(k string) string { return env[k] }); got != LocaleKO {
		t.Fatalf("LANG alone: got %s, want ko", got)
	}
	if got := FromEnv(func(string) string { return "" }); got != LocaleEN {
		t.Fatalf("empty env must default to en: got %s", got)
	}
}

func TestRenderFallback(t *testing.T) {
	c := Catalogs{
		LocaleEN: {
			"news.sacked":  "{club} have sacked {manager}.",
			"news.en_only": "English only line.",
		},
		LocaleKO: {
			"news.sacked": "{club}, {manager} 감독 경질.",
		},
	}
	p := map[string]any{"club": "Alderton FC", "manager": "Kim Dojin"}

	if got := c.Render(LocaleKO, "news.sacked", p); got != "Alderton FC, Kim Dojin 감독 경질." {
		t.Fatalf("ko render: %q", got)
	}
	// ko missing key → en fallback (FR-35c).
	if got := c.Render(LocaleKO, "news.en_only", nil); got != "English only line." {
		t.Fatalf("fallback to en: %q", got)
	}
	// Unknown key everywhere → key itself, never an error.
	if got := c.Render(LocaleKO, "news.unknown", nil); got != "news.unknown" {
		t.Fatalf("unknown key: %q", got)
	}
	// Unknown param stays literal.
	if got := c.Render(LocaleEN, "news.sacked", map[string]any{"club": "X"}); got != "X have sacked {manager}." {
		t.Fatalf("partial params: %q", got)
	}
}
