package shell

import (
	"context"
	"errors"
	"io"
	"io/fs"
	"path"
	"slices"
	"strings"
	"syscall"
	"time"

	"learn-linux/internal/contract"
	"learn-linux/internal/vfs"
	"mvdan.cc/sh/v3/interp"
)

func (s *execution) open(ctx context.Context, name string, flag int, mode fs.FileMode) (io.ReadWriteCloser, error) {
	if name == "" {
		return nil, &fs.PathError{Op: "open", Path: name, Err: contract.ErrNotExist}
	}
	resolved := vfs.FilePath(interp.HandlerCtx(ctx).Dir, name)
	if path.Clean(resolved) == "/dev/null" {
		return nullFile{}, nil
	}
	f, err := s.vfs.OpenFile(resolved, flag, mode)
	if err != nil {
		return nil, &fs.PathError{Op: "open", Path: name, Err: err}
	}
	return f, nil
}

func (s *execution) stat(ctx context.Context, name string, followSymlinks bool) (fs.FileInfo, error) {
	if name == "" {
		return nil, contract.ErrNotExist
	}
	resolved := vfs.FilePath(interp.HandlerCtx(ctx).Dir, name)
	if path.Clean(resolved) == "/dev/null" {
		return vfsFileInfo{NodeInfo: &contract.NodeInfo{Name: "null", Path: "/dev/null", Mode: 0666}, device: true}, nil
	}
	stat := s.vfs.Stat
	if !followSymlinks {
		stat = s.vfs.Lstat
	}
	info, err := stat(resolved)
	if err != nil {
		return nil, err
	}
	user := s.engine.cfg.User
	return vfsFileInfo{NodeInfo: info, owned: info.Owner == user, inGroup: info.Group == user}, nil
}

func (s *execution) access(ctx context.Context, path string, mode interp.AccessMode) error {
	info, err := s.stat(ctx, path, true)
	if err != nil {
		return err
	}
	perm := info.Mode().Perm()
	if available := (perm | perm>>3 | perm>>6) & 7; available&fs.FileMode(mode) != fs.FileMode(mode) {
		return contract.ErrPermission
	}
	return nil
}

func (s *execution) readDir(ctx context.Context, path string) ([]fs.DirEntry, error) {
	infos, err := s.vfs.ReadDir(vfs.FilePath(interp.HandlerCtx(ctx).Dir, path))
	switch {
	case errors.Is(err, contract.ErrNotExist), errors.Is(err, contract.ErrSymlinkLoop):
		return nil, fs.ErrNotExist
	case errors.Is(err, contract.ErrNotDir):
		return nil, syscall.ENOTDIR
	case errors.Is(err, contract.ErrPermission):
		return nil, fs.ErrPermission
	case err != nil:
		return nil, err
	}
	entries := make([]fs.DirEntry, 0, len(infos))
	for _, info := range infos {
		entries = append(entries, vfsDirEntry{vfsFileInfo{NodeInfo: info}})
	}
	slices.SortFunc(entries, func(a, b fs.DirEntry) int { return strings.Compare(a.Name(), b.Name()) })
	return entries, nil
}

type nullFile struct{}

func (nullFile) Read([]byte) (int, error)    { return 0, io.EOF }
func (nullFile) Write(p []byte) (int, error) { return len(p), nil }
func (nullFile) Close() error                { return nil }

type vfsFileInfo struct {
	*contract.NodeInfo
	device  bool
	owned   bool
	inGroup bool
}

func (f vfsFileInfo) Name() string       { return f.NodeInfo.Name }
func (f vfsFileInfo) Size() int64        { return f.NodeInfo.Size }
func (f vfsFileInfo) ModTime() time.Time { return f.NodeInfo.ModTime }
func (f vfsFileInfo) Sys() any           { return nil }
func (f vfsFileInfo) OwnedByUser() bool  { return f.owned }
func (f vfsFileInfo) OwnedByGroup() bool { return f.inGroup }

func (f vfsFileInfo) SameFile(other fs.FileInfo) bool {
	info, ok := other.(vfsFileInfo)
	return ok && f.Path == info.Path
}

func (f vfsFileInfo) Mode() fs.FileMode {
	mode := f.NodeInfo.Mode.Perm()
	for _, bit := range []struct{ posix, goMode fs.FileMode }{
		{04000, fs.ModeSetuid}, {02000, fs.ModeSetgid}, {01000, fs.ModeSticky},
	} {
		if f.NodeInfo.Mode&(bit.posix|bit.goMode) != 0 {
			mode |= bit.goMode
		}
	}
	switch {
	case f.device:
		mode |= fs.ModeDevice | fs.ModeCharDevice
	case f.IsDir():
		mode |= fs.ModeDir
	case f.NodeInfo.Type == contract.TypeSymlink:
		mode |= fs.ModeSymlink
	}
	return mode
}

type vfsDirEntry struct{ vfsFileInfo }

func (d vfsDirEntry) Type() fs.FileMode          { return d.Mode().Type() }
func (d vfsDirEntry) Info() (fs.FileInfo, error) { return d.vfsFileInfo, nil }
