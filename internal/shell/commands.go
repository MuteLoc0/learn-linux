package shell

import (
	"context"
	"errors"
	"fmt"
	"io"
	"maps"
	"runtime"
	"slices"
	"strconv"
	"strings"

	"learn-linux/internal/contract"
	"mvdan.cc/sh/v3/interp"
	"mvdan.cc/sh/v3/syntax"
)

func (s *execution) exec(ctx context.Context, args []string) error {
	hc := interp.HandlerCtx(ctx)
	if args[0] == dispatchCommand {
		if len(args) < 2 {
			return status(127)
		}
		args = args[1:]
		if args[0] == "status" && len(args) == 2 {
			code, _ := strconv.Atoi(args[1])
			return status(code)
		}
	}

	name, rest := args[0], args[1:]
	switch name {
	case "cd":
		return s.cd(ctx, hc, rest)
	case "alias":
		return s.alias(ctx, hc, rest)
	case "unalias":
		s.mu.Lock()
		if slices.Equal(rest, []string{"-a"}) {
			rest = slices.Sorted(maps.Keys(s.aliasNames))
		}
		for _, alias := range rest {
			delete(s.aliasInput, alias)
		}
		s.mu.Unlock()
		return hc.Builtin(ctx, append([]string{name}, rest...))
	case "exit":
		s.mu.Lock()
		s.sawExit = true
		s.mu.Unlock()
		return hc.Builtin(ctx, args)
	case "pwd":
		return s.pwd(ctx, hc, rest)
	case "command", "builtin":
		return s.indirect(ctx, hc, name, rest)
	case "type":
		return s.describe(hc, name, rest)
	case "source", ".":
		if len(rest) == 0 {
			return diagnostic(hc.Stderr, name, 2, "filename argument required")
		}
		data, err := s.vfs.ReadFile(s.resolve(hc.Dir, rest[0]))
		if err != nil {
			return diagnostic(hc.Stderr, name, 1, "%s: %s", rest[0], fileError(err))
		}
		return s.eval(ctx, hc, name, string(data))
	case "eval":
		return s.eval(ctx, hc, name, strings.Join(rest, " "))
	case "trap":
		trapArgs := rest
		for len(trapArgs) > 0 && trapArgs[0] == "-" {
			trapArgs = trapArgs[1:]
		}
		if len(trapArgs) > 0 && trapArgs[0] == "--" {
			trapArgs = trapArgs[1:]
		}
		if len(trapArgs) >= 2 {
			if _, err := parseScript(trapArgs[0], ""); err != nil {
				return diagnostic(hc.Stderr, name, 2, "%v", err)
			}
		}
		return hc.Builtin(ctx, args)
	case "test", "[":
		return s.test(ctx, hc, args)
	case "unset":
		for _, arg := range rest {
			key, index, ok := strings.Cut(arg, "[")
			if !ok || !syntax.ValidName(key) || !strings.HasSuffix(index, "]") {
				continue
			}
			node, err := syntax.NewParser().Arithmetic(strings.NewReader(strings.TrimSuffix(index, "]")))
			if err == nil && node != nil {
				if err := validateSyntax(node); err != nil {
					return diagnostic(hc.Stderr, name, 2, "%v", err)
				}
			}
		}
		return hc.Builtin(ctx, args)
	case "sh", "bash":
		return s.shellCommand(ctx, hc, name, rest)
	}

	if strings.ContainsRune(name, '/') {
		return s.runPath(ctx, hc, name, rest)
	}
	if run, ok := s.engine.Registry().Lookup(name); ok {
		return status(run(s.execContext(ctx, hc), rest))
	}
	switch name {
	case "env":
		env := envStrings(hc.Env, true)
		for _, key := range slices.Sorted(maps.Keys(env)) {
			fmt.Fprintf(hc.Stdout, "%s=%s\n", key, env[key])
		}
		return nil
	case "history":
		return s.printHistory(hc, rest)
	default:
		fmt.Fprintf(hc.Stderr, "bash: %s: command not found\n", name)
		return status(127)
	}
}

func (s *execution) alias(ctx context.Context, hc interp.HandlerContext, args []string) error {
	for _, arg := range args {
		name, value, defined := strings.Cut(arg, "=")
		if !defined {
			continue
		}
		for word, err := range syntax.NewParser().WordsSeq(strings.NewReader(value)) {
			if err == nil {
				err = validateSyntax(word)
			}
			if err != nil {
				return diagnostic(hc.Stderr, "alias", 2, "%v", err)
			}
		}
		s.mu.Lock()
		if s.aliasNames == nil {
			s.aliasNames = make(map[string]struct{})
			s.aliasInput = make(map[string]string)
		}
		s.aliasNames[name] = struct{}{}
		s.aliasInput[name] = value
		s.mu.Unlock()
	}
	return hc.Builtin(ctx, append([]string{"alias"}, args...))
}

