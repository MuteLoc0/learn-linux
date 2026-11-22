package tips

import (
	"fmt"
	"math/rand/v2"
	"slices"
	"strings"
	"sync"
	"testing"

	"learn-linux/internal/contract"
	"learn-linux/internal/i18n"
)

var testChapterIDs = []int{-1, 0, 1, 2, 3, 4, 5, 6, 7, 8, 99}

func TestBuiltInTipPools(t *testing.T) {
	provider := NewProvider()
	globals := provider.TipsForChapter(-1)
	if len(globals) < 7 || len(globals) > 8 {
		t.Fatalf("got %d global tips, want 7 or 8", len(globals))
	}
	for _, key := range []string{
		contract.KeyTipLanguage, contract.KeyTipTab, contract.KeyTipHistory,
		contract.KeyTipClear, contract.KeyTipManual,
	} {
		if !slices.Contains(globals, key) {
			t.Errorf("required global tip %q is missing", key)
		}
	}

	for _, chapterID := range testChapterIDs {
		t.Run(fmt.Sprintf("chapter_%d", chapterID), func(t *testing.T) {
			keys := provider.TipsForChapter(chapterID)
			for _, key := range globals {
				if !slices.Contains(keys, key) {
					t.Errorf("pool is missing global tip %q", key)
				}
			}
			seen := make(map[string]bool)
			scoped := 0
			for _, key := range keys {
				if seen[key] {
					t.Errorf("duplicate tip %q", key)
				}
				seen[key] = true
				if strings.HasPrefix(key, "ui.tips.global.") {
					continue
				}
				scoped++
				if !strings.HasPrefix(key, fmt.Sprintf("ui.tips.ch%d.", chapterID)) {
					t.Errorf("tip %q is scoped to a different chapter", key)
				}
			}
			switch {
			case chapterID >= 1 && chapterID <= 8:
				if scoped < 3 || scoped > 4 {
					t.Errorf("got %d chapter-specific tips, want 3 or 4", scoped)
				}
			case scoped != 0:
				t.Errorf("got %d chapter-specific tips, want only global tips", scoped)
			}
		})
	}
}

func TestSharedAndSandboxTips(t *testing.T) {
	provider := newProvider([]tip{
		{Key: "global"},
		{Key: "global"},
		{Key: "sandbox", Chapters: []int{0}},
		{Key: "shared", Chapters: []int{7, 8, 7}},
		{Key: "global", Chapters: []int{8}},
	}, nil)

	for _, tc := range []struct {
		chapterID int
		want      []string
	}{
		{-1, []string{"global"}},
		{0, []string{"global", "sandbox"}},
		{1, []string{"global"}},
		{7, []string{"global", "shared"}},
		{8, []string{"global", "shared"}},
		{99, []string{"global"}},
	} {
		if got := provider.TipsForChapter(tc.chapterID); !slices.Equal(got, tc.want) {
			t.Errorf("TipsForChapter(%d) = %v, want %v", tc.chapterID, got, tc.want)
		}
	}
}

func TestPickTipAvoidsRepeatingWithinEachChapter(t *testing.T) {
	rng := rand.New(rand.NewPCG(1, 2))
	for _, tc := range []struct {
		name string
		pick func(int) int
	}{
		{"first_index", func(int) int { return 0 }},
		{"last_index", func(n int) int { return n - 1 }},
		{"out_of_bounds_negative", func(int) int { return -7 }},
		{"out_of_bounds_high", func(n int) int { return n + 5 }},
		{"seeded_rng", rng.IntN},
		{"default_rng", nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			provider := NewProviderWithRand(tc.pick)
			pools := make(map[int][]string)
			for _, chapterID := range testChapterIDs {
				pools[chapterID] = provider.TipsForChapter(chapterID)
			}
			last := make(map[int]string)
			for range 100 {
				for _, chapterID := range testChapterIDs {
					key := provider.PickTip(chapterID)
					if !slices.Contains(pools[chapterID], key) {
						t.Fatalf("PickTip(%d) returned ineligible key %q", chapterID, key)
					}
					if key == last[chapterID] {
						t.Fatalf("PickTip(%d) repeated %q", chapterID, key)
					}
					last[chapterID] = key
				}
			}
		})
	}
}

