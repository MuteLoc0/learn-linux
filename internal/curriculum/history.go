package curriculum

import (
	"path"
	"slices"
	"strings"

	"learn-linux/internal/contract"
	"mvdan.cc/sh/v3/expand"
	"mvdan.cc/sh/v3/syntax"
)

type commandHistory struct {
	records []contract.CommandRecord
	last    contract.CommandRecord
}

func (h commandHistory) any(match func(contract.CommandRecord) bool) bool {
	if match(h.last) {
		return true
	}
	for i := len(h.records) - 1; i >= 0; i-- {
		if match(h.records[i]) {
			return true
		}
	}
	return false
}

func (h commandHistory) successful(match func(contract.CommandRecord) bool) bool {
	return h.any(func(record contract.CommandRecord) bool {
		return record.ExitCode == 0 && match(record)
	})
}

func (h commandHistory) ran(name string, checkArgs func([]string) bool) bool {
	return h.successful(func(record contract.CommandRecord) bool {
		return matchesCommand(record, name, checkArgs)
	})
}

func (h commandHistory) produced(output string) bool {
	return h.successful(func(record contract.CommandRecord) bool {
		return strings.Contains(record.Stdout, output)
	})
}

func matchesCommand(record contract.CommandRecord, name string, checkArgs func([]string) bool) bool {
	if path.Base(record.Command) == name && (checkArgs == nil || checkArgs(record.Args)) {
		return true
	}
	return rawCommand(record.Raw, name, checkArgs)
}

func rawCommand(raw, name string, checkArgs func([]string) bool) bool {
	if !strings.Contains(raw, name) {
		return false
	}
	cfg := &expand.Config{VirtualFS: true}
	return matchSyntax(raw, func(node syntax.Node) bool {
		call, ok := node.(*syntax.CallExpr)
		if !ok || len(call.Args) == 0 {
			return false
		}
		command, err := expand.Literal(cfg, call.Args[0])
		if err != nil || path.Base(command) != name {
			return false
		}
		if checkArgs == nil {
			return true
		}
		args := make([]string, 0, len(call.Args)-1)
		for _, word := range call.Args[1:] {
			arg, _ := expand.Literal(cfg, word)
			args = append(args, arg)
		}
		return checkArgs(args)
	})
}

func hasAndOperator(raw string) bool {
	if !strings.Contains(raw, "&&") {
		return false
	}
	return matchSyntax(raw, func(node syntax.Node) bool {
		binary, ok := node.(*syntax.BinaryCmd)
		return ok && binary.Op == syntax.AndStmt
	})
}

func matchSyntax(raw string, match func(syntax.Node) bool) bool {
	tree, err := syntax.NewParser().Parse(strings.NewReader(raw), "")
	if err != nil {
		return false
	}
	found := false
	syntax.Walk(tree, func(node syntax.Node) bool {
		if !found && node != nil {
			found = match(node)
		}
		return !found
	})
	return found
}

func hasOption(args []string, short string, long ...string) bool {
	for _, arg := range args {
		if arg == "--" {
			return false
		}
		if option, ok := strings.CutPrefix(arg, "--"); ok {
			if slices.Contains(long, option) {
				return true
			}
		} else if option, ok := strings.CutPrefix(arg, "-"); ok && strings.ContainsAny(option, short) {
			return true
		}
	}
	return false
}

func recursiveErrorSearch(args []string) bool {
	var recursive, hasPattern, matchesError bool
	for len(args) > 0 {
		arg := args[0]
		args = args[1:]
		if arg == "--" {
			break
		}
		if option, long := strings.CutPrefix(arg, "--"); long {
			name, value, attached := strings.Cut(option, "=")
			switch name {
			case "recursive", "dereference-recursive":
				recursive = true
			case "regexp":
				if !attached && len(args) > 0 {
					value, args = args[0], args[1:]
				}
				hasPattern = true
				matchesError = matchesError || strings.Contains(value, "ERROR")
			}
			continue
		}
		if flags, short := strings.CutPrefix(arg, "-"); short && flags != "" {
			options, pattern, explicit := strings.Cut(flags, "e")
			recursive = recursive || strings.ContainsAny(options, "rR")
			if explicit {
				if pattern == "" && len(args) > 0 {
					pattern, args = args[0], args[1:]
				}
				hasPattern = true
				matchesError = matchesError || strings.Contains(pattern, "ERROR")
			}
			continue
		}
		if !hasPattern {
			hasPattern = true
			matchesError = strings.Contains(arg, "ERROR")
		}
	}
	if !hasPattern && len(args) > 0 {
		matchesError = strings.Contains(args[0], "ERROR")
	}
	return recursive && matchesError
}
