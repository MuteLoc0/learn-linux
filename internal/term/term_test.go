package term

import (
	"errors"
	"io"
	"os"
	"slices"
	"strings"
	"testing"
	"time"

	"learn-linux/internal/contract"
)

func runeEvent(r rune) contract.KeyEvent {
	return contract.KeyEvent{Kind: contract.KeyRune, Rune: r}
}

func TestKeyDecoder(t *testing.T) {
	tests := []struct {
		name, input string
		want        []contract.KeyEvent
	}{
		{"ASCII and UTF-8", "a中国🙂", []contract.KeyEvent{runeEvent('a'), runeEvent('中'), runeEvent('国'), runeEvent('🙂')}},
		{"controls", "\x01\x03\x04\x05\x08\x09\x0b\x0c\x0f\x12\x15\x17\x18\x7f", events(
			contract.KeyCtrlA, contract.KeyCtrlC, contract.KeyCtrlD, contract.KeyCtrlE, contract.KeyBackspace,
			contract.KeyTab, contract.KeyCtrlK, contract.KeyCtrlL, contract.KeyCtrlO, contract.KeyCtrlR,
			contract.KeyCtrlU, contract.KeyCtrlW, contract.KeyCtrlX, contract.KeyBackspace)},
		{"line endings", "\r\n\n\r\r\n", events(contract.KeyEnter, contract.KeyEnter, contract.KeyEnter, contract.KeyEnter)},
		{"CSI", "\x1b[A\x1b[B\x1b[C\x1b[D\x1b[H\x1b[F", events(
			contract.KeyUp, contract.KeyDown, contract.KeyRight, contract.KeyLeft, contract.KeyHome, contract.KeyEnd)},
		{"SS3", "\x1bOA\x1bOB\x1bOC\x1bOD\x1bOH\x1bOF", events(
			contract.KeyUp, contract.KeyDown, contract.KeyRight, contract.KeyLeft, contract.KeyHome, contract.KeyEnd)},
		{"CSI tilde", "\x1b[1~\x1b[3~\x1b[4~\x1b[5~\x1b[6~\x1b[7~\x1b[8~", events(
			contract.KeyHome, contract.KeyDelete, contract.KeyEnd, contract.KeyPageUp, contract.KeyPageDown, contract.KeyHome, contract.KeyEnd)},
		{"escape before EOF", "\x1b", events(contract.KeyEscape)},
		{"escape then rune", "\x1bz", []contract.KeyEvent{keyEvent(contract.KeyEscape), runeEvent('z')}},
		{"unsupported CSI", "\x1b[15~x", []contract.KeyEvent{keyEvent(contract.KeyUnknown), runeEvent('x')}},
		{"invalid UTF-8 keeps following bytes", "\xe4\xb8x", []contract.KeyEvent{runeEvent('�'), runeEvent('�'), runeEvent('x')}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := NewKeyDecoder(strings.NewReader(tt.input))
			for _, want := range tt.want {
				checkKey(t, d, want)
			}
			checkKey(t, d, keyEvent(contract.KeyEOF))
		})
	}
}

func TestKeyDecoderLoneEscapeWhileInputRemainsOpen(t *testing.T) {
	input, writer := io.Pipe()
	defer input.Close()
	defer writer.Close()
	d := NewKeyDecoder(input)
	writeDone := make(chan error, 1)
	go func() {
		_, err := writer.Write([]byte("\x1b"))
		writeDone <- err
	}()
	result := make(chan keyResult, 1)
	go func() {
		key, err := d.ReadKey()
		result <- keyResult{key, err}
	}()
	select {
	case got := <-result:
		if got.err != nil || got.key.Kind != contract.KeyEscape {
			t.Fatalf("ReadKey() = %+v, %v", got.key, got.err)
		}
	case <-time.After(time.Second):
		t.Fatal("lone Escape did not return while input was open")
	}
	if err := <-writeDone; err != nil {
		t.Fatal(err)
	}
	go func() {
		_, err := writer.Write([]byte("中"))
		writeDone <- err
	}()
	checkKey(t, d, runeEvent('中'))
	if err := <-writeDone; err != nil {
		t.Fatal(err)
	}
}

type keyResult struct {
	key contract.KeyEvent
	err error
}

