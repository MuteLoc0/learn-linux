package ui

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"learn-linux/internal/contract"
)

type stubI18N struct {
	lang    contract.Lang
	strings map[string]string
}

func newStubI18N(lang contract.Lang) *stubI18N {
	m := map[string]string{
		contract.KeyMOTDOSName:           "LearnLinux v1.0",
		contract.KeyMOTDWelcome:          "Welcome to LearnLinux, a virtualized terminal that never touches host files.",
		contract.KeyMOTDProgress:         "Progress: %d/%d",
		contract.KeyMOTDQuickCmds:        "Quick commands",
		contract.KeyTipsPrefix:           "tips: ",
		contract.KeyTipTab:               "Press Tab to autocomplete commands and paths.",
		contract.KeyTipCDPrevious:        "Use cd - to switch back to your previous directory.",
		contract.KeyMissionHeader:        "Mission",
		contract.KeyMissionCommands:      "Commands",
		contract.KeyMissionGoals:         "Objectives",
		contract.KeyMissionDone:          "DONE",
		contract.KeyMissionTodo:          "TODO",
		contract.KeyMissionAllPassed:     "All chapter objectives completed!",
		contract.KeyMissionSandboxDesc:   "Free exploration mode with full virtual filesystem.",
		contract.KeyHintHeader:           "Hint (%d/%d)",
		contract.KeyHintNone:             "No hints available.",
		contract.KeyChapterHeader:        "Chapters",
		contract.KeyChapterColID:         "ID",
		contract.KeyChapterColTitle:      "Title",
		contract.KeyChapterColStatus:     "Status",
		contract.KeyChapterStatusDone:    "Done",
		contract.KeyChapterStatusCurrent: "Current",
		contract.KeyChapterStatusOpen:    "Open",
		contract.KeyChapterUsage:         "Usage: chapter <n> | next | sandbox",
		contract.KeyClearedTitle:         "Chapter %d Cleared!",
		contract.KeyClearedBody:          "You mastered %s.",
		contract.KeyClearedNextHint:      "Run 'chapter %d' to continue.",
		contract.KeyClearedAllDone:       "Congratulations! You completed all chapters!",
		contract.KeySoftLockPrefix:       "Soft-lock detected:",
		contract.KeySoftLockAction:       "Run 'reset' to restore the chapter.",
		contract.KeyManSectionName:       "NAME",
		contract.KeyManSectionSynopsis:   "SYNOPSIS",
		contract.KeyManSectionDesc:       "DESCRIPTION",
		contract.KeyManSectionOptions:    "OPTIONS",
		contract.KeyManSectionExamples:   "EXAMPLES",
		contract.KeyManSectionSeeAlso:    "SEE ALSO",
		contract.KeyHelpTitle:            "LinuxLearn Help",
		contract.KeyHelpSubtitle:         "Interactive shell & tutorial reference",
		contract.KeyHelpTutorialCmds:     "Tutorial Commands",
		contract.KeyHelpShortcuts:        "Keyboard Shortcuts",
		contract.KeyHelpTip:              "Use 'man <cmd>' for detailed command documentation.",
		contract.KeyLangSwitched:         "Language switched to %s",
		contract.KeyResetDone:            "Chapter %d (%s) has been reset.",
		contract.KeySandboxTitle:         "Sandbox Mode",
		contract.KeySandboxDesc:          "Experiment freely in a safe in-memory Linux filesystem.",
		contract.KeyPagerHelp:            "q:quit /:search n:next",
		contract.KeyPagerSearchPrompt:    "/",
		contract.KeyPagerEOF:             "END",
		contract.KeyEditorHelp:           "Ctrl+O Save  Ctrl+X Exit",
		contract.KeyEditorModified:       "Modified",
		contract.KeyEditorConfirmExit:    "Save modified buffer? (Y/N)",
		contract.KeyPanicHeader:          "CRITICAL SYSTEM FAILURE",
		contract.KeyPanicDeleting:        "removed '%s'",
		contract.KeyPanicKernelPanic:     "Simulated panic triggered by: %s",
		contract.KeyPanicJoke:            "Relax! This is a pure in-memory virtual sandbox.",
		contract.KeyPanicCountdown:       "Restoring virtual snapshot in %ds...",
		contract.KeyPanicRestored:        "Virtual filesystem restored safely.",
		"ch1.title":                     "Navigation & Listing",
		"ch1.subtitle":                  "Moving around the filesystem",
		"ch1.intro":                     "Learn how to inspect your current directory and list files.",
		"ch1.goal.1":                    "Print current working directory with pwd",
		"ch1.goal.2":                    "List hidden files in home directory",
		"ch2.title":                     "文件与目录管理",
		"ch2.subtitle":                  "创建、复制与移动",
		"ch2.intro":                     "在虚拟终端中安全地管理文件和目录。",
		"ch1.softlock":                  "Required workspace file was deleted.",
	}
	if lang == contract.LangZH {
		m[contract.KeyChapterColID] = "编号"
		m[contract.KeyChapterColTitle] = "章节标题"
		m[contract.KeyChapterColStatus] = "状态"
		m[contract.KeyChapterStatusDone] = "已完成"
		m[contract.KeyChapterStatusCurrent] = "进行中"
		m[contract.KeyChapterStatusOpen] = "未开始"
	}
	return &stubI18N{lang: lang, strings: m}
}

