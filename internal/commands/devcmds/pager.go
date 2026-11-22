package devcmds

import (
	"errors"
	"fmt"
	"io"
	"sort"
	"strings"
	"unicode/utf8"

	"learn-linux/internal/contract"
)

func less(ec *contract.ExecContext, args []string) int {
	title := ""
	var data []byte
	var err error
	if len(args) > 0 && args[0] != "-" {
		title = args[0]
		data, err = ec.VFS.ReadFile(resolve(ec, title))
	} else if ec.Stdin != nil {
		data, err = io.ReadAll(ec.Stdin)
	}
	if err != nil {
		msg := err.Error()
		if errors.Is(err, contract.ErrNotExist) {
			msg = "No such file or directory"
		}
		return commandError(ec, "less", "%s: %s", title, msg)
	}
	if !ec.IsTTY || ec.Term == nil || !ec.Term.IsRawMode() || ec.UI == nil {
		if _, err := ec.Stdout.Write(data); err != nil {
			return commandError(ec, "less", "%v", err)
		}
		return 0
	}

	width, height := terminalSize(ec)
	state := contract.PagerState{
		Title: title, Lines: textLines(data), CurrentMatch: -1,
		Width: width, Height: height,
	}
	defer ec.Term.ClearScreen()
	for {
		if err := ec.Term.WriteString(ec.UI.RenderPager(ec.I18N, state)); err != nil {
			return commandError(ec, "less", "%v", err)
		}
		key, err := ec.Term.ReadKey()
		if errors.Is(err, io.EOF) || key.Kind == contract.KeyEOF {
			return 0
		}
		if err != nil {
			return commandError(ec, "less", "%v", err)
		}
		if key.Kind == contract.KeyResize {
			state.Width, state.Height = key.Width, key.Height
			continue
		}
		if state.SearchMode {
			pagerSearch(ec.I18N, &state, key)
			continue
		}
		state.StatusMsg = ""
		r := key.Rune
		if key.Kind != contract.KeyRune {
			r = 0
		}
		page := max(1, state.Height-2)
		bottom := max(0, len(state.Lines)-max(2, state.Height-1))
		switch {
		case r == 'q' || key.Kind == contract.KeyCtrlC || key.Kind == contract.KeyEscape:
			return 0
		case r == 'j' || key.Kind == contract.KeyDown || key.Kind == contract.KeyEnter:
			state.TopLine = min(state.TopLine+1, max(bottom, state.TopLine))
		case r == 'k' || key.Kind == contract.KeyUp:
			state.TopLine = max(0, state.TopLine-1)
		case r == ' ' || key.Kind == contract.KeyPageDown:
			state.TopLine = min(state.TopLine+page, max(bottom, state.TopLine))
		case r == 'b' || key.Kind == contract.KeyPageUp:
			state.TopLine = max(0, state.TopLine-page)
		case r == 'g' || key.Kind == contract.KeyHome:
			state.TopLine = 0
		case r == 'G' || key.Kind == contract.KeyEnd:
			state.TopLine = bottom
		case r == '/':
			state.SearchMode, state.SearchInput = true, ""
		case r == 'n' || r == 'N':
			if count := len(state.MatchLines); count > 0 {
				next := state.CurrentMatch + 1
				if r == 'N' {
					next = state.CurrentMatch - 1
					if state.CurrentMatch < 0 {
						next = count - 1
					}
				}
				state.CurrentMatch = (next + count) % count
				state.TopLine = state.MatchLines[state.CurrentMatch]
			}
		}
	}
}

func pagerSearch(i18n contract.I18N, state *contract.PagerState, key contract.KeyEvent) {
	switch key.Kind {
	case contract.KeyRune:
		state.SearchInput += string(key.Rune)
	case contract.KeyBackspace:
		_, size := utf8.DecodeLastRuneInString(state.SearchInput)
		state.SearchInput = state.SearchInput[:len(state.SearchInput)-size]
	case contract.KeyEscape, contract.KeyCtrlC:
		state.SearchMode, state.SearchInput = false, ""
	case contract.KeyEnter:
		state.SearchMode = false
		state.SearchQuery = state.SearchInput
		state.MatchLines = nil
		state.CurrentMatch = -1
		query := strings.ToLower(state.SearchQuery)
		if query == "" {
			return
		}
		for i, line := range state.Lines {
			if strings.Contains(strings.ToLower(line), query) {
				state.MatchLines = append(state.MatchLines, i)
			}
		}
		at := sort.SearchInts(state.MatchLines, state.TopLine)
		if at == len(state.MatchLines) {
			state.StatusMsg = localized(i18n, contract.KeyPagerNotFound, "Pattern not found: %s", state.SearchQuery)
			return
		}
		state.CurrentMatch = at
		state.TopLine = state.MatchLines[at]
	}
}

func terminalSize(ec *contract.ExecContext) (int, int) {
	width, height := ec.TermWidth, ec.TermHeight
	if width <= 0 || height <= 0 {
		w, h := ec.Term.Size()
		if width <= 0 {
			width = w
		}
		if height <= 0 {
			height = h
		}
	}
	if width <= 0 {
		width = 80
	}
	if height <= 0 {
		height = 24
	}
	return width, height
}

func textLines(data []byte) []string {
	text := strings.ReplaceAll(string(data), "\r\n", "\n")
	return strings.Split(strings.TrimSuffix(text, "\n"), "\n")
}

func localized(i18n contract.I18N, key, fallback string, args ...any) string {
	if i18n != nil {
		return i18n.T(key, args...)
	}
	return fmt.Sprintf(fallback, args...)
}
