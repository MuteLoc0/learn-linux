package ui

import (
	"fmt"
	"strings"

	"learn-linux/internal/contract"
)

// RenderPager renders the fullscreen `less` pager for state.Width x state.Height.
func (r *Renderer) RenderPager(i18n contract.I18N, state contract.PagerState) string {
	w, h := normalizeViewport(state.Width, state.Height, 20, 3)
	contentRows := h - 1

	var b strings.Builder
	b.WriteString("\x1b[H")

	top := state.TopLine
	if top < 0 {
		top = 0
	}

	for row := 0; row < contentRows; row++ {
		idx := top + row
		if idx < len(state.Lines) {
			expanded := expandTabs(state.Lines[idx], 4)
			vis := truncatePlain(expanded, w, "")
			if state.SearchQuery != "" {
				vis = highlightSearchMatches(vis, state.SearchQuery)
			}
			b.WriteString(vis)
		} else {
			b.WriteString(styled("~", ansiDim))
		}
		b.WriteString("\x1b[K\r\n")
	}

	// Bottom inverse-video status bar
	var status string
	switch {
	case state.SearchMode:
		status = trText(i18n, contract.KeyPagerSearchPrompt) + state.SearchInput
	case state.StatusMsg != "":
		status = state.StatusMsg
	default:
		total := len(state.Lines)
		startLine := minInt(top+1, total)
		endLine := minInt(top+contentRows, total)
		posInfo := trText(i18n, contract.KeyPagerEOF)
		if total > 0 && endLine < total {
			pct := (endLine * 100) / total
			posInfo = fmt.Sprintf("L%d-%d/%d (%d%%)", startLine, endLine, total, pct)
		} else if total > 0 {
			posInfo = fmt.Sprintf("L%d-%d/%d (%s)", startLine, endLine, total, posInfo)
		}
		help := trText(i18n, contract.KeyPagerHelp)
		title := state.Title
		if title != "" {
			status = fmt.Sprintf(" %s  %s  %s", title, posInfo, help)
		} else {
			status = fmt.Sprintf(" %s  %s", posInfo, help)
		}
	}

	status = padRight(truncatePlain(status, w, "…"), w)
	b.WriteString(styled(status, ansiReverse))
	b.WriteString("\x1b[K")
	return b.String()
}

func highlightSearchMatches(line, query string) string {
	if query == "" {
		return line
	}
	lowerLine := strings.ToLower(line)
	lowerQuery := strings.ToLower(query)
	if !strings.Contains(lowerLine, lowerQuery) {
		return line
	}
	// Work on runes when byte lengths diverge on Unicode case folding; fast path on equal byte lengths.
	if len(lowerLine) == len(line) {
		var b strings.Builder
		offset := 0
		for {
			idx := strings.Index(lowerLine[offset:], lowerQuery)
			if idx < 0 {
				b.WriteString(line[offset:])
				break
			}
			start := offset + idx
			end := start + len(lowerQuery)
			b.WriteString(line[offset:start])
			b.WriteString(styled(line[start:end], ansiReverse))
			offset = end
		}
		return b.String()
	}
	lineRunes := []rune(line)
	queryRunes := []rune(lowerQuery)
	if len(queryRunes) == 0 {
		return line
	}
	var b strings.Builder
	for i := 0; i < len(lineRunes); {
		if i+len(queryRunes) <= len(lineRunes) && strings.ToLower(string(lineRunes[i:i+len(queryRunes)])) == lowerQuery {
			b.WriteString(styled(string(lineRunes[i:i+len(queryRunes)]), ansiReverse))
			i += len(queryRunes)
			continue
		}
		b.WriteRune(lineRunes[i])
		i++
	}
	return b.String()
}