func (s *stubI18N) Lang() contract.Lang { return s.lang }
func (s *stubI18N) SetLang(lang contract.Lang) error {
	s.lang = lang
	return nil
}
func (s *stubI18N) Has(key string) bool {
	_, ok := s.strings[key]
	return ok
}
func (s *stubI18N) T(key string, args ...any) string {
	tmpl, ok := s.strings[key]
	if !ok {
		return key
	}
	if len(args) > 0 {
		return fmt.Sprintf(tmpl, args...)
	}
	return tmpl
}
func (s *stubI18N) ManPage(cmd string) (contract.ManDoc, bool) { return contract.ManDoc{}, false }
func (s *stubI18N) AllManCommands() []string                   { return nil }

type mockTerminal struct {
	width  int
	height int
	buf    strings.Builder
	events []string
}

func (m *mockTerminal) EnterAltScreen() error    { m.events = append(m.events, "alt_enter"); return nil }
func (m *mockTerminal) ExitAltScreen() error     { m.events = append(m.events, "alt_exit"); return nil }
func (m *mockTerminal) EnableRawMode() error     { return nil }
func (m *mockTerminal) DisableRawMode() error    { return nil }
func (m *mockTerminal) IsRawMode() bool          { return false }
func (m *mockTerminal) Size() (int, int)         { return m.width, m.height }
func (m *mockTerminal) ReadKey() (contract.KeyEvent, error) {
	return contract.KeyEvent{}, nil
}
func (m *mockTerminal) Write(p []byte) (int, error) { return m.buf.Write(p) }
func (m *mockTerminal) WriteString(s string) error {
	_, err := m.buf.WriteString(s)
	return err
}
func (m *mockTerminal) ClearScreen() error {
	m.events = append(m.events, "clear")
	return nil
}
func (m *mockTerminal) MoveCursor(row, col int) error {
	m.events = append(m.events, fmt.Sprintf("move:%d,%d", row, col))
	return nil
}
func (m *mockTerminal) HideCursor() error { m.events = append(m.events, "hide"); return nil }
func (m *mockTerminal) ShowCursor() error { m.events = append(m.events, "show"); return nil }
func (m *mockTerminal) BackendName() string { return "mock" }
func (m *mockTerminal) Close() error        { return nil }

