package ui

import (
	"fmt"
	"strings"
	"time"

	"learn-linux/internal/contract"
)

// Renderer implements contract.UIRenderer with adaptive ANSI terminal styling.
type Renderer struct {
	// PanicTickDuration controls the delay per countdown tick in PlayDestructivePanic.
	// Defaults to 1 second; set to 0 in unit tests for instant execution.
	PanicTickDuration time.Duration
}

// NewRenderer creates a Renderer with default timing and terminal styling.
func NewRenderer() *Renderer {
	return &Renderer{
		PanicTickDuration: time.Second,
	}
}

var _ contract.UIRenderer = (*Renderer)(nil)

// RenderMOTD renders the virtual Linux login banner, progress bar, active chapter, and quick strip.
func (r *Renderer) RenderMOTD(i18n contract.I18N, ch contract.ChapterMeta, completedCount, totalChapters int, tipKey string, termWidth int) string {
	w := clampProseWidth(termWidth)
	ruleW := minInt(w, 78)

	var b strings.Builder
	osName := trText(i18n, contract.KeyMOTDOSName)
	b.WriteString(styled(osName, boldCyan))
	b.WriteByte('\n')
	b.WriteString(hairline(ruleW))
	b.WriteByte('\n')

	welcome := trText(i18n, contract.KeyMOTDWelcome)
	for _, line := range wrapText(welcome, w) {
		b.WriteString(line)
		b.WriteByte('\n')
	}

	progText, usedArgs := tr(i18n, contract.KeyMOTDProgress, completedCount, totalChapters)
	bar := progressBar(completedCount, totalChapters, 14)
	if usedArgs {
		b.WriteString(fmt.Sprintf("%s  %s\n", styled(progText, ansiBold), bar))
	} else {
		b.WriteString(fmt.Sprintf("%s  %s  %d/%d\n", styled(progText, ansiBold), bar, completedCount, totalChapters))
	}

	b.WriteByte('\n')

	// Active chapter summary with clean left accent (no heavy box borders).
	accent := styled("▎", fgCyan)
	chTitle := trText(i18n, ch.TitleKey)
	chSub := trText(i18n, ch.SubtitleKey)
	tag := formatChapterTag(ch)
	b.WriteString(fmt.Sprintf("%s %s %s", accent, styled(tag, boldCyan), styled(chTitle, ansiBold)))
	if chSub != "" && chSub != ch.SubtitleKey {
		b.WriteString(styled(" · "+chSub, ansiDim))
	}
	b.WriteByte('\n')

	intro := trText(i18n, ch.IntroKey)
	for _, line := range wrapText(intro, w-3) {
		b.WriteString(fmt.Sprintf("%s %s\n", accent, line))
	}

	b.WriteByte('\n')

	quickLabel := trText(i18n, contract.KeyMOTDQuickCmds)
	writeQuickCmds(&b, quickLabel, w)

	writeTipLines(&b, i18n, tipKey, w)

	b.WriteString(hairline(ruleW))
	return b.String()
}