// RenderEditor renders the fullscreen `nano`-style editor with accurate cursor placement.
func (r *Renderer) RenderEditor(i18n contract.I18N, state contract.EditorState) string {
	w, h := normalizeViewport(state.Width, state.Height, 20, 5)
	contentRows := h - 4

	var b strings.Builder
	b.WriteString("\x1b[H")

	// Row 1: Top inverse header bar
	modFlag := ""
	if state.Modified {
		modFlag = "[" + trText(i18n, contract.KeyEditorModified) + "]"
	}
	leftTitle := "  GNU nano (virtual)"
	rightPad := 2
	filePart := state.FilePath
	availCenter := w - displayWidth(leftTitle) - displayWidth(modFlag) - rightPad - 2
	if availCenter > 0 && filePart != "" {
		filePart = truncatePlain(filePart, availCenter, "…")
	} else {
		filePart = ""
	}
	leftSpace := maxInt(1, (w-displayWidth(filePart))/2-displayWidth(leftTitle))
	if displayWidth(leftTitle)+leftSpace+displayWidth(filePart)+displayWidth(modFlag)+rightPad > w {
		leftSpace = 1
	}
	header := leftTitle + strings.Repeat(" ", leftSpace) + filePart
	if modFlag != "" {
		rem := w - displayWidth(header) - displayWidth(modFlag) - 1
		if rem > 0 {
			header += strings.Repeat(" ", rem) + modFlag + " "
		} else {
			header += " " + modFlag
		}
	}
	header = padRight(truncatePlain(header, w, ""), w)
	b.WriteString(styled(header, ansiReverse))
	b.WriteString("\x1b[K\r\n")

	// Calculate horizontal scroll if CursorCol exceeds viewport width
	scrollTop := maxInt(0, state.ScrollTop)
	curRow := clampInt(state.CursorRow, 0, maxInt(0, len(state.Lines)-1))
	curRunes := []rune{}
	if curRow < len(state.Lines) {
		curRunes = []rune(expandTabs(state.Lines[curRow], 4))
	}
	curColRunes := clampInt(state.CursorCol, 0, len(curRunes))
	if curRow < len(state.Lines) && strings.ContainsRune(state.Lines[curRow], '\t') {
		curColRunes = expandedRuneCol(state.Lines[curRow], state.CursorCol, 4)
	}
	cursorCellX := 0
	for i := 0; i < curColRunes && i < len(curRunes); i++ {
		cursorCellX += runeWidth(curRunes[i])
	}
	scrollX := 0
	if cursorCellX >= w {
		scrollX = cursorCellX - w + 4
	}

	// Rows 2..h-3: Editable text viewport
	for rIdx := 0; rIdx < contentRows; rIdx++ {
		lineIdx := scrollTop + rIdx
		if lineIdx < len(state.Lines) {
			expanded := expandTabs(state.Lines[lineIdx], 4)
			vis := slicePlainByCols(expanded, scrollX, w)
			b.WriteString(vis)
		}
		b.WriteString("\x1b[K\r\n")
	}

	// Row h-2: Status / confirm exit prompt
	if state.ConfirmExit {
		prompt := truncatePlain(" "+trText(i18n, contract.KeyEditorConfirmExit)+" ", w, "")
		b.WriteString(styled(padRight(prompt, w), ansiReverse))
	} else if state.StatusMsg != "" {
		msg := "[ " + state.StatusMsg + " ]"
		msg = truncatePlain(msg, w, "")
		leftPad := maxInt(0, (w-displayWidth(msg))/2)
		b.WriteString(strings.Repeat(" ", leftPad))
		b.WriteString(styled(msg, ansiReverse))
	}
	b.WriteString("\x1b[K\r\n")

	// Rows h-1..h: Bottom shortcut bar
	row1, row2 := buildEditorHelpRows(i18n, w)
	b.WriteString(row1)
	b.WriteString("\x1b[K\r\n")
	b.WriteString(row2)
	b.WriteString("\x1b[K")

	// Final cursor position (1-based ANSI row;col)
	screenRow := 2 + (curRow - scrollTop)
	screenRow = clampInt(screenRow, 2, 1+contentRows)
	screenCol := 1 + (cursorCellX - scrollX)
	screenCol = clampInt(screenCol, 1, w)
	b.WriteString(fmt.Sprintf("\x1b[%d;%dH", screenRow, screenCol))

	return b.String()
}

func buildEditorHelpRows(i18n contract.I18N, w int) (string, string) {
	help := trText(i18n, contract.KeyEditorHelp)
	if w < 34 {
		r1 := styled("^O", ansiReverse) + " Save " + styled("^X", ansiReverse) + " Exit"
		r2 := styled("^K", ansiReverse) + " Cut  " + styled("^A/^E", ansiReverse) + " Bol/Eol"
		if displayWidth(r1) > w {
			r1 = truncatePlain(stripANSI(r1), w, "")
		}
		if displayWidth(r2) > w {
			r2 = truncatePlain(stripANSI(r2), w, "")
		}
		return r1, r2
	}
	row1 := styled("^O", ansiReverse) + " Write Out   " +
		styled("^X", ansiReverse) + " Exit   " +
		styled("^K", ansiReverse) + " Cut"
	row2 := styled("^A", ansiReverse) + " Home        " +
		styled("^E", ansiReverse) + " End    " +
		styled("  ", ansiReset) + styled(truncatePlain(help, maxInt(4, w-30), "…"), ansiDim)
	return row1, row2
}

func normalizeViewport(width, height, minW, minH int) (int, int) {
	if width <= 0 {
		width = 80
	}
	if height <= 0 {
		height = 24
	}
	if width < minW {
		width = minW
	}
	if height < minH {
		height = minH
	}
	return width, height
}

func expandTabs(s string, tabWidth int) string {
	if !strings.ContainsRune(s, '\t') {
		return s
	}
	return strings.ReplaceAll(s, "\t", strings.Repeat(" ", tabWidth))
}

func expandedRuneCol(s string, rawRuneCol, tabWidth int) int {
	col := 0
	i := 0
	for _, r := range s {
		if i >= rawRuneCol {
			break
		}
		if r == '\t' {
			col += tabWidth
		} else {
			col++
		}
		i++
	}
	return col
}

func clampInt(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}
