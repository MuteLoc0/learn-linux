package shell

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"maps"
	"slices"
	"strings"
	"sync"

	"learn-linux/internal/contract"
	"mvdan.cc/sh/v3/expand"
	"mvdan.cc/sh/v3/interp"
	"mvdan.cc/sh/v3/syntax"
)

const dispatchCommand = "\x00virtual-shell"

type execution struct {
	engine  *Engine
	vfs     contract.VFS
	tty     *terminalOutput
	history []string

	mu         sync.Mutex
	aliasNames map[string]struct{}
	aliasInput map[string]string
	primary    *syntax.CallExpr
	primaryArg []string
	sawExit    bool
}

func (e *Engine) executeFile(ctx context.Context, file *syntax.File, record *contract.CommandRecord, out, errOut *terminalOutput) error {
	e.mu.Lock()
	cwd, revision, lastStatus := e.cwd, e.dirRevision, e.lastStatus
	vars, aliases := maps.Clone(e.vars), maps.Clone(e.aliases)
	e.envOverrides = make(shellEnv)
	run := &execution{engine: e, vfs: e.vfs, tty: out, history: slices.Clone(e.history)}
	e.mu.Unlock()

	record.Command, record.Args, run.primary = firstCommand(file, vars)
	runner, err := run.newRunner(cwd, vars, nil, nil, out, errOut, true)
	if err != nil {
		record.ExitCode = 1
		return err
	}
	for _, name := range slices.Sorted(maps.Keys(aliases)) {
		if err := runner.Run(ctx, literalCall("alias", name+"="+aliases[name])); err != nil {
			record.ExitCode = 1
			return err
		}
	}
	// The bare call sets $? without triggering an ERR trap or shell options.
	_ = runner.Run(ctx, literalCall(dispatchCommand, "status", fmt.Sprint(lastStatus)))
	err = runner.Run(ctx, &syntax.Block{Stmts: file.Stmts})
	record.ExitCode, err = exitCode(err)

	run.mu.Lock()
	primary, sawExit := run.primaryArg, run.sawExit
	aliasNames := slices.Sorted(maps.Keys(run.aliasNames))
	run.mu.Unlock()
	if len(primary) > 0 {
		record.Command, record.Args = primary[0], primary[1:]
	}

	aliases = run.readAliases(context.WithoutCancel(ctx), runner, aliasNames)
	e.mu.Lock()
	e.aliases = aliases
	e.vars = make(shellEnv, len(runner.Vars))
	for key, value := range runner.Vars {
		if value.Declared() {
			e.vars[key] = value
		}
	}
	maps.Copy(e.vars, e.envOverrides)
	if e.dirRevision == revision {
		if pwd := normalizePOSIXPath(runner.Vars["PWD"].String()); strings.HasPrefix(pwd, "/") {
			e.cwd = pwd
		} else {
			e.cwd = normalizePOSIXPath(runner.Dir)
		}
	} else {
		e.vars["PWD"] = scalar(e.cwd, true)
	}
	e.lastStatus = record.ExitCode
	e.mu.Unlock()

	if runner.Exited() && sawExit && e.cfg.OnTutorialAction != nil {
		code, actionErr := e.cfg.OnTutorialAction(contract.TutorialAction{Name: "exit"})
		if record.ExitCode == 0 {
			record.ExitCode = code
			e.mu.Lock()
			e.lastStatus = code
			e.mu.Unlock()
		}
		if actionErr != nil {
			fmt.Fprintf(errOut, "bash: exit: %v\n", actionErr)
			return actionErr
		}
	}
	return err
}

