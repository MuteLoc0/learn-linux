package corecmds

import (
	"bytes"
	"fmt"
	"io"
	"math/big"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"unicode/utf8"

	"learn-linux/internal/contract"
)

func cat(ec *contract.ExecContext, args []string) int {
	options, paths, err := parseFlags(args, "n", nil, false)
	if err != nil {
		return diagnostic(ec, "cat", err.Error(), 1)
	}
	number := hasFlag(options, "n")
	code, lineNum := 0, 1
	atLineStart := true
	for _, name := range stdinDefault(paths) {
		data, err := readInput(ec, name)
		if err != nil {
			if err == contract.ErrIsDir {
				err = fmt.Errorf("Is a directory")
			}
			code = diagnostic(ec, "cat", fmt.Sprintf("%s: %v", name, err), 1)
			continue
		}
		if !number {
			ec.Stdout.Write(data)
			continue
		}
		for len(data) > 0 {
			if atLineStart {
				fmt.Fprintf(ec.Stdout, "%6d\t", lineNum)
				lineNum++
			}
			end := bytes.IndexByte(data, '\n') + 1
			atLineStart = end > 0
			if !atLineStart {
				end = len(data)
			}
			ec.Stdout.Write(data[:end])
			data = data[end:]
		}
	}
	return code
}

// eachLine yields logical lines without their terminating newlines.
func eachLine(data string) func(func(string) bool) {
	return func(yield func(string) bool) {
		for data != "" {
			line, rest, _ := strings.Cut(data, "\n")
			if !yield(line) {
				return
			}
			data = rest
		}
	}
}

func head(ec *contract.ExecContext, args []string) int {
	return endOfInput(ec, args, "head")
}

func tail(ec *contract.ExecContext, args []string) int {
	return endOfInput(ec, args, "tail")
}

func endOfInput(ec *contract.ExecContext, args []string, command string) int {
	options, paths, err := parseFlags(args, "n:c:", nil, true)
	if err != nil {
		return diagnostic(ec, command, err.Error(), 1)
	}
	count, byBytes, fromStart, omitEnd := 10, false, false, false
	for _, opt := range options {
		byBytes = opt.name == "c"
		fromStart = command == "tail" && strings.HasPrefix(opt.value, "+")
		omitEnd = command == "head" && strings.HasPrefix(opt.value, "-")
		value := strings.TrimPrefix(strings.TrimPrefix(opt.value, "+"), "-")
		count, err = parseCount(value, 0)
		if err != nil || strings.HasPrefix(value, "+") || strings.HasPrefix(value, "-") {
			unit := "line"
			if byBytes {
				unit = "byte"
			}
			return diagnostic(ec, command, fmt.Sprintf("invalid number of %ss: '%s'", unit, opt.value), 1)
		}
	}
	code := 0
	printed := false
	for _, name := range stdinDefault(paths) {
		data, err := readInput(ec, name)
		if err != nil {
			code = diagnostic(ec, command, fmt.Sprintf("cannot open '%s' for reading: %v", name, err), 1)
			continue
		}
		if len(paths) > 1 {
			if printed {
				fmt.Fprintln(ec.Stdout)
			}
			label := name
			if name == "-" {
				label = "standard input"
			}
			fmt.Fprintf(ec.Stdout, "==> %s <==\n", label)
			printed = true
		}
		if command == "head" && !omitEnd {
			ec.Stdout.Write(data[:inputOffset(data, count, byBytes, false)])
		} else if command == "tail" && fromStart {
			ec.Stdout.Write(data[inputOffset(data, max(count-1, 0), byBytes, false):])
		} else {
			offset := inputOffset(data, count, byBytes, true)
			if omitEnd {
				ec.Stdout.Write(data[:offset])
			} else {
				ec.Stdout.Write(data[offset:])
			}
		}
	}
	return code
}

func inputOffset(data []byte, count int, byBytes, fromEnd bool) int {
	if byBytes {
		if fromEnd {
			return len(data) - min(count, len(data))
		}
		return min(count, len(data))
	}
	if fromEnd {
		offset := len(data)
		for count > 0 && offset > 0 {
			offset = bytes.LastIndexByte(data[:offset-1], '\n') + 1
			count--
		}
		return offset
	}
	offset := 0
	for count > 0 && offset < len(data) {
		end := bytes.IndexByte(data[offset:], '\n')
		if end < 0 {
			return len(data)
		}
		offset += end + 1
		count--
	}
	return offset
}

