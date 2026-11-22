package devcmds

import (
	"errors"
	"io"
	"slices"
	"strings"

	"learn-linux/internal/contract"
)

func nano(ec *contract.ExecContext, args []string) int {
	if ec.Term == nil || ec.UI == nil {
		return commandError(ec, "nano", "interactive terminal required")
	}
	filename := "untitled.txt"
	if len(args) > 0 {
		filename = args[0]
	}
	p := resolve(ec, filename)
	data, err := ec.VFS.ReadFile(p)
	if err != nil && !errors.Is(err, contract.ErrNotExist) {
		msg := err.Error()
		if errors.Is(err, contract.ErrIsDir) {
			msg = "Is a directory"
		}
		return commandError(ec, "nano", "%s: %s", filename, msg)
	}

	width, height := terminalSize(ec)
	state := contract.EditorState{FilePath: filename, Lines: textLines(data), Width: width, Height: height}
	save := func() bool {
		data := strings.Join(state.Lines, "\n") + "\n"
		if err := ec.VFS.WriteFile(p, []byte(data), 0644); err != nil {
			state.StatusMsg = err.Error()
			return false
		}
		state.Modified = false
		state.StatusMsg = localized(ec.I18N, contract.KeyEditorSaved, "Wrote %d lines to %s", len(state.Lines), state.FilePath)
		return true
	}
	defer ec.Term.ClearScreen()
	for {
		if err := ec.Term.WriteString(ec.UI.RenderEditor(ec.I18N, state)); err != nil {
			return commandError(ec, "nano", "%v", err)
		}
		key, err := ec.Term.ReadKey()
		if errors.Is(err, io.EOF) || key.Kind == contract.KeyEOF {
			return 0
		}
		if err != nil {
			return commandError(ec, "nano", "%v", err)
		}
		if key.Kind == contract.KeyResize {
			state.Width, state.Height = key.Width, key.Height
			revealCursor(&state)
			continue
		}
		if state.ConfirmExit {
			switch {
			case key.Kind == contract.KeyRune && (key.Rune == 'y' || key.Rune == 'Y'):
				if save() {
					return 0
				}
				state.ConfirmExit = false
			case key.Kind == contract.KeyRune && (key.Rune == 'n' || key.Rune == 'N'):
				return 0
			case key.Kind == contract.KeyEscape || key.Kind == contract.KeyCtrlC:
				state.ConfirmExit, state.StatusMsg = false, ""
			}
			continue
		}
		switch key.Kind {
		case contract.KeyCtrlO:
			save()
		case contract.KeyCtrlX:
			if !state.Modified {
				return 0
			}
			state.ConfirmExit = true
		default:
			editLine(&state, key)
			revealCursor(&state)
		}
	}
}

func editLine(state *contract.EditorState, key contract.KeyEvent) {
	row, col := state.CursorRow, state.CursorCol
	line := []rune(state.Lines[row])
	changed := false
	switch key.Kind {
	case contract.KeyRune:
		state.Lines[row] = string(slices.Insert(line, col, key.Rune))
		col++
		changed = true
	case contract.KeyTab:
		state.Lines[row] = string(slices.Insert(line, col, '\t'))
		col++
		changed = true
	case contract.KeyEnter:
		state.Lines[row] = string(line[:col])
		state.Lines = slices.Insert(state.Lines, row+1, string(line[col:]))
		row, col = row+1, 0
		changed = true
	case contract.KeyBackspace:
		if col > 0 {
			state.Lines[row] = string(slices.Delete(line, col-1, col))
			col--
			changed = true
		} else if row > 0 {
			col = len([]rune(state.Lines[row-1]))
			state.Lines[row-1] += state.Lines[row]
			state.Lines = slices.Delete(state.Lines, row, row+1)
			row--
			changed = true
		}
	case contract.KeyDelete:
		if col < len(line) {
			state.Lines[row] = string(slices.Delete(line, col, col+1))
			changed = true
		} else if row+1 < len(state.Lines) {
			state.Lines[row] += state.Lines[row+1]
			state.Lines = slices.Delete(state.Lines, row+1, row+2)
			changed = true
		}
	case contract.KeyCtrlK:
		changed = len(state.Lines) > 1 || len(line) > 0
		if row == len(state.Lines)-1 {
			state.Lines[row] = ""
		} else {
			state.Lines = slices.Delete(state.Lines, row, row+1)
		}
		col = 0
	case contract.KeyLeft:
		if col > 0 {
			col--
		} else if row > 0 {
			row--
			col = len([]rune(state.Lines[row]))
		}
	case contract.KeyRight:
		if col < len(line) {
			col++
		} else if row+1 < len(state.Lines) {
			row, col = row+1, 0
		}
	case contract.KeyUp:
		row = max(0, row-1)
	case contract.KeyDown:
		row = min(row+1, len(state.Lines)-1)
	case contract.KeyHome, contract.KeyCtrlA:
		col = 0
	case contract.KeyEnd, contract.KeyCtrlE:
		col = len(line)
	}
	state.CursorRow = row
	state.CursorCol = min(col, len([]rune(state.Lines[row])))
	if changed {
		state.Modified, state.StatusMsg = true, ""
	}
}

func revealCursor(state *contract.EditorState) {
	rows := max(1, state.Height-4)
	state.ScrollTop = min(state.ScrollTop, max(0, len(state.Lines)-rows))
	if state.CursorRow < state.ScrollTop {
		state.ScrollTop = state.CursorRow
	} else if state.CursorRow >= state.ScrollTop+rows {
		state.ScrollTop = state.CursorRow - rows + 1
	}
}
