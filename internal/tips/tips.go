package tips

import (
	"math/rand/v2"
	"slices"
	"sync"

	"learn-linux/internal/contract"
)

var _ contract.TipProvider = (*Provider)(nil)

type tip struct {
	Key      string
	Chapters []int // Empty means the tip applies to every chapter.
}

var catalog = []tip{
	{Key: contract.KeyTipLanguage},
	{Key: contract.KeyTipTab},
	{Key: contract.KeyTipHistory},
	{Key: contract.KeyTipClear},
	{Key: contract.KeyTipManual},
	{Key: contract.KeyTipHint},
	{Key: contract.KeyTipReset},
	{Key: contract.KeyTipCursor},

	{Key: contract.KeyTipCDPrevious, Chapters: []int{1}},
	{Key: contract.KeyTipTreeDepth, Chapters: []int{1}},
	{Key: contract.KeyTipReadableSize, Chapters: []int{1}},

	{Key: contract.KeyTipTouchTime, Chapters: []int{2}},
	{Key: contract.KeyTipVerboseTransfer, Chapters: []int{2}},
	{Key: contract.KeyTipRelativeLink, Chapters: []int{2}},

	{Key: contract.KeyTipLessMatches, Chapters: []int{3}},
	{Key: contract.KeyTipPreviewLines, Chapters: []int{3}},
	{Key: contract.KeyTipNanoExit, Chapters: []int{3}},

	{Key: contract.KeyTipFindFiles, Chapters: []int{4}},
	{Key: contract.KeyTipGrepLiteral, Chapters: []int{4}},
	{Key: contract.KeyTipUniqCount, Chapters: []int{4}},

	{Key: contract.KeyTipPreviewPipe, Chapters: []int{5}},
	{Key: contract.KeyTipRedirectIn, Chapters: []int{5}},
	{Key: contract.KeyTipRedirectOut, Chapters: []int{5}},
	{Key: contract.KeyTipChainStatus, Chapters: []int{5}},

	{Key: contract.KeyTipOwnerExecute, Chapters: []int{6}},
	{Key: contract.KeyTipEnv, Chapters: []int{6}},
	{Key: contract.KeyTipModeDigits, Chapters: []int{6}},

	{Key: contract.KeyTipTarList, Chapters: []int{7}},
	{Key: contract.KeyTipMakeDryRun, Chapters: []int{7}},
	{Key: contract.KeyTipGzipKeep, Chapters: []int{7}},

	{Key: contract.KeyTipLogLine, Chapters: []int{8}},
	{Key: contract.KeyTipVerifyConfig, Chapters: []int{8}},
	{Key: contract.KeyTipTarBase, Chapters: []int{8}},
	{Key: contract.KeyTipVerifyBuild, Chapters: []int{8}},
}

// Provider selects tips from immutable pools and tracks the last pick for each chapter.
type Provider struct {
	global    []string
	byChapter map[int][]string
	randIntn  func(int) int

	mu                 sync.Mutex
	lastIndexByChapter map[int]int
}

// NewProvider creates a randomized provider for the built-in tips.
func NewProvider() *Provider {
	return NewProviderWithRand(nil)
}

// NewProviderWithRand uses randIntn to choose a tip index in [0, n).
// The provider serializes calls to randIntn. A nil callback uses the default RNG.
func NewProviderWithRand(randIntn func(int) int) *Provider {
	return newProvider(catalog, randIntn)
}

func newProvider(entries []tip, randIntn func(int) int) *Provider {
	if randIntn == nil {
		randIntn = rand.IntN
	}
	var global []string
	byChapter := make(map[int][]string)
	for _, entry := range entries {
		if len(entry.Chapters) == 0 {
			if !slices.Contains(global, entry.Key) {
				global = append(global, entry.Key)
			}
			continue
		}
		for _, chapterID := range entry.Chapters {
			if !slices.Contains(byChapter[chapterID], entry.Key) {
				byChapter[chapterID] = append(byChapter[chapterID], entry.Key)
			}
		}
	}
	for chapterID, scoped := range byChapter {
		combined := slices.Clone(global)
		for _, key := range scoped {
			if !slices.Contains(combined, key) {
				combined = append(combined, key)
			}
		}
		byChapter[chapterID] = combined
	}
	return &Provider{
		global:             global,
		byChapter:          byChapter,
		randIntn:           randIntn,
		lastIndexByChapter: make(map[int]int),
	}
}

// PickTip returns a random eligible key, or an empty string if the pool is empty.
func (p *Provider) PickTip(chapterID int) string {
	keys := p.eligibleTips(chapterID)
	switch len(keys) {
	case 0:
		return ""
	case 1:
		return keys[0]
	}

	p.mu.Lock()
	defer p.mu.Unlock()

	choices := len(keys)
	previous, hasPrevious := p.lastIndexByChapter[chapterID]
	if hasPrevious {
		choices--
	}
	// Map the random choice into the pool, skipping the previous index.
	index := p.randIntn(choices)
	if index < 0 || index >= choices {
		index = ((index % choices) + choices) % choices
	}
	if hasPrevious && index >= previous {
		index++
	}
	p.lastIndexByChapter[chapterID] = index
	return keys[index]
}

// TipsForChapter returns a copy of the eligible keys for chapterID.
func (p *Provider) TipsForChapter(chapterID int) []string {
	return slices.Clone(p.eligibleTips(chapterID))
}

func (p *Provider) eligibleTips(chapterID int) []string {
	if keys, ok := p.byChapter[chapterID]; ok {
		return keys
	}
	return p.global
}
