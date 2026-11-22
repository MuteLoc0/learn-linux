package term

import (
	"errors"
	"fmt"
	"io"
	"slices"
	"strings"
	"unicode"
	"unicode/utf8"

	"learn-linux/internal/contract"
)

// StandardLineReader reads command lines from a Terminal.
type StandardLineReader struct {
	terminal contract.Terminal
}

var _ contract.LineReader = (*StandardLineReader)(nil)

func NewLineReader(t contract.Terminal) *StandardLineReader {
	return &StandardLineReader{terminal: t}
}

func (r *StandardLineReader) ReadLine(prompt string, history []string,
	complete func(string, int) contract.CompletionResult, renderCandidates func([]string, int) string,
) (line string, interrupted bool, eof bool, err error) {
	t := r.terminal
	if !t.IsRawMode() {
		line, err := r.readCookedLine(prompt)
		if errors.Is(err, io.EOF) {
			// Deliver a final line without a newline before reporting EOF.
			return line, false, line == "", nil
		}
		return line, false, false, err
	}

	width, _ := t.Size()
	editor := lineEditor{
		prompt:       prompt,
		history:      history,
		historyIndex: len(history),
		display:      lineDisplay{terminal: t, width: width},
	}
	if err := t.WriteString(prompt); err != nil {
		return "", false, false, err
	}
	editor.display.record(prompt, len(prompt))

	for {
		key, err := t.ReadKey()
		if errors.Is(err, io.EOF) {
			return "", false, true, nil
		}
		if err != nil {
			return "", false, false, err
		}
		if key.Kind == contract.KeyEOF {
			return "", false, true, nil
		}

		if editor.search != nil {
			if editor.search.handle(key, history) {
				return history[editor.search.index], false, false, editor.display.finish("\r\n")
			}
			if key.Kind == contract.KeyEscape || key.Kind == contract.KeyCtrlC {
				editor.search = nil
			}
		} else {
			switch key.Kind {
			case contract.KeyEnter:
				return string(editor.buf), false, false, editor.display.finish("\r\n")
			case contract.KeyCtrlC:
				return "", true, false, editor.display.finish("^C\r\n")
			case contract.KeyCtrlD:
				if len(editor.buf) == 0 {
					return "", false, true, nil
				}
				editor.edit(key)
			case contract.KeyTab:
				if err := editor.complete(complete, renderCandidates); err != nil {
					return "", false, false, err
				}
			case contract.KeyCtrlL:
				if err := t.ClearScreen(); err != nil {
					return "", false, false, err
				}
				editor.display.reset()
			default:
				editor.edit(key)
			}
		}
		if key.Kind == contract.KeyResize && key.Width > 0 {
			editor.display.width = key.Width
		}
		if err := editor.redraw(); err != nil {
			return "", false, false, err
		}
	}
}

func (r *StandardLineReader) readCookedLine(prompt string) (string, error) {
	if err := r.terminal.WriteString(prompt); err != nil {
		return "", err
	}
	if input, ok := r.terminal.(interface{ readCookedLine() (string, error) }); ok {
		return input.readCookedLine()
	}
	var line strings.Builder
	for {
		key, err := r.terminal.ReadKey()
		if err != nil {
			return line.String(), err
		}
		switch key.Kind {
		case contract.KeyRune:
			line.WriteRune(key.Rune)
		case contract.KeyTab:
			line.WriteByte('\t')
		case contract.KeyEnter:
			return strings.TrimSuffix(line.String(), "\r"), nil
		case contract.KeyEOF, contract.KeyCtrlD:
			return strings.TrimSuffix(line.String(), "\r"), io.EOF
		}
	}
}

type lineEditor struct {
	prompt       string
	buf          []rune
	pos          int
	history      []string
	historyIndex int
	draft        []rune
	draftPos     int
	search       *historySearch
	display      lineDisplay
}

