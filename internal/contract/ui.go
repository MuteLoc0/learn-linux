package contract

// PromptState provides context to render the virtual Linux shell prompt (PS1).
type PromptState struct {
	User         string
	Host         string
	CWD          string
	Home         string
	ChapterID    int
	IsSandbox    bool
	LastExitCode int
}

// LSItem represents a single entry formatted by the ls command.
type LSItem struct {
	Info         *NodeInfo
	DisplayName  string
	IsExecutable bool
}

// TreeNode represents a node in the directory tree output.
type TreeNode struct {
	Info     *NodeInfo
	Children []*TreeNode
}

// GrepMatch represents a single matched line from grep.
type GrepMatch struct {
	FilePath    string
	LineNum     int
	Line        string
	MatchRanges [][2]int
	ShowFile    bool
	ShowLineNum bool
}

// PagerState holds rendering state for the built-in interactive less pager.
type PagerState struct {
	Title        string
	Lines        []string
	TopLine      int
	SearchQuery  string
	SearchMode   bool
	SearchInput  string
	MatchLines   []int
	CurrentMatch int
	StatusMsg    string
	Width        int
	Height       int
}

// EditorState holds rendering state for the built-in lightweight nano editor.
type EditorState struct {
	FilePath    string
	Lines       []string
	CursorRow   int
	CursorCol   int
	ScrollTop   int
	Modified    bool
	StatusMsg   string
	ConfirmExit bool
	Width       int
	Height      int
}

// UIRenderer centralizes all terminal typography, ANSI styling, formatting, and animations.
// Owned exclusively by Gemini 4 Hydrogen so visual quality stays cohesive across the app.
type UIRenderer interface {
	RenderMOTD(i18n I18N, ch ChapterMeta, completedCount, totalChapters int, termWidth int) string
	RenderPrompt(state PromptState) string
	RenderMission(i18n I18N, state ChapterState, termWidth int) string
	RenderHint(i18n I18N, chapterID int, hintIdx, totalHints int, hintText string, termWidth int) string
	RenderChapterList(i18n I18N, chapters []ChapterMeta, currentID int, completed map[int]bool, termWidth int) string
	RenderChapterCleared(i18n I18N, ch ChapterMeta, nextID int, hasNext bool, termWidth int) string
	RenderSoftLockNotice(i18n I18N, reasonKey string) string
	RenderManPage(i18n I18N, doc ManDoc, termWidth int) string
	RenderHelp(i18n I18N, termWidth int) string
	RenderLangSwitched(i18n I18N, lang Lang) string
	RenderResetDone(i18n I18N, ch ChapterMeta) string
	RenderSandboxBanner(i18n I18N, termWidth int) string
	RenderCompletions(candidates []string, termWidth int) string
	RenderLSGrid(items []LSItem, termWidth int, color bool) string
	RenderLSLong(items []LSItem, humanReadable bool, color bool) string
	RenderTree(root *TreeNode, dirCount, fileCount int, color bool) string
	RenderGrepMatch(m GrepMatch, color bool) string
	RenderError(cmd, msg string) string
	RenderPager(i18n I18N, state PagerState) string
	RenderEditor(i18n I18N, state EditorState) string
	PlayDestructivePanic(term Terminal, i18n I18N, cmd string, seconds int) error
}
