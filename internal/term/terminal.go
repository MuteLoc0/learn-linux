package term

import (
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"sync"

	"learn-linux/internal/contract"
)

type terminalIO struct {
	mu           sync.Mutex
	closeOnce    sync.Once
	decoder      *KeyDecoder
	output       io.Writer
	closed       chan struct{}
	ansi         bool
	inAltScreen  bool
	cursorHidden bool
}

func newTerminalIO(input io.Reader, output io.Writer, ansi bool) terminalIO {
	closed := make(chan struct{})
	decoder := NewKeyDecoder(input)
	decoder.closed = closed
	return terminalIO{decoder: decoder, output: output, closed: closed, ansi: ansi}
}

func (t *terminalIO) closeInput() {
	t.closeOnce.Do(func() {
		close(t.closed)
	})
}

func (t *terminalIO) readCookedLine() (string, error) {
	var line strings.Builder
	skipLF := t.decoder.previousCR
	t.decoder.previousCR = false
	for {
		b, err := t.decoder.readByte(nil, nil)
		if skipLF && err == nil && b == '\n' {
			skipLF = false
			continue
		}
		skipLF = false
		if err != nil || b == '\n' {
			return strings.TrimSuffix(line.String(), "\r"), err
		}
		line.WriteByte(b)
	}
}

func (t *terminalIO) writeStringLocked(s string) error {
	_, err := io.WriteString(t.output, s)
	return err
}

func (t *terminalIO) Write(p []byte) (int, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.output.Write(p)
}

func (t *terminalIO) WriteString(s string) error {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.writeStringLocked(s)
}

func (t *terminalIO) EnterAltScreen() error {
	t.mu.Lock()
	defer t.mu.Unlock()
	if !t.ansi || t.inAltScreen {
		return nil
	}
	t.inAltScreen = true
	return t.writeStringLocked("\x1b[?1049h\x1b[2J\x1b[H")
}

func (t *terminalIO) ExitAltScreen() error {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.exitAltScreenLocked()
}

func (t *terminalIO) exitAltScreenLocked() error {
	if !t.inAltScreen {
		return nil
	}
	if err := t.writeStringLocked("\x1b[?25h\x1b[?1049l"); err != nil {
		return err
	}
	t.inAltScreen = false
	t.cursorHidden = false
	return nil
}

func (t *terminalIO) ClearScreen() error { return t.WriteString("\x1b[2J\x1b[H") }

func (t *terminalIO) MoveCursor(row, col int) error {
	return t.WriteString(fmt.Sprintf("\x1b[%d;%dH", row, col))
}

func (t *terminalIO) HideCursor() error {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.cursorHidden = true
	return t.writeStringLocked("\x1b[?25l")
}

func (t *terminalIO) ShowCursor() error {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.showCursorLocked()
}

func (t *terminalIO) showCursorLocked() error {
	if err := t.writeStringLocked("\x1b[?25h"); err != nil {
		return err
	}
	t.cursorHidden = false
	return nil
}

func (t *terminalIO) restoreScreen() error {
	t.mu.Lock()
	defer t.mu.Unlock()
	err := t.exitAltScreenLocked()
	if t.cursorHidden {
		return errors.Join(err, t.showCursorLocked())
	}
	return err
}

func (t *terminalIO) ReadKey() (contract.KeyEvent, error) { return t.decoder.ReadKey() }

func keyEvent(kind contract.KeyKind) contract.KeyEvent { return contract.KeyEvent{Kind: kind} }

// Standard terminal instances use the process's standard streams without closing them.
func stdStreams(ansi bool) terminalIO { return newTerminalIO(os.Stdin, os.Stdout, ansi) }