func writeQuickCmds(b *strings.Builder, quickLabel string, width int) {
	if !strings.Contains(quickLabel, "mission") {
		quickCmds := []string{"mission", "hint", "chapter", "man", "lang", "sandbox", "exit"}
		styledCmds := make([]string, len(quickCmds))
		for i, c := range quickCmds {
			if c == "exit" {
				styledCmds[i] = styled(c, fgRed)
			} else {
				styledCmds[i] = styled(c, fgCyan)
			}
		}
		b.WriteString(fmt.Sprintf("%s %s\n", styled(quickLabel+":", ansiDim), strings.Join(styledCmds, styled(" · ", ansiDim))))
		return
	}

	prefix := ""
	rest := quickLabel
	if idx := strings.Index(quickLabel, "："); idx >= 0 {
		end := idx + len("：")
		prefix = quickLabel[:end]
		rest = strings.TrimSpace(quickLabel[end:])
	} else if idx := strings.IndexByte(quickLabel, ':'); idx >= 0 {
		prefix = quickLabel[:idx+1] + " "
		rest = strings.TrimSpace(quickLabel[idx+1:])
	}

	rawItems := strings.Split(rest, "|")
	items := make([]string, 0, len(rawItems))
	for _, raw := range rawItems {
		if item := strings.TrimSpace(raw); item != "" {
			items = append(items, item)
		}
	}

	sepPlain := " | "
	sepW := displayWidth(sepPlain)
	sepStyled := styled(sepPlain, ansiDim)

	curW := 0
	if prefix != "" {
		b.WriteString(styled(prefix, ansiDim))
		curW = displayWidth(prefix)
	}
	lineHasItem := false
	for _, item := range items {
		itemW := displayWidth(item)
		needW := itemW
		if lineHasItem {
			needW += sepW
		}
		if lineHasItem && curW+needW > width {
			b.WriteByte('\n')
			curW = 0
			lineHasItem = false
		}
		if lineHasItem {
			b.WriteString(sepStyled)
			curW += sepW
		}
		b.WriteString(styleQuickCmdItem(item))
		curW += itemW
		lineHasItem = true
	}
	b.WriteByte('\n')
}

func styleQuickCmdItem(item string) string {
	if item == "exit" || strings.HasPrefix(item, "exit ") || strings.HasPrefix(item, "exit(") {
		return styled(item, fgRed)
	}
	return styled(item, fgCyan)
}

func resolveTipLine(i18n contract.I18N, tipKey string) string {
	if tipKey == "" {
		return ""
	}
	tip := strings.TrimSpace(trText(i18n, tipKey))
	if tip == "" {
		return ""
	}
	prefix := trText(i18n, contract.KeyTipsPrefix)
	if prefix == "" || prefix == contract.KeyTipsPrefix {
		prefix = "tips: "
	}
	return prefix + tip
}

func writeTipLines(b *strings.Builder, i18n contract.I18N, tipKey string, width int) {
	tipLine := resolveTipLine(i18n, tipKey)
	if tipLine == "" {
		return
	}
	for _, line := range wrapText(tipLine, width) {
		b.WriteString(styled(line, ansiDim))
		b.WriteByte('\n')
	}
}

// RenderPrompt renders the classic Linux shell prompt (PS1) with adaptive ANSI hierarchy.
func (r *Renderer) RenderPrompt(state contract.PromptState) string {
	user := state.User
	if user == "" {
		user = "learner"
	}
	host := state.Host
	if host == "" {
		host = "linux-virtual"
	}

	cwd := state.CWD
	if cwd == "" {
		cwd = "/"
	}
	home := strings.TrimRight(state.Home, "/")
	if home != "" {
		if cwd == home {
			cwd = "~"
		} else if strings.HasPrefix(cwd, home+"/") {
			cwd = "~" + cwd[len(home):]
		}
	}

	var tag string
	if state.IsSandbox {
		tag = " " + styled("[sandbox]", ansiDim)
	} else if state.ChapterID > 0 {
		tag = " " + styled(fmt.Sprintf("[ch%d]", state.ChapterID), ansiDim)
	}

	dollarColor := fgGreen
	if state.LastExitCode != 0 {
		dollarColor = fgRed
	}

	return styled(user+"@"+host, boldGreen) +
		styled(":", ansiDim) +
		styled(cwd, boldBlue) +
		tag +
		styled("$", dollarColor) + " "
}

