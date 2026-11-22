package app

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"os/signal"
	"slices"
	"strconv"
	"strings"
	"syscall"

	"learn-linux/internal/commands/corecmds"
	"learn-linux/internal/commands/devcmds"
	"learn-linux/internal/contract"
	"learn-linux/internal/curriculum"
	"learn-linux/internal/i18n"
	"learn-linux/internal/shell"
	"learn-linux/internal/store"
	"learn-linux/internal/term"
	"learn-linux/internal/ui"
)

const (
	defaultUser = "learner"
	defaultHost = "linux-virtual"
	defaultHome = "/home/learner"
)

// Config configures the top-level learn-linux interactive application.
type Config struct {
	Term           contract.Terminal
	Store          contract.ProgressStore
	I18N           contract.I18N
	UI             contract.UIRenderer
	Curriculum     contract.Curriculum
	PanicSeconds   int
	PanicSecondsSet bool
	InitialChapter int
	StartSandbox   bool
	InitialLang    contract.Lang
	NoAltScreen    bool
}

// App coordinates the terminal, shell engine, curriculum, UI renderer, and persistence.
type App struct {
	term       contract.Terminal
	reader     contract.LineReader
	store      contract.ProgressStore
	i18n       contract.I18N
	ui         contract.UIRenderer
	curriculum contract.Curriculum
	shell      contract.ShellEngine
	out        *rawCRLFWriter

	progress                *contract.Progress
	chapterID               int
	hintIdx                 int
	lastExitCode            int
	chapterHistory          []contract.CommandRecord
	softLockNotified        bool
	chapterClearedInSession bool
	exitRequested           bool
	destructiveTriggered    bool
	destructiveCmd          string

	panicSeconds int
	noAltScreen  bool
}

// New constructs and wires a ready-to-run App.
func New(cfg Config) (*App, error) {
	t := cfg.Term
	if t == nil {
		var err error
		t, err = term.NewStdTerminal()
		if err != nil {
			return nil, fmt.Errorf("init terminal: %w", err)
		}
	}

	st := cfg.Store
	if st == nil {
		st = store.NewFileStore("")
	}

	prog, err := st.Load()
	if err != nil || prog == nil {
		prog = &contract.Progress{
			CurrentChapter:    1,
			CompletedChapters: []int{},
			Language:          contract.LangZH,
		}
	}

	lang := prog.Language
	if cfg.InitialLang == contract.LangZH || cfg.InitialLang == contract.LangEN {
		lang = cfg.InitialLang
		prog.Language = lang
	}
	if lang != contract.LangZH && lang != contract.LangEN {
		lang = contract.LangZH
		prog.Language = lang
	}

	bundle := cfg.I18N
	if bundle == nil {
		bundle, err = i18n.NewBundle(lang)
		if err != nil {
			return nil, fmt.Errorf("init i18n: %w", err)
		}
	} else {
		_ = bundle.SetLang(lang)
	}

	renderer := cfg.UI
	if renderer == nil {
		renderer = ui.NewRenderer()
	}

	cur := cfg.Curriculum
	if cur == nil {
		cur = curriculum.NewEngine()
	}

	panicSec := 5
	if cfg.PanicSecondsSet {
		panicSec = cfg.PanicSeconds
	}

	chapterID := prog.CurrentChapter
	if cfg.StartSandbox {
		chapterID = 0
	} else if cfg.InitialChapter > 0 {
		chapterID = cfg.InitialChapter
	}
	if chapterID < 0 || chapterID > len(cur.Chapters()) {
		chapterID = 1
	}

	a := &App{
		term:         t,
		reader:       term.NewLineReader(t),
		store:        st,
		i18n:         bundle,
		ui:           renderer,
		curriculum:   cur,
		out:          &rawCRLFWriter{term: t},
		progress:     prog,
		chapterID:    chapterID,
		panicSeconds: panicSec,
		noAltScreen:  cfg.NoAltScreen,
	}

	initialVFS, startCWD, err := a.loadStageVFS(chapterID)
	if err != nil {
		return nil, fmt.Errorf("load initial stage: %w", err)
	}

	reg := shell.NewRegistry()
	corecmds.Register(reg)
	devcmds.Register(reg)

	eng, err := shell.NewEngine(shell.Config{
		VFS:              initialVFS,
		Registry:         reg,
		CWD:              startCWD,
		Home:             defaultHome,
		User:             defaultUser,
		Host:             defaultHost,
		Term:             t,
		UI:               renderer,
		I18N:             bundle,
		Stdout:           a.out,
		Stderr:           a.out,
		OnDestructive:    a.onDestructive,
		OnTutorialAction: a.handleTutorialAction,
	})
	if err != nil {
		return nil, fmt.Errorf("init shell engine: %w", err)
	}
	if len(prog.CommandHistory) > 0 {
		eng.SetHistory(prog.CommandHistory)
	}
	a.shell = eng
	return a, nil
}

