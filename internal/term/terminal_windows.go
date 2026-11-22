//go:build windows

package term

import (
	"context"
	"encoding/binary"
	"errors"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"time"

	"learn-linux/internal/contract"

	"golang.org/x/sys/windows"
	"golang.org/x/term"
)

type standardTerminal struct {
	terminalIO
	stateMu   sync.Mutex
	console   bool
	mintty    bool
	minttyRaw bool
	sttyMode  string
	state     *term.State
	outMode   uint32
	outSaved  bool
	outRaw    bool
}

var _ contract.Terminal = (*standardTerminal)(nil)

// NewStdTerminal uses the process's standard input and output.
func NewStdTerminal() (contract.Terminal, error) {
	in, out := windows.Handle(os.Stdin.Fd()), windows.Handle(os.Stdout.Fd())
	var inputMode, outputMode uint32
	consoleInput := windows.GetConsoleMode(in, &inputMode) == nil
	console := consoleInput && windows.GetConsoleMode(out, &outputMode) == nil
	mintty := !consoleInput && isMinttyPipe(in) && isMinttyPipe(out)
	t := &standardTerminal{terminalIO: stdStreams(console || mintty), console: console, mintty: mintty}
	if console {
		if err := t.enableVTOutput(false); err != nil {
			return nil, err
		}
		// Re-enable VT if an ANSI operation follows a raw-mode restore.
		t.output = consoleOutput{terminal: t}
	}
	return t, nil
}

func isMinttyPipe(handle windows.Handle) bool {
	kind, err := windows.GetFileType(handle)
	if err != nil || kind != windows.FILE_TYPE_PIPE {
		return false
	}
	// FILE_NAME_INFO contains a byte count followed by a UTF-16 name.
	var info [4 + 512]byte
	if err := windows.GetFileInformationByHandleEx(handle, windows.FileNameInfo, &info[0], uint32(len(info))); err != nil {
		return false
	}
	length := binary.LittleEndian.Uint32(info[:4])
	if length > uint32(len(info)-4) || length%2 != 0 {
		return false
	}
	name := make([]uint16, length/2)
	_, _ = binary.Decode(info[4:4+length], binary.LittleEndian, name)
	pipe := strings.TrimPrefix(windows.UTF16ToString(name), `\Device\NamedPipe`)
	return (strings.HasPrefix(pipe, `\msys-`) || strings.HasPrefix(pipe, `\cygwin-`)) &&
		strings.Contains(pipe, "-pty") &&
		(strings.Contains(pipe, "-from-master") || strings.Contains(pipe, "-to-master"))
}

type consoleOutput struct {
	terminal *standardTerminal
}

func (w consoleOutput) Write(p []byte) (int, error) {
	if err := w.terminal.ensureVTOutput(); err != nil {
		return 0, err
	}
	return os.Stdout.Write(p)
}

func (t *standardTerminal) ensureVTOutput() error {
	t.stateMu.Lock()
	defer t.stateMu.Unlock()
	return t.enableVTOutputLocked(t.state != nil)
}

func (t *standardTerminal) enableVTOutput(rawMode bool) error {
	t.stateMu.Lock()
	defer t.stateMu.Unlock()
	return t.enableVTOutputLocked(rawMode)
}

func (t *standardTerminal) enableVTOutputLocked(rawMode bool) error {
	if t.outSaved && t.outRaw == rawMode {
		return nil
	}
	handle := windows.Handle(os.Stdout.Fd())
	baseMode := t.outMode
	if !t.outSaved {
		var mode uint32
		if err := windows.GetConsoleMode(handle, &mode); err != nil {
			return err
		}
		baseMode = mode
	}
	vtMode := baseMode | windows.ENABLE_PROCESSED_OUTPUT | windows.ENABLE_VIRTUAL_TERMINAL_PROCESSING
	if rawMode {
		vtMode |= windows.DISABLE_NEWLINE_AUTO_RETURN
	} else {
		vtMode &^= windows.DISABLE_NEWLINE_AUTO_RETURN
	}
	if err := windows.SetConsoleMode(handle, vtMode); err != nil {
		return err
	}
	t.outMode, t.outSaved, t.outRaw = baseMode, true, rawMode
	return nil
}

func (t *standardTerminal) EnableRawMode() error {
	t.stateMu.Lock()
	defer t.stateMu.Unlock()
	if t.state != nil || t.minttyRaw {
		return nil
	}
	if t.console {
		if err := t.enableVTOutputLocked(true); err != nil {
			return err
		}
		// On Windows, MakeRaw also sets ENABLE_VIRTUAL_TERMINAL_INPUT.
		state, err := term.MakeRaw(int(os.Stdin.Fd()))
		if err != nil {
			return errors.Join(err, t.disableRawModeLocked())
		}
		t.state = state
	}
	if t.mintty {
		state, err := runStty("-g")
		if err != nil {
			return nil
		}
		t.sttyMode = strings.TrimSpace(state)
		if _, err := runStty("raw", "-echo"); err != nil {
			return t.disableRawModeLocked()
		}
		t.minttyRaw = true
	}
	return nil
}

func (t *standardTerminal) DisableRawMode() error {
	t.stateMu.Lock()
	defer t.stateMu.Unlock()
	return t.disableRawModeLocked()
}

func (t *standardTerminal) disableRawModeLocked() error {
	var inputErr, outputErr, sttyErr error
	if t.state != nil {
		inputErr = term.Restore(int(os.Stdin.Fd()), t.state)
		if inputErr == nil {
			t.state = nil
		}
	}
	if t.outSaved {
		outputErr = windows.SetConsoleMode(windows.Handle(os.Stdout.Fd()), t.outMode)
		if outputErr == nil {
			t.outSaved = false
			t.outRaw = false
		}
	}
	if t.sttyMode != "" {
		_, sttyErr = runStty(t.sttyMode)
		if sttyErr == nil {
			t.sttyMode = ""
			t.minttyRaw = false
		}
	}
	return errors.Join(inputErr, outputErr, sttyErr)
}

func (t *standardTerminal) IsRawMode() bool {
	t.stateMu.Lock()
	defer t.stateMu.Unlock()
	return t.state != nil || t.minttyRaw
}

func (t *standardTerminal) Size() (width, height int) {
	if t.mintty {
		if out, err := runStty("size"); err == nil {
			fields := strings.Fields(out)
			if len(fields) == 2 {
				height, _ = strconv.Atoi(fields[0])
				width, _ = strconv.Atoi(fields[1])
			}
		}
		if width <= 0 {
			width, _ = strconv.Atoi(os.Getenv("COLUMNS"))
		}
		if height <= 0 {
			height, _ = strconv.Atoi(os.Getenv("LINES"))
		}
	} else {
		width, height, _ = term.GetSize(int(os.Stdout.Fd()))
	}
	if width <= 0 {
		width = 80
	}
	if height <= 0 {
		height = 24
	}
	return width, height
}

func (t *standardTerminal) BackendName() string {
	t.stateMu.Lock()
	defer t.stateMu.Unlock()
	switch {
	case t.console:
		return "windows-conpty"
	case t.minttyRaw:
		return "git-bash-mintty-stty"
	default:
		return "fallback-line"
	}
}

func (t *standardTerminal) Close() error {
	t.closeInput()
	return errors.Join(t.restoreScreen(), t.DisableRawMode())
}

func runStty(args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "stty", args...)
	cmd.Stdin = os.Stdin
	cmd.WaitDelay = time.Second
	out, err := cmd.Output()
	return string(out), err
}