// RenderMission renders the current chapter's title, intro, commands, and goal checklist.
func (r *Renderer) RenderMission(i18n contract.I18N, state contract.ChapterState, termWidth int) string {
	w := clampProseWidth(termWidth)
	ruleW := minInt(w, 78)

	var b strings.Builder
	ch := state.Meta
	title := trText(i18n, ch.TitleKey)
	sub := trText(i18n, ch.SubtitleKey)

	b.WriteString(styled("▸ ", boldCyan))
	if ch.IsSandbox {
		b.WriteString(styled(title, ansiBold))
	} else {
		header, usedArgs := tr(i18n, contract.KeyMissionHeader, ch.ID, title)
		if usedArgs {
			b.WriteString(styled(header, ansiBold))
		} else {
			b.WriteString(styled(fmt.Sprintf("%s · Ch.%d %s", header, ch.ID, title), ansiBold))
		}
	}
	if sub != "" && sub != ch.SubtitleKey {
		b.WriteString(styled("  "+sub, ansiDim))
	}
	b.WriteByte('\n')
	b.WriteString(hairline(ruleW))
	b.WriteByte('\n')

	intro := trText(i18n, ch.IntroKey)
	for _, line := range wrapText(intro, w) {
		b.WriteString(line)
		b.WriteByte('\n')
	}

	if ch.IsSandbox {
		b.WriteByte('\n')
		for _, line := range wrapText(trText(i18n, contract.KeyMissionSandboxDesc), w) {
			b.WriteString(styled(line, fgMagenta))
			b.WriteByte('\n')
		}
	}

	if len(ch.Commands) > 0 {
		b.WriteByte('\n')
		cmdLabel := trText(i18n, contract.KeyMissionCommands)
		pills := make([]string, len(ch.Commands))
		for i, c := range ch.Commands {
			pills[i] = styled(c, boldCyan)
		}
		b.WriteString(styled(cmdLabel+": ", ansiDim))
		b.WriteString(strings.Join(pills, styled("  ", ansiDim)))
		b.WriteByte('\n')
	}

	if tipLine := resolveTipLine(i18n, state.TipKey); tipLine != "" {
		b.WriteByte('\n')
		for _, line := range wrapText(tipLine, w) {
			b.WriteString(styled(line, ansiDim))
			b.WriteByte('\n')
		}
	}

	if len(state.Goals) > 0 {
		b.WriteByte('\n')
		goalsLabel := trText(i18n, contract.KeyMissionGoals)
		b.WriteString(styled(goalsLabel, ansiBold))
		b.WriteByte('\n')

		doneBadge := styled("["+trText(i18n, contract.KeyMissionDone)+"]", dimGreen)
		todoBadge := styled("["+trText(i18n, contract.KeyMissionTodo)+"]", ansiDim)

		for _, g := range state.Goals {
			desc := trText(i18n, g.DescKey)
			wrapped := wrapText(desc, maxInt(20, w-14))
			icon := styled("○", ansiDim)
			badge := todoBadge
			if g.Completed {
				icon = styled("✓", boldGreen)
				badge = doneBadge
			}
			for i, line := range wrapped {
				if i == 0 {
					b.WriteString(fmt.Sprintf("  %s %s", icon, line))
					if len(wrapped) == 1 {
						b.WriteString("  " + badge)
					}
				} else {
					b.WriteString(fmt.Sprintf("    %s", line))
					if i == len(wrapped)-1 {
						b.WriteString("  " + badge)
					}
				}
				b.WriteByte('\n')
			}
		}
	}

	if state.AllPassed {
		b.WriteByte('\n')
		b.WriteString(styled("✓ "+trText(i18n, contract.KeyMissionAllPassed), boldGreen))
		b.WriteByte('\n')
	}

	if state.SoftLocked && state.SoftLockReasonKey != "" {
		b.WriteByte('\n')
		b.WriteString(r.RenderSoftLockNotice(i18n, state.SoftLockReasonKey))
		b.WriteByte('\n')
	}

	return strings.TrimRight(b.String(), "\n")
}

// RenderHint renders a warm amber/yellow accented hint block with wrapping.
func (r *Renderer) RenderHint(i18n contract.I18N, chapterID int, hintIdx, totalHints int, hintText string, termWidth int) string {
	w := clampProseWidth(termWidth)
	bar := styled("▎", fgYellow)

	dispIdx := hintIdx
	if dispIdx <= 0 && totalHints > 0 {
		dispIdx = 1
	}

	var header string
	if totalHints > 0 {
		resolved, usedArgs := tr(i18n, contract.KeyHintHeader, dispIdx, totalHints)
		if usedArgs {
			header = resolved
		} else {
			header = fmt.Sprintf("%s (%d/%d)", resolved, dispIdx, totalHints)
		}
	} else {
		header = trText(i18n, contract.KeyHintHeader)
	}

	body := hintText
	if body == "" {
		body = trText(i18n, contract.KeyHintNone)
	}

	var b strings.Builder
	b.WriteString(fmt.Sprintf("%s %s\n", bar, styled(header, boldYellow)))
	for _, line := range wrapText(body, maxInt(20, w-2)) {
		b.WriteString(fmt.Sprintf("%s %s\n", bar, line))
	}
	return strings.TrimRight(b.String(), "\n")
}