// Run starts the fullscreen virtual Linux terminal session and blocks until exit.
func (a *App) Run(ctx context.Context) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	sigCh := make(chan os.Signal, 2)
	signal.Notify(sigCh, syscall.SIGTERM)
	defer signal.Stop(sigCh)
	go func() {
		select {
		case <-sigCh:
			cancel()
			_ = a.term.Close()
		case <-ctx.Done():
		}
	}()

	if !a.noAltScreen {
		_ = a.term.EnterAltScreen()
	}
	_ = a.term.EnableRawMode()
	defer func() {
		a.persistProgress()
		_ = a.term.Close()
	}()

	width, _ := a.term.Size()
	if a.term.BackendName() == "fallback-line" {
		a.writeText(a.i18n.T(contract.KeyTermFallback) + "\n")
	}

	meta, ok := a.curriculum.ChapterMeta(a.chapterID)
	if !ok {
		meta = contract.ChapterMeta{ID: a.chapterID}
	}
	a.writeText(a.ui.RenderMOTD(a.i18n, meta, len(a.progress.CompletedChapters), len(a.curriculum.Chapters()), width))
	a.writeText("\n")

	for {
		if ctx.Err() != nil {
			break
		}
		prompt := a.ui.RenderPrompt(contract.PromptState{
			User:         defaultUser,
			Host:         defaultHost,
			CWD:          a.shell.CWD(),
			Home:         defaultHome,
			ChapterID:    a.chapterID,
			IsSandbox:    a.chapterID == 0,
			LastExitCode: a.lastExitCode,
		})

		line, interrupted, eof, err := a.reader.ReadLine(
			prompt,
			a.shell.History(),
			a.shell.Complete,
			func(candidates []string, w int) string {
				return normalizeCRLF(a.ui.RenderCompletions(candidates, w), a.term.IsRawMode())
			},
		)
		if err != nil {
			if err == io.EOF {
				a.writeText(a.i18n.T(contract.KeyExitGoodbye) + "\n")
				break
			}
			return err
		}
		if eof {
			a.writeText(a.i18n.T(contract.KeyExitGoodbye) + "\n")
			break
		}
		if interrupted {
			a.lastExitCode = 130
			continue
		}

		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}

		a.shell.AddHistory(trimmed)
		a.destructiveTriggered = false
		a.destructiveCmd = ""
		a.exitRequested = false

		rec, _ := a.shell.Execute(ctx, trimmed)
		a.lastExitCode = rec.ExitCode

		if a.exitRequested {
			a.writeText(a.i18n.T(contract.KeyExitGoodbye) + "\n")
			break
		}

		if a.destructiveTriggered {
			_ = a.ui.PlayDestructivePanic(a.term, a.i18n, a.destructiveCmd, a.panicSeconds)
			if err := a.reloadStage(a.chapterID); err == nil {
				w, _ := a.term.Size()
				if m, ok := a.curriculum.ChapterMeta(a.chapterID); ok {
					a.writeText("\n" + a.ui.RenderResetDone(a.i18n, m) + "\n")
					if a.chapterID > 0 {
						st := a.curriculum.Evaluate(a.chapterID, a.shell.VFS(), a.shell.Env(), a.chapterHistory, contract.CommandRecord{})
						a.writeText(a.ui.RenderMission(a.i18n, st, w) + "\n")
					}
				}
			}
			continue
		}

		if rec.ExitCode == 127 && rec.Command != "" {
			a.writeText(a.i18n.T(contract.KeyCmdNotFound, rec.Command) + "\n")
		}

		if a.chapterID == 0 || isTutorialCommand(rec.Command) {
			continue
		}

		a.chapterHistory = append(a.chapterHistory, rec)
		a.evaluateAfterCommand(rec)
	}

	return nil
}

func (a *App) onDestructive(cmdLine string) {
	a.destructiveTriggered = true
	a.destructiveCmd = cmdLine
}

func (a *App) evaluateAfterCommand(rec contract.CommandRecord) {
	state := a.curriculum.Evaluate(a.chapterID, a.shell.VFS(), a.shell.Env(), a.chapterHistory, rec)
	if state.SoftLocked && !a.softLockNotified {
		a.softLockNotified = true
		a.writeText("\n" + a.ui.RenderSoftLockNotice(a.i18n, state.SoftLockReasonKey) + "\n")
	} else if !state.SoftLocked {
		a.softLockNotified = false
	}

	if state.AllPassed && !a.chapterClearedInSession {
		a.chapterClearedInSession = true
		a.markChapterCompleted(a.chapterID)
		a.persistProgress()

		total := len(a.curriculum.Chapters())
		hasNext := a.chapterID < total
		nextID := a.chapterID + 1
		w, _ := a.term.Size()
		a.writeText("\n" + a.ui.RenderChapterCleared(a.i18n, state.Meta, nextID, hasNext, w) + "\n")
	}
}