func (s *execution) newRunner(cwd string, env shellEnv, params []string, stdin io.Reader, stdout, stderr io.Writer, interactive bool) (*interp.Runner, error) {
	// interp.Dir would stat this path on the host. Set the virtual working
	// directory before New has a chance to apply its host defaults.
	initial := maps.Clone(env)
	tmp, hadTmp := initial["TMPDIR"]
	initial["TMPDIR"] = scalar(interpreterTempDir(), false)
	for _, key := range []string{"HOME", "UID", "EUID", "GID"} {
		if !initial[key].IsSet() {
			initial[key] = scalar("", false)
		}
	}
	runner, err := interp.New(
		func(r *interp.Runner) error {
			r.Dir = cwd
			r.Params = slices.Clone(params)
			return nil
		},
		interp.Env(initial),
		interp.StdIO(stdin, stdout, stderr),
		interp.Interactive(interactive),
		interp.OpenHandler(s.open),
		interp.StatHandler(s.stat),
		interp.AccessHandler(s.access),
		interp.ReadDirHandler2(s.readDir),
		interp.CallHandler(s.call),
		interp.ExecHandlers(func(interp.ExecHandlerFunc) interp.ExecHandlerFunc { return s.exec }),
	)
	if err != nil {
		return nil, err
	}
	runner.Reset()
	if hadTmp {
		initial["TMPDIR"] = tmp
	} else {
		delete(initial, "TMPDIR")
	}
	for _, key := range []string{"HOME", "UID", "EUID", "GID"} {
		if !env[key].IsSet() {
			delete(initial, key)
		}
	}
	return runner, nil
}

func (s *execution) call(ctx context.Context, args []string) ([]string, error) {
	if s.primary != nil && interp.HandlerCtx(ctx).Pos == s.primary.Args[0].Pos() {
		s.mu.Lock()
		if s.primaryArg == nil {
			s.primaryArg = slices.Clone(args)
		}
		s.mu.Unlock()
	}
	if virtualBuiltin(args[0]) {
		return append([]string{dispatchCommand}, args...), nil
	}
	return args, nil
}

func virtualBuiltin(name string) bool {
	switch name {
	case "cd", "alias", "unalias", "history", "exit", "pwd", "type", "command", "builtin", "source", ".", "eval", "trap", "test", "[", "unset":
		return true
	default:
		return false
	}
}

func (s *execution) readAliases(ctx context.Context, runner *interp.Runner, names []string) map[string]string {
	aliases := make(map[string]string)
	if len(names) == 0 {
		return aliases
	}
	var out bytes.Buffer
	query := runner.Subshell()
	_ = interp.StdIO(nil, &out, io.Discard)(query)
	for _, name := range names {
		out.Reset()
		_ = query.Run(ctx, literalCall("alias", name))
		if value, ok := strings.CutPrefix(out.String(), "alias "+name+"='"); ok {
			aliases[name] = strings.TrimSuffix(value, "'\n")
		}
	}
	return aliases
}

func exitCode(err error) (int, error) {
	if err == nil {
		return 0, nil
	}
	if status, ok := interp.IsExitStatus(err); ok {
		return int(status), nil
	}
	return 1, err
}

func status(code int) error {
	if code == 0 {
		return nil
	}
	return interp.ExitStatus(uint8(code))
}

func firstCommand(file *syntax.File, env shellEnv) (string, []string, *syntax.CallExpr) {
	var cmd string
	var args []string
	var call *syntax.CallExpr
	var found bool
	syntax.Walk(file, func(node syntax.Node) bool {
		if found {
			return false
		}
		switch node := node.(type) {
		case *syntax.CallExpr:
			if len(node.Args) > 0 {
				call = node
				cmd, _ = expand.Literal(&expand.Config{Env: env}, node.Args[0])
				for _, word := range node.Args[1:] {
					arg, _ := expand.Literal(&expand.Config{Env: env}, word)
					args = append(args, arg)
				}
			}
			found = true
		case *syntax.DeclClause:
			cmd = node.Variant.Value
			for _, arg := range node.Args {
				if arg.Name == nil {
					value, _ := expand.Literal(&expand.Config{Env: env}, arg.Value)
					args = append(args, value)
				} else if arg.Value != nil {
					value, _ := expand.Literal(&expand.Config{Env: env}, arg.Value)
					args = append(args, arg.Name.Value+"="+value)
				} else {
					args = append(args, arg.Name.Value)
				}
			}
			found = true
		}
		return !found
	})
	return cmd, args, call
}
