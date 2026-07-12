package narrative

import "strconv"

// ArticleTemplateKey selects a stable presentation variant for a news item.
// News IDs are public, persisted facts; mixing them here varies prose without
// consuming simulation RNG or changing the world hash. Deck and body calls use
// the same slot so their editorial angle stays paired across Console and MCP.
func ArticleTemplateKey(section, class string, newsID int64) string {
	base := "news.article." + section + "." + class
	count := articleVariantCounts[class]
	if count < 2 || newsID <= 0 {
		return base
	}
	x := uint64(newsID) + 0x9e3779b97f4a7c15
	x = (x ^ (x >> 30)) * 0xbf58476d1ce4e5b9
	x = (x ^ (x >> 27)) * 0x94d049bb133111eb
	x ^= x >> 31
	variant := int(x%uint64(count)) + 1
	if variant == 1 {
		return base
	}
	return base + "." + strconv.Itoa(variant)
}

var articleVariantCounts = map[string]int{
	"injury":           3,
	"matchday.results": 3,
	"matchday.totw":    3,
}
