//go:build !windows

package term

import (
	"errors"
	"os"
	"os/signal"
	"sync"
	"syscall"

	"learn-linux/internal/contract"

	"golang.org/x/term"
)

type standardTerminal struct {
	terminalIO
	stateMu sync.Mutex
	tty     bool
	state   *term.State
	resize  chan os.Signal
}

var _ contract.Terminal = (*standardTerminal)(nil)

// NewStdTerminal uses the process's standard input and output.
func NewStdTerminal() (contract.Terminal, error) {
	tty := term.IsTerminal(int(os.Stdin.Fd()))
	t := &standardTerminal{
		terminalIO: stdStreams(tty && term.IsTerminal(int(os.Stdout.Fd()))),
		tty:        tty,
	}
	if tty {
		t.resize = make(chan os.Signal, 1)
		signal.Notify(t.resize, syscall.SIGWINCH)
	}
	return t, nil
}

func (t *standardTerminal) EnableRawMode() error {
	t.stateMu.Lock()
	defer t.stateMu.Unlock()
	if !t.tty || t.state != nil {
		return nil
	}
	state, err := term.MakeRaw(int(os.Stdin.Fd()))
	if err == nil {
		t.state = state
	}
	return err
}

func (t *standardTerminal) DisableRawMode() error {
	t.stateMu.Lock()
	defer t.stateMu.Unlock()
	if t.state == nil {
		return nil
	}
	if err := term.Restore(int(os.Stdin.Fd()), t.state); err != nil {
		return err
	}
	t.state = nil
	return nil
}

func (t *standardTerminal) IsRawMode() bool {
	t.stateMu.Lock()
	defer t.stateMu.Unlock()
	return t.state != nil
}

func (t *standardTerminal) Size() (width, height int) {
	width, height, err := term.GetSize(int(os.Stdout.Fd()))
	if err != nil || width <= 0 || height <= 0 {
		return 80, 24
	}
	return width, height
}

func (t *standardTerminal) ReadKey() (contract.KeyEvent, error) {
	event, err := t.decoder.readKey(t.resize)
	if event.Kind == contract.KeyResize {
		event.Width, event.Height = t.Size()
	}
	return event, err
}

func (t *standardTerminal) BackendName() string {
	if t.tty {
		return "posix"
	}
	return "fallback-line"
}

func (t *standardTerminal) Close() error {
	t.closeInput()
	if t.resize != nil {
		signal.Stop(t.resize)
	}
	return errors.Join(t.restoreScreen(), t.DisableRawMode())
}
