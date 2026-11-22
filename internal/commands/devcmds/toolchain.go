package devcmds

import (
	"errors"
	"fmt"
	"maps"
	"path"
	"regexp"
	"slices"
	"strings"

	"learn-linux/internal/contract"
)

var (
	cToken   = regexp.MustCompile(`(?m)//[^\n]*|(?s:/\*.*?(?:\*/|\z))|"(?:\\.|[^"\\\n])*"|'(?:\\.|[^'\\\n])*'|[[:alpha:]_][[:alnum:]_]*|[^[:space:]]`)
	cEscapes = strings.NewReplacer(`\n`, "\n", `\t`, "\t", `\r`, "\r", `\"`, `"`, `\\`, `\`, `\0`, "\x00")
)

func gcc(ec *contract.ExecContext, args []string) int {
	output := "a.out"
	var sources []string
	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch {
		case arg == "--":
			sources = append(sources, args[i+1:]...)
			i = len(args)
		case arg == "-o" || arg == "-I":
			i++
			if i == len(args) {
				return commandError(ec, "gcc", "error: missing argument to '%s'", arg)
			}
			if arg == "-o" {
				output = args[i]
			}
		case strings.HasPrefix(arg, "-o"):
			output = arg[2:]
		case arg == "-Wall" || arg == "-Wextra" || arg == "-g" ||
			arg == "-O0" || arg == "-O1" || arg == "-O2" || arg == "-O3" ||
			strings.HasPrefix(arg, "-std=") || strings.HasPrefix(arg, "-I"):
		case strings.HasPrefix(arg, "-"):
			return commandError(ec, "gcc", "error: unrecognized command-line option '%s'", arg)
		default:
			sources = append(sources, arg)
		}
	}
	if len(sources) == 0 {
		return commandError(ec, "gcc", "fatal error: no input files")
	}

	var result strings.Builder
	var srcPaths []string
	hasMain := false
	for _, source := range sources {
		p := resolve(ec, source)
		data, err := ec.VFS.ReadFile(p)
		if errors.Is(err, contract.ErrNotExist) {
			return commandError(ec, "gcc", "error: %s: No such file or directory", source)
		}
		if err != nil {
			return commandError(ec, "gcc", "error: %s: %v", source, err)
		}
		if path.Ext(source) != ".c" {
			return commandError(ec, "gcc", "error: %s: file format not recognized", source)
		}
		program, main, err := inspectC(string(data))
		if err != nil {
			return commandError(ec, "gcc", "%s: error: %v", source, err)
		}
		if hasMain && main {
			return commandError(ec, "gcc", "error: multiple definition of 'main'")
		}
		hasMain = hasMain || main
		result.WriteString(program)
		srcPaths = append(srcPaths, p)
	}
	if !hasMain {
		return commandError(ec, "gcc", "error: undefined reference to 'main'")
	}
	if err := ec.VFS.SetVirtualBinary(resolve(ec, output), 0755, result.String(), strings.Join(srcPaths, " ")); err != nil {
		return commandError(ec, "gcc", "error: cannot open output file %s: %v", output, err)
	}
	return 0
}

func inspectC(source string) (string, bool, error) {
	// Tokenize comments and literals before inspecting punctuation so that their
	// contents cannot introduce spurious braces or calls.
	tokens := cToken.FindAllString(source, -1)
	if len(tokens) > 0 && strings.HasPrefix(tokens[len(tokens)-1], "/*") && !strings.HasSuffix(tokens[len(tokens)-1], "*/") {
		return "", false, fmt.Errorf("unterminated comment")
	}
	tokens = slices.DeleteFunc(tokens, func(token string) bool {
		return strings.HasPrefix(token, "//") || strings.HasPrefix(token, "/*")
	})
	var output strings.Builder
	type delimiter struct {
		close string
		index int
	}
	var delimiters []delimiter
	closing := map[string]string{"{": "}", "(": ")", "[": "]"}
	main := false
	for i, token := range tokens {
		switch token {
		case "{", "(", "[":
			delimiters = append(delimiters, delimiter{closing[token], i})
		case "}", ")", "]":
			if len(delimiters) == 0 || delimiters[len(delimiters)-1].close != token {
				return "", false, fmt.Errorf("unexpected '%s' token", token)
			}
			open := delimiters[len(delimiters)-1].index
			delimiters = delimiters[:len(delimiters)-1]
			if token == ")" && len(delimiters) == 0 && open > 0 &&
				tokens[open-1] == "main" && i+1 < len(tokens) && tokens[i+1] == "{" {
				main = true
			}
			if token == "}" && len(delimiters) == 0 && open > 0 && tokens[open-1] == ")" && missingCSemicolon(tokens[open+1:i]) {
				return "", false, fmt.Errorf("expected ';' before '}' token")
			}
		case `"`, "'":
			return "", false, fmt.Errorf("missing terminating %s character", token)
		case "printf", "puts":
			if i+3 >= len(tokens) || tokens[i+1] != "(" || !strings.HasPrefix(tokens[i+2], `"`) {
				continue
			}
			for _, literal := range tokens[i+2:] {
				if len(literal) < 2 || literal[0] != '"' {
					break
				}
				output.WriteString(cEscapes.Replace(literal[1 : len(literal)-1]))
			}
			if token == "puts" {
				output.WriteByte('\n')
			}
		}
	}
	if len(delimiters) != 0 {
		return "", false, fmt.Errorf("expected declaration or statement at end of input")
	}
	return output.String(), main, nil
}

