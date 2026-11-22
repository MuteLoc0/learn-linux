package shell

import (
	"maps"
	"slices"
	"strings"
	"unicode/utf8"

	"learn-linux/internal/contract"
	"mvdan.cc/sh/v3/syntax"
)

var shellWords = strings.Fields(
	"alias bash break builtin case cd command continue declare dirs do done echo elif else env esac eval exec exit export " +
		"false fi for function getopts hash help history if in local mapfile popd printf pushd pwd read readarray readonly " +
		"return select set sh shift shopt source test then time times trap true type typeset unalias unset until wait while",
)

func (e *Engine) Complete(line string, cursor int) contract.CompletionResult {
	cursor = min(max(cursor, 0), len(line))
	for cursor < len(line) && !utf8.RuneStart(line[cursor]) {
		cursor--
	}
	start, end, command := completionSpan(line, cursor)
	prefix := unquotePrefix(line[start:cursor])
	result := contract.CompletionResult{ReplaceStart: start, ReplaceEnd: end}
	cwd, vfs, aliases := e.completionState()
	if command && !strings.ContainsRune(prefix, '/') && prefix != "~" {
		names := make(map[string]struct{})
		for _, name := range e.Registry().Names() {
			names[name] = struct{}{}
		}
		for _, name := range shellWords {
			names[name] = struct{}{}
		}
		for name := range aliases {
			names[name] = struct{}{}
		}
		for name := range maps.Keys(names) {
			if strings.HasPrefix(name, prefix) {
				result.Matches = append(result.Matches, escapeCompletion(name))
			}
		}
	} else {
		result.IsPath = true
		result.Matches = completePath(vfs, cwd, e.cfg.Home, prefix, command)
	}
	slices.Sort(result.Matches)
	result.CommonPrefix = commonPrefix(result.Matches)
	return result
}

func (e *Engine) completionState() (string, contract.VFS, map[string]string) {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.cwd, e.vfs, maps.Clone(e.aliases)
}

func completePath(vfs contract.VFS, cwd, home, prefix string, command bool) []string {
	if prefix == "~" {
		return []string{"~/"}
	}
	split := strings.LastIndexByte(prefix, '/') + 1
	directory, base := prefix[:split], prefix[split:]
	dir := vfs.ResolvePath(cwd, home, directory)
	entries, err := vfs.ReadDir(dir)
	if err != nil {
		return nil
	}
	var matches, executable []string
	for _, entry := range entries {
		name := entry.Name
		if !strings.HasPrefix(name, base) || strings.HasPrefix(name, ".") && !strings.HasPrefix(base, ".") {
			continue
		}
		info := entry
		if entry.Type == contract.TypeSymlink {
			if target, err := vfs.Stat(vfs.ResolvePath(dir, home, name)); err == nil {
				info = target
			}
		}
		candidate := escapeCompletion(directory + name)
		if strings.HasPrefix(directory, "~/") {
			candidate = "~/" + escapeCompletion(directory[2:]+name)
		}
		if info.IsDir() {
			candidate += "/"
		}
		matches = append(matches, candidate)
		if info.IsDir() || info.IsExecutable() {
			executable = append(executable, candidate)
		}
	}
	if command && strings.HasPrefix(prefix, "./") && len(executable) > 0 {
		return executable
	}
	return matches
}

func completionSpan(line string, cursor int) (start, end int, command bool) {
	command, redirect := true, false
	for i := 0; i < len(line); {
		switch line[i] {
		case ' ', '\t', '\r':
			if i == cursor {
				return i, i, command && !redirect
			}
			i++
		case '|', '&', ';', '\n', '(', ')':
			if i == cursor {
				return i, i, command && !redirect
			}
			command, redirect = true, false
			i++
		case '<', '>':
			if i == cursor {
				return i, i, false
			}
			redirect = true
			i++
			for i < len(line) && strings.ContainsRune("<>&|", rune(line[i])) {
				i++
			}
			if i > cursor {
				return cursor, cursor, false
			}
		default:
			end := wordEnd(line, i)
			if i <= cursor && cursor <= end {
				return i, end, command && !redirect
			}
			word := line[i:end]
			if redirect {
				redirect = false
			} else if command && end < len(line) && strings.ContainsRune("<>", rune(line[end])) && digitsOnly(word) {
				// A file descriptor is part of the redirect operator.
			} else if command {
				key, _, assignment := strings.Cut(word, "=")
				command = assignment && syntax.ValidName(key)
				switch word {
				case "if", "then", "elif", "else", "while", "until", "do", "!", "{":
					command = true
				}
			}
			i = end
		}
	}
	return cursor, cursor, command && !redirect
}

func wordEnd(line string, start int) int {
	var quote byte
	for i := start; i < len(line); i++ {
		b := line[i]
		if b == '\\' && quote != '\'' && i+1 < len(line) {
			i++
			continue
		}
		if quote != 0 {
			if b == quote {
				quote = 0
			}
			continue
		}
		switch b {
		case '\'', '"':
			quote = b
		case ' ', '\t', '\r', '\n', '|', '&', ';', '(', ')', '<', '>':
			return i
		}
	}
	return len(line)
}

func digitsOnly(text string) bool {
	return strings.Trim(text, "0123456789") == ""
}

func unquotePrefix(raw string) string {
	var result strings.Builder
	var quote byte
	for i := 0; i < len(raw); i++ {
		b := raw[i]
		switch {
		case quote != 0 && b == quote:
			quote = 0
		case quote == 0 && (b == '\'' || b == '"'):
			quote = b
		case b == '\\' && quote != '\'' && i+1 < len(raw) &&
			(quote == 0 || strings.ContainsRune("\\\"`$\n", rune(raw[i+1]))):
			i++
			if raw[i] != '\n' {
				result.WriteByte(raw[i])
			}
		default:
			result.WriteByte(b)
		}
	}
	return result.String()
}

func escapeCompletion(text string) string {
	var result strings.Builder
	for _, r := range text {
		if strings.ContainsRune(" \t\r\\\"'`$&|;()<>*?[]{}#~!", r) {
			result.WriteByte('\\')
		}
		if r == '\n' {
			result.WriteString("'\n'")
		} else {
			result.WriteRune(r)
		}
	}
	return result.String()
}

func commonPrefix(matches []string) string {
	if len(matches) == 0 {
		return ""
	}
	first, last := matches[0], matches[len(matches)-1]
	n := 0
	for n < len(first) && n < len(last) && first[n] == last[n] {
		n++
	}
	for n < len(first) && !utf8.RuneStart(first[n]) {
		n--
	}
	return first[:n]
}
