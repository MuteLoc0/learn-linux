package term

import (
	"bytes"

	"learn-linux/internal/contract"
)

// MockTerminal provides queued key events and captures output for tests.
// A zero-value MockTerminal starts in cooked mode at 80 by 24.
type MockTerminal struct {
	Events []contract.KeyEvent
	Output bytes.Buffer
	Raw    bool
	Width  int
	Height int

	screen terminalIO
}

var _ contract.Terminal = (*MockTerminal)(nil)

// NewMockTerminal creates a raw-mode mock with the given input events.
func NewMockTerminal(events ...contract.KeyEvent) *MockTerminal {
	t := &MockTerminal{Raw: true}
	t.QueueKeys(events...)
	return t
}

// QueueKeys appends events to the input queue in arrival order.
func (t *MockTerminal) QueueKeys(events ...contract.KeyEvent) {
	t.Events = append(t.Events, events...)
}

func (t *MockTerminal) ReadKey() (contract.KeyEvent, error) {
	if len(t.Events) == 0 {
		return keyEvent(contract.KeyEOF), nil
	}
	event := t.Events[0]
	t.Events = t.Events[1:]
	if event.Kind == contract.KeyResize {
		t.Width, t.Height = event.Width, event.Height
	}
	return event, nil
}

func (t *MockTerminal) Write(p []byte) (int, error) { return t.Output.Write(p) }

func (t *MockTerminal) WriteString(s string) error {
	_, err := t.Output.WriteString(s)
	return err
}

func (t *MockTerminal) Size() (width, height int) {
	width, height = t.Width, t.Height
	if width <= 0 {
		width = 80
	}
	if height <= 0 {
		height = 24
	}
	return width, height
}

func (t *MockTerminal) EnableRawMode() error {
	t.Raw = true
	return nil
}

func (t *MockTerminal) DisableRawMode() error {
	t.Raw = false
	return nil
}

func (t *MockTerminal) IsRawMode() bool     { return t.Raw }
func (t *MockTerminal) BackendName() string { return "mock" }

func (t *MockTerminal) ansiScreen() *terminalIO {
	t.screen.ansi = true
	t.screen.output = &t.Output
	return &t.screen
}

func (t *MockTerminal) EnterAltScreen() error { return t.ansiScreen().EnterAltScreen() }
func (t *MockTerminal) ExitAltScreen() error  { return t.ansiScreen().ExitAltScreen() }
func (t *MockTerminal) ClearScreen() error    { return t.ansiScreen().ClearScreen() }

func (t *MockTerminal) MoveCursor(row, col int) error {
	return t.ansiScreen().MoveCursor(row, col)
}

func (t *MockTerminal) HideCursor() error { return t.ansiScreen().HideCursor() }
func (t *MockTerminal) ShowCursor() error { return t.ansiScreen().ShowCursor() }

func (t *MockTerminal) Close() error {
	t.Raw = false
	return t.ansiScreen().restoreScreen()
}