func (s *execution) eval(ctx context.Context, hc interp.HandlerContext, name, code string) error {
	if _, err := parseScript(code, ""); err != nil {
		return diagnostic(hc.Stderr, name, 2, "%v", err)
	}
	// Evaluate in the current interpreter so changes are visible immediately.
	return hc.Builtin(ctx, []string{"eval", code})
}

func (s *execution) cd(ctx context.Context, hc interp.HandlerContext, args []string) error {
	physical := false
	for len(args) > 0 && strings.HasPrefix(args[0], "-") && args[0] != "-" {
		switch args[0] {
		case "-P":
			physical = true
			args = args[1:]
		case "-L":
			physical = false
			args = args[1:]
		case "--":
			args = args[1:]
			goto parsed
		default:
			return diagnostic(hc.Stderr, "cd", 2, "invalid option: %q", args[0])
		}
	}
parsed:
	if len(args) > 1 {
		return diagnostic(hc.Stderr, "cd", 1, "too many arguments")
	}
	curDir := s.currentDir(hc)
	var target string
	printNew := false
	switch {
	case len(args) == 0:
		home := hc.Env.Get("HOME").String()
		if home == "" {
			home = s.engine.cfg.Home
		}
		if home == "" {
			return diagnostic(hc.Stderr, "cd", 1, "HOME not set")
		}
		target = home
	case args[0] == "-":
		old := hc.Env.Get("OLDPWD").String()
		if old == "" {
			return diagnostic(hc.Stderr, "cd", 1, "OLDPWD not set")
		}
		target = old
		printNew = true
	default:
		target = args[0]
	}

	resolved := s.resolve(curDir, target)
	info, err := s.vfs.Stat(resolved)
	if err != nil {
		return diagnostic(hc.Stderr, "cd", 1, "%s: %s", target, fileError(err))
	}
	if !info.IsDir() {
		return diagnostic(hc.Stderr, "cd", 1, "%s: Not a directory", target)
	}
	perm := info.Mode.Perm()
	if (perm|perm>>3|perm>>6)&1 == 0 {
		return diagnostic(hc.Stderr, "cd", 1, "%s: Permission denied", target)
	}
	if physical {
		resolved = info.Path
	}

	if runtime.GOOS != "windows" {
		if err := hc.Builtin(ctx, []string{"cd", resolved}); err != nil {
			return err
		}
	} else {
		if err := hc.Builtin(ctx, []string{"export", "OLDPWD=" + curDir, "PWD=" + resolved}); err != nil {
			return err
		}
	}
	if printNew {
		fmt.Fprintln(hc.Stdout, resolved)
	}
	return nil
}

func (s *execution) pwd(ctx context.Context, hc interp.HandlerContext, args []string) error {
	physical := false
	for _, arg := range args {
		switch arg {
		case "-P":
			physical = true
		case "-L":
			physical = false
		default:
			return diagnostic(hc.Stderr, "pwd", 2, "invalid option: %q", arg)
		}
	}
	curDir := s.currentDir(hc)
	if !physical {
		fmt.Fprintln(hc.Stdout, curDir)
		return nil
	}
	info, err := s.vfs.Stat(curDir)
	if err != nil {
		return diagnostic(hc.Stderr, "pwd", 1, "%s: %s", curDir, fileError(err))
	}
	fmt.Fprintln(hc.Stdout, info.Path)
	return nil
}

func (s *execution) indirect(ctx context.Context, hc interp.HandlerContext, name string, args []string) error {
	if name == "command" && len(args) > 0 {
		switch args[0] {
		case "-v", "-V":
			return s.describe(hc, name, args)
		case "--", "-p":
			args = args[1:]
		}
	}
	if len(args) == 0 {
		return nil
	}
	target := args[0]
	if name == "builtin" && !interp.IsBuiltin(target) {
		return status(1)
	}
	if virtualBuiltin(target) {
		return s.exec(ctx, args)
	}
	if interp.IsBuiltin(target) {
		return hc.Builtin(ctx, args)
	}
	return s.exec(ctx, args)
}