func (e *lineEditor) edit(key contract.KeyEvent) {
	switch key.Kind {
	case contract.KeyRune:
		e.buf = slices.Insert(e.buf, e.pos, key.Rune)
		e.pos++
	case contract.KeyBackspace:
		if e.pos > 0 {
			e.buf = slices.Delete(e.buf, e.pos-1, e.pos)
			e.pos--
		}
	case contract.KeyDelete, contract.KeyCtrlD:
		if e.pos < len(e.buf) {
			e.buf = slices.Delete(e.buf, e.pos, e.pos+1)
		}
	case contract.KeyLeft:
		e.pos = max(0, e.pos-1)
	case contract.KeyRight:
		e.pos = min(len(e.buf), e.pos+1)
	case contract.KeyHome, contract.KeyCtrlA:
		e.pos = 0
	case contract.KeyEnd, contract.KeyCtrlE:
		e.pos = len(e.buf)
	case contract.KeyCtrlU:
		e.buf = slices.Delete(e.buf, 0, e.pos)
		e.pos = 0
	case contract.KeyCtrlK:
		e.buf = e.buf[:e.pos]
	case contract.KeyCtrlW:
		start := e.pos
		for start > 0 && unicode.IsSpace(e.buf[start-1]) {
			start--
		}
		for start > 0 && !unicode.IsSpace(e.buf[start-1]) {
			start--
		}
		e.buf = slices.Delete(e.buf, start, e.pos)
		e.pos = start
	case contract.KeyUp:
		e.visitHistory(e.historyIndex - 1)
	case contract.KeyDown:
		e.visitHistory(e.historyIndex + 1)
	case contract.KeyCtrlR:
		e.search = &historySearch{}
		e.search.find(e.history, len(e.history)-1)
	}
}

func (e *lineEditor) visitHistory(index int) {
	if index < 0 || index > len(e.history) {
		return
	}
	if e.historyIndex == len(e.history) {
		e.draft, e.draftPos = e.buf, e.pos
	}
	e.historyIndex = index
	if index == len(e.history) {
		e.buf, e.pos = e.draft, e.draftPos
	} else {
		e.buf = []rune(e.history[index])
		e.pos = len(e.buf)
	}
}

func (e *lineEditor) complete(complete func(string, int) contract.CompletionResult,
	renderCandidates func([]string, int) string,
) error {
	if complete == nil {
		return nil
	}
	line := string(e.buf)
	cursor := len(string(e.buf[:e.pos]))
	res := complete(line, cursor)
	if len(res.Matches) == 0 {
		return nil
	}
	start, end := res.ReplaceStart, res.ReplaceEnd
	if start < 0 || start > cursor || cursor > end || end > len(line) ||
		(start < len(line) && !utf8.RuneStart(line[start])) ||
		(end < len(line) && !utf8.RuneStart(line[end])) {
		return nil
	}
	suffix := line[end:]
	var replacement string
	switch {
	case len(res.Matches) == 1:
		replacement = res.Matches[0]
		if !strings.HasSuffix(replacement, "/") {
			if strings.HasPrefix(suffix, " ") || strings.HasPrefix(suffix, "\t") {
				replacement += suffix[:1]
				suffix = suffix[1:]
			} else {
				replacement += " "
			}
		}
	case len(res.CommonPrefix) > cursor-start && strings.HasPrefix(res.CommonPrefix, line[start:cursor]):
		replacement = res.CommonPrefix
	default:
		candidates := strings.Join(res.Matches, "  ")
		if renderCandidates != nil {
			candidates = renderCandidates(res.Matches, e.display.width)
		}
		candidates = strings.ReplaceAll(strings.ReplaceAll(candidates, "\r\n", "\n"), "\n", "\r\n")
		if !strings.HasSuffix(candidates, "\r\n") {
			candidates += "\r\n"
		}
		return e.display.finish("\r\n" + candidates)
	}
	e.buf = []rune(line[:start] + replacement + suffix)
	e.pos = utf8.RuneCountInString(line[:start]) + utf8.RuneCountInString(replacement)
	return nil
}