func TestWidthAndCJKAlignment(t *testing.T) {
	if w := displayWidth("hello"); w != 5 {
		t.Fatalf("expected width 5, got %d", w)
	}
	if w := displayWidth("中文标题"); w != 8 {
		t.Fatalf("expected CJK width 8, got %d", w)
	}
	if w := displayWidth(styled("中文", boldCyan) + "ab"); w != 6 {
		t.Fatalf("expected ANSI+CJK width 6, got %d", w)
	}

	padded := padRight(styled("中文", fgGreen), 8)
	if displayWidth(padded) != 8 {
		t.Fatalf("expected padded display width 8, got %d", displayWidth(padded))
	}

	wrapped := wrapText("你好世界HelloWorld测试换行", 8)
	for _, line := range wrapped {
		if displayWidth(line) > 8 {
			t.Fatalf("wrapped line %q exceeds width 8 (got %d)", line, displayWidth(line))
		}
	}
}

func TestRenderMOTDAndPrompt(t *testing.T) {
	r := NewRenderer()
	i18n := newStubI18N(contract.LangEN)
	ch := contract.ChapterMeta{
		ID:          1,
		TitleKey:    "ch1.title",
		SubtitleKey: "ch1.subtitle",
		IntroKey:    "ch1.intro",
	}

	motd := r.RenderMOTD(i18n, ch, 2, 8, contract.KeyTipTab, 80)
	plain := stripANSI(motd)
	for _, want := range []string{"LearnLinux v1.0", "Progress: 2/8", "[ch1]", "Navigation & Listing", "mission", "sandbox", "exit", "tips: Press Tab to autocomplete commands and paths."} {
		if !strings.Contains(plain, want) {
			t.Errorf("MOTD missing %q in:\n%s", want, plain)
		}
	}
	if strings.Contains(plain, "▸ learn-linux") {
		t.Errorf("MOTD should not contain legacy banner prefix:\n%s", plain)
	}
	if !strings.Contains(motd, styled("exit", fgRed)) {
		t.Errorf("MOTD should highlight exit in red, got:\n%q", motd)
	}
	// Fits comfortably inside 80x24 terminal.
	if lines := strings.Count(plain, "\n") + 1; lines > 20 {
		t.Errorf("MOTD too tall for 80x24: %d lines", lines)
	}

	// Structured pipe-separated quick commands with red exit (退出)
	zhI18N := newStubI18N(contract.LangZH)
	zhI18N.strings[contract.KeyMOTDQuickCmds] = "快捷命令：mission (查看任务) | hint (获取提示) | check (检查进度) | chapter (切换章节) | man <命令> (查手册) | help (帮助) | exit (退出)"
	motdZH := r.RenderMOTD(zhI18N, ch, 0, 8, contract.KeyTipTab, 80)
	if !strings.Contains(motdZH, styled("exit (退出)", fgRed)) {
		t.Errorf("MOTD should highlight 'exit (退出)' in red, got:\n%q", motdZH)
	}
	if !strings.Contains(motdZH, styled("tips: Press Tab to autocomplete commands and paths.", ansiDim)) {
		t.Errorf("MOTD should style tips line in dim grey, got:\n%q", motdZH)
	}

	// Narrow 60-col check
	motdNarrow := r.RenderMOTD(i18n, ch, 0, 8, contract.KeyTipTab, 45)
	if motdNarrow == "" {
		t.Fatal("narrow MOTD empty")
	}

	// Prompt tests
	p1 := stripANSI(r.RenderPrompt(contract.PromptState{
		User:         "learner",
		Host:         "linux-virtual",
		CWD:          "/home/learner/projects",
		Home:         "/home/learner",
		ChapterID:    3,
		LastExitCode: 0,
	}))
	if p1 != "learner@linux-virtual:~/projects [ch3]$ " {
		t.Errorf("unexpected prompt: %q", p1)
	}

	pErr := r.RenderPrompt(contract.PromptState{
		User:         "learner",
		Host:         "linux-virtual",
		CWD:          "/home/learner",
		Home:         "/home/learner",
		IsSandbox:    true,
		LastExitCode: 1,
	})
	if !strings.Contains(pErr, fgRed) || stripANSI(pErr) != "learner@linux-virtual:~ [sandbox]$ " {
		t.Errorf("unexpected error/sandbox prompt: %q", pErr)
	}
}

