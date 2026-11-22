package shell

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"maps"
	"slices"
	"sync"

	"learn-linux/internal/contract"
	"mvdan.cc/sh/v3/expand"
)

type Config struct {
	VFS              contract.VFS
	Registry         contract.CommandRegistry
	CWD              string
	Home             string
	User             string
	Host             string
	Term             contract.Terminal
	UI               contract.UIRenderer
	I18N             contract.I18N
	Stdout           io.Writer
	Stderr           io.Writer
	OnDestructive    func(cmdLine string)
	OnTutorialAction func(action contract.TutorialAction) (int, error)
}

type Engine struct {
	cfg Config

	runMu sync.Mutex
	ioMu  sync.Mutex
	mu    sync.RWMutex

	vfs          contract.VFS
	cwd          string
	vars         shellEnv
	aliases      map[string]string
	history      []string
	lastStatus   int
	dirRevision  uint64
	envOverrides shellEnv
}

var _ contract.ShellEngine = (*Engine)(nil)

func NewEngine(cfg Config) (*Engine, error) {
	if cfg.VFS == nil {
		return nil, errors.New("shell: VFS is required")
	}
	if cfg.Registry == nil {
		cfg.Registry = NewRegistry()
	}
	if cfg.Home == "" {
		cfg.Home = "/home/learner"
	}
	if cfg.CWD == "" {
		cfg.CWD = "/home/learner"
	}
	if cfg.User == "" {
		cfg.User = "learner"
	}
	if cfg.Host == "" {
		cfg.Host = "linux-virtual"
	}
	cwd := cfg.VFS.ResolvePath("/", cfg.Home, cfg.CWD)
	env := shellEnv{}
	for key, value := range map[string]string{
		"HOME": cfg.Home, "PWD": cwd, "USER": cfg.User, "LOGNAME": cfg.User,
		"HOSTNAME": cfg.Host, "PATH": "/usr/local/bin:/usr/bin:/bin",
		"SHELL": "/bin/bash", "TMPDIR": "/tmp",
	} {
		env[key] = scalar(value, true)
	}
	for _, key := range []string{"UID", "EUID", "GID"} {
		vr := scalar("1000", false)
		vr.ReadOnly = true
		env[key] = vr
	}
	return &Engine{
		cfg: cfg, vfs: cfg.VFS, cwd: cwd, vars: env,
		aliases: make(map[string]string), envOverrides: make(shellEnv),
	}, nil
}

func (e *Engine) Registry() contract.CommandRegistry { return e.cfg.Registry }

func (e *Engine) CWD() string {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.cwd
}

func (e *Engine) SetCWD(path string) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.setCWD(path)
}

func (e *Engine) setCWD(path string) {
	if newDir := e.vfs.ResolvePath(e.cwd, e.cfg.Home, path); newDir != e.cwd {
		e.setEnv("OLDPWD", e.cwd)
		e.setEnv("PWD", newDir)
		e.cwd = newDir
		e.dirRevision++
	}
}

func (e *Engine) VFS() contract.VFS {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.vfs
}

func (e *Engine) SetVFS(vfs contract.VFS, cwd string) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.vfs = vfs
	e.dirRevision++
	e.setCWD(cwd)
}

func (e *Engine) Env() map[string]string {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return envStrings(e.vars, false)
}

func (e *Engine) SetEnv(key, val string) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.setEnv(key, val)
}

func (e *Engine) setEnv(key, val string) {
	vr := scalar(val, true)
	e.vars[key] = vr
	e.envOverrides[key] = vr
}

func (e *Engine) Aliases() map[string]string {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return maps.Clone(e.aliases)
}

func (e *Engine) History() []string {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return slices.Clone(e.history)
}

func (e *Engine) SetHistory(history []string) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.history = slices.Clone(history)
}

func (e *Engine) AddHistory(line string) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.history = append(e.history, line)
}

func (e *Engine) Execute(ctx context.Context, line string) (contract.CommandRecord, error) {
	e.runMu.Lock()
	defer e.runMu.Unlock()

	out, errOut := newOutput(&e.ioMu, e.cfg.Stdout), newOutput(&e.ioMu, e.cfg.Stderr)
	record := contract.CommandRecord{Raw: line}
	file, err := parseScript(line, "")
	if err != nil {
		fmt.Fprintf(errOut, "bash: %v\n", err)
		record.ExitCode = 2
		e.mu.Lock()
		e.lastStatus = record.ExitCode
		e.mu.Unlock()
		err = nil
	} else {
		err = e.executeFile(ctx, file, &record, out, errOut)
	}
	record.Stdout, record.Stderr, record.CWD = out.finish(), errOut.finish(), e.CWD()
	return record, err
}

type shellEnv map[string]expand.Variable

func (e shellEnv) Get(key string) expand.Variable { return e[key] }

func (e shellEnv) Each(visit func(string, expand.Variable) bool) {
	for key, value := range e {
		if !visit(key, value) {
			return
		}
	}
}

func scalar(value string, exported bool) expand.Variable {
	return expand.Variable{Set: true, Kind: expand.String, Str: value, Exported: exported}
}

func envStrings(env expand.Environ, exportedOnly bool) map[string]string {
	result := make(map[string]string)
	for name, value := range env.Each {
		if value.IsSet() && (!exportedOnly || value.Exported) {
			result[name] = value.String()
		} else {
			delete(result, name)
		}
	}
	return result
}

type terminalOutput struct {
	mu     *sync.Mutex
	buf    bytes.Buffer
	writer io.Writer
}

func newOutput(mu *sync.Mutex, live io.Writer) *terminalOutput {
	out := &terminalOutput{mu: mu}
	out.writer = &out.buf
	if live != nil {
		out.writer = io.MultiWriter(&out.buf, live)
	}
	return out
}

func (out *terminalOutput) Write(p []byte) (int, error) {
	out.mu.Lock()
	defer out.mu.Unlock()
	return out.writer.Write(p)
}

func (out *terminalOutput) finish() string {
	out.mu.Lock()
	defer out.mu.Unlock()
	out.writer = io.Discard
	return out.buf.String()
}