func wc(ec *contract.ExecContext, args []string) int {
	options, paths, err := parseFlags(args, "lwcm", nil, false)
	if err != nil {
		return diagnostic(ec, "wc", err.Error(), 1)
	}
	type counts struct{ lines, words, chars, bytes int }
	var total counts
	printCounts := func(c counts, name string) {
		var values []string
		for _, col := range []struct {
			flag    string
			count   int
			showAll bool
		}{
			{"l", c.lines, true}, {"w", c.words, true}, {"m", c.chars, false}, {"c", c.bytes, true},
		} {
			if hasFlag(options, col.flag) || (len(options) == 0 && col.showAll) {
				values = append(values, strconv.Itoa(col.count))
			}
		}
		if name != "" {
			values = append(values, name)
		}
		fmt.Fprintln(ec.Stdout, strings.Join(values, " "))
	}
	code := 0
	for _, name := range stdinDefault(paths) {
		data, err := readInput(ec, name)
		if err != nil {
			code = diagnostic(ec, "wc", fmt.Sprintf("%s: %v", name, err), 1)
			continue
		}
		c := counts{bytes.Count(data, []byte("\n")), len(bytes.Fields(data)), utf8.RuneCount(data), len(data)}
		total.lines += c.lines
		total.words += c.words
		total.chars += c.chars
		total.bytes += c.bytes
		if len(paths) == 0 {
			name = ""
		}
		printCounts(c, name)
	}
	if len(paths) > 1 {
		printCounts(total, "total")
	}
	return code
}

var leadingNumber = regexp.MustCompile("^[[:blank:]]*(-?([0-9]+(\\.[0-9]*)?|\\.[0-9]+))")

func sortLines(ec *contract.ExecContext, args []string) int {
	options, paths, err := parseFlags(args, "rnufk:t:", nil, false)
	if err != nil {
		return diagnostic(ec, "sort", err.Error(), 1)
	}
	var field int
	var delim string
	for _, opt := range options {
		switch opt.name {
		case "k":
			field, err = parseCount(opt.value, 1)
			if err != nil {
				return diagnostic(ec, "sort", err.Error(), 1)
			}
		case "t":
			if utf8.RuneCountInString(opt.value) != 1 {
				return diagnostic(ec, "sort", fmt.Sprintf("invalid field separator '%s'", opt.value), 1)
			}
			delim = opt.value
		}
	}
	type keyedLine struct {
		line, key string
		number    big.Rat
	}
	var lines []keyedLine
	numeric, fold := hasFlag(options, "n"), hasFlag(options, "f")
	for _, name := range stdinDefault(paths) {
		data, err := readInput(ec, name)
		if err != nil {
			return diagnostic(ec, "sort", fmt.Sprintf("cannot read: %s: %v", name, err), 1)
		}
		for line := range eachLine(string(data)) {
			key := line
			if field > 0 {
				fields := strings.Fields(line)
				if delim != "" {
					fields = strings.Split(line, delim)
				}
				key = ""
				if field <= len(fields) {
					key = fields[field-1]
				}
			}
			if fold {
				key = strings.ToUpper(key)
			}
			entry := keyedLine{line: line, key: key}
			if numeric {
				if match := leadingNumber.FindStringSubmatch(key); match != nil {
					entry.number.SetString(match[1])
				}
			}
			lines = append(lines, entry)
		}
	}
	compareKey := func(a, b keyedLine) int {
		if numeric {
			return a.number.Cmp(&b.number)
		}
		return strings.Compare(a.key, b.key)
	}
	unique, reverse := hasFlag(options, "u"), hasFlag(options, "r")
	slices.SortStableFunc(lines, func(a, b keyedLine) int {
		order := compareKey(a, b)
		if order == 0 && !unique {
			order = strings.Compare(a.line, b.line)
		}
		if reverse {
			return -order
		}
		return order
	})
	for i, line := range lines {
		if !unique || i == 0 || compareKey(lines[i-1], line) != 0 {
			fmt.Fprintln(ec.Stdout, line.line)
		}
	}
	return 0
}