func (s *execution) describe(hc interp.HandlerContext, name string, args []string) error {
	mode := ""
	if len(args) > 0 && strings.HasPrefix(args[0], "-") {
		mode, args = args[0], args[1:]
	}
	code := 0
	for _, arg := range args {
		kind, description := "", arg
		if strings.ContainsRune(arg, '/') {
			if info, err := s.vfs.Stat(s.resolve(hc.Dir, arg)); err == nil && info.IsExecutable() {
				kind = "file"
			}
		} else if syntax.IsKeyword(arg) {
			kind = "keyword"
		} else if interp.IsBuiltin(arg) {
			kind = "builtin"
		} else if _, ok := s.engine.Registry().Lookup(arg); ok || arg == "env" || arg == "sh" || arg == "bash" {
			kind = "builtin"
		}
		if kind == "" || (mode == "-P" || mode == "-p") && kind != "file" {
			code = 1
			if mode == "" {
				fmt.Fprintf(hc.Stderr, "bash: %s: %s: not found\n", name, arg)
			}
			continue
		}
		switch mode {
		case "-t":
			description = kind
		case "", "-V":
			description = fmt.Sprintf("%s is a shell %s", arg, kind)
			if kind == "file" {
				description = arg + " is " + arg
			}
		}
		fmt.Fprintln(hc.Stdout, description)
	}
	return status(code)
}

func (s *execution) test(ctx context.Context, hc interp.HandlerContext, args []string) error {
	name := args[0]
	operands := args[1:]
	if name == "[" && len(operands) > 0 && operands[len(operands)-1] == "]" {
		operands = operands[:len(operands)-1]
	}
	if len(operands) == 2 && (operands[0] == "-O" || operands[0] == "-G") {
		info, err := s.vfs.Stat(s.resolve(hc.Dir, operands[1]))
		if err != nil {
			return status(1)
		}
		if operands[0] == "-O" && info.Owner == s.engine.cfg.User ||
			operands[0] == "-G" && info.Group == s.engine.cfg.User {
			return nil
		}
		return status(1)
	}
	// The native ownership operators expect a host syscall.Stat_t.
	if len(operands) > 2 && (slices.Contains(operands, "-O") || slices.Contains(operands, "-G")) {
		return diagnostic(hc.Stderr, name, 2, "use a separate test for virtual file ownership checks")
	}
	return hc.Builtin(ctx, args)
}

func (s *execution) printHistory(hc interp.HandlerContext, args []string) error {
	start := 0
	if len(args) > 1 {
		return diagnostic(hc.Stderr, "history", 2, "too many arguments")
	}
	if len(args) == 1 {
		n, err := strconv.Atoi(args[0])
		if err != nil || n < 0 {
			return diagnostic(hc.Stderr, "history", 2, "%s: numeric argument required", args[0])
		}
		start = max(0, len(s.history)-n)
	}
	for i := start; i < len(s.history); i++ {
		fmt.Fprintf(hc.Stdout, "%3d  %s\n", i+1, s.history[i])
	}
	return nil
}

func (s *execution) execContext(ctx context.Context, hc interp.HandlerContext) *contract.ExecContext {
	cfg := s.engine.cfg
	s.mu.Lock()
	aliases := maps.Clone(s.aliasInput)
	s.mu.Unlock()
	width, height := 80, 24
	if cfg.Term != nil {
		width, height = cfg.Term.Size()
	}
	curDir := s.currentDir(hc)
	ec := &contract.ExecContext{
		Ctx: ctx, VFS: s.vfs, Stdin: hc.Stdin, Stdout: hc.Stdout, Stderr: hc.Stderr,
		CWD: curDir, Home: cfg.Home, User: cfg.User, Host: cfg.Host, Env: envStrings(hc.Env, true),
		Aliases: aliases, History: slices.Clone(s.history),
		Term: cfg.Term, UI: cfg.UI, I18N: cfg.I18N, IsTTY: hc.Stdout == s.tty,
		TermWidth: width, TermHeight: height,
		OnDestructive: cfg.OnDestructive, OnTutorialAction: cfg.OnTutorialAction,
	}
	ec.SetCWD = func(dir string) {
		resolved := s.resolve(ec.CWD, dir)
		if s.cd(ctx, hc, []string{resolved}) == nil {
			ec.Env["OLDPWD"], ec.Env["PWD"], ec.CWD = ec.CWD, resolved, resolved
		}
	}
	ec.RunSubcommand = func(ctx context.Context, line string, stdin io.Reader, stdout, stderr io.Writer) int {
		// The make recipe inherits the command's working directory and any
		// environment values the registered command supplied to the recipe.
		env := make(shellEnv)
		for name, value := range ec.Env {
			env[name] = scalar(value, true)
		}
		code, err := exitCode(s.child(ctx, "sh", line, ec.CWD, env, nil, stdin, stdout, stderr))
		if err != nil && stderr != nil {
			fmt.Fprintln(stderr, err)
		}
		return code
	}
	return ec
}