func TestRenderMissionHintChapterListAndCleared(t *testing.T) {
	r := NewRenderer()
	i18n := newStubI18N(contract.LangEN)
	ch1 := contract.ChapterMeta{
		ID:          1,
		TitleKey:    "ch1.title",
		SubtitleKey: "ch1.subtitle",
		IntroKey:    "ch1.intro",
		Commands:    []string{"pwd", "ls", "cd"},
	}
	ch2 := contract.ChapterMeta{
		ID:          2,
		TitleKey:    "ch2.title",
		SubtitleKey: "ch2.subtitle",
		IntroKey:    "ch2.intro",
		Commands:    []string{"mkdir", "touch", "cp", "mv"},
	}

	state := contract.ChapterState{
		Meta: ch1,
		Goals: []contract.TaskGoal{
			{ID: "g1", DescKey: "ch1.goal.1", Completed: true},
			{ID: "g2", DescKey: "ch1.goal.2", Completed: false},
		},
		TipKey: contract.KeyTipCDPrevious,
	}
	missionRaw := r.RenderMission(i18n, state, 80)
	mission := stripANSI(missionRaw)
	for _, want := range []string{"Navigation & Listing", "✓", "○", "DONE", "TODO", "pwd", "tips: Use cd - to switch back to your previous directory."} {
		if !strings.Contains(mission, want) {
			t.Errorf("RenderMission missing %q in:\n%s", want, mission)
		}
	}
	if !strings.Contains(mission, "Commands: pwd  ls  cd\n\ntips: Use cd - to switch back to your previous directory.\n\nObjectives") {
		t.Errorf("RenderMission should place tips one blank line below Commands and above Objectives:\n%s", mission)
	}
	if !strings.Contains(missionRaw, styled("tips: Use cd - to switch back to your previous directory.", ansiDim)) {
		t.Errorf("RenderMission should style tips in dim grey matching Commands label:\n%q", missionRaw)
	}

	state.AllPassed = true
	state.SoftLocked = true
	state.SoftLockReasonKey = "ch1.softlock"
	missionPassed := stripANSI(r.RenderMission(i18n, state, 120))
	if !strings.Contains(missionPassed, "All chapter objectives completed!") || !strings.Contains(missionPassed, "Soft-lock detected:") {
		t.Errorf("RenderMission missing all-passed or soft-lock notice:\n%s", missionPassed)
	}

	hint := stripANSI(r.RenderHint(i18n, 1, 1, 3, "Try running ls -la inside your home directory.", 60))
	if !strings.Contains(hint, "Hint (1/3)") || !strings.Contains(hint, "▎") {
		t.Errorf("unexpected RenderHint:\n%s", hint)
	}

	// Chapter list with CJK title alignment
	zhI18N := newStubI18N(contract.LangZH)
	table := r.RenderChapterList(zhI18N, []contract.ChapterMeta{ch1, ch2}, 2, map[int]bool{1: true}, 90)
	tableLines := strings.Split(stripANSI(table), "\n")
	// Header row and data rows should have aligned status column end positions.
	row1W := displayWidth(tableLines[2])
	row2W := displayWidth(tableLines[4])
	row3W := displayWidth(tableLines[5])
	if row1W != row2W || row2W != row3W {
		t.Errorf("CJK chapter table rows misaligned: header=%d row1=%d row2=%d\n%s", row1W, row2W, row3W, stripANSI(table))
	}

	clearedNext := stripANSI(r.RenderChapterCleared(i18n, ch1, 2, true, 80))
	if !strings.Contains(clearedNext, "Chapter 1 Cleared!") || !strings.Contains(clearedNext, "chapter 2") {
		t.Errorf("unexpected RenderChapterCleared (hasNext=true):\n%s", clearedNext)
	}
	clearedFinal := stripANSI(r.RenderChapterCleared(i18n, ch1, 0, false, 80))
	if !strings.Contains(clearedFinal, "Congratulations!") {
		t.Errorf("unexpected RenderChapterCleared (hasNext=false):\n%s", clearedFinal)
	}
}

