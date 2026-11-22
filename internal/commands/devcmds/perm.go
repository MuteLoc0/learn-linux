package devcmds

import (
	"fmt"
	"io/fs"
	"strconv"
	"strings"

	"learn-linux/internal/contract"
)

type modeChange struct {
	who  fs.FileMode
	bits fs.FileMode
	op   byte
}

func chmod(ec *contract.ExecContext, args []string) int {
	var recursive, verbose bool
options:
	for len(args) > 0 {
		switch args[0] {
		case "-R":
			recursive = true
		case "-v":
			verbose = true
		case "-Rv", "-vR":
			recursive, verbose = true, true
		case "--":
			args = args[1:]
			break options
		default:
			break options
		}
		args = args[1:]
	}
	switch len(args) {
	case 0:
		return commandError(ec, "chmod", "missing operand")
	case 1:
		return commandError(ec, "chmod", "missing operand after '%s'", args[0])
	}
	changes, ok := parseMode(args[0])
	if !ok {
		return commandError(ec, "chmod", "invalid mode: '%s'", args[0])
	}

	status := 0
	for _, name := range args[1:] {
		root := resolve(ec, name)
		apply := func(p string, info *contract.NodeInfo) error {
			mode := info.Mode & 07777
			for _, change := range changes {
				switch change.op {
				case '+':
					mode |= change.bits
				case '-':
					mode &^= change.bits
				case '=':
					mode = mode&^change.who | change.bits
				}
			}
			if err := ec.VFS.Chmod(p, mode); err != nil {
				status = commandError(ec, "chmod", "changing permissions of '%s': %v", p, err)
			} else if verbose {
				fmt.Fprintf(ec.Stdout, "mode of '%s' set to %04o\n", p, mode)
			}
			return nil
		}
		info, err := ec.VFS.Stat(root)
		if err == nil {
			if recursive && info.IsDir() {
				err = ec.VFS.Walk(info.Path, func(p string, child *contract.NodeInfo) error {
					if child.Type == contract.TypeSymlink {
						return nil
					}
					return apply(p, child)
				})
			} else {
				err = apply(root, info)
			}
		}
		if err != nil {
			status = commandError(ec, "chmod", "cannot access '%s': %v", name, err)
		}
	}
	return status
}

func parseMode(mode string) ([]modeChange, bool) {
	if mode != "" && strings.Trim(mode, "01234567") == "" {
		value, err := strconv.ParseUint(mode, 8, 12)
		return []modeChange{{who: 07777, bits: fs.FileMode(value), op: '='}}, err == nil
	}
	var changes []modeChange
	for _, clause := range strings.Split(mode, ",") {
		i := strings.IndexAny(clause, "+-=")
		if i < 0 {
			return nil, false
		}
		var who, bits fs.FileMode
		for _, ch := range clause[:i] {
			switch ch {
			case 'u':
				who |= 0700
			case 'g':
				who |= 0070
			case 'o':
				who |= 0007
			case 'a':
				who |= 0777
			default:
				return nil, false
			}
		}
		if who == 0 {
			who = 0777
		}
		for _, ch := range clause[i+1:] {
			switch ch {
			case 'r':
				bits |= 0444
			case 'w':
				bits |= 0222
			case 'x':
				bits |= 0111
			default:
				return nil, false
			}
		}
		changes = append(changes, modeChange{who, who & bits, clause[i]})
	}
	return changes, true
}
