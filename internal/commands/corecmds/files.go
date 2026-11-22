package corecmds

import (
	"errors"
	"fmt"
	"path"
	"slices"
	"strings"
	"time"

	"learn-linux/internal/contract"
)

func mkdir(ec *contract.ExecContext, args []string) int {
	options, paths, err := parseFlags(args, "p", nil, false)
	if err != nil {
		return diagnostic(ec, "mkdir", err.Error(), 1)
	}
	if len(paths) == 0 {
		return diagnostic(ec, "mkdir", "missing operand", 1)
	}
	create := ec.VFS.Mkdir
	if hasFlag(options, "p") {
		create = ec.VFS.MkdirAll
	}
	code := 0
	for _, name := range paths {
		if err := create(resolve(ec, name), 0755); err != nil {
			code = diagnostic(ec, "mkdir", fmt.Sprintf("cannot create directory '%s': %v", name, err), 1)
		}
	}
	return code
}

func touch(ec *contract.ExecContext, args []string) int {
	_, paths, err := parseFlags(args, "", nil, false)
	if err != nil {
		return diagnostic(ec, "touch", err.Error(), 1)
	}
	if len(paths) == 0 {
		return diagnostic(ec, "touch", "missing file operand", 1)
	}
	code := 0
	now := time.Now()
	for _, name := range paths {
		if err := ec.VFS.Touch(resolve(ec, name), now); err != nil {
			code = diagnostic(ec, "touch", fmt.Sprintf("cannot touch '%s': %v", name, err), 1)
		}
	}
	return code
}

func cp(ec *contract.ExecContext, args []string) int {
	return transfer(ec, args, "cp", "rRfv")
}

func mv(ec *contract.ExecContext, args []string) int {
	return transfer(ec, args, "mv", "fv")
}

func transfer(ec *contract.ExecContext, args []string, command, flags string) int {
	options, paths, err := parseFlags(args, flags, nil, false)
	if err != nil {
		return diagnostic(ec, command, err.Error(), 1)
	}
	switch len(paths) {
	case 0:
		return diagnostic(ec, command, "missing file operand", 1)
	case 1:
		return diagnostic(ec, command, fmt.Sprintf("missing destination file operand after '%s'", paths[0]), 1)
	}
	target := paths[len(paths)-1]
	targetPath := resolve(ec, target)
	targetInfo, targetErr := ec.VFS.Stat(targetPath)
	targetDir := targetErr == nil && targetInfo.IsDir()
	if !targetDir && (len(paths) > 2 || strings.HasSuffix(target, "/")) {
		if targetErr == nil {
			targetErr = contract.ErrNotDir
		} else if strings.HasSuffix(target, "/") {
			targetErr = contract.ErrNotDir
		}
		return diagnostic(ec, command, fmt.Sprintf("target '%s': %v", target, targetErr), 1)
	}

	recursive := hasFlag(options, "r") || hasFlag(options, "R")
	force, verbose := hasFlag(options, "f"), hasFlag(options, "v")
	code := 0
	for _, source := range paths[:len(paths)-1] {
		src := resolve(ec, source)
		dst, destName := targetPath, target
		if targetDir {
			dst = path.Join(dst, path.Base(src))
			destName = appendPath(destName, path.Base(src))
		}
		if command == "cp" {
			info, statErr := ec.VFS.Lstat(src)
			switch {
			case statErr != nil:
				code = diagnostic(ec, command, fmt.Sprintf("cannot stat '%s': %v", source, statErr), 1)
				continue
			case info.IsDir() && !recursive:
				code = diagnostic(ec, command, fmt.Sprintf("-r not specified; omitting directory '%s'", source), 1)
				continue
			}
			err = copyEntry(ec.VFS, src, dst, info, recursive, force)
		} else {
			// The VFS rename replaces existing files without prompting.
			err = ec.VFS.Rename(src, dst)
		}
		if err != nil {
			action := "copy"
			if command == "mv" {
				action = "move"
			}
			code = diagnostic(ec, command, fmt.Sprintf("cannot %s '%s' to '%s': %v", action, source, destName, err), 1)
		} else if verbose {
			if command == "mv" {
				fmt.Fprint(ec.Stdout, "renamed ")
			}
			fmt.Fprintf(ec.Stdout, "'%s' -> '%s'\n", source, destName)
		}
	}
	return code
}

