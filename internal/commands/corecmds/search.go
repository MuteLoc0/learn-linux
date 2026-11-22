package corecmds

import (
	"fmt"
	"io/fs"
	"path"
	"regexp"
	"strings"

	"learn-linux/internal/contract"
)

func grep(ec *contract.ExecContext, args []string) int {
	options, operands, err := parseFlags(args, "inrRvEFclqw", map[string]flagValue{"color": optionalValue}, false)
	if err != nil {
		return diagnostic(ec, "grep", err.Error(), 2)
	}
	if len(operands) == 0 {
		return diagnostic(ec, "grep", "missing pattern", 2)
	}
	color, err := colorEnabled(options, ec.IsTTY)
	if err != nil {
		return diagnostic(ec, "grep", err.Error(), 2)
	}
	mode := ""
	for _, opt := range options {
		if opt.name == "E" || opt.name == "F" {
			mode = opt.name
		}
	}
	pattern := operands[0]
	switch mode {
	case "F":
		lines := strings.Split(pattern, "\n")
		for i, line := range lines {
			lines[i] = regexp.QuoteMeta(line)
		}
		pattern = strings.Join(lines, "|")
	case "E":
		pattern = strings.ReplaceAll(pattern, "\n", "|")
	default:
		pattern = basicRegexp(pattern)
	}
	if hasFlag(options, "w") {
		pattern = `\b(?:` + pattern + `)\b`
	}
	if hasFlag(options, "i") {
		pattern = "(?i)" + pattern
	}
	expr, err := regexp.Compile(pattern)
	if err != nil {
		return diagnostic(ec, "grep", err.Error(), 2)
	}

	paths := operands[1:]
	follow := hasFlag(options, "R")
	recursive := follow || hasFlag(options, "r")
	if recursive && len(paths) == 0 {
		paths = []string{"."}
	}
	showFile := len(paths) > 1 || recursive
	showNum := hasFlag(options, "n")
	invert := hasFlag(options, "v")
	filesOnly, countOnly, quiet := hasFlag(options, "l"), hasFlag(options, "c"), hasFlag(options, "q")
	code := 1
	scan := func(name string, data []byte) {
		count := 0
		for i, line := range numberedLines(string(data)) {
			index := expr.FindStringIndex(line)
			if (index == nil) != invert {
				continue
			}
			count++
			if quiet {
				code = 0
				return
			}
			if filesOnly {
				fmt.Fprintln(ec.Stdout, name)
				break
			}
			if !countOnly {
				match := contract.GrepMatch{
					FilePath: name, LineNum: i, Line: line,
					ShowFile: showFile, ShowLineNum: showNum,
				}
				if index != nil && color && ec.UI != nil {
					for _, rg := range expr.FindAllStringIndex(line, -1) {
						match.MatchRanges = append(match.MatchRanges, [2]int{rg[0], rg[1]})
					}
				}
				if ec.UI != nil {
					fmt.Fprintln(ec.Stdout, ec.UI.RenderGrepMatch(match, color))
				} else {
					if match.ShowFile {
						fmt.Fprintf(ec.Stdout, "%s:", match.FilePath)
					}
					if match.ShowLineNum {
						fmt.Fprintf(ec.Stdout, "%d:", match.LineNum)
					}
					fmt.Fprintln(ec.Stdout, match.Line)
				}
			}
		}
		if countOnly && !filesOnly && !quiet {
			if showFile {
				fmt.Fprintf(ec.Stdout, "%s:", name)
			}
			fmt.Fprintln(ec.Stdout, count)
		}
		if count > 0 && code == 1 {
			code = 0
		}
	}
	report := func(name string, err error) {
		code = diagnostic(ec, "grep", fmt.Sprintf("%s: %v", name, err), 2)
	}

	ancestors := make(map[string]bool)
	var visit func(p, name string, info *contract.NodeInfo)
	visit = func(p, name string, info *contract.NodeInfo) {
		if !info.IsDir() || !recursive {
			data, err := ec.VFS.ReadFile(p)
			if err != nil {
				report(name, err)
				return
			}
			scan(name, data)
			return
		}
		if ancestors[info.Path] {
			report(name, fmt.Errorf("recursive directory loop"))
			return
		}
		ancestors[info.Path] = true
		defer delete(ancestors, info.Path)
		children, err := ec.VFS.ReadDir(p)
		if err != nil {
			report(name, err)
			return
		}
		for _, child := range children {
			childPath, childName := path.Join(p, child.Name), appendPath(name, child.Name)
			if child.Type == contract.TypeSymlink {
				if !follow {
					continue
				}
				child, err = ec.VFS.Stat(childPath)
				if err != nil {
					report(childName, err)
					continue
				}
			}
			visit(childPath, childName, child)
		}
	}
	for _, name := range stdinDefault(paths) {
		if name == "-" {
			data, err := readInput(ec, name)
			if err != nil {
				report("(standard input)", err)
				continue
			}
			scan("(standard input)", data)
			continue
		}
		p := resolve(ec, name)
		info, err := ec.VFS.Stat(p)
		if err != nil {
			report(name, err)
			continue
		}
		visit(p, name, info)
	}
	return code
}

func numberedLines(data string) func(func(int, string) bool) {
	return func(yield func(int, string) bool) {
		i := 1
		for line := range eachLine(data) {
			if !yield(i, line) {
				return
			}
			i++
		}
	}
}