func TestRenderManHelpNoticesAndCompletions(t *testing.T) {
	r := NewRenderer()
	i18n := newStubI18N(contract.LangEN)

	doc := contract.ManDoc{
		Name:        "grep",
		Summary:     "print lines that match patterns",
		Synopsis:    "grep [OPTIONS] PATTERN [FILE...]",
		Description: "grep searches for PATTERNS in each FILE and prints matching lines.",
		Options:     []contract.ManOption{{Flag: "-i, --ignore-case", Desc: "Ignore case distinctions."}},
		Examples:    []contract.ManExample{{Cmd: "grep -rn 'error' /var/log", Desc: "Search recursively with line numbers."}},
		SeeAlso:     []string{"find", "sed"},
	}
	man := stripANSI(r.RenderManPage(i18n, doc, 80))
	for _, want := range []string{"GREP(1)", "NAME", "SYNOPSIS", "-i, --ignore-case", "$ grep -rn", "find(1)"} {
		if !strings.Contains(man, want) {
			t.Errorf("RenderManPage missing %q", want)
		}
	}

	help := stripANSI(r.RenderHelp(i18n, 80))
	for _, want := range []string{"LinuxLearn Help", "mission", "Tab", "Ctrl+C"} {
		if !strings.Contains(help, want) {
			t.Errorf("RenderHelp missing %q", want)
		}
	}

	if got := stripANSI(r.RenderLangSwitched(i18n, contract.LangZH)); !strings.Contains(got, "zh") {
		t.Errorf("unexpected RenderLangSwitched: %q", got)
	}
	if got := stripANSI(r.RenderResetDone(i18n, contract.ChapterMeta{ID: 1, TitleKey: "ch1.title"})); !strings.Contains(got, "Chapter 1") {
		t.Errorf("unexpected RenderResetDone: %q", got)
	}
	if got := stripANSI(r.RenderSandboxBanner(i18n, 70)); !strings.Contains(got, "Sandbox Mode") {
		t.Errorf("unexpected RenderSandboxBanner: %q", got)
	}
	if got := stripANSI(r.RenderSoftLockNotice(i18n, "ch1.softlock")); !strings.Contains(got, "reset") {
		t.Errorf("unexpected RenderSoftLockNotice: %q", got)
	}
	if got := stripANSI(r.RenderError("cat", "no such file")); got != "bash: cat: no such file" {
		t.Errorf("unexpected RenderError: %q", got)
	}

	comp := r.RenderCompletions([]string{"docs/", "notes.txt", "script.sh", "archive.tar.gz"}, 60)
	if !strings.Contains(comp, boldBlue) || !strings.Contains(stripANSI(comp), "docs/") {
		t.Errorf("unexpected RenderCompletions: %q", comp)
	}
}

