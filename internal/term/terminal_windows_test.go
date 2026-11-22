package term

import (
	"fmt"
	"os"
	"testing"

	"golang.org/x/sys/windows"
)

func TestMinttyPipeDetection(t *testing.T) {
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	defer w.Close()
	if isMinttyPipe(windows.Handle(r.Fd())) || isMinttyPipe(windows.Handle(w.Fd())) {
		t.Fatal("ordinary redirected pipe detected as a mintty terminal")
	}

	name, err := windows.UTF16PtrFromString(fmt.Sprintf(`\\.\pipe\msys-1234567890abcdef-pty%d-from-master`, os.Getpid()))
	if err != nil {
		t.Fatal(err)
	}
	pipe, err := windows.CreateNamedPipe(name, windows.PIPE_ACCESS_DUPLEX,
		windows.PIPE_TYPE_BYTE, 1, 4096, 4096, 0, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer windows.CloseHandle(pipe)
	if !isMinttyPipe(pipe) {
		t.Fatal("mintty-style PTY pipe was not detected")
	}
}