func uniq(ec *contract.ExecContext, args []string) int {
	options, paths, err := parseFlags(args, "cdui", nil, false)
	if err != nil {
		return diagnostic(ec, "uniq", err.Error(), 1)
	}
	if len(paths) > 2 {
		return diagnostic(ec, "uniq", fmt.Sprintf("extra operand '%s'", paths[2]), 1)
	}
	name := stdinDefault(paths)[0]
	data, err := readInput(ec, name)
	if err != nil {
		return diagnostic(ec, "uniq", fmt.Sprintf("cannot read '%s': %v", name, err), 1)
	}
	var buffer bytes.Buffer
	out := ec.Stdout
	if len(paths) == 2 {
		out = &buffer
	}
	var previous string
	count := 0
	counts, duplicates, unique, fold := hasFlag(options, "c"), hasFlag(options, "d"), hasFlag(options, "u"), hasFlag(options, "i")
	printGroup := func() {
		if count == 0 || (duplicates && count == 1) || (unique && count > 1) {
			return
		}
		if counts {
			fmt.Fprintf(out, "%7d ", count)
		}
		fmt.Fprintln(out, previous)
	}
	for line := range eachLine(string(data)) {
		if count != 0 && (line == previous || (fold && strings.EqualFold(line, previous))) {
			count++
			continue
		}
		printGroup()
		previous, count = line, 1
	}
	printGroup()
	if len(paths) == 2 {
		if err := ec.VFS.WriteFile(resolve(ec, paths[1]), buffer.Bytes(), 0644); err != nil {
			return diagnostic(ec, "uniq", fmt.Sprintf("cannot write '%s': %v", paths[1], err), 1)
		}
	}
	return 0
}

type cutRange struct{ first, last int }

func parseCutRanges(value string) ([]cutRange, error) {
	var ranges []cutRange
	for _, part := range strings.Split(value, ",") {
		first, last, ranged := strings.Cut(part, "-")
		start, end := 1, 0
		var err error
		if first != "" || !ranged || last == "" {
			start, err = parseCount(first, 1)
			if err != nil {
				return nil, fmt.Errorf("invalid field or character range: '%s'", part)
			}
		}
		if !ranged {
			end = start
		} else if last != "" {
			end, err = parseCount(last, start)
			if err != nil {
				return nil, fmt.Errorf("invalid field or character range: '%s'", part)
			}
		}
		ranges = append(ranges, cutRange{start, end})
	}
	return ranges, nil
}

func cut(ec *contract.ExecContext, args []string) int {
	options, paths, err := parseFlags(args, "d:f:c:", nil, false)
	if err != nil {
		return diagnostic(ec, "cut", err.Error(), 1)
	}
	delim := "\t"
	var ranges []cutRange
	mode := ""
	for _, opt := range options {
		if opt.name == "d" {
			if utf8.RuneCountInString(opt.value) != 1 {
				return diagnostic(ec, "cut", "the delimiter must be a single character", 1)
			}
			delim = opt.value
			continue
		}
		if mode != "" && mode != opt.name {
			return diagnostic(ec, "cut", "only one type of list may be specified", 1)
		}
		mode = opt.name
		ranges, err = parseCutRanges(opt.value)
		if err != nil {
			return diagnostic(ec, "cut", err.Error(), 1)
		}
	}
	if mode == "" {
		return diagnostic(ec, "cut", "you must specify a list of fields or characters", 1)
	}
	if mode == "c" && hasFlag(options, "d") {
		return diagnostic(ec, "cut", "an input delimiter may be specified only when operating on fields", 1)
	}
	selected := func(index int) bool {
		for _, r := range ranges {
			if r.first <= index && (r.last == 0 || index <= r.last) {
				return true
			}
		}
		return false
	}
	code := 0
	for _, name := range stdinDefault(paths) {
		data, err := readInput(ec, name)
		if err != nil {
			code = diagnostic(ec, "cut", fmt.Sprintf("%s: %v", name, err), 1)
			continue
		}
		for line := range eachLine(string(data)) {
			if mode == "c" {
				var result strings.Builder
				for i, char := range []rune(line) {
					if selected(i + 1) {
						result.WriteRune(char)
					}
				}
				fmt.Fprintln(ec.Stdout, result.String())
			} else if !strings.Contains(line, delim) {
				fmt.Fprintln(ec.Stdout, line)
			} else {
				var result []string
				for i, field := range strings.Split(line, delim) {
					if selected(i + 1) {
						result = append(result, field)
					}
				}
				fmt.Fprintln(ec.Stdout, strings.Join(result, delim))
			}
		}
	}
	return code
}

