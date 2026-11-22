package term

import (
	"bufio"
	"errors"
	"io"
	"os"
	"time"
	"unicode/utf8"

	"learn-linux/internal/contract"
)

const escapeTimeout = 25 * time.Millisecond

var (
	errKeyTimeout = errors.New("key sequence timed out")
	errResize     = errors.New("terminal resized")
)

// KeyDecoder reads UTF-8 and ANSI/VT key sequences from a byte stream.
// ReadKey should be called by one input consumer.
type KeyDecoder struct {
	reader     *bufio.Reader
	pending    <-chan byteResult
	closed     <-chan struct{}
	replay     []byte
	previousCR bool
}

type byteResult struct {
	value byte
	err   error
}

func NewKeyDecoder(r io.Reader) *KeyDecoder {
	return &KeyDecoder{reader: bufio.NewReader(r)}
}

func (d *KeyDecoder) ReadKey() (contract.KeyEvent, error) { return d.readKey(nil) }

func (d *KeyDecoder) readKey(resize <-chan os.Signal) (contract.KeyEvent, error) {
	for {
		b, err := d.readByte(resize, nil)
		switch {
		case errors.Is(err, io.EOF):
			return keyEvent(contract.KeyEOF), nil
		case err == errResize:
			return keyEvent(contract.KeyResize), nil
		case err != nil:
			return contract.KeyEvent{}, err
		}
		skipLF := d.previousCR && b == '\n'
		d.previousCR = b == '\r'
		if skipLF {
			continue
		}
		if b == '\x1b' {
			return d.readEscape()
		}
		if kind, ok := controlKeys[b]; ok {
			return keyEvent(kind), nil
		}
		if b < ' ' {
			return keyEvent(contract.KeyUnknown), nil
		}
		r, err := d.readRune(b)
		return contract.KeyEvent{Kind: contract.KeyRune, Rune: r}, err
	}
}

var controlKeys = map[byte]contract.KeyKind{
	0x01: contract.KeyCtrlA,
	0x03: contract.KeyCtrlC,
	0x04: contract.KeyCtrlD,
	0x05: contract.KeyCtrlE,
	0x08: contract.KeyBackspace,
	0x09: contract.KeyTab,
	0x0a: contract.KeyEnter,
	0x0b: contract.KeyCtrlK,
	0x0c: contract.KeyCtrlL,
	0x0d: contract.KeyEnter,
	0x0f: contract.KeyCtrlO,
	0x12: contract.KeyCtrlR,
	0x15: contract.KeyCtrlU,
	0x17: contract.KeyCtrlW,
	0x18: contract.KeyCtrlX,
	0x7f: contract.KeyBackspace,
}

var cursorKeys = map[byte]contract.KeyKind{
	'A': contract.KeyUp,
	'B': contract.KeyDown,
	'C': contract.KeyRight,
	'D': contract.KeyLeft,
	'H': contract.KeyHome,
	'F': contract.KeyEnd,
}

var tildeKeys = map[byte]contract.KeyKind{
	'1': contract.KeyHome,
	'3': contract.KeyDelete,
	'4': contract.KeyEnd,
	'5': contract.KeyPageUp,
	'6': contract.KeyPageDown,
	'7': contract.KeyHome,
	'8': contract.KeyEnd,
}

func (d *KeyDecoder) readEscape() (contract.KeyEvent, error) {
	timer := time.NewTimer(escapeTimeout)
	defer timer.Stop()
	prefix, err := d.readByte(nil, timer.C)
	if err != nil {
		return escapeEnd(err)
	}
	if prefix != '[' && prefix != 'O' {
		d.replay = append([]byte{prefix}, d.replay...)
		return keyEvent(contract.KeyEscape), nil
	}

	var parameter byte
	length := 0
	for {
		b, err := d.readByte(nil, timer.C)
		if err != nil {
			return escapeEnd(err)
		}
		switch {
		case b >= 0x40 && b <= 0x7e:
			if length == 0 {
				return keyEvent(cursorKeys[b]), nil
			}
			if prefix == '[' && b == '~' && length == 1 {
				return keyEvent(tildeKeys[parameter]), nil
			}
			return keyEvent(contract.KeyUnknown), nil
		case b >= 0x20 && b <= 0x3f:
			parameter = b
			length++
		default:
			d.replay = append([]byte{b}, d.replay...)
			return keyEvent(contract.KeyUnknown), nil
		}
	}
}

func escapeEnd(err error) (contract.KeyEvent, error) {
	if err == errKeyTimeout || errors.Is(err, io.EOF) {
		return keyEvent(contract.KeyEscape), nil
	}
	return contract.KeyEvent{}, err
}

func (d *KeyDecoder) readRune(first byte) (rune, error) {
	if first < utf8.RuneSelf {
		return rune(first), nil
	}
	buf := [utf8.UTFMax]byte{first}
	n := 1
	for !utf8.FullRune(buf[:n]) {
		b, err := d.readByte(nil, nil)
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return 0, err
		}
		buf[n] = b
		n++
	}
	r, size := utf8.DecodeRune(buf[:n])
	if size < n {
		d.replay = append(buf[size:n:n], d.replay...)
	}
	return r, nil
}

func (d *KeyDecoder) readByte(resize <-chan os.Signal, timeout <-chan time.Time) (byte, error) {
	select {
	case <-d.closed:
		return 0, io.EOF
	case <-resize:
		return 0, errResize
	default:
	}
	if len(d.replay) > 0 {
		b := d.replay[0]
		d.replay = d.replay[1:]
		return b, nil
	}
	if d.pending == nil {
		if d.reader.Buffered() > 0 || (resize == nil && timeout == nil && d.closed == nil) {
			return d.reader.ReadByte()
		}
		// Retain an outstanding read if Escape times out or a resize arrives.
		// Buffered delivery lets the reader finish even if input has since closed.
		result := make(chan byteResult, 1)
		d.pending = result
		go func() {
			b, err := d.reader.ReadByte()
			result <- byteResult{b, err}
		}()
	}
	select {
	case result := <-d.pending:
		d.pending = nil
		return result.value, result.err
	case <-resize:
		return 0, errResize
	case <-timeout:
		return 0, errKeyTimeout
	case <-d.closed:
		return 0, io.EOF
	}
}
