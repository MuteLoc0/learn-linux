package contract

// TipProvider selects contextual or global command tips for display in MOTD and mission views.
// Returned strings are i18n keys resolved through I18N so tips respect live language switching.
type TipProvider interface {
	// PickTip returns a random eligible key, or "" when no tips are eligible.
	PickTip(chapterID int) string
	// TipsForChapter returns a copy of the global and chapter-specific keys.
	TipsForChapter(chapterID int) []string
}