func echo(ec *contract.ExecContext, args []string) int {
	newline, expand := true, false
	for len(args) > 0 {
		flags, ok := strings.CutPrefix(args[0], "-")
		if !ok || flags == "" || strings.Trim(flags, "neE") != "" {
			break
		}
		for _, flag := range flags {
			switch flag {
			case 'n':
				newline = false
			case 'e':
				expand = true
			case 'E':
				expand = false
			}
		}
		args = args[1:]
	}
	line := strings.Join(args, " ")
	if expand {
		if writeEscaped(ec.Stdout, line) {
			return 0
		}
	} else {
		fmt.Fprint(ec.Stdout, line)
	}
	if newline {
		fmt.Fprintln(ec.Stdout)
	}
	return 0
}

func writeEscaped(out io.Writer, s string) (stopped bool) {
	for len(s) > 0 {
		plain, rest, found := strings.Cut(s, "\\")
		fmt.Fprint(out, plain)
		if !found {
			return false
		}
		expanded, consumed, stop := escape(rest)
		fmt.Fprint(out, expanded)
		if stop {
			return true
		}
		s = rest[consumed:]
	}
	return false
}

// escape decodes the text immediately after a backslash.
func escape(s string) (string, int, bool) {
	if s == "" {
		return "\\", 0, false
	}
	const codes, values = "abefnrtv\\", "\a\b\x1b\f\n\r\t\v\\"
	if i := strings.IndexByte(codes, s[0]); i >= 0 {
		return values[i : i+1], 1, false
	}
	switch s[0] {
	case 'c':
		return "", 1, true
	case 'x':
		for size := min(2, len(s)-1); size > 0; size-- {
			if n, err := strconv.ParseUint(s[1:1+size], 16, 8); err == nil {
				return string([]byte{byte(n)}), 1 + size, false
			}
		}
	}
	return "\\" + s[:1], 1, false
}

var conversion = regexp.MustCompile("^%[-+ #0]*[0-9]*(\\.[0-9]*)?[sdiufxXob]")

func printf(ec *contract.ExecContext, args []string) int {
	if len(args) > 0 && args[0] == "--" {
		args = args[1:]
	}
	if len(args) == 0 {
		return diagnostic(ec, "printf", "missing format operand", 1)
	}

	format, values := args[0], args[1:]
	code := 0
	for {
		consumed := 0
		for rest := format; rest != ""; {
			switch {
			case rest[0] == '\\':
				value, n, stop := escape(rest[1:])
				fmt.Fprint(ec.Stdout, value)
				if stop {
					return code
				}
				rest = rest[1+n:]
			case strings.HasPrefix(rest, "%%"):
				fmt.Fprint(ec.Stdout, "%")
				rest = rest[2:]
			case rest[0] == '%':
				spec := conversion.FindString(rest)
				if spec == "" {
					return diagnostic(ec, "printf", fmt.Sprintf("invalid format specification: '%s'", rest), 1)
				}
				value := ""
				if consumed < len(values) {
					value = values[consumed]
				}
				consumed++
				if spec[len(spec)-1] == 'b' {
					if writeEscaped(ec.Stdout, value) {
						return code
					}
				} else if err := printValue(ec.Stdout, spec, value); err != nil {
					code = diagnostic(ec, "printf", fmt.Sprintf("%s: invalid number", value), 1)
				}
				rest = rest[len(spec):]
			default:
				n := strings.IndexAny(rest, "\\%")
				if n < 0 {
					n = len(rest)
				}
				fmt.Fprint(ec.Stdout, rest[:n])
				rest = rest[n:]
			}
		}
		if consumed == 0 || consumed >= len(values) {
			return code
		}
		values = values[consumed:]
	}
}

