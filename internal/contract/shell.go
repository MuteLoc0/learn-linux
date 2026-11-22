package contract

import (
	"context"
	"io"
)

// TutorialAction represents a tutorial builtin invocation delegated back to the app controller.
type TutorialAction struct {
	Name string
	Args []string
}

// ExecContext carries execution state for a single built-in command invocation.
type ExecContext struct {
	Ctx              context.Context
	VFS              VFS
	Stdin            io.Reader
	Stdout           io.Writer
	Stderr           io.Writer
	CWD              string
	SetCWD           func(path string)
	Home             string
	User             string
	Host             string
	Env              map[string]string
	Aliases          map[string]string
	History          []string
	Term             Terminal
	UI               UIRenderer
	I18N             I18N
	IsTTY            bool
	TermWidth        int
	TermHeight       int
	OnDestructive    func(cmdLine string)
	OnTutorialAction func(action TutorialAction) (int, error)
	RunSubcommand    func(ctx context.Context, line string, stdin io.Reader, stdout, stderr io.Writer) int
}

// CommandHandler executes a built-in virtual Linux command and returns a POSIX exit code.
type CommandHandler func(ec *ExecContext, args []string) int

// CommandRegistry stores built-in commands available in the virtual shell.
type CommandRegistry interface {
	Register(name string, fn CommandHandler)
	Lookup(name string) (CommandHandler, bool)
	Names() []string
}

// CompletionResult represents Tab-completion candidates for the current input line.
type CompletionResult struct {
	Matches      []string
	CommonPrefix string
	ReplaceStart int
	ReplaceEnd   int
	IsPath       bool
}

// ShellEngine parses and executes Bash-compatible commands entirely within VFS.
type ShellEngine interface {
	Execute(ctx context.Context, line string) (CommandRecord, error)
	Complete(line string, cursor int) CompletionResult
	CWD() string
	SetCWD(path string)
	Env() map[string]string
	SetEnv(key, val string)
	Aliases() map[string]string
	History() []string
	SetHistory(h []string)
	AddHistory(line string)
	SetVFS(vfs VFS, cwd string)
	VFS() VFS
	Registry() CommandRegistry
}