// RenderChapterList renders a CJK-width-aligned table of all chapters and their completion status.
func (r *Renderer) RenderChapterList(i18n contract.I18N, chapters []contract.ChapterMeta, currentID int, completed map[int]bool, termWidth int) string {
	w := termWidth
	if w <= 0 {
		w = 80
	}
	if w < 60 {
		w = 60
	}
	ruleW := minInt(w, 88)

	var b strings.Builder
	b.WriteString(styled("▸ "+trText(i18n, contract.KeyChapterHeader), boldCyan))
	b.WriteByte('\n')
	b.WriteString(hairline(ruleW))
	b.WriteByte('\n')

	colID := trText(i18n, contract.KeyChapterColID)
	colTitle := trText(i18n, contract.KeyChapterColTitle)
	colStatus := trText(i18n, contract.KeyChapterColStatus)
	colCmds := trText(i18n, contract.KeyMissionCommands)

	statusDone := trText(i18n, contract.KeyChapterStatusDone)
	statusCurr := trText(i18n, contract.KeyChapterStatusCurrent)
	statusOpen := trText(i18n, contract.KeyChapterStatusOpen)

	idW := maxInt(displayWidth(colID), 4) + 2 // room for "▸ 01"
	statusW := maxInt(displayWidth(colStatus), maxInt(displayWidth("✓ "+statusDone), maxInt(displayWidth("▸ "+statusCurr), displayWidth("○ "+statusOpen))))
	showCmds := ruleW >= 68
	cmdsW := 0
	if showCmds {
		cmdsW = minInt(22, maxInt(14, ruleW/4))
	}
	gaps := 4
	if showCmds {
		gaps = 6
	}
	titleW := maxInt(16, ruleW-idW-statusW-cmdsW-gaps)

	// Table header
	b.WriteString(styled(padRight(colID, idW), ansiBold))
	b.WriteString("  ")
	b.WriteString(styled(padRight(colTitle, titleW), ansiBold))
	if showCmds {
		b.WriteString("  ")
		b.WriteString(styled(padRight(colCmds, cmdsW), ansiBold))
	}
	b.WriteString("  ")
	b.WriteString(styled(padRight(colStatus, statusW), ansiBold))
	b.WriteByte('\n')
	b.WriteString(hairline(ruleW))
	b.WriteByte('\n')

	for _, ch := range chapters {
		isCurr := ch.ID == currentID
		isDone := completed != nil && completed[ch.ID]

		idLabel := fmt.Sprintf("  %02d", ch.ID)
		if ch.IsSandbox {
			idLabel = "  SB"
		}
		if isCurr {
			idLabel = strings.Replace(idLabel, "  ", "▸ ", 1)
		}

		title := truncatePlain(trText(i18n, ch.TitleKey), titleW, "…")
		var statusCell string
		switch {
		case isDone:
			statusCell = styled(padRight("✓ "+statusDone, statusW), fgGreen)
		case isCurr:
			statusCell = styled(padRight("▸ "+statusCurr, statusW), boldCyan)
		default:
			statusCell = styled(padRight("○ "+statusOpen, statusW), ansiDim)
		}

		if isCurr {
			b.WriteString(styled(padRight(idLabel, idW), boldCyan))
			b.WriteString("  ")
			b.WriteString(styled(padRight(title, titleW), ansiBold))
		} else {
			b.WriteString(padRight(idLabel, idW))
			b.WriteString("  ")
			b.WriteString(padRight(title, titleW))
		}

		if showCmds {
			cmdPreview := truncatePlain(strings.Join(ch.Commands, ", "), cmdsW, "…")
			b.WriteString("  ")
			b.WriteString(styled(padRight(cmdPreview, cmdsW), ansiDim))
		}

		b.WriteString("  ")
		b.WriteString(statusCell)
		b.WriteByte('\n')
	}

	b.WriteString(hairline(ruleW))
	b.WriteByte('\n')
	b.WriteString(styled(trText(i18n, contract.KeyChapterUsage), ansiDim))
	return b.String()
}

