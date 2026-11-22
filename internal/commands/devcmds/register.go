package devcmds

import (
	"fmt"

	"learn-linux/internal/contract"
)

// Register adds the development, system, tutorial, and interactive commands.
func Register(reg contract.CommandRegistry) {
	for name, handler := range map[string]contract.CommandHandler{
		"chmod": chmod, "tar": tarCommand, "gzip": gzipCommand("gzip", false),
		"gunzip": gzipCommand("gunzip", true), "gcc": gcc, "make": makeCommand,
		"which": which(reg), "whoami": whoami, "hostname": hostname, "uname": uname,
		"date": date, "id": id, "clear": clear, "history": history, "env": env,
		"true":  func(*contract.ExecContext, []string) int { return 0 },
		"false": func(*contract.ExecContext, []string) int { return 1 },
		"less":  less, "nano": nano,
	} {
		reg.Register(name, handler)
	}
	for _, name := range []string{"mission", "hint", "check", "chapter", "reset", "sandbox", "man", "lang", "help", "exit"} {
		reg.Register(name, tutorial(name))
	}
}

func commandError(ec *contract.ExecContext, name, format string, args ...any) int {
	fmt.Fprintf(ec.Stderr, "%s: %s\n", name, fmt.Sprintf(format, args...))
	return 1
}

func resolve(ec *contract.ExecContext, name string) string {
	return ec.VFS.ResolvePath(ec.CWD, ec.Home, name)
}
