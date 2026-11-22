package corecmds

import (
	"cmp"
	"fmt"
	"io"
	"path"
	"slices"
	"strconv"
	"strings"

	"learn-linux/internal/contract"
)

func pwd(ec *contract.ExecContext, _ []string) int {
	fmt.Fprintln(ec.Stdout, ec.CWD)
	return 0
}

type listing struct {
	options []option
	color   bool
	sortBy  string
	code    int
	printed bool
}

func ls(ec *contract.ExecContext, args []string) int {
	options, paths, err := parseFlags(args, "alhRtSr1", map[string]flagValue{"color": optionalValue, "all": noValue}, false)
	if err != nil {
		return diagnostic(ec, "ls", err.Error(), 2)
	}
	color, err := colorEnabled(options, ec.IsTTY)
	if err != nil {
		return diagnostic(ec, "ls", err.Error(), 2)
	}
	if len(paths) == 0 {
		paths = []string{"."}
	}
	l := listing{options: options, color: color}
	for _, opt := range options {
		if opt.name == "t" || opt.name == "S" {
			l.sortBy = opt.name
		}
	}

	var files, dirs []contract.LSItem
	for _, name := range paths {
		p := resolve(ec, name)
		info, err := ec.VFS.Lstat(p)
		if err == nil && info.Type == contract.TypeSymlink &&
			(!hasFlag(options, "l") || strings.HasSuffix(name, "/")) {
			if target, targetErr := ec.VFS.Stat(p); targetErr == nil && target.IsDir() {
				info = target
			}
		}
		if err != nil {
			l.code = diagnostic(ec, "ls", fmt.Sprintf("cannot access '%s': %v", name, err), 1)
			continue
		}
		item := contract.LSItem{Info: info, DisplayName: name, IsExecutable: info.IsExecutable()}
		if info.IsDir() {
			dirs = append(dirs, item)
		} else {
			files = append(files, item)
		}
	}

	l.sort(files)
	l.render(ec, files, false)
	l.sort(dirs)
	header := len(paths) > 1 || hasFlag(options, "R")
	for _, dir := range dirs {
		l.directory(ec, resolve(ec, dir.DisplayName), dir.DisplayName, header)
	}
	return l.code
}

func (l *listing) sort(items []contract.LSItem) {
	slices.SortStableFunc(items, func(a, b contract.LSItem) int {
		var order int
		switch l.sortBy {
		case "t":
			order = b.Info.ModTime.Compare(a.Info.ModTime)
		case "S":
			order = cmp.Compare(b.Info.Size, a.Info.Size)
		}
		if order == 0 {
			order = strings.Compare(a.DisplayName, b.DisplayName)
		}
		if hasFlag(l.options, "r") {
			return -order
		}
		return order
	})
}

func (l *listing) directory(ec *contract.ExecContext, p, name string, header bool) {
	children, err := ec.VFS.ReadDir(p)
	if err != nil {
		l.code = diagnostic(ec, "ls", fmt.Sprintf("cannot open directory '%s': %v", name, err), 1)
		return
	}
	var items []contract.LSItem
	all := hasFlag(l.options, "a") || hasFlag(l.options, "all")
	if all {
		for _, dot := range []string{".", ".."} {
			info, err := ec.VFS.Stat(path.Join(p, dot))
			if err != nil {
				l.code = diagnostic(ec, "ls", fmt.Sprintf("cannot access '%s': %v", appendPath(name, dot), err), 1)
				continue
			}
			items = append(items, contract.LSItem{Info: info, DisplayName: dot})
		}
	}
	for _, child := range children {
		if all || !strings.HasPrefix(child.Name, ".") {
			items = append(items, contract.LSItem{Info: child, DisplayName: child.Name, IsExecutable: child.IsExecutable()})
		}
	}
	l.sort(items)
	if header {
		if l.printed {
			fmt.Fprintln(ec.Stdout)
		}
		fmt.Fprintf(ec.Stdout, "%s:\n", name)
		l.printed = true
	}
	l.render(ec, items, true)
	if hasFlag(l.options, "R") {
		for _, item := range items {
			if item.Info.IsDir() && item.DisplayName != "." && item.DisplayName != ".." {
				l.directory(ec, path.Join(p, item.DisplayName), appendPath(name, item.DisplayName), true)
			}
		}
	}
}

func (l *listing) render(ec *contract.ExecContext, items []contract.LSItem, directory bool) {
	if len(items) == 0 && (!directory || !hasFlag(l.options, "l")) {
		return
	}
	l.printed = true
	switch {
	case hasFlag(l.options, "l"):
		human := hasFlag(l.options, "h")
		if ec.UI != nil {
			output := ec.UI.RenderLSLong(items, human, l.color)
			if !directory {
				if first, rest, ok := strings.Cut(output, "\n"); ok && strings.HasPrefix(first, "total ") {
					output = rest
				}
			}
			fmt.Fprintln(ec.Stdout, output)
			return
		}
		plainLongListing(ec.Stdout, items, directory, human)
	case ec.UI != nil && ec.IsTTY && !hasFlag(l.options, "1"):
		fmt.Fprintln(ec.Stdout, ec.UI.RenderLSGrid(items, ec.TermWidth, l.color))
	default:
		for i, item := range items {
			name := item.DisplayName
			if l.color && ec.UI != nil {
				name = ec.UI.RenderLSGrid(items[i:i+1], ec.TermWidth, true)
			}
			fmt.Fprintln(ec.Stdout, name)
		}
	}
}