func missingCSemicolon(tokens []string) bool {
	// Check the simple statement forms understood by the simulator.
	if len(tokens) == 0 || tokens[len(tokens)-1] == ";" || tokens[len(tokens)-1] == "}" {
		return false
	}
	for i := len(tokens) - 1; i >= 0; i-- {
		switch tokens[i] {
		case "return", "printf", "puts":
			return true
		case ";", "{", "}":
			return false
		}
	}
	return false
}

type makeRule struct {
	deps    []string
	recipes []string
}

type makeVariable struct {
	value     string
	recursive bool
}

type makefile struct {
	first     string
	rules     map[string]*makeRule
	variables map[string]makeVariable
}

var (
	makeAssignment = regexp.MustCompile(`^([[:alnum:]_.-]+)[ \t]*(:?=)[ \t]*(.*)$`)
	makeReference  = regexp.MustCompile(`\$\(([^()]+)\)|\$\{([^{}]+)\}|\$[$@<^]`)
)

func makeCommand(ec *contract.ExecContext, args []string) int {
	dir := ec.CWD
	filename := ""
	var dryRun bool
	var targets []string
	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch arg {
		case "-f", "-C":
			i++
			if i == len(args) {
				return commandError(ec, "make", "option '%s' requires an argument", arg)
			}
			if arg == "-f" {
				filename = args[i]
			} else {
				dir = ec.VFS.ResolvePath(dir, ec.Home, args[i])
				if !ec.VFS.IsDir(dir) {
					return commandError(ec, "make", "*** %s: No such directory. Stop.", dir)
				}
			}
		case "-n":
			dryRun = true
		case "--":
			targets = append(targets, args[i+1:]...)
			i = len(args)
		default:
			if strings.HasPrefix(arg, "-") {
				return commandError(ec, "make", "unrecognized option '%s'", arg)
			}
			targets = append(targets, arg)
		}
	}

	read := func(name string) ([]byte, error) {
		return ec.VFS.ReadFile(ec.VFS.ResolvePath(dir, ec.Home, name))
	}
	var data []byte
	var err error
	if filename == "" {
		filename = "Makefile"
		data, err = read(filename)
		if errors.Is(err, contract.ErrNotExist) {
			filename = "makefile"
			data, err = read(filename)
		}
	} else {
		data, err = read(filename)
	}
	if err != nil {
		return commandError(ec, "make", "%s: %v", filename, err)
	}
	spec, err := parseMakefile(string(data), filename)
	if err != nil {
		return commandError(ec, "make", "%v", err)
	}
	if len(targets) == 0 {
		if spec.first == "" {
			return commandError(ec, "make", "*** No targets. Stop.")
		}
		targets = []string{spec.first}
	}

	if dir != ec.CWD && !dryRun {
		// RunSubcommand inherits this command's directory and environment.
		previous, previousEnv := ec.CWD, ec.Env
		ec.CWD = dir
		ec.Env = make(map[string]string, len(previousEnv)+1)
		maps.Copy(ec.Env, previousEnv)
		ec.Env["PWD"] = dir
		defer func() { ec.CWD, ec.Env = previous, previousEnv }()
	}
	return spec.build(ec, dir, targets, dryRun)
}

