package contract

import (
	"errors"
	"io"
	"io/fs"
	"time"
)

// Sentinel errors returned by VFS operations.
var (
	ErrNotExist        = errors.New("no such file or directory")
	ErrExist           = errors.New("file exists")
	ErrPermission      = errors.New("permission denied")
	ErrIsDir           = errors.New("is a directory")
	ErrNotDir          = errors.New("not a directory")
	ErrNotEmpty        = errors.New("directory not empty")
	ErrSymlinkLoop     = errors.New("too many levels of symbolic links")
	ErrInvalidPath     = errors.New("invalid path")
	ErrDestructiveRoot = errors.New("destructive root operation detected")
)

// FileType identifies the kind of VFS node.
type FileType int

const (
	TypeFile FileType = iota
	TypeDir
	TypeSymlink
)

// NodeInfo is an immutable snapshot of a VFS node's metadata.
type NodeInfo struct {
	Name                string
	Path                string
	Type                FileType
	Mode                fs.FileMode
	Size                int64
	ModTime             time.Time
	Owner               string
	Group               string
	LinkTarget          string
	IsVirtualBinary     bool
	VirtualBinaryOutput string
	VirtualBinarySource string
}

// IsDir reports whether the node represents a directory.
func (n *NodeInfo) IsDir() bool {
	return n != nil && n.Type == TypeDir
}

// IsExecutable reports whether any execute bit is set on a regular file.
func (n *NodeInfo) IsExecutable() bool {
	return n != nil && n.Type == TypeFile && (n.Mode&0111 != 0)
}

// VFS defines the pure in-memory POSIX-like virtual filesystem.
type VFS interface {
	Stat(path string) (*NodeInfo, error)
	Lstat(path string) (*NodeInfo, error)
	ReadFile(path string) ([]byte, error)
	WriteFile(path string, data []byte, perm fs.FileMode) error
	AppendFile(path string, data []byte, perm fs.FileMode) error
	OpenFile(path string, flag int, perm fs.FileMode) (io.ReadWriteCloser, error)
	Mkdir(path string, perm fs.FileMode) error
	MkdirAll(path string, perm fs.FileMode) error
	ReadDir(path string) ([]*NodeInfo, error)
	Remove(path string) error
	RemoveAll(path string) error
	Rename(oldPath, newPath string) error
	Copy(src, dst string, recursive bool) error
	Symlink(target, linkPath string) error
	Readlink(path string) (string, error)
	Chmod(path string, mode fs.FileMode) error
	Touch(path string, modTime time.Time) error
	Exists(path string) bool
	IsDir(path string) bool
	ResolvePath(cwd, home, p string) string
	SetVirtualBinary(path string, perm fs.FileMode, output string, sourceInfo string) error
	Clone() VFS
	Reset(snapshot VFS)
	Walk(root string, fn func(path string, info *NodeInfo) error) error
}