func (e *lineEditor) redraw() error {
	if e.search != nil {
		var match string
		if e.search.index >= 0 {
			match = e.history[e.search.index]
		}
		text := "(reverse-i-search)'" + string(e.search.query) + "': " + match
		return e.display.redraw(text, len(text))
	}
	return e.display.redraw(e.prompt+string(e.buf), len(e.prompt)+len(string(e.buf[:e.pos])))
}

type historySearch struct {
	query []rune
	index int
}

func (s *historySearch) find(history []string, start int) {
	query := string(s.query)
	s.index = -1
	for i := start; i >= 0; i-- {
		if strings.Contains(history[i], query) {
			s.index = i
			return
		}
	}
}

func (s *historySearch) handle(key contract.KeyEvent, history []string) (accepted bool) {
	switch key.Kind {
	case contract.KeyEnter:
		return s.index >= 0
	case contract.KeyRune:
		s.query = append(s.query, key.Rune)
		s.find(history, len(history)-1)
	case contract.KeyBackspace:
		if len(s.query) > 0 {
			s.query = s.query[:len(s.query)-1]
			s.find(history, len(history)-1)
		}
	case contract.KeyCtrlR:
		previous := s.index
		s.find(history, previous-1)
		if s.index == -1 {
			s.index = previous
		}
	}
	return false
}

type screenPosition struct{ row, col int }

type lineDisplay struct {
	terminal contract.Terminal
	width    int
	cursor   screenPosition
	end      screenPosition
}

func (d *lineDisplay) record(text string, cursor int) {
	d.end = wrappedPosition(text, d.width)
	d.cursor = wrappedPosition(text[:cursor], d.width)
}

func (d *lineDisplay) reset() {
	d.cursor, d.end = screenPosition{}, screenPosition{}
}

func (d *lineDisplay) redraw(text string, cursor int) error {
	var out strings.Builder
	out.WriteByte('\r')
	if d.cursor.row > 0 {
		fmt.Fprintf(&out, "\x1b[%dA", d.cursor.row)
	}
	out.WriteString("\x1b[J")
	out.WriteString(text)
	d.record(text, cursor)
	if d.cursor.row != d.end.row {
		out.WriteByte('\r')
		fmt.Fprintf(&out, "\x1b[%dA", d.end.row-d.cursor.row)
		if d.cursor.col > 0 {
			fmt.Fprintf(&out, "\x1b[%dC", d.cursor.col)
		}
	} else if d.cursor.col < d.end.col {
		fmt.Fprintf(&out, "\x1b[%dD", d.end.col-d.cursor.col)
	}
	return d.terminal.WriteString(out.String())
}

func (d *lineDisplay) finish(text string) error {
	var out strings.Builder
	if d.cursor != d.end {
		if d.cursor.row < d.end.row {
			fmt.Fprintf(&out, "\x1b[%dB", d.end.row-d.cursor.row)
		}
		if !strings.HasPrefix(text, "\r") {
			out.WriteByte('\r')
			if d.end.col > 0 {
				fmt.Fprintf(&out, "\x1b[%dC", d.end.col)
			}
		}
	}
	out.WriteString(text)
	d.reset()
	return d.terminal.WriteString(out.String())
}

func wrappedPosition(text string, width int) screenPosition {
	if width <= 0 {
		width = 80
	}
	var pos screenPosition
	for _, r := range StripANSI(text) {
		if r == '\n' {
			pos.row++
			pos.col = 0
			continue
		}
		cells := min(runeWidth(r), width)
		if cells > 0 && pos.col+cells > width {
			pos.row++
			pos.col = 0
		}
		pos.col += cells
	}
	// At the right margin the cursor stays in the last cell until another
	// printable character triggers autowrap.
	pos.col = min(pos.col, width-1)
	return pos
}
