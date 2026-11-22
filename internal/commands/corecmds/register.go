package corecmds

import (
	"fmt"
	"io"

	"learn-linux/internal/contract"
)

// Register installs the core commands in the virtual shell.
func Register(reg contract.CommandRegistry) {
	for _, cmd := range []struct {
		name string
		run  contract.CommandHandler
	}{
		{"pwd", pwd}, {"ls", ls}, {"tree", tree},
		{"mkdir", mkdir}, {"touch", touch}, {"cp", cp}, {"mv", mv}, {"rm", rm}, {"ln", ln},
		{"cat", cat}, {"head", head}, {"tail", tail}, {"wc", wc}, {"sort", sortLines},
		{"uniq", uniq}, {"cut", cut}, {"echo", echo}, {"printf", printf}, {"stat", stat}, {"file", file},
		{"grep", grep}, {"find", find},
	} {
		reg.Register(cmd.name, cmd.run)
	}
}

func diagnostic(ec *contract.ExecContext, command, message string, code int) int {
	if ec.UI != nil {
		fmt.Fprintln(ec.Stderr, ec.UI.RenderError(command, message))
	} else {
		fmt.Fprintf(ec.Stderr, "%s: %s\n", command, message)
	}
	return code
}

func resolve(ec *contract.ExecContext, arg string) string {
	return ec.VFS.ResolvePath(ec.CWD, ec.Home, arg)
}

func readInput(ec *contract.ExecContext, name string) ([]byte, error) {
	if name != "-" {
		return ec.VFS.ReadFile(resolve(ec, name))
	}
	if ec.Stdin == nil {
		return nil, nil
	}
	return io.ReadAll(ec.Stdin)
}

func stdinDefault(paths []string) []string {
	if len(paths) == 0 {
		return []string{"-"}
	}
	return paths
}