func copyEntry(v contract.VFS, src, dst string, info *contract.NodeInfo, recursive, force bool) error {
	target, targetErr := v.Lstat(dst)
	if info.IsDir() && targetErr == nil && target.IsDir() {
		if target.Path == info.Path || strings.HasPrefix(target.Path, strings.TrimSuffix(info.Path, "/")+"/") {
			return contract.ErrInvalidPath
		}
		children, err := v.ReadDir(src)
		if err != nil {
			return err
		}
		for _, child := range children {
			if err := copyEntry(v, path.Join(src, child.Name), path.Join(dst, child.Name), child, recursive, force); err != nil {
				return err
			}
		}
		return nil
	}
	err := v.Copy(src, dst, recursive)
	if force && errors.Is(err, contract.ErrPermission) && targetErr == nil &&
		!target.IsDir() && target.Path != info.Path {
		if removeErr := v.Remove(dst); removeErr != nil {
			return removeErr
		}
		return v.Copy(src, dst, recursive)
	}
	return err
}

func rm(ec *contract.ExecContext, args []string) int {
	options, paths, err := parseFlags(args, "rRfiv", map[string]flagValue{"no-preserve-root": noValue}, false)
	if err != nil {
		return diagnostic(ec, "rm", err.Error(), 1)
	}
	recursive := hasFlag(options, "r") || hasFlag(options, "R")
	force := hasFlag(options, "f")
	if len(paths) == 0 {
		if force {
			return 0
		}
		return diagnostic(ec, "rm", "missing operand", 1)
	}

	resolved := make([]string, len(paths))
	destructive := false
	for i, raw := range paths {
		p := resolve(ec, raw)
		resolved[i] = p
		systemDir := p == "/bin" || p == "/etc" || p == "/usr"
		if p == "/" || p == "/*" || (force && systemDir) {
			destructive = true
		}
	}
	// The shell can expand /* before the handler sees the arguments.
	allSystemDirs := slices.Contains(resolved, "/bin") && slices.Contains(resolved, "/etc") && slices.Contains(resolved, "/usr")
	if recursive && (destructive || allSystemDirs) {
		if ec.OnDestructive != nil {
			ec.OnDestructive("rm " + strings.Join(args, " "))
			return 0
		}
		return diagnostic(ec, "rm", "it is dangerous to operate recursively on '/'", 1)
	}

	code := 0
	for i, p := range resolved {
		name := paths[i]
		info, err := ec.VFS.Lstat(p)
		switch {
		case force && errors.Is(err, contract.ErrNotExist):
			continue
		case err != nil:
		case info.IsDir() && !recursive:
			err = errors.New("Is a directory")
		case info.IsDir():
			err = ec.VFS.RemoveAll(p)
		default:
			err = ec.VFS.Remove(p)
		}
		if force && errors.Is(err, contract.ErrNotExist) {
			continue
		}
		if err != nil {
			code = diagnostic(ec, "rm", fmt.Sprintf("cannot remove '%s': %v", name, err), 1)
		} else if hasFlag(options, "v") {
			if info.IsDir() {
				fmt.Fprintf(ec.Stdout, "removed directory '%s'\n", name)
			} else {
				fmt.Fprintf(ec.Stdout, "removed '%s'\n", name)
			}
		}
	}
	return code
}

func ln(ec *contract.ExecContext, args []string) int {
	options, paths, err := parseFlags(args, "sf", nil, false)
	if err != nil {
		return diagnostic(ec, "ln", err.Error(), 1)
	}
	if len(paths) == 0 {
		return diagnostic(ec, "ln", "missing file operand", 1)
	}
	if len(paths) > 2 {
		return diagnostic(ec, "ln", fmt.Sprintf("extra operand '%s'", paths[2]), 1)
	}

	target := paths[0]
	name := path.Base(target)
	if len(paths) == 2 {
		name = paths[1]
		if ec.VFS.IsDir(resolve(ec, name)) {
			name = appendPath(name, path.Base(target))
		}
	}
	dst := resolve(ec, name)
	symbolic, force := hasFlag(options, "s"), hasFlag(options, "f")
	if symbolic {
		dstInfo, statErr := ec.VFS.Lstat(dst)
		if statErr == nil && force && !dstInfo.IsDir() {
			targetPath := ec.VFS.ResolvePath(path.Dir(dstInfo.Path), ec.Home, target)
			if targetPath == dstInfo.Path {
				err = fmt.Errorf("'%s' and '%s' are the same file", target, name)
			} else {
				err = ec.VFS.Remove(dst)
			}
		}
		if err == nil {
			err = ec.VFS.Symlink(target, dst)
		}
	} else {
		src := resolve(ec, target)
		info, statErr := ec.VFS.Lstat(src)
		switch {
		case statErr != nil:
			err = statErr
		case info.IsDir():
			err = fmt.Errorf("hard link not allowed for directory")
		default:
			if _, destErr := ec.VFS.Lstat(dst); destErr == nil && !force {
				err = contract.ErrExist
			} else {
				err = ec.VFS.Copy(src, dst, false)
			}
		}
	}
	if err != nil {
		kind := "hard"
		if symbolic {
			kind = "symbolic"
		}
		return diagnostic(ec, "ln", fmt.Sprintf("failed to create %s link '%s': %v", kind, name, err), 1)
	}
	return 0
}