func TestKeyDecoderDeliversResizeBeforePendingInput(t *testing.T) {
	input, writer := io.Pipe()
	defer input.Close()
	defer writer.Close()
	d := NewKeyDecoder(input)
	resize := make(chan os.Signal, 1)
	result := make(chan keyResult, 1)
	go func() {
		key, err := d.readKey(resize)
		result <- keyResult{key, err}
	}()
	resize <- os.Interrupt
	select {
	case got := <-result:
		if got.err != nil || got.key.Kind != contract.KeyResize {
			t.Fatalf("readKey() = %+v, %v", got.key, got.err)
		}
	case <-time.After(time.Second):
		t.Fatal("resize did not wake ReadKey")
	}
	go func() {
		_, _ = writer.Write([]byte("x"))
	}()
	checkKey(t, d, runeEvent('x'))
}

func checkKey(t *testing.T, d *KeyDecoder, want contract.KeyEvent) {
	t.Helper()
	got, err := d.ReadKey()
	if err != nil || got != want {
		t.Fatalf("ReadKey() = %+v, %v, want %+v", got, err, want)
	}
}

func TestDisplayWidthAndStripANSI(t *testing.T) {
	tests := []struct {
		input, stripped string
		width           int
	}{
		{"", "", 0},
		{"abc", "abc", 3},
		{"中国", "中国", 4},
		{"a\x1b[31m中国\x1b[0mＡ!", "a中国Ａ!", 8},
		{"\x1b[38;2;12;34;56m中文\x1b[m", "中文", 4},
		{"\x1b]0;title\a\x1b]8;;https://example.invalid\x1b\\链接\x1b]8;;\x1b\\", "链接", 4},
		{"\x1b[?25l\x1b[2J\x1b[1;4H\x1b(Babc\x1b[?25h", "abc", 3},
		{"\x1bPterminal data\x1b\\X", "X", 1},
		{"cafe\u0301\u200d", "cafe\u0301\u200d", 4},
		{"ｶＡ한𠀀🙂", "ｶＡ한𠀀🙂", 9},
		{"Ω·", "Ω·", 2},
		{"a\r\nb", "a\r\nb", 2},
	}
	for _, tt := range tests {
		if got := StripANSI(tt.input); got != tt.stripped {
			t.Errorf("StripANSI(%q) = %q, want %q", tt.input, got, tt.stripped)
		}
		if got := DisplayWidth(tt.input); got != tt.width {
			t.Errorf("DisplayWidth(%q) = %d, want %d", tt.input, got, tt.width)
		}
	}
}

func TestLineReaderEditing(t *testing.T) {
	mock := mockInput("a中文b", contract.KeyLeft, contract.KeyBackspace, contract.KeyHome,
		contract.KeyDelete, contract.KeyRight, runeEvent('!'), contract.KeyEnd, runeEvent('?'), contract.KeyEnter)
	readLine(t, mock, nil, "中!b?")
	if got := mock.Output.String(); !strings.Contains(got, "a中文b\x1b[1D") {
		t.Errorf("missing cursor movement over ASCII in %q", got)
	}

	mock = mockInput("a中文", contract.KeyLeft, contract.KeyLeft, runeEvent('界'), contract.KeyEnter)
	readLine(t, mock, nil, "a界中文")
	if got := mock.Output.String(); !strings.Contains(got, "a界中文\x1b[4D") {
		t.Errorf("missing four-cell cursor movement in %q", got)
	}
}

func TestLineReaderCursorAcrossWrappedRows(t *testing.T) {
	mock := mockInput("A中B文", contract.KeyHome, runeEvent('!'), contract.KeyEnter)
	mock.Width = 6
	readLine(t, mock, nil, "!A中B文")
	if got := mock.Output.String(); !strings.Contains(got, "> A中B文\r\x1b[1A\x1b[2C") {
		t.Errorf("missing move from wrapped row to the prompt in %q", got)
	}
}

func TestLineReaderControlEditing(t *testing.T) {
	mock := mockInput("abc xyz", contract.KeyCtrlA, contract.KeyRight, contract.KeyCtrlK, runeEvent('你'),
		contract.KeyCtrlE, contract.KeyCtrlU, runeEvent('一'), runeEvent(' '), runeEvent('二'),
		runeEvent('三'), runeEvent(' '), contract.KeyCtrlW, contract.KeyCtrlL,
		runeEvent('四'), contract.KeyCtrlA, contract.KeyCtrlD, contract.KeyCtrlE, contract.KeyEnter)
	readLine(t, mock, nil, " 四")
	if got := mock.Output.String(); !strings.Contains(got, "\x1b[2J\x1b[H\r\x1b[J> 一 ") {
		t.Errorf("missing clear and redraw in %q", got)
	}
}