func printValue(out io.Writer, spec, value string) error {
	verb := spec[len(spec)-1]
	if spec[len(spec)-2] == '.' {
		spec = spec[:len(spec)-1] + "0" + string(verb)
	}
	if verb == 's' {
		fmt.Fprintf(out, spec, value)
		return nil
	}
	if value == "" {
		value = "0"
	}
	switch verb {
	case 'f':
		n, err := strconv.ParseFloat(value, 64)
		fmt.Fprintf(out, spec, n)
		return err
	case 'd', 'i':
		n, err := strconv.ParseInt(value, 0, 64)
		fmt.Fprintf(out, spec[:len(spec)-1]+"d", n)
		return err
	default:
		n, err := strconv.ParseUint(value, 0, 64)
		if verb == 'u' {
			spec = spec[:len(spec)-1] + "d"
		}
		fmt.Fprintf(out, spec, n)
		return err
	}
}

func modeString(info *contract.NodeInfo) string {
	mode := []byte("----------")
	switch info.Type {
	case contract.TypeDir:
		mode[0] = 'd'
	case contract.TypeSymlink:
		mode[0] = 'l'
	}
	for i, ch := range []byte("rwxrwxrwx") {
		if info.Mode&(1<<(8-i)) != 0 {
			mode[i+1] = ch
		}
	}
	for i, bit := range []uint32{04000, 02000, 01000} {
		if uint32(info.Mode)&bit == 0 {
			continue
		}
		index := (i + 1) * 3
		if mode[index] == 'x' {
			mode[index] = "sst"[i]
		} else {
			mode[index] = "SST"[i]
		}
	}
	return string(mode)
}

func stat(ec *contract.ExecContext, args []string) int {
	return inspect(ec, args, "stat", func(name, _ string, info *contract.NodeInfo) error {
		kind, label := "regular file", name
		switch info.Type {
		case contract.TypeDir:
			kind = "directory"
		case contract.TypeSymlink:
			kind = "symbolic link"
			label = fmt.Sprintf("%s -> %s", name, info.LinkTarget)
		}
		fmt.Fprintf(ec.Stdout, "  File: %s\n  Size: %d\tType: %s\nAccess: (%04o/%s)  Uid: (%s)  Gid: (%s)\nModify: %s\n",
			label, info.Size, kind, info.Mode&07777, modeString(info), info.Owner, info.Group,
			info.ModTime.Format("2006-01-02 15:04:05.000000000 -0700"))
		return nil
	})
}

func file(ec *contract.ExecContext, args []string) int {
	return inspect(ec, args, "file", func(name, p string, info *contract.NodeInfo) error {
		kind := ""
		switch {
		case info.IsDir():
			kind = "directory"
		case info.Type == contract.TypeSymlink:
			kind = "symbolic link to " + info.LinkTarget
		case info.IsVirtualBinary:
			kind = "ELF 64-bit LSB executable (virtual)"
		default:
			data, err := ec.VFS.ReadFile(p)
			if err != nil {
				return err
			}
			kind = classifyFile(data)
		}
		fmt.Fprintf(ec.Stdout, "%s: %s\n", name, kind)
		return nil
	})
}

func classifyFile(data []byte) string {
	switch {
	case len(data) == 0:
		return "empty"
	case bytes.HasPrefix(data, []byte("\x7fELF")):
		return "ELF 64-bit LSB executable (virtual)"
	case bytes.HasPrefix(data, []byte("#!")):
		return "Bourne-Again shell script, ASCII text executable"
	case bytes.HasPrefix(data, []byte("\x1f\x8b")):
		return "gzip compressed data"
	case !utf8.Valid(data) || bytes.IndexByte(data, 0) >= 0:
		return "data"
	default:
		for _, ch := range data {
			if ch >= utf8.RuneSelf {
				return "UTF-8 Unicode text"
			}
		}
		return "ASCII text"
	}
}

func inspect(ec *contract.ExecContext, args []string, command string, printInfo func(string, string, *contract.NodeInfo) error) int {
	_, paths, err := parseFlags(args, "", nil, false)
	if err != nil {
		return diagnostic(ec, command, err.Error(), 1)
	}
	if len(paths) == 0 {
		return diagnostic(ec, command, "missing operand", 1)
	}
	code := 0
	for _, name := range paths {
		p := resolve(ec, name)
		info, err := ec.VFS.Lstat(p)
		if err == nil {
			err = printInfo(name, p, info)
		}
		if err != nil {
			code = diagnostic(ec, command, fmt.Sprintf("cannot access '%s': %v", name, err), 1)
		}
	}
	return code
}