func (a *App) handleTutorialAction(action contract.TutorialAction) (int, error) {
	w, _ := a.term.Size()
	switch action.Name {
	case "mission":
		if a.chapterID == 0 {
			a.writeBlock(a.ui.RenderSandboxBanner(a.i18n, w))
			return 0, nil
		}
		st := a.curriculum.Evaluate(a.chapterID, a.shell.VFS(), a.shell.Env(), a.chapterHistory, contract.CommandRecord{})
		a.writeBlock(a.ui.RenderMission(a.i18n, st, w))
		return 0, nil

	case "hint":
		if a.chapterID == 0 {
			a.writeBlock(a.i18n.T(contract.KeyHintNone))
			return 0, nil
		}
		key, nextIdx, total := a.curriculum.NextHintKey(a.chapterID, a.hintIdx)
		if key == "" || total == 0 {
			a.writeBlock(a.i18n.T(contract.KeyHintNone))
			return 0, nil
		}
		displayIdx := a.hintIdx + 1
		if displayIdx > total {
			displayIdx = total
		}
		a.hintIdx = nextIdx
		a.writeBlock(a.ui.RenderHint(a.i18n, a.chapterID, displayIdx, total, a.i18n.T(key), w))
		return 0, nil

	case "check":
		if a.chapterID == 0 {
			a.writeBlock(a.ui.RenderSandboxBanner(a.i18n, w))
			return 0, nil
		}
		st := a.curriculum.Evaluate(a.chapterID, a.shell.VFS(), a.shell.Env(), a.chapterHistory, contract.CommandRecord{})
		a.writeBlock(a.ui.RenderMission(a.i18n, st, w))
		if st.AllPassed {
			if !a.chapterClearedInSession {
				a.chapterClearedInSession = true
				a.markChapterCompleted(a.chapterID)
				a.persistProgress()
			}
			a.writeBlock(a.i18n.T(contract.KeyCheckPassed))
			return 0, nil
		}
		a.writeBlock(a.i18n.T(contract.KeyCheckIncomplete))
		return 0, nil

	case "chapter":
		if len(action.Args) == 0 || action.Args[0] == "list" {
			a.writeBlock(a.ui.RenderChapterList(a.i18n, a.curriculum.Chapters(), a.chapterID, a.completedMap(), w))
			return 0, nil
		}
		arg := strings.TrimSpace(action.Args[0])
		if arg == "sandbox" {
			return a.switchStage(0)
		}
		if arg == "next" {
			total := len(a.curriculum.Chapters())
			if a.chapterID == 0 {
				return a.switchStage(1)
			}
			if a.chapterID < total {
				return a.switchStage(a.chapterID + 1)
			}
			a.writeBlock(a.i18n.T(contract.KeyClearedAllDone))
			return 0, nil
		}
		id, err := strconv.Atoi(arg)
		if err != nil || id < 1 || id > len(a.curriculum.Chapters()) {
			a.writeBlock(a.ui.RenderError("chapter", a.i18n.T(contract.KeyChapterNotFound, arg)))
			a.writeBlock(a.i18n.T(contract.KeyChapterUsage))
			return 1, nil
		}
		return a.switchStage(id)

	case "reset":
		if len(action.Args) > 0 && action.Args[0] == "--all-progress" {
			_ = a.store.Reset()
			a.progress.CompletedChapters = nil
			a.progress.CurrentChapter = 1
			return a.switchStage(1)
		}
		if err := a.reloadStage(a.chapterID); err != nil {
			a.writeBlock(a.ui.RenderError("reset", err.Error()))
			return 1, nil
		}
		if m, ok := a.curriculum.ChapterMeta(a.chapterID); ok {
			a.writeBlock(a.ui.RenderResetDone(a.i18n, m))
		}
		return 0, nil

	case "sandbox":
		return a.switchStage(0)

	case "man":
		if len(action.Args) == 0 {
			a.writeBlock(a.i18n.T(contract.KeyManUsage))
			a.writeBlock(a.ui.RenderCompletions(a.i18n.AllManCommands(), w))
			return 0, nil
		}
		cmd := action.Args[0]
		doc, ok := a.i18n.ManPage(cmd)
		if !ok {
			a.writeBlock(a.ui.RenderError("man", a.i18n.T(contract.KeyManNotFound, cmd)))
			return 1, nil
		}
		a.writeBlock(a.ui.RenderManPage(a.i18n, doc, w))
		return 0, nil

	case "lang":
		if len(action.Args) == 0 {
			a.writeBlock(a.i18n.T(contract.KeyLangUsage))
			return 0, nil
		}
		target := contract.Lang(strings.ToLower(strings.TrimSpace(action.Args[0])))
		if err := a.i18n.SetLang(target); err != nil {
			a.writeBlock(a.ui.RenderError("lang", a.i18n.T(contract.KeyLangUsage)))
			return 1, nil
		}
		a.progress.Language = target
		a.persistProgress()
		a.writeBlock(a.ui.RenderLangSwitched(a.i18n, target))
		return 0, nil

	case "help":
		a.writeBlock(a.ui.RenderHelp(a.i18n, w))
		return 0, nil

	case "exit":
		a.exitRequested = true
		return 0, nil
	}
	return 0, nil
}