// basicRegexp maps basic syntax for grouping, intervals, and alternation to
// the extended syntax accepted by Go's regexp package.
func basicRegexp(pattern string) string {
	var translated strings.Builder
	for i := 0; i < len(pattern); i++ {
		ch := pattern[i]
		switch {
		case ch == '\\' && i+1 < len(pattern):
			i++
			if !strings.ContainsRune("()+?{}|", rune(pattern[i])) {
				translated.WriteByte('\\')
			}
			translated.WriteByte(pattern[i])
		case ch == '[':
			end := bracketEnd(pattern, i+1)
			if end >= 0 {
				translated.WriteString(pattern[i : end+1])
				i = end
			} else {
				translated.WriteByte('[')
			}
		case strings.ContainsRune("()+?{}|", rune(ch)):
			translated.WriteByte('\\')
			translated.WriteByte(ch)
		case ch == '\n':
			translated.WriteByte('|')
		default:
			translated.WriteByte(ch)
		}
	}
	return translated.String()
}

func bracketEnd(pattern string, start int) int {
	if start < len(pattern) && pattern[start] == '^' {
		start++
	}
	if start < len(pattern) && pattern[start] == ']' {
		start++
	}
	for i := start; i < len(pattern); i++ {
		switch {
		case pattern[i] == '\\' && i+1 < len(pattern):
			i++
		case strings.HasPrefix(pattern[i:], "[:"):
			if end := strings.Index(pattern[i+2:], ":]"); end >= 0 {
				i += end + 3
			}
		case pattern[i] == ']':
			return i
		}
	}
	return -1
}

type findTest func(name string, info *contract.NodeInfo) (bool, error)

type findExpression struct {
	test   findTest
	negate bool
}

func find(ec *contract.ExecContext, args []string) int {
	var roots []string
	if len(args) > 0 && args[0] == "--" {
		args = args[1:]
	}
	for len(args) > 0 && !strings.HasPrefix(args[0], "-") && args[0] != "!" {
		roots = append(roots, args[0])
		args = args[1:]
	}
	if len(roots) == 0 {
		roots = []string{"."}
	}

	minDepth, maxDepth := 0, -1
	var expressions []findExpression
	negate := false
	for i := 0; i < len(args); i++ {
		op := args[i]
		if op == "-not" || op == "!" {
			negate = !negate
			if i+1 == len(args) {
				return diagnostic(ec, "find", fmt.Sprintf("missing argument to '%s'", op), 1)
			}
			continue
		}

		var test findTest
		switch op {
		case "-empty":
			test = func(p string, info *contract.NodeInfo) (bool, error) {
				if info.IsDir() {
					entries, err := ec.VFS.ReadDir(p)
					return len(entries) == 0, err
				}
				return info.Type == contract.TypeFile && info.Size == 0, nil
			}
		case "-name", "-iname", "-type", "-mindepth", "-maxdepth":
			if i+1 == len(args) {
				return diagnostic(ec, "find", fmt.Sprintf("missing argument to '%s'", op), 1)
			}
			i++
			value := args[i]
			switch op {
			case "-name", "-iname":
				if _, err := path.Match(value, ""); err != nil {
					return diagnostic(ec, "find", fmt.Sprintf("invalid pattern '%s'", value), 1)
				}
				test = nameTest(value, op == "-iname")
			case "-type":
				typ, ok := map[string]contract.FileType{
					"f": contract.TypeFile, "d": contract.TypeDir, "l": contract.TypeSymlink,
				}[value]
				if !ok {
					return diagnostic(ec, "find", fmt.Sprintf("invalid argument '%s' to '-type'", value), 1)
				}
				test = func(_ string, info *contract.NodeInfo) (bool, error) { return info.Type == typ, nil }
			case "-mindepth", "-maxdepth":
				depth, err := parseCount(value, 0)
				if err != nil {
					return diagnostic(ec, "find", err.Error(), 1)
				}
				if op == "-maxdepth" {
					maxDepth = depth
				} else {
					minDepth = depth
				}
				test = func(string, *contract.NodeInfo) (bool, error) { return true, nil }
			}
		default:
			return diagnostic(ec, "find", fmt.Sprintf("unknown predicate '%s'", op), 1)
		}
		expressions = append(expressions, findExpression{test, negate})
		negate = false
	}

	code := 0
	for _, root := range roots {
		p := resolve(ec, root)
		walkErr := ec.VFS.Walk(p, func(visited string, info *contract.NodeInfo) error {
			name, depth := root, 0
			if visited != p {
				relative := strings.TrimPrefix(visited, strings.TrimSuffix(p, "/")+"/")
				name = appendPath(root, relative)
				depth = strings.Count(relative, "/") + 1
			}
			if depth >= minDepth && (maxDepth < 0 || depth <= maxDepth) {
				matches := true
				for _, expr := range expressions {
					// Use the walked path for I/O and retain the operand's basename for -name.
					entry := *info
					entry.Name = path.Base(name)
					ok, err := expr.test(visited, &entry)
					if err != nil {
						code = diagnostic(ec, "find", fmt.Sprintf("'%s': %v", name, err), 1)
						matches = false
						break
					}
					if ok == expr.negate {
						matches = false
						break
					}
				}
				if matches {
					fmt.Fprintln(ec.Stdout, name)
				}
			}
			if maxDepth >= 0 && depth >= maxDepth && info.IsDir() {
				return fs.SkipDir
			}
			return nil
		})
		if walkErr != nil {
			code = diagnostic(ec, "find", fmt.Sprintf("'%s': %v", root, walkErr), 1)
		}
	}
	return code
}

func nameTest(pattern string, folded bool) findTest {
	if folded {
		pattern = strings.ToLower(pattern)
	}
	return func(_ string, info *contract.NodeInfo) (bool, error) {
		name := info.Name
		if folded {
			name = strings.ToLower(name)
		}
		return path.Match(pattern, name)
	}
}