// RenderChapterCleared renders a celebratory green/gold completion banner.
func (r *Renderer) RenderChapterCleared(i18n contract.I18N, ch contract.ChapterMeta, nextID int, hasNext bool, termWidth int) string {
	w := clampProseWidth(termWidth)
	ruleW := minInt(w, 76)

	title := trText(i18n, ch.TitleKey)
	clearedTitle, usedTitleArgs := tr(i18n, contract.KeyClearedTitle, ch.ID, title)
	if !usedTitleArgs && !ch.IsSandbox {
		clearedTitle = fmt.Sprintf("%s · Ch.%d", clearedTitle, ch.ID)
	}

	var b strings.Builder
	b.WriteString(styled(strings.Repeat("─", ruleW), fgGreen))
	b.WriteByte('\n')
	b.WriteString(styled("★ "+clearedTitle, boldGreen))
	b.WriteByte('\n')

	body, _ := tr(i18n, contract.KeyClearedBody, title, ch.ID)
	for _, line := range wrapText(body, w) {
		b.WriteString(line)
		b.WriteByte('\n')
	}

	if hasNext {
		nextHint, usedNextArgs := tr(i18n, contract.KeyClearedNextHint, nextID)
		if !usedNextArgs {
			nextHint = fmt.Sprintf("%s (chapter %d)", nextHint, nextID)
		}
		b.WriteString(styled("→ ", boldYellow))
		b.WriteString(styled(nextHint, fgCyan))
		b.WriteByte('\n')
	} else {
		allDone := trText(i18n, contract.KeyClearedAllDone)
		b.WriteString(styled("★ "+allDone, boldYellow))
		b.WriteByte('\n')
	}

	b.WriteString(styled(strings.Repeat("─", ruleW), fgGreen))
	return b.String()
}

// RenderSoftLockNotice renders an amber warning line highlighting the reset command.
func (r *Renderer) RenderSoftLockNotice(i18n contract.I18N, reasonKey string) string {
	prefix := trText(i18n, contract.KeySoftLockPrefix)
	reason := trText(i18n, reasonKey)
	action := highlightToken(trText(i18n, contract.KeySoftLockAction), "reset", boldCyan)
	return fmt.Sprintf("%s %s %s %s", styled("⚠", boldYellow), styled(prefix, fgYellow), reason, action)
}