func (a *App) switchStage(id int) (int, error) {
	if err := a.reloadStage(id); err != nil {
		a.writeBlock(a.ui.RenderError("chapter", err.Error()))
		return 1, nil
	}
	a.progress.CurrentChapter = id
	a.persistProgress()
	w, _ := a.term.Size()
	if id == 0 {
		a.writeBlock(a.ui.RenderSandboxBanner(a.i18n, w))
		return 0, nil
	}
	st := a.curriculum.Evaluate(a.chapterID, a.shell.VFS(), a.shell.Env(), a.chapterHistory, contract.CommandRecord{})
	a.writeBlock(a.ui.RenderMission(a.i18n, st, w))
	return 0, nil
}

func (a *App) reloadStage(id int) error {
	v, startCWD, err := a.loadStageVFS(id)
	if err != nil {
		return err
	}
	a.chapterID = id
	a.hintIdx = 0
	a.chapterHistory = nil
	a.softLockNotified = false
	a.chapterClearedInSession = false
	a.shell.SetVFS(v, startCWD)
	return nil
}

func (a *App) loadStageVFS(id int) (contract.VFS, string, error) {
	if id == 0 {
		return a.curriculum.LoadSandbox()
	}
	return a.curriculum.LoadChapter(id)
}

func (a *App) markChapterCompleted(id int) {
	if id <= 0 {
		return
	}
	if !slices.Contains(a.progress.CompletedChapters, id) {
		a.progress.CompletedChapters = append(a.progress.CompletedChapters, id)
		slices.Sort(a.progress.CompletedChapters)
	}
}

func (a *App) completedMap() map[int]bool {
	m := make(map[int]bool, len(a.progress.CompletedChapters))
	for _, id := range a.progress.CompletedChapters {
		m[id] = true
	}
	return m
}

func (a *App) persistProgress() {
	if a.store == nil || a.progress == nil {
		return
	}
	if a.shell != nil {
		a.progress.CommandHistory = a.shell.History()
	}
	_ = a.store.Save(a.progress)
}

func (a *App) writeText(s string) {
	if s == "" {
		return
	}
	_, _ = a.out.Write([]byte(s))
}

func (a *App) writeBlock(s string) {
	if s == "" {
		return
	}
	if !strings.HasSuffix(s, "\n") {
		s += "\n"
	}
	_, _ = a.out.Write([]byte(s))
}

func isTutorialCommand(cmd string) bool {
	switch cmd {
	case "mission", "hint", "check", "chapter", "reset", "sandbox", "man", "lang", "help", "exit":
		return true
	default:
		return false
	}
}

// rawCRLFWriter translates bare LF (\n) into CRLF (\r\n) whenever the underlying
// terminal is in raw mode (where OPOST / ONLCR is disabled), preventing staircase output.
type rawCRLFWriter struct {
	term contract.Terminal
}

func (w *rawCRLFWriter) Write(p []byte) (int, error) {
	if !w.term.IsRawMode() || !bytes.ContainsRune(p, '\n') {
		return w.term.Write(p)
	}
	var buf bytes.Buffer
	buf.Grow(len(p) + 16)
	for i, b := range p {
		if b == '\n' && (i == 0 || p[i-1] != '\r') {
			buf.WriteByte('\r')
		}
		buf.WriteByte(b)
	}
	if _, err := w.term.Write(buf.Bytes()); err != nil {
		return 0, err
	}
	return len(p), nil
}

func normalizeCRLF(s string, raw bool) string {
	if !raw || !strings.Contains(s, "\n") {
		return s
	}
	var b strings.Builder
	b.Grow(len(s) + 16)
	for i := 0; i < len(s); i++ {
		if s[i] == '\n' && (i == 0 || s[i-1] != '\r') {
			b.WriteByte('\r')
		}
		b.WriteByte(s[i])
	}
	return b.String()
}
