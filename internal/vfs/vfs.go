package vfs

import (
	"io/fs"
	"path"
	"slices"
	"strings"
	"sync"
	"time"

	"learn-linux/internal/contract"
)

const maxSymlinkHops = 40

// MemVFS is a POSIX-like filesystem backed by an in-memory tree.
type MemVFS struct {
	mu   sync.RWMutex
	root *node
}

type node struct {
	info     contract.NodeInfo
	data     []byte
	children map[string]*node
}

var _ contract.VFS = (*MemVFS)(nil)

func NewMemVFS() *MemVFS {
	return &MemVFS{root: newNode("/", contract.TypeDir, 0755)}
}

func newNode(p string, kind contract.FileType, mode fs.FileMode) *node {
	owner := "root"
	if p == "/home/learner" || strings.HasPrefix(p, "/home/learner/") {
		owner = "learner"
	}
	n := &node{info: contract.NodeInfo{
		Name: path.Base(p), Path: p, Type: kind, Mode: mode & 07777,
		Owner: owner, Group: owner, ModTime: time.Now(),
	}}
	if kind == contract.TypeDir {
		n.children = make(map[string]*node)
	}
	return n
}

func defaultMode(mode, fallback fs.FileMode) fs.FileMode {
	if mode == 0 {
		return fallback
	}
	return mode
}

func absolutePath(cwd, p string) string {
	if strings.HasPrefix(p, "/") {
		return path.Clean(p)
	}
	return path.Join("/", cwd, p)
}

func (v *MemVFS) ResolvePath(cwd, home, p string) string {
	switch {
	case p == "~":
		p = home
	case strings.HasPrefix(p, "~/"):
		p = strings.TrimSuffix(home, "/") + p[1:]
	}
	return absolutePath(cwd, p)
}

type entry struct {
	parent *node
	name   string
	node   *node
}

func (e entry) path() string {
	if e.parent == nil {
		return "/"
	}
	return path.Join(e.parent.info.Path, e.name)
}

func (e entry) create(kind contract.FileType, mode fs.FileMode) *node {
	n := newNode(e.path(), kind, mode)
	e.parent.children[e.name] = n
	return n
}

// lookup requires the caller to hold the filesystem lock. When only the
// final component is missing, it also returns the location for creation.
func (v *MemVFS) lookup(p string, followFinal bool) (entry, error) {
	var parts []string
	if p = strings.Trim(p, "/"); p != "" {
		parts = strings.Split(p, "/")
	}
	ancestors := []*node{v.root}
	hops := 0
	for len(parts) > 0 {
		current := ancestors[len(ancestors)-1]
		if !current.info.IsDir() {
			return entry{}, contract.ErrNotDir
		}
		name := parts[0]
		parts = parts[1:]
		switch name {
		case "", ".":
			continue
		case "..":
			if len(ancestors) > 1 {
				ancestors = ancestors[:len(ancestors)-1]
			}
			continue
		}
		child := current.children[name]
		if child == nil {
			if len(parts) == 0 {
				return entry{parent: current, name: name}, contract.ErrNotExist
			}
			return entry{}, contract.ErrNotExist
		}
		if child.info.Type == contract.TypeSymlink && (followFinal || len(parts) > 0) {
			hops++
			if hops > maxSymlinkHops {
				return entry{}, contract.ErrSymlinkLoop
			}
			target := child.info.LinkTarget
			if target == "" {
				return entry{}, contract.ErrNotExist
			}
			if strings.HasPrefix(target, "/") {
				ancestors = ancestors[:1]
			}
			parts = append(strings.Split(target, "/"), parts...)
			continue
		}
		ancestors = append(ancestors, child)
	}
	if len(ancestors) == 1 {
		return entry{node: v.root}, nil
	}
	current := ancestors[len(ancestors)-1]
	return entry{parent: ancestors[len(ancestors)-2], name: current.info.Name, node: current}, nil
}

func (v *MemVFS) stat(p string, followFinal bool) (*contract.NodeInfo, error) {
	v.mu.RLock()
	defer v.mu.RUnlock()
	e, err := v.lookup(p, followFinal)
	if err != nil {
		return nil, err
	}
	info := e.node.info
	return &info, nil
}

func (v *MemVFS) Stat(p string) (*contract.NodeInfo, error) {
	return v.stat(p, true)
}

func (v *MemVFS) Lstat(p string) (*contract.NodeInfo, error) {
	return v.stat(p, false)
}

func (v *MemVFS) Exists(p string) bool {
	_, err := v.Stat(p)
	return err == nil
}

func (v *MemVFS) IsDir(p string) bool {
	info, err := v.Stat(p)
	return err == nil && info.IsDir()
}

func (v *MemVFS) Chmod(p string, mode fs.FileMode) error {
	v.mu.Lock()
	defer v.mu.Unlock()
	e, err := v.lookup(p, true)
	if err != nil {
		return err
	}
	e.node.info.Mode = mode & 07777
	return nil
}

func (v *MemVFS) Mkdir(p string, perm fs.FileMode) error {
	v.mu.Lock()
	defer v.mu.Unlock()
	return v.mkdir(p, defaultMode(perm, 0755))
}

func (v *MemVFS) mkdir(p string, mode fs.FileMode) error {
	e, err := v.lookup(p, false)
	if err == nil {
		return contract.ErrExist
	}
	if err != contract.ErrNotExist || e.parent == nil {
		return err
	}
	e.create(contract.TypeDir, mode)
	return nil
}

func (v *MemVFS) MkdirAll(p string, perm fs.FileMode) error {
	v.mu.Lock()
	defer v.mu.Unlock()
	return v.mkdirAll(absolutePath("/", p), defaultMode(perm, 0755))
}

func (v *MemVFS) mkdirAll(p string, mode fs.FileMode) error {
	e, err := v.lookup(p, true)
	if err == nil {
		if !e.node.info.IsDir() {
			return contract.ErrNotDir
		}
		return nil
	}
	if err != contract.ErrNotExist {
		return err
	}
	if err := v.mkdirAll(path.Dir(p), mode); err != nil {
		return err
	}
	return v.mkdir(p, mode)
}

func (n *node) childNames() []string {
	names := make([]string, 0, len(n.children))
	for name := range n.children {
		names = append(names, name)
	}
	slices.Sort(names)
	return names
}

func (v *MemVFS) ReadDir(p string) ([]*contract.NodeInfo, error) {
	v.mu.RLock()
	defer v.mu.RUnlock()
	e, err := v.lookup(p, true)
	if err != nil {
		return nil, err
	}
	if !e.node.info.IsDir() {
		return nil, contract.ErrNotDir
	}
	infos := make([]*contract.NodeInfo, 0, len(e.node.children))
	for _, name := range e.node.childNames() {
		info := e.node.children[name].info
		infos = append(infos, &info)
	}
	return infos, nil
}

func (v *MemVFS) Symlink(target, linkPath string) error {
	v.mu.Lock()
	defer v.mu.Unlock()
	e, err := v.lookup(linkPath, false)
	if err == nil {
		return contract.ErrExist
	}
	if err != contract.ErrNotExist || e.parent == nil {
		return err
	}
	n := e.create(contract.TypeSymlink, 0777)
	n.info.LinkTarget = target
	n.info.Size = int64(len(target))
	return nil
}

func (v *MemVFS) Readlink(p string) (string, error) {
	info, err := v.Lstat(p)
	if err != nil {
		return "", err
	}
	if info.Type != contract.TypeSymlink {
		return "", contract.ErrInvalidPath
	}
	return info.LinkTarget, nil
}