func parseMakefile(data, filename string) (*makefile, error) {
	spec := &makefile{rules: make(map[string]*makeRule), variables: make(map[string]makeVariable)}
	var active []*makeRule
	for lineNo, line := range strings.Split(data, "\n") {
		line = strings.TrimSuffix(line, "\r")
		if len(active) != 0 && (strings.HasPrefix(line, "\t") || strings.HasPrefix(line, "  ")) {
			recipe := strings.TrimSpace(line)
			if recipe != "" && !strings.HasPrefix(recipe, "#") {
				for _, rule := range active {
					rule.recipes = append(rule.recipes, recipe)
				}
			}
			continue
		}
		line, _, _ = strings.Cut(line, "#")
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		active = nil
		if fields := makeAssignment.FindStringSubmatch(line); fields != nil {
			value := fields[3]
			recursive := fields[2] == "="
			if !recursive {
				var err error
				value, err = spec.expand(value, nil)
				if err != nil {
					return nil, fmt.Errorf("%s:%d: %w", filename, lineNo+1, err)
				}
			}
			spec.variables[fields[1]] = makeVariable{value, recursive}
			continue
		}
		line, err := spec.expand(line, nil)
		if err != nil {
			return nil, fmt.Errorf("%s:%d: %w", filename, lineNo+1, err)
		}
		targets, deps, ok := strings.Cut(line, ":")
		if !ok || strings.TrimSpace(targets) == "" {
			return nil, fmt.Errorf("%s:%d: *** missing separator. Stop.", filename, lineNo+1)
		}
		for _, target := range strings.Fields(targets) {
			if spec.first == "" && target != ".PHONY" {
				spec.first = target
			}
			rule := spec.rules[target]
			if rule == nil {
				rule = &makeRule{}
				spec.rules[target] = rule
			}
			rule.deps = append(rule.deps, strings.Fields(deps)...)
			active = append(active, rule)
		}
	}
	return spec, nil
}

func (spec *makefile) expand(input string, automatic map[string]string) (string, error) {
	active := make(map[string]bool)
	var expandErr error
	var expand func(string) string
	expand = func(input string) string {
		return makeReference.ReplaceAllStringFunc(input, func(ref string) string {
			if ref == "$$" {
				return "$"
			}
			if len(ref) == 2 {
				return automatic[ref]
			}
			name := ref[2 : len(ref)-1]
			variable := spec.variables[name]
			if !variable.recursive {
				return variable.value
			}
			if active[name] {
				expandErr = fmt.Errorf("recursive variable '%s' references itself", name)
				return ""
			}
			active[name] = true
			value := expand(variable.value)
			delete(active, name)
			return value
		})
	}
	result := expand(input)
	return result, expandErr
}

func (spec *makefile) build(ec *contract.ExecContext, dir string, targets []string, dryRun bool) int {
	done, active := make(map[string]bool), make(map[string]bool)
	var visit func(string, string) int
	visit = func(target, parent string) int {
		if active[target] {
			return commandError(ec, "make", "*** Circular %s <- %s dependency. Stop.", parent, target)
		}
		if done[target] {
			return 0
		}
		rule := spec.rules[target]
		if rule == nil {
			p := ec.VFS.ResolvePath(dir, ec.Home, target)
			if ec.VFS.Exists(p) {
				done[target] = true
				return 0
			}
			return commandError(ec, "make", "*** No rule to make target '%s'. Stop.", target)
		}
		active[target] = true
		for _, dep := range rule.deps {
			if status := visit(dep, target); status != 0 {
				return status
			}
		}
		automatic := map[string]string{"$@": target, "$^": strings.Join(rule.deps, " ")}
		if len(rule.deps) != 0 {
			automatic["$<"] = rule.deps[0]
		}
		for _, recipe := range rule.recipes {
			recipe, silent := strings.CutPrefix(recipe, "@")
			line, err := spec.expand(strings.TrimSpace(recipe), automatic)
			if err != nil {
				return commandError(ec, "make", "%v", err)
			}
			if dryRun || !silent {
				fmt.Fprintln(ec.Stdout, line)
			}
			if dryRun {
				continue
			}
			if ec.RunSubcommand == nil {
				return commandError(ec, "make", "cannot execute recipe for target '%s'", target)
			}
			if status := ec.RunSubcommand(ec.Ctx, line, ec.Stdin, ec.Stdout, ec.Stderr); status != 0 {
				fmt.Fprintf(ec.Stderr, "make: *** [%s] Error %d\n", target, status)
				return status
			}
		}
		delete(active, target)
		done[target] = true
		return 0
	}
	for _, target := range targets {
		if status := visit(target, ""); status != 0 {
			return status
		}
	}
	return 0
}
