package corecmds

import (
	"fmt"
	"strconv"
	"strings"
)

type flagValue int

const (
	noValue flagValue = iota
	requiredValue
	optionalValue
)

type option struct {
	name  string
	value string
}

// parseFlags keeps options in command-line order so competing options can use
// the last occurrence. A colon after a short option marks a required value.
func parseFlags(args []string, short string, long map[string]flagValue, numeric bool) ([]option, []string, error) {
	var options []option
	var operands []string
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if arg == "--" {
			operands = append(operands, args[i+1:]...)
			break
		}
		if name, isLong := strings.CutPrefix(arg, "--"); isLong {
			name, value, attached := strings.Cut(name, "=")
			kind, ok := long[name]
			switch {
			case !ok:
				return nil, nil, fmt.Errorf("unrecognized option '%s'", arg)
			case kind == noValue && attached:
				return nil, nil, fmt.Errorf("option '--%s' doesn't allow an argument", name)
			case kind == requiredValue && !attached:
				if i+1 == len(args) {
					return nil, nil, fmt.Errorf("option '--%s' requires an argument", name)
				}
				i++
				value = args[i]
			}
			options = append(options, option{name, value})
			continue
		}
		if len(arg) < 2 || arg[0] != '-' {
			operands = append(operands, arg)
			continue
		}
		if numeric && arg[1] >= '0' && arg[1] <= '9' {
			options = append(options, option{"n", arg[1:]})
			continue
		}
		for j := 1; j < len(arg); j++ {
			ch := arg[j]
			index := strings.IndexByte(short, ch)
			if index < 0 || ch == ':' {
				return nil, nil, fmt.Errorf("invalid option -- '%c'", ch)
			}
			opt := option{name: string(ch)}
			if index+1 < len(short) && short[index+1] == ':' {
				if j+1 < len(arg) {
					opt.value = arg[j+1:]
				} else {
					if i+1 == len(args) {
						return nil, nil, fmt.Errorf("option requires an argument -- '%c'", ch)
					}
					i++
					opt.value = args[i]
				}
				options = append(options, opt)
				break
			}
			options = append(options, opt)
		}
	}
	return options, operands, nil
}

func hasFlag(options []option, name string) bool {
	for _, opt := range options {
		if opt.name == name {
			return true
		}
	}
	return false
}

func colorEnabled(options []option, isTTY bool) (bool, error) {
	color := isTTY
	for _, opt := range options {
		if opt.name != "color" {
			continue
		}
		switch opt.value {
		case "", "always":
			color = true
		case "auto":
			color = isTTY
		case "never":
			color = false
		default:
			return false, fmt.Errorf("invalid argument '%s' for '--color'", opt.value)
		}
	}
	return color, nil
}

func parseCount(value string, minimum int) (int, error) {
	n, err := strconv.Atoi(value)
	if err != nil || n < minimum {
		return 0, fmt.Errorf("invalid count: '%s'", value)
	}
	return n, nil
}
