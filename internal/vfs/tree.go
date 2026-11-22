package vfs

import (
	"io/fs"
	"path"
	"slices"
	"strings"

	"learn-linux/internal/contract"
)

func (v *MemVFS) Remove(p string) error {
	return v.remove(p, false)
}

func (v *MemVFS) RemoveAll(p string) error {
	return v.remove(p, true)
}

func (v *MemVFS) remove(p string, recursive bool) error {
	v.mu.Lock()
	defer v.mu.Unlock()
	e, err := v.lookup(p, false)
	if recursive && err == contract.ErrNotExist {
		return nil
	}
	if err != nil {
		return err
	}
	if e.parent == nil {
		return contract.ErrDestructiveRoot
	}
	if !recursive && len(e.node.children) > 0 {
		return contract.ErrNotEmpty
	}
	delete(e.parent.children, e.name)
	return nil
}

func (v *MemVFS) destination(p string) (entry, error) {
	e, err := v.lookup(p, false)
	if err == contract.ErrNotExist && e.parent != nil {
		return e, nil
	}
	return e, err
}

func (e entry) checkReplacement(source *node) error {
	switch {
	case e.node == nil:
		return nil
	case len(e.node.children) > 0:
		return contract.ErrNotEmpty
	case e.node.info.IsDir() && !source.info.IsDir():
		return contract.ErrIsDir
	case !e.node.info.IsDir() && source.info.IsDir():
		return contract.ErrNotDir
	default:
		return nil
	}
}

func (v *MemVFS) Rename(oldPath, newPath string) error {
	v.mu.Lock()
	defer v.mu.Unlock()
	src, err := v.lookup(oldPath, false)
	if err != nil {
		return err
	}
	if src.parent == nil {
		return contract.ErrDestructiveRoot
	}
	if inside(absolutePath("/", oldPath), absolutePath("/", newPath)) {
		return contract.ErrInvalidPath
	}
	dst, err := v.destination(newPath)
	if err != nil {
		return err
	}
	if src.node == dst.node {
		return nil
	}
	if inside(src.path(), dst.path()) {
		return contract.ErrInvalidPath
	}
	if err := dst.checkReplacement(src.node); err != nil {
		return err
	}
	delete(src.parent.children, src.name)
	src.node.moveTo(dst.path())
	dst.parent.children[dst.name] = src.node
	return nil
}

func (n *node) moveTo(p string) {
	n.info.Path = p
	n.info.Name = path.Base(p)
	for name, child := range n.children {
		child.moveTo(path.Join(p, name))
	}
}

func inside(dir, p string) bool {
	return p != dir && strings.HasPrefix(p, strings.TrimSuffix(dir, "/")+"/")
}

func (v *MemVFS) Copy(srcPath, dstPath string, recursive bool) error {
	v.mu.Lock()
	defer v.mu.Unlock()
	src, err := v.lookup(srcPath, false)
	if err != nil {
		return err
	}
	if src.node.info.IsDir() {
		if !recursive {
			return contract.ErrIsDir
		}
		if inside(absolutePath("/", srcPath), absolutePath("/", dstPath)) {
			return contract.ErrInvalidPath
		}
	}
	dst, err := v.destination(dstPath)
	if err != nil {
		return err
	}
	if src.node == dst.node || inside(src.path(), dst.path()) {
		return contract.ErrInvalidPath
	}
	if err := dst.checkReplacement(src.node); err != nil {
		return err
	}
	dst.parent.children[dst.name] = src.node.cloneTo(dst.path())
	return nil
}

func (n *node) cloneTo(p string) *node {
	result := &node{info: n.info, data: slices.Clone(n.data)}
	result.info.Path = p
	result.info.Name = path.Base(p)
	if n.children != nil {
		result.children = make(map[string]*node, len(n.children))
		for name, child := range n.children {
			result.children[name] = child.cloneTo(path.Join(p, name))
		}
	}
	return result
}

func (v *MemVFS) Clone() contract.VFS {
	v.mu.RLock()
	defer v.mu.RUnlock()
	return &MemVFS{root: v.root.cloneTo("/")}
}

func (v *MemVFS) Reset(snapshot contract.VFS) {
	if snapshot == v {
		return
	}
	cloned := snapshot.Clone()
	var root *node
	if mem, ok := cloned.(*MemVFS); ok {
		root = mem.root
	} else {
		root = importTree(cloned)
		if root == nil {
			return
		}
	}
	v.mu.Lock()
	defer v.mu.Unlock()
	v.root = root
}

// importTree reconstructs a snapshot from another VFS implementation. An
// incomplete snapshot is discarded because Reset cannot return an error.
func importTree(snapshot contract.VFS) *node {
	nodes := make(map[string]*node)
	var files []*node
	err := snapshot.Walk("/", func(p string, info *contract.NodeInfo) error {
		n := &node{info: *info}
		switch n.info.Type {
		case contract.TypeDir:
			n.children = make(map[string]*node)
		case contract.TypeFile:
			files = append(files, n)
		}
		nodes[p] = n
		if p != "/" {
			nodes[path.Dir(p)].children[path.Base(p)] = n
		}
		return nil
	})
	if err != nil {
		return nil
	}
	for _, n := range files {
		p := n.info.Path
		if n.info.Mode&0444 == 0 {
			if err := snapshot.Chmod(p, n.info.Mode|0400); err != nil {
				return nil
			}
		}
		data, err := snapshot.ReadFile(p)
		if err != nil {
			return nil
		}
		n.data = slices.Clone(data)
	}
	return nodes["/"]
}

type walkEntry struct {
	path string
	info contract.NodeInfo
	end  int
}

func (n *node) collectWalk(p string, entries *[]walkEntry) {
	index := len(*entries)
	*entries = append(*entries, walkEntry{path: p, info: n.info})
	for _, name := range n.childNames() {
		n.children[name].collectWalk(path.Join(p, name), entries)
	}
	(*entries)[index].end = len(*entries)
}

func (v *MemVFS) walkEntries(root string) ([]walkEntry, error) {
	v.mu.RLock()
	defer v.mu.RUnlock()
	e, err := v.lookup(root, false)
	if err != nil {
		return nil, err
	}
	var entries []walkEntry
	e.node.collectWalk(absolutePath("/", root), &entries)
	return entries, nil
}

func (v *MemVFS) Walk(root string, fn func(p string, info *contract.NodeInfo) error) error {
	entries, err := v.walkEntries(root)
	if err != nil {
		return err
	}
	for i := 0; i < len(entries); {
		e := entries[i]
		isDir := e.info.IsDir()
		switch err := fn(e.path, &e.info); {
		case err == nil:
			i++
		case err == fs.SkipDir && isDir:
			i = e.end
		default:
			return err
		}
	}
	return nil
}
