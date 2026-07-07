package narrative

import (
	"fmt"
	"strings"
)

// Locale identifies an available language catalog. en + ko are maintained
// together (FR-35c);
// English is the universal fallback — a missing key or catalog never errors.
type Locale string

const (
	LocaleEN Locale = "en" // default & fallback
	LocaleKO Locale = "ko"
)

var Supported = []Locale{LocaleEN, LocaleKO}

// ResolveTag normalizes a language tag or POSIX locale string to an available
// Locale: "ko", "ko-KR", "ko_KR.UTF-8" → ko; anything else → en.
func ResolveTag(tag string) Locale {
	if loc, ok := TryResolveTag(tag); ok {
		return loc
	}
	return LocaleEN
}

// TryResolveTag normalizes a language tag or POSIX locale string only when it
// matches an available catalog. The bool lets callers distinguish "unsupported"
// from the English fallback used by ResolveTag.
func TryResolveTag(tag string) (Locale, bool) {
	tag = strings.ToLower(strings.TrimSpace(tag))
	// Strip POSIX encoding/modifier: "ko_KR.UTF-8@x" → "ko_kr"
	if i := strings.IndexAny(tag, ".@"); i >= 0 {
		tag = tag[:i]
	}
	// Primary subtag: "ko-kr" / "ko_kr" → "ko"
	if i := strings.IndexAny(tag, "-_"); i >= 0 {
		tag = tag[:i]
	}
	for _, l := range Supported {
		if tag == string(l) {
			return l, true
		}
	}
	return "", false
}

// FromEnv resolves the system language the POSIX way: LC_ALL beats
// LC_MESSAGES beats LANG (docs/07 §6). lookup is os.Getenv in production.
func FromEnv(lookup func(string) string) Locale {
	for _, key := range []string{"LC_ALL", "LC_MESSAGES", "LANG"} {
		if v := lookup(key); v != "" && v != "C" && v != "POSIX" {
			return ResolveTag(v)
		}
	}
	return LocaleEN
}

// Catalogs maps locale → message key → template. Templates use {name}
// placeholders (proper pluralization/formatting comes with implementation).
type Catalogs map[Locale]map[string]string

// Render resolves key in the requested locale with English fallback
// (FR-35c): locale catalog → en catalog → the key itself. Never errors.
func (c Catalogs) Render(loc Locale, key string, params map[string]any) string {
	tmpl, ok := c.lookup(loc, key)
	if !ok {
		if tmpl, ok = c.lookup(LocaleEN, key); !ok {
			return key
		}
	}
	return interpolate(tmpl, params)
}

func (c Catalogs) lookup(loc Locale, key string) (string, bool) {
	m, ok := c[loc]
	if !ok {
		return "", false
	}
	t, ok := m[key]
	return t, ok
}

func interpolate(tmpl string, params map[string]any) string {
	if len(params) == 0 {
		return tmpl
	}
	var b strings.Builder
	for {
		i := strings.IndexByte(tmpl, '{')
		if i < 0 {
			b.WriteString(tmpl)
			return b.String()
		}
		j := strings.IndexByte(tmpl[i:], '}')
		if j < 0 {
			b.WriteString(tmpl)
			return b.String()
		}
		b.WriteString(tmpl[:i])
		name := tmpl[i+1 : i+j]
		if v, ok := params[name]; ok {
			b.WriteString(toString(v))
		} else {
			b.WriteString(tmpl[i : i+j+1])
		}
		tmpl = tmpl[i+j+1:]
	}
}

func toString(v any) string {
	if s, ok := v.(string); ok {
		return s
	}
	return fmt.Sprint(v)
}