func TestRenderLSGridLongTreeAndGrep(t *testing.T) {
	r := NewRenderer()
	ts := time.Date(2026, 11, 22, 19, 0, 0, 0, time.UTC)

	items := []contract.LSItem{
		{DisplayName: "docs", Info: &contract.NodeInfo{Name: "docs", Type: contract.TypeDir, Mode: 0o755, Size: 4096, ModTime: ts}},
		{DisplayName: "run.sh", IsExecutable: true, Info: &contract.NodeInfo{Name: "run.sh", Type: contract.TypeFile, Mode: 0o755, Size: 512, ModTime: ts}},
		{DisplayName: "latest", Info: &contract.NodeInfo{Name: "latest", Type: contract.TypeSymlink, Mode: 0o777, Size: 12, LinkTarget: "docs/v2", ModTime: ts}},
		{DisplayName: "app.tar.gz", Info: &contract.NodeInfo{Name: "app.tar.gz", Type: contract.TypeFile, Mode: 0o644, Size: 1258291, ModTime: ts}},
		{DisplayName: "readme.md", Info: &contract.NodeInfo{Name: "readme.md", Type: contract.TypeFile, Mode: 0o644, Size: 120, ModTime: ts}},
	}

	gridColor := r.RenderLSGrid(items, 80, true)
	if !strings.Contains(gridColor, boldBlue) || !strings.Contains(gridColor, boldGreen) || !strings.Contains(gridColor, boldCyan) || !strings.Contains(gridColor, boldRed) {
		t.Errorf("RenderLSGrid missing expected type colors: %q", gridColor)
	}
	gridPlain := r.RenderLSGrid(items, 80, false)
	if strings.Contains(gridPlain, "\x1b[") {
		t.Errorf("RenderLSGrid(color=false) emitted ANSI escapes: %q", gridPlain)
	}

	longHuman := stripANSI(r.RenderLSLong(items, true, false))
	for _, want := range []string{"total ", "drwxr-xr-x", "4.0K", "1.2M", "latest -> docs/v2"} {
		if !strings.Contains(longHuman, want) {
			t.Errorf("RenderLSLong(-h) missing %q in:\n%s", want, longHuman)
		}
	}
	longBytes := stripANSI(r.RenderLSLong(items, false, true))
	if !strings.Contains(longBytes, "1258291") {
		t.Errorf("RenderLSLong(bytes) missing raw byte size:\n%s", longBytes)
	}

	root := &contract.TreeNode{
		Info: &contract.NodeInfo{Name: "project", Type: contract.TypeDir},
		Children: []*contract.TreeNode{
			{
				Info: &contract.NodeInfo{Name: "src", Type: contract.TypeDir},
				Children: []*contract.TreeNode{
					{Info: &contract.NodeInfo{Name: "main.go", Type: contract.TypeFile}},
				},
			},
			{Info: &contract.NodeInfo{Name: "README.md", Type: contract.TypeFile}},
		},
	}
	treeOut := stripANSI(r.RenderTree(root, 1, 2, false))
	for _, want := range []string{"├── src", "│   └── main.go", "└── README.md", "1 directory, 2 files"} {
		if !strings.Contains(treeOut, want) {
			t.Errorf("RenderTree missing %q in:\n%s", want, treeOut)
		}
	}

	// GrepMatch with overlapping, unsorted, and out-of-bounds ranges.
	gm := contract.GrepMatch{
		FilePath:    "/var/log/app.log",
		LineNum:     42,
		Line:        "ERROR timeout connecting",
		ShowFile:    true,
		ShowLineNum: true,
		MatchRanges: [][2]int{{10, 999}, {0, 5}, {-3, 3}, {2, 8}},
	}
	grepColored := r.RenderGrepMatch(gm, true)
	if !strings.Contains(grepColored, boldRed) || stripANSI(grepColored) != "/var/log/app.log:42:ERROR timeout connecting" {
		t.Errorf("unexpected RenderGrepMatch: %q", grepColored)
	}
	grepPlain := r.RenderGrepMatch(gm, false)
	if grepPlain != "/var/log/app.log:42:ERROR timeout connecting" {
		t.Errorf("unexpected plain RenderGrepMatch: %q", grepPlain)
	}
}

