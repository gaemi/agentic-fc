package narrative

import (
	"strconv"
	"testing"
)

func TestArticleTemplateKeyIsStableAndDistributesPool(t *testing.T) {
	counts := map[string]int{}
	for id := int64(1); id <= 300; id++ {
		key := ArticleTemplateKey("body", "injury", id)
		if key != "news.article.body.injury" && key != "news.article.body.injury.2" && key != "news.article.body.injury.3" {
			t.Fatalf("injury id %d selected unexpected key %q", id, key)
		}
		if again := ArticleTemplateKey("body", "injury", id); again != key {
			t.Fatalf("injury id %d selection changed: %q then %q", id, key, again)
		}
		counts[key]++
	}
	if len(counts) != articleVariantCounts["injury"] {
		t.Fatalf("selected %d injury variants, want %d: %v", len(counts), articleVariantCounts["injury"], counts)
	}
	for key, count := range counts {
		if count < 75 || count > 125 {
			t.Fatalf("%s selected %d times out of 300, want a reasonably even distribution: %v", key, count, counts)
		}
	}
	if got := ArticleTemplateKey("deck", "board", 2); got != "news.article.deck.board" {
		t.Fatalf("single-template class key = %q", got)
	}
	if got := ArticleTemplateKey("body", "injury", 0); got != "news.article.body.injury" {
		t.Fatalf("zero-id compatibility key = %q", got)
	}
}

func TestArticleVariantPoolsExistInEveryLocale(t *testing.T) {
	for class, count := range articleVariantCounts {
		for _, loc := range []Locale{LocaleEN, LocaleKO} {
			sections := []string{"deck", "body"}
			if class == "matchday.results" {
				sections = append(sections, "title")
			}
			for _, section := range sections {
				base := "news.article." + section + "." + class
				for variant := 1; variant <= count; variant++ {
					key := base
					if variant > 1 {
						key += "." + strconv.Itoa(variant)
					}
					if text, ok := Default[loc][key]; !ok || text == "" {
						t.Errorf("%s article pool is missing %s", loc, key)
					}
				}
			}
		}
	}
}