func TestLineReaderInterruptAndEOF(t *testing.T) {
	for _, tt := range []struct {
		name, input string
		key         contract.KeyKind
		interrupted bool
		eof         bool
	}{
		{"Ctrl+C", "draft", contract.KeyCtrlC, true, false},
		{"Ctrl+D", "", contract.KeyCtrlD, false, true},
		{"stream EOF", "", contract.KeyEOF, false, true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			mock := mockInput(tt.input, tt.key)
			line, interrupted, eof, err := NewLineReader(mock).ReadLine("> ", nil, nil, nil)
			if line != "" || interrupted != tt.interrupted || eof != tt.eof || err != nil {
				t.Fatalf("ReadLine() = %q, %v, %v, %v", line, interrupted, eof, err)
			}
			if tt.interrupted && !strings.HasSuffix(mock.Output.String(), "^C\r\n") {
				t.Errorf("output = %q, want ^C with CRLF", mock.Output.String())
			}
		})
	}
}

func TestLineReaderHistoryRestoresDraftAndCursor(t *testing.T) {
	mock := mockInput("草稿", contract.KeyLeft, contract.KeyUp, contract.KeyUp, contract.KeyUp,
		contract.KeyDown, contract.KeyDown, contract.KeyDown, runeEvent('新'), contract.KeyEnter)
	readLine(t, mock, []string{"old", "new"}, "草新稿")
	if got := mock.Output.String(); !strings.Contains(got, "> old") || !strings.Contains(got, "> new") {
		t.Errorf("history was not shown in %q", got)
	}
}

func TestLineReaderReverseSearch(t *testing.T) {
	history := []string{"ls", "cat 旧", "pwd", "cat 新", "exit"}
	mock := mockInput("draft", contract.KeyCtrlR, runeEvent('c'), runeEvent('a'), contract.KeyCtrlR, contract.KeyEnter)
	readLine(t, mock, history, "cat 旧")
	out := mock.Output.String()
	for _, want := range []string{"(reverse-i-search)'': exit", "(reverse-i-search)'ca': cat 新", "(reverse-i-search)'ca': cat 旧"} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in %q", want, out)
		}
	}
	for _, cancel := range []contract.KeyKind{contract.KeyEscape, contract.KeyCtrlC} {
		mock := mockInput("一二", contract.KeyLeft, contract.KeyCtrlR, runeEvent('旧'), contract.KeyBackspace,
			runeEvent('新'), cancel, runeEvent('三'), contract.KeyEnter)
		readLine(t, mock, history, "一三二")
	}
}

func TestLineReaderCompletion(t *testing.T) {
	t.Run("single match uses UTF-8 byte offsets and preserves suffix", func(t *testing.T) {
		mock := mockInput("中 fi -l", contract.KeyLeft, contract.KeyLeft, contract.KeyLeft, contract.KeyTab, runeEvent('x'), contract.KeyEnter)
		complete := func(line string, cursor int) contract.CompletionResult {
			if line != "中 fi -l" || cursor != len("中 fi") {
				t.Fatalf("complete(%q, %d)", line, cursor)
			}
			return contract.CompletionResult{Matches: []string{"file"}, ReplaceStart: len("中 "), ReplaceEnd: cursor}
		}
		line, interrupted, eof, err := NewLineReader(mock).ReadLine("> ", nil, complete, nil)
		if line != "中 file x-l" || interrupted || eof || err != nil {
			t.Fatalf("ReadLine() = %q, %v, %v, %v", line, interrupted, eof, err)
		}
	})
	t.Run("directory keeps cursor next to slash", func(t *testing.T) {
		mock := mockInput("cd 文", contract.KeyTab, runeEvent('a'), contract.KeyEnter)
		line, _, _, err := NewLineReader(mock).ReadLine("> ", nil, func(line string, cursor int) contract.CompletionResult {
			return contract.CompletionResult{Matches: []string{"文档/"}, ReplaceStart: 3, ReplaceEnd: cursor}
		}, nil)
		if line != "cd 文档/a" || err != nil {
			t.Fatalf("ReadLine() = %q, %v", line, err)
		}
	})
	t.Run("common prefix and second Tab list", func(t *testing.T) {
		mock := mockInput("f", contract.KeyTab, contract.KeyTab, contract.KeyEnter)
		mock.Width = 40
		var calls []string
		rendered := 0
		matches := []string{"fizz", "file"}
		line, _, _, err := NewLineReader(mock).ReadLine("> ", nil, func(line string, cursor int) contract.CompletionResult {
			calls = append(calls, line)
			return contract.CompletionResult{Matches: matches, CommonPrefix: "fi", ReplaceEnd: cursor}
		}, func(candidates []string, width int) string {
			rendered++
			if !slices.Equal(candidates, matches) || width != 40 {
				t.Fatalf("render(%q, %d)", candidates, width)
			}
			return "fizz\nfile"
		})
		if err != nil || line != "fi" || rendered != 1 || !slices.Equal(calls, []string{"f", "fi"}) {
			t.Fatalf("line=%q err=%v rendered=%d calls=%q", line, err, rendered, calls)
		}
		if got := mock.Output.String(); !strings.Contains(got, "\r\nfizz\r\nfile\r\n\r\x1b[J> fi") {
			t.Errorf("candidates or prompt missing in %q", got)
		}
	})
	t.Run("no matches", func(t *testing.T) {
		mock := mockInput("?", contract.KeyTab, contract.KeyEnter)
		line, _, _, err := NewLineReader(mock).ReadLine("> ", nil, func(string, int) contract.CompletionResult {
			return contract.CompletionResult{}
		}, nil)
		if line != "?" || err != nil {
			t.Fatalf("ReadLine() = %q, %v", line, err)
		}
	})
}