func TestRenderPagerAndEditor(t *testing.T) {
	r := NewRenderer()
	i18n := newStubI18N(contract.LangEN)

	pagerOut := r.RenderPager(i18n, contract.PagerState{
		Title:       "notes.txt",
		Lines:       []string{"hello world", "second line hello"},
		TopLine:     0,
		SearchQuery: "hello",
		Width:       60,
		Height:      6,
	})
	if !strings.Contains(pagerOut, ansiReverse+"hello"+ansiReset) {
		t.Errorf("RenderPager did not highlight search query in reverse video:\n%q", pagerOut)
	}
	if !strings.Contains(stripANSI(pagerOut), "~") || !strings.Contains(stripANSI(pagerOut), "notes.txt") {
		t.Errorf("RenderPager missing ~ EOF marker or title:\n%s", stripANSI(pagerOut))
	}

	pagerSearch := stripANSI(r.RenderPager(i18n, contract.PagerState{
		Lines:       []string{"alpha"},
		SearchMode:  true,
		SearchInput: "alp",
		Width:       50,
		Height:      5,
	}))
	if !strings.Contains(pagerSearch, "/alp") {
		t.Errorf("RenderPager search prompt missing /alp:\n%s", pagerSearch)
	}

	editorOut := r.RenderEditor(i18n, contract.EditorState{
		FilePath:  "/home/learner/todo.txt",
		Lines:     []string{"first line", "hello 中文"},
		CursorRow: 1,
		CursorCol: 8, // after "hello 中文" (5 + 1 + 2*2 = 10 cells -> col 11)
		ScrollTop: 0,
		Modified:  true,
		StatusMsg: "Wrote 2 lines",
		Width:     80,
		Height:    12,
	})
	if !strings.Contains(stripANSI(editorOut), "GNU nano (virtual)") || !strings.Contains(stripANSI(editorOut), "[Modified]") {
		t.Errorf("RenderEditor missing header/modified badge:\n%s", stripANSI(editorOut))
	}
	// Row 1 = header, Row 2 = line 0, Row 3 = line 1; Col = 1 + (5 + 1 + 4) = 11 -> \x1b[3;11H
	if !strings.HasSuffix(editorOut, "\x1b[3;11H") {
		t.Errorf("RenderEditor expected trailing cursor escape \\x1b[3;11H, got suffix %q", editorOut[maxInt(0, len(editorOut)-16):])
	}

	editorConfirm := stripANSI(r.RenderEditor(i18n, contract.EditorState{
		FilePath:    "a.txt",
		Lines:       []string{"x"},
		ConfirmExit: true,
		Width:       60,
		Height:      10,
	}))
	if !strings.Contains(editorConfirm, "Save modified buffer?") {
		t.Errorf("RenderEditor missing ConfirmExit prompt:\n%s", editorConfirm)
	}
}

func TestPlayDestructivePanic(t *testing.T) {
	r := NewRenderer()
	r.PanicTickDuration = 0 // zero-delay for instant deterministic test
	i18n := newStubI18N(contract.LangEN)
	term := &mockTerminal{width: 80, height: 24}

	start := time.Now()
	if err := r.PlayDestructivePanic(term, i18n, "rm -rf /", 3); err != nil {
		t.Fatalf("PlayDestructivePanic error: %v", err)
	}
	if elapsed := time.Since(start); elapsed > 200*time.Millisecond {
		t.Errorf("PlayDestructivePanic with zero tick took too long: %v", elapsed)
	}

	out := stripANSI(term.buf.String())
	for _, want := range []string{
		"CRITICAL SYSTEM FAILURE",
		"removed '/bin/ls'",
		"Kernel panic - not syncing",
		"rm -rf /",
		"Relax! This is a pure in-memory virtual sandbox.",
		"Restoring virtual snapshot in 3s...",
		"Restoring virtual snapshot in 1s...",
		"Virtual filesystem restored safely.",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("PlayDestructivePanic output missing %q\n%s", want, out)
		}
	}

	if len(term.events) == 0 || term.events[0] != "hide" || term.events[len(term.events)-1] != "show" {
		t.Errorf("expected hide/show cursor lifecycle, got %v", term.events)
	}
}