func plainLongListing(out io.Writer, items []contract.LSItem, directory, human bool) {
	var blocks int64
	var rows []string
	for _, item := range items {
		info := item.Info
		blocks += (info.Size + 1023) / 1024
		size := strconv.FormatInt(info.Size, 10)
		if human && info.Size >= 1024 {
			scaled := float64(info.Size)
			for _, unit := range []string{"K", "M", "G", "T", "P", "E"} {
				scaled /= 1024
				if scaled < 1024 {
					precision := 0
					if scaled < 10 {
						precision = 1
					}
					size = strconv.FormatFloat(scaled, 'f', precision, 64) + unit
					break
				}
			}
		}
		name := item.DisplayName
		if info.Type == contract.TypeSymlink {
			name += " -> " + info.LinkTarget
		}
		rows = append(rows, fmt.Sprintf("%s 1 %s %s %s %s %s", modeString(info),
			info.Owner, info.Group, size, info.ModTime.Format("Jan _2 15:04"), name))
	}
	if directory {
		fmt.Fprintf(out, "total %d\n", blocks)
	}
	for _, row := range rows {
		fmt.Fprintln(out, row)
	}
}

// appendPath keeps the spelling of a command-line path, including a leading ./.
func appendPath(base, name string) string {
	if strings.HasSuffix(base, "/") {
		return base + name
	}
	return base + "/" + name
}

func tree(ec *contract.ExecContext, args []string) int {
	options, paths, err := parseFlags(args, "aL:d", nil, false)
	if err != nil {
		return diagnostic(ec, "tree", err.Error(), 1)
	}
	maxDepth := -1
	for _, opt := range options {
		if opt.name == "L" {
			maxDepth, err = parseCount(opt.value, 1)
			if err != nil {
				return diagnostic(ec, "tree", err.Error(), 1)
			}
		}
	}
	if len(paths) == 0 {
		paths = []string{"."}
	}
	code := 0
	for _, name := range paths {
		p := resolve(ec, name)
		info, err := ec.VFS.Stat(p)
		if err == nil && !info.IsDir() {
			err = contract.ErrNotDir
		}
		if err != nil {
			code = diagnostic(ec, "tree", fmt.Sprintf("'%s': %v", name, err), 1)
			continue
		}
		rootInfo := *info
		rootInfo.Name = name
		root := &contract.TreeNode{Info: &rootInfo}
		var dirs, files int
		all, onlyDirs := hasFlag(options, "a"), hasFlag(options, "d")
		var walk func(*contract.TreeNode, string, int)
		walk = func(parent *contract.TreeNode, p string, depth int) {
			if depth == maxDepth {
				return
			}
			children, err := ec.VFS.ReadDir(p)
			if err != nil {
				code = diagnostic(ec, "tree", fmt.Sprintf("'%s': %v", p, err), 1)
				return
			}
			for _, child := range children {
				if (!all && strings.HasPrefix(child.Name, ".")) || (onlyDirs && !child.IsDir()) {
					continue
				}
				node := &contract.TreeNode{Info: child}
				parent.Children = append(parent.Children, node)
				if child.IsDir() {
					dirs++
					walk(node, path.Join(p, child.Name), depth+1)
				} else {
					files++
				}
			}
		}
		walk(root, p, 0)
		if ec.UI != nil {
			fmt.Fprintln(ec.Stdout, ec.UI.RenderTree(root, dirs, files, ec.IsTTY))
		} else {
			fmt.Fprintln(ec.Stdout, name)
			plainTree(ec.Stdout, root.Children, "")
			fmt.Fprintf(ec.Stdout, "\n%d %s, %d %s\n", dirs, plural(dirs, "directory", "directories"), files, plural(files, "file", "files"))
		}
	}
	return code
}

func plainTree(out io.Writer, children []*contract.TreeNode, prefix string) {
	for i, child := range children {
		branch, indent := "├── ", "│   "
		if i == len(children)-1 {
			branch, indent = "└── ", "    "
		}
		name := child.Info.Name
		if child.Info.Type == contract.TypeSymlink {
			name += " -> " + child.Info.LinkTarget
		}
		fmt.Fprintln(out, prefix+branch+name)
		plainTree(out, child.Children, prefix+indent)
	}
}

func plural(count int, singular, plural string) string {
	if count == 1 {
		return singular
	}
	return plural
}