func TestLineReaderCookedMode(t *testing.T) {
	mock := mockInput("echo\t中\r", contract.KeyEnter)
	mock.Raw = false
	readLine(t, mock, nil, "echo\t中")
	line, interrupted, eof, err := NewLineReader(mock).ReadLine("> ", nil, nil, nil)
	if line != "" || interrupted || !eof || err != nil || mock.Output.String() != "> > " {
		t.Fatalf("ReadLine() = %q, %v, %v, %v, output %q", line, interrupted, eof, err, mock.Output.String())
	}

	cooked := &cookedTestTerminal{
		Terminal: &MockTerminal{},
		input:    newTerminalIO(strings.NewReader("abc\r\n\t中\r间\nlast"), io.Discard, false),
	}
	reader := NewLineReader(cooked)
	for _, want := range []string{"abc", "\t中\r间", "last"} {
		line, interrupted, eof, err := reader.ReadLine("> ", nil, nil, nil)
		if line != want || interrupted || eof || err != nil {
			t.Fatalf("ReadLine() = %q, %v, %v, %v, want %q", line, interrupted, eof, err, want)
		}
	}
	if line, _, eof, err := reader.ReadLine("> ", nil, nil, nil); line != "" || !eof || err != nil {
		t.Fatalf("ReadLine() at EOF = %q, %v, %v", line, eof, err)
	}
}

type cookedTestTerminal struct {
	contract.Terminal
	input terminalIO
}

func (t *cookedTestTerminal) readCookedLine() (string, error) { return t.input.readCookedLine() }

func TestMockTerminalLifecycle(t *testing.T) {
	mock := NewMockTerminal(contract.KeyEvent{Kind: contract.KeyResize, Width: 100, Height: 50})
	event, err := mock.ReadKey()
	if width, height := mock.Size(); err != nil || event.Kind != contract.KeyResize || width != 100 || height != 50 {
		t.Fatalf("resize: %+v, %v, size %d by %d", event, err, width, height)
	}
	if err := errors.Join(mock.EnterAltScreen(), mock.EnterAltScreen(), mock.HideCursor(), mock.Close(), mock.Close()); err != nil {
		t.Fatal(err)
	}
	want := "\x1b[?1049h\x1b[2J\x1b[H\x1b[?25l\x1b[?25h\x1b[?1049l"
	if mock.IsRawMode() || mock.Output.String() != want {
		t.Fatalf("raw=%v, output=%q, want %q", mock.IsRawMode(), mock.Output.String(), want)
	}
}

func events(kinds ...contract.KeyKind) []contract.KeyEvent {
	keys := make([]contract.KeyEvent, 0, len(kinds))
	for _, kind := range kinds {
		keys = append(keys, keyEvent(kind))
	}
	return keys
}

func mockInput(text string, tail ...any) *MockTerminal {
	mock := NewMockTerminal()
	for _, r := range text {
		mock.QueueKeys(runeEvent(r))
	}
	for _, input := range tail {
		switch input := input.(type) {
		case contract.KeyEvent:
			mock.QueueKeys(input)
		case contract.KeyKind:
			mock.QueueKeys(keyEvent(input))
		default:
			panic("unsupported test input")
		}
	}
	return mock
}

func readLine(t *testing.T, mock *MockTerminal, history []string, want string) {
	t.Helper()
	line, interrupted, eof, err := NewLineReader(mock).ReadLine("> ", history, nil, nil)
	if line != want || interrupted || eof || err != nil {
		t.Fatalf("ReadLine() = %q, %v, %v, %v, want %q", line, interrupted, eof, err, want)
	}
}