func TestPickTipWithZeroOrOneEligibleTip(t *testing.T) {
	provider := newProvider([]tip{{Key: "only", Chapters: []int{1}}}, func(n int) int {
		t.Fatalf("RNG called with %d choices for a pool of at most one tip", n)
		return 0
	})
	if got := provider.TipsForChapter(99); len(got) != 0 {
		t.Errorf("TipsForChapter(99) = %v, want an empty pool", got)
	}
	if got := provider.PickTip(99); got != "" {
		t.Errorf("PickTip(99) = %q, want an empty string", got)
	}
	for range 3 {
		if got := provider.PickTip(1); got != "only" {
			t.Errorf("PickTip(1) = %q, want only", got)
		}
	}
}

func TestTipPoolsAreDefensiveCopies(t *testing.T) {
	entries := []tip{{Key: "global"}, {Key: "scoped", Chapters: []int{1}}}
	provider := newProvider(entries, nil)
	entries[0].Key = "mutated"
	entries[1].Chapters[0] = 0

	for _, tc := range []struct {
		chapterID int
		want      []string
	}{
		{0, []string{"global"}},
		{1, []string{"global", "scoped"}},
	} {
		keys := provider.TipsForChapter(tc.chapterID)
		if !slices.Equal(keys, tc.want) {
			t.Fatalf("TipsForChapter(%d) after catalog mutation = %v, want %v", tc.chapterID, keys, tc.want)
		}
		keys[0] = "mutated"
		if got := provider.TipsForChapter(tc.chapterID); !slices.Equal(got, tc.want) {
			t.Errorf("TipsForChapter(%d) after slice mutation = %v, want %v", tc.chapterID, got, tc.want)
		}
	}
}

func TestProviderConcurrentAccess(t *testing.T) {
	rng := rand.New(rand.NewPCG(3, 4))
	provider := NewProviderWithRand(rng.IntN)
	var wg sync.WaitGroup
	for worker := range 8 {
		wg.Go(func() {
			for i := range 100 {
				chapterID := testChapterIDs[(worker+i)%len(testChapterIDs)]
				keys := provider.TipsForChapter(chapterID)
				pick := provider.PickTip(chapterID)
				if !slices.Contains(keys, pick) {
					t.Errorf("PickTip(%d) returned ineligible key %q", chapterID, pick)
					return
				}
				keys[0] = "mutated by caller"
			}
		})
	}
	wg.Wait()
}

func TestBuiltInTipsAreBilingual(t *testing.T) {
	provider := NewProvider()
	bundle, err := i18n.NewBundle(contract.LangZH)
	if err != nil {
		t.Fatalf("NewBundle: %v", err)
	}
	keys := make(map[string]bool)
	for _, chapterID := range testChapterIDs {
		for _, key := range provider.TipsForChapter(chapterID) {
			keys[key] = true
		}
	}
	for _, lang := range []contract.Lang{contract.LangZH, contract.LangEN} {
		if err := bundle.SetLang(lang); err != nil {
			t.Fatalf("SetLang(%s): %v", lang, err)
		}
		if got := bundle.T(contract.KeyTipsPrefix); got != "tips: " {
			t.Errorf("%s prefix = %q, want %q", lang, got, "tips: ")
		}
		for key := range keys {
			value := strings.TrimSpace(bundle.T(key))
			if !bundle.Has(key) || value == "" || value == key {
				t.Errorf("%s tip %q has no translation", lang, key)
			}
			lower := strings.ToLower(value)
			if strings.HasPrefix(lower, "tips:") || strings.HasPrefix(lower, "tip:") ||
				strings.HasPrefix(value, "小技巧") {
				t.Errorf("%s tip %q has a redundant prefix: %q", lang, key, value)
			}
		}
	}
}