// RenderManPage renders a command manual entry in authentic Unix man page typography.
func (r *Renderer) RenderManPage(i18n contract.I18N, doc contract.ManDoc, termWidth int) string {
	w := clampProseWidth(termWidth)
	headerTag := fmt.Sprintf("%s(1)", strings.ToUpper(doc.Name))
	midLabel := "LinuxLearn Manual"
	leftW := displayWidth(headerTag)
	midW := displayWidth(midLabel)
	gap1 := maxInt(2, (w-2*leftW-midW)/2)
	gap2 := maxInt(2, w-2*leftW-midW-gap1)

	var b strings.Builder
	b.WriteString(styled(headerTag, ansiBold))
	b.WriteString(strings.Repeat(" ", gap1))
	b.WriteString(styled(midLabel, ansiDim))
	b.WriteString(strings.Repeat(" ", gap2))
	b.WriteString(styled(headerTag, ansiBold))
	b.WriteString("\n\n")

	writeSection := func(key string) {
		b.WriteString(styled(trText(i18n, key), ansiBold))
		b.WriteByte('\n')
	}

	// NAME
	writeSection(contract.KeyManSectionName)
	b.WriteString("    ")
	b.WriteString(styled(doc.Name, boldCyan))
	if doc.Summary != "" {
		b.WriteString(" - " + doc.Summary)
	}
	b.WriteString("\n\n")

	// SYNOPSIS
	if doc.Synopsis != "" {
		writeSection(contract.KeyManSectionSynopsis)
		for _, line := range wrapText(doc.Synopsis, maxInt(20, w-4)) {
			b.WriteString("    ")
			b.WriteString(styled(line, ansiBold))
			b.WriteByte('\n')
		}
		b.WriteByte('\n')
	}

	// DESCRIPTION
	if doc.Description != "" {
		writeSection(contract.KeyManSectionDesc)
		for _, line := range wrapText(doc.Description, maxInt(20, w-4)) {
			b.WriteString("    " + line + "\n")
		}
		b.WriteByte('\n')
	}

	// OPTIONS
	if len(doc.Options) > 0 {
		writeSection(contract.KeyManSectionOptions)
		for _, opt := range doc.Options {
			b.WriteString("    ")
			b.WriteString(styled(opt.Flag, boldCyan))
			b.WriteByte('\n')
			for _, line := range wrapText(opt.Desc, maxInt(20, w-8)) {
				b.WriteString("        " + line + "\n")
			}
		}
		b.WriteByte('\n')
	}

	// EXAMPLES
	if len(doc.Examples) > 0 {
		writeSection(contract.KeyManSectionExamples)
		for _, ex := range doc.Examples {
			b.WriteString("    ")
			b.WriteString(styled("$ ", fgGreen))
			b.WriteString(styled(ex.Cmd, fgCyan))
			b.WriteByte('\n')
			if ex.Desc != "" {
				for _, line := range wrapText(ex.Desc, maxInt(20, w-8)) {
					b.WriteString("        " + styled(line, ansiDim) + "\n")
				}
			}
		}
		b.WriteByte('\n')
	}

	// SEE ALSO
	if len(doc.SeeAlso) > 0 {
		writeSection(contract.KeyManSectionSeeAlso)
		refs := make([]string, len(doc.SeeAlso))
		for i, ref := range doc.SeeAlso {
			refs[i] = styled(ref+"(1)", fgCyan)
		}
		b.WriteString("    " + strings.Join(refs, ", ") + "\n")
	}

	return strings.TrimRight(b.String(), "\n")
}

// RenderHelp renders a scannable quick-reference card for tutorial commands and shell shortcuts.
func (r *Renderer) RenderHelp(i18n contract.I18N, termWidth int) string {
	w := clampProseWidth(termWidth)
	ruleW := minInt(w, 78)

	var b strings.Builder
	b.WriteString(styled("▸ "+trText(i18n, contract.KeyHelpTitle), boldCyan))
	sub := trText(i18n, contract.KeyHelpSubtitle)
	if sub != "" && sub != contract.KeyHelpSubtitle {
		b.WriteString(styled("  "+sub, ansiDim))
	}
	b.WriteByte('\n')
	b.WriteString(hairline(ruleW))
	b.WriteByte('\n')

	tutText := trText(i18n, contract.KeyHelpTutorialCmds)
	writeHelpBlock(&b, tutText, w, []string{
		"mission", "hint", "check", "chapter", "reset", "man", "lang", "sandbox", "help", "clear", "exit",
	})

	b.WriteByte('\n')

	shortText := trText(i18n, contract.KeyHelpShortcuts)
	writeHelpBlock(&b, shortText, w, []string{
		"Tab", "Ctrl+C", "Ctrl+D", "Ctrl+L", "Ctrl+A", "Ctrl+E", "Ctrl+U", "Ctrl+K", "Ctrl+W", "Up/Down",
	})

	tip := trText(i18n, contract.KeyHelpTip)
	if tip != "" {
		b.WriteByte('\n')
		for _, line := range wrapText(tip, w) {
			b.WriteString(styled(line, ansiDim))
			b.WriteByte('\n')
		}
	}

	b.WriteString(hairline(ruleW))
	return b.String()
}

