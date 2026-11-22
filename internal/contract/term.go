package contract

// KeyKind classifies a decoded keyboard or terminal event.
type KeyKind int

const (
	KeyUnknown KeyKind = iota
	KeyRune
	KeyEnter
	KeyBackspace
	KeyDelete
	KeyTab
	KeyUp
	KeyDown
	KeyLeft
	KeyRight
	KeyHome
	KeyEnd
	KeyPageUp
	KeyPageDown
	KeyEscape
	KeyCtrlA
	KeyCtrlC
	KeyCtrlD
	KeyCtrlE
	KeyCtrlK
	KeyCtrlL
	KeyCtrlO
	KeyCtrlR
	KeyCtrlU
	KeyCtrlW
	KeyCtrlX
	KeyEOF
	KeyResize
)

// KeyEvent represents a single terminal input or resize event.
type KeyEvent struct {
	Kind   KeyKind
	Rune   rune
	Width  int
	Height int
}

// Terminal abstracts cross-platform raw terminal I/O across macOS zsh,
// Linux bash, Windows PowerShell/WT, and Windows Git Bash (Mintty).
type Terminal interface {
	EnterAltScreen() error
	ExitAltScreen() error
	EnableRawMode() error
	DisableRawMode() error
	IsRawMode() bool
	Size() (width, height int)
	ReadKey() (KeyEvent, error)
	Write(p []byte) (int, error)
	WriteString(s string) error
	ClearScreen() error
	MoveCursor(row, col int) error
	HideCursor() error
	ShowCursor() error
	BackendName() string
	Close() error
}

// LineReader reads an interactive command line with editing, history, and Tab completion.
type LineReader interface {
	ReadLine(prompt string, history []string, complete func(line string, cursor int) CompletionResult, renderCandidates func(candidates []string, width int) string) (line string, interrupted bool, eof bool, err error)
}
