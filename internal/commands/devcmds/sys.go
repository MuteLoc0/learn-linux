package devcmds

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"learn-linux/internal/contract"
	"learn-linux/internal/vfs"
	"mvdan.cc/sh/v3/interp"
)

func which(reg contract.CommandRegistry) contract.CommandHandler {
	return func(ec *contract.ExecContext, args []string) int {
		if len(args) == 0 {
			return commandError(ec, "which", "missing operand")
		}
		status := 0
		for _, name := range args {
			_, registered := reg.Lookup(name)
			switch {
			case name == "sh" || name == "bash":
				fmt.Fprintln(ec.Stdout, "/bin/"+name)
			case registered:
				fmt.Fprintln(ec.Stdout, "/usr/bin/"+name)
			case interp.IsImplementedBuiltin(name):
				fmt.Fprintf(ec.Stdout, "%s: shell built-in command\n", name)
			default:
				if searchPath, ok := ec.Env["PATH"]; ok {
					if filename := vfs.LookPath(ec.VFS, ec.CWD, searchPath, name, true); filename != "" {
						fmt.Fprintln(ec.Stdout, filename)
						continue
					}
				}
				status = 1
			}
		}
		return status
	}
}

func username(ec *contract.ExecContext) string {
	if ec.User == "" {
		return "learner"
	}
	return ec.User
}

func whoami(ec *contract.ExecContext, _ []string) int {
	fmt.Fprintln(ec.Stdout, username(ec))
	return 0
}

func hostname(ec *contract.ExecContext, _ []string) int {
	host := ec.Host
	if data, err := ec.VFS.ReadFile("/etc/hostname"); err == nil {
		host = strings.TrimRight(string(data), "\r\n")
	}
	fmt.Fprintln(ec.Stdout, host)
	return 0
}

func uname(ec *contract.ExecContext, args []string) int {
	selected := make(map[rune]bool)
	for _, arg := range args {
		if len(arg) < 2 || arg[0] != '-' {
			return commandError(ec, "uname", "extra operand '%s'", arg)
		}
		for _, flag := range arg[1:] {
			if !strings.ContainsRune("asnrmo", flag) {
				return commandError(ec, "uname", "invalid option -- '%c'", flag)
			}
			selected[flag] = true
		}
	}
	if len(selected) == 0 {
		selected['s'] = true
	}
	fields := []struct {
		flag  rune
		value string
	}{
		{'s', "Linux"}, {'n', "linux-virtual"}, {'r', "6.8.0-virtual"},
		{'m', "x86_64"}, {'o', "GNU/Linux"},
	}
	var values []string
	for _, field := range fields {
		if selected[field.flag] || selected['a'] {
			values = append(values, field.value)
		}
	}
	fmt.Fprintln(ec.Stdout, strings.Join(values, " "))
	return 0
}

func date(ec *contract.ExecContext, args []string) int {
	now := time.Now()
	var format string
	var custom bool
	for _, arg := range args {
		switch {
		case arg == "-u":
			now = now.UTC()
		case strings.HasPrefix(arg, "+") && !custom:
			format, custom = arg[1:], true
		default:
			return commandError(ec, "date", "invalid argument '%s'", arg)
		}
	}
	if !custom {
		fmt.Fprintln(ec.Stdout, now.Format("Mon Jan _2 15:04:05 MST 2006"))
		return 0
	}
	output := strings.NewReplacer(
		"%%", "%", "%Y", now.Format("2006"), "%m", now.Format("01"), "%d", now.Format("02"),
		"%H", now.Format("15"), "%M", now.Format("04"), "%S", now.Format("05"),
		"%s", strconv.FormatInt(now.Unix(), 10), "%F", now.Format("2006-01-02"),
		"%T", now.Format("15:04:05"), "%Z", now.Format("MST"), "%z", now.Format("-0700"),
		"%n", "\n", "%t", "\t",
	).Replace(format)
	fmt.Fprintln(ec.Stdout, output)
	return 0
}

func id(ec *contract.ExecContext, _ []string) int {
	user := username(ec)
	fmt.Fprintf(ec.Stdout, "uid=1000(%[1]s) gid=1000(%[1]s) groups=1000(%[1]s),27(sudo)\n", user)
	return 0
}

func clear(ec *contract.ExecContext, _ []string) int {
	if ec.Term != nil {
		if err := ec.Term.ClearScreen(); err != nil {
			return commandError(ec, "clear", "%v", err)
		}
	} else {
		fmt.Fprint(ec.Stdout, "\x1b[2J\x1b[H")
	}
	return 0
}

func history(ec *contract.ExecContext, args []string) int {
	start := 0
	if len(args) > 1 {
		return commandError(ec, "history", "too many arguments")
	}
	if len(args) == 1 {
		n, err := strconv.Atoi(args[0])
		if err != nil || n < 0 {
			return commandError(ec, "history", "%s: numeric argument required", args[0])
		}
		start = max(0, len(ec.History)-n)
	}
	for i := start; i < len(ec.History); i++ {
		fmt.Fprintf(ec.Stdout, "%3d  %s\n", i+1, ec.History[i])
	}
	return 0
}

func env(ec *contract.ExecContext, _ []string) int {
	keys := make([]string, 0, len(ec.Env))
	for key := range ec.Env {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		fmt.Fprintf(ec.Stdout, "%s=%s\n", key, ec.Env[key])
	}
	return 0
}