func writeHelpBlock(b *strings.Builder, text string, width int, fallbackTokens []string) {
	if strings.Contains(text, "\n") {
		lines := strings.Split(text, "\n")
		for i, line := range lines {
			trimmed := strings.TrimSpace(line)
			if i == 0 && !strings.HasPrefix(line, " ") && !strings.HasPrefix(line, "\t") {
				b.WriteString(styled(trimmed, ansiBold))
				b.WriteByte('\n')
				continue
			}
			// Highlight leading token if separated by spaces or colon.
			if idx := strings.Index(trimmed, "  "); idx > 0 {
				tok := trimmed[:idx]
				rest := strings.TrimSpace(trimmed[idx:])
				b.WriteString(fmt.Sprintf("  %s  %s\n", padRight(styled(tok, fgCyan), 14), rest))
			} else {
				for _, wl := range wrapText(trimmed, maxInt(20, width-2)) {
					b.WriteString("  " + wl + "\n")
				}
			}
		}
		return
	}

	b.WriteString(styled(text, ansiBold))
	b.WriteByte('\n')
	styledToks := make([]string, len(fallbackTokens))
	for i, t := range fallbackTokens {
		styledToks[i] = styled(t, fgCyan)
	}
	b.WriteString("  " + strings.Join(styledToks, styled(" · ", ansiDim)) + "\n")
}

// RenderLangSwitched renders a crisp confirmation after switching UI language.
func (r *Renderer) RenderLangSwitched(i18n contract.I18N, lang contract.Lang) string {
	msg, usedArgs := tr(i18n, contract.KeyLangSwitched, string(lang))
	if !usedArgs && lang != "" {
		msg = fmt.Sprintf("%s (%s)", msg, lang)
	}
	return fmt.Sprintf("%s %s", styled("✓", boldGreen), msg)
}

// RenderResetDone renders a crisp notice after resetting a chapter environment.
func (r *Renderer) RenderResetDone(i18n contract.I18N, ch contract.ChapterMeta) string {
	title := trText(i18n, ch.TitleKey)
	if title == "" {
		title = formatChapterTag(ch)
	}
	raw := trText(i18n, contract.KeyResetDone)
	var msg string
	var usedArgs bool
	if countFormatVerbs(raw) >= 2 {
		msg, usedArgs = tr(i18n, contract.KeyResetDone, ch.ID, title)
	} else {
		msg, usedArgs = tr(i18n, contract.KeyResetDone, title)
	}
	if !usedArgs {
		msg = fmt.Sprintf("%s · %s", msg, formatChapterTag(ch))
	}
	return fmt.Sprintf("%s %s", styled("↺", boldCyan), msg)
}

// RenderSandboxBanner renders the free-exploration sandbox mode header.
func (r *Renderer) RenderSandboxBanner(i18n contract.I18N, termWidth int) string {
	w := clampProseWidth(termWidth)
	ruleW := minInt(w, 76)

	var b strings.Builder
	b.WriteString(styled("▸ "+trText(i18n, contract.KeySandboxTitle), boldMagenta))
	b.WriteByte('\n')
	b.WriteString(hairline(ruleW))
	b.WriteByte('\n')
	for _, line := range wrapText(trText(i18n, contract.KeySandboxDesc), w) {
		b.WriteString(line)
		b.WriteByte('\n')
	}
	b.WriteString(hairline(ruleW))
	return b.String()
}

// RenderCompletions formats Tab-completion candidates into a bash-style column-major grid.
func (r *Renderer) RenderCompletions(candidates []string, termWidth int) string {
	if len(candidates) == 0 {
		return ""
	}
	w := clampGridWidth(termWidth)
	entries := make([]gridEntry, len(candidates))
	for i, c := range candidates {
		colored := c
		if strings.HasSuffix(c, "/") {
			colored = styled(c, boldBlue)
		}
		entries[i] = gridEntry{
			plainWidth: displayWidth(c),
			rendered:   colored,
		}
	}
	return formatColumnGrid(entries, w)
}

// RenderError formats a shell-style error line with subtle red accent.
func (r *Renderer) RenderError(cmd, msg string) string {
	if cmd == "" {
		return styled("error:", fgRed) + " " + msg
	}
	return styled("bash:", fgRed) + " " + cmd + ": " + msg
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
