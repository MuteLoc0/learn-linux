package shell

import (
	"context"
	"io"
	"io/fs"
	"runtime"
	"slices"
	"strings"
	"time"

	"learn-linux/internal/contract"
	"mvdan.cc/sh/v3/interp"
)

// The interpreter reserves a filename prefix under its temp directory and
// opens those names directly. Give it a directory that cannot exist on the
// host, even if the user changes the shell's visible TMPDIR variable.
func interpreterTempDir() string {
	if runtime.GOOS == "windows" {
		return "C:\\\x00"
	}
	return "/\x00"
}

func normalizePOSIXPath(p string) string {
	if p == "" {
		return ""
	}
	p = strings.ReplaceAll(p, "\\", "/")
	if len(p) >= 2 && p[1] == ':' && ((p[0] >= 'a' && p[0] <= 'z') || (p[0] >= 'A' && p[0] <= 'Z')) {
		p = p[2:]
		if p == "" {
			p = "/"
		}
	}
	return p
}

func (s *execution) currentDir(hc interp.HandlerContext) string {
	if pwd := normalizePOSIXPath(hc.Env.Get("PWD").String()); strings.HasPrefix(pwd, "/") {
		return pwd
	}
	if dir := normalizePOSIXPath(hc.Dir); dir != "" {
		return dir
	}
	return "/"
}

func (s *execution) resolve(cwd, path string) string {
	return s.vfs.ResolvePath(normalizePOSIXPath(cwd), s.engine.cfg.Home, normalizePOSIXPath(path))
}

func (s *execution) open(ctx context.Context, path string, flag int, mode fs.FileMode) (io.ReadWriteCloser, error) {
	resolved := s.resolve(s.currentDir(interp.HandlerCtx(ctx)), path)
	if resolved == "/dev/null" {
		return nullFile{}, nil
	}
	f, err := s.vfs.OpenFile(resolved, flag, mode)
	if err != nil {
		return nil, &fs.PathError{Op: "open", Path: path, Err: err}
	}
	return f, nil
}

func (s *execution) stat(ctx context.Context, path string, followSymlinks bool) (fs.FileInfo, error) {
	path = s.resolve(s.currentDir(interp.HandlerCtx(ctx)), path)
	if path == "/dev/null" {
		return vfsFileInfo{NodeInfo: &contract.NodeInfo{Name: "null", Path: path, Mode: 0666}, device: true}, nil
	}
	stat := s.vfs.Stat
	if !followSymlinks {
		stat = s.vfs.Lstat
	}
	info, err := stat(path)
	if err != nil {
		return nil, err
	}
	return vfsFileInfo{NodeInfo: info}, nil
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
	infos, err := s.vfs.ReadDir(s.resolve(s.currentDir(interp.HandlerCtx(ctx)), path))
	if err != nil {
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
	device bool
}

func (f vfsFileInfo) Name() string       { return f.NodeInfo.Name }
func (f vfsFileInfo) Size() int64        { return f.NodeInfo.Size }
func (f vfsFileInfo) ModTime() time.Time { return f.NodeInfo.ModTime }
func (f vfsFileInfo) Sys() any           { return nil }

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