func (s *execution) shellCommand(ctx context.Context, hc interp.HandlerContext, name string, args []string) error {
	if len(args) == 0 {
		if hc.Stdin == nil {
			return nil
		}
		data, err := io.ReadAll(hc.Stdin)
		if err != nil {
			return diagnostic(hc.Stderr, name, 1, "%v", err)
		}
		return s.runChild(ctx, hc, name, string(data), nil)
	}
	if args[0] == "-c" {
		if len(args) < 2 {
			return diagnostic(hc.Stderr, name, 2, "-c: option requires an argument")
		}
		var params []string
		if len(args) > 2 {
			name, params = args[2], args[3:]
		}
		return s.runChild(ctx, hc, name, args[1], params)
	}
	if args[0] == "--" {
		args = args[1:]
		if len(args) == 0 {
			return s.shellCommand(ctx, hc, name, nil)
		}
	}
	return s.readScript(ctx, hc, args[0], s.resolve(s.currentDir(hc), args[0]), args[1:])
}

func (s *execution) runPath(ctx context.Context, hc interp.HandlerContext, name string, args []string) error {
	path := s.resolve(s.currentDir(hc), name)
	info, err := s.vfs.Stat(path)
	switch {
	case err != nil:
		code := 126
		if errors.Is(err, contract.ErrNotExist) || errors.Is(err, contract.ErrNotDir) {
			code = 127
		}
		return diagnostic(hc.Stderr, name, code, "%s", fileError(err))
	case info.IsDir():
		return diagnostic(hc.Stderr, name, 126, "Is a directory")
	case !info.IsExecutable():
		return diagnostic(hc.Stderr, name, 126, "Permission denied")
	case info.IsVirtualBinary:
		_, _ = io.WriteString(hc.Stdout, info.VirtualBinaryOutput)
		return nil
	}
	return s.readScript(ctx, hc, name, path, args)
}

func (s *execution) readScript(ctx context.Context, hc interp.HandlerContext, name, path string, args []string) error {
	data, err := s.vfs.ReadFile(path)
	if err != nil {
		code := 126
		if errors.Is(err, contract.ErrNotExist) || errors.Is(err, contract.ErrNotDir) {
			code = 127
		}
		return diagnostic(hc.Stderr, name, code, "%s", fileError(err))
	}
	// The parser treats a shebang as a comment, preserving script line numbers.
	return s.runChild(ctx, hc, name, string(data), args)
}

func (s *execution) runChild(ctx context.Context, hc interp.HandlerContext, name, code string, params []string) error {
	env := make(shellEnv)
	maps.Insert(env, hc.Env.Each)
	return s.child(ctx, name, code, s.currentDir(hc), env, params, hc.Stdin, hc.Stdout, hc.Stderr)
}

func (s *execution) child(ctx context.Context, name, code, cwd string, env shellEnv, params []string, stdin io.Reader, stdout, stderr io.Writer) error {
	if stderr == nil {
		stderr = io.Discard
	}
	file, err := parseScript(code, name)
	if err != nil {
		return diagnostic(stderr, name, 2, "%v", err)
	}
	child := &execution{engine: s.engine, vfs: s.vfs, tty: s.tty, history: s.history}
	runner, err := child.newRunner(cwd, env, params, stdin, stdout, stderr, false)
	if err != nil {
		return err
	}
	return runner.Run(ctx, file)
}

func diagnostic(w io.Writer, name string, code int, format string, args ...any) error {
	fmt.Fprintf(w, "bash: %s: %s\n", name, fmt.Sprintf(format, args...))
	return status(code)
}

func fileError(err error) string {
	switch {
	case errors.Is(err, contract.ErrNotExist):
		return "No such file or directory"
	case errors.Is(err, contract.ErrPermission):
		return "Permission denied"
	case errors.Is(err, contract.ErrIsDir):
		return "Is a directory"
	case errors.Is(err, contract.ErrNotDir):
		return "Not a directory"
	default:
		return err.Error()
	}
}
