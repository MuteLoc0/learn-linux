package vfs_test

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"learn-linux/internal/contract"
	"learn-linux/internal/vfs"
	"mvdan.cc/sh/v3/interp"
	"mvdan.cc/sh/v3/syntax"
)

func must(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}

func wantError(t *testing.T, got, want error) {
	t.Helper()
	if !errors.Is(got, want) {
		t.Fatalf("error = %v, want %v", got, want)
	}
}

func wantFile(t *testing.T, v contract.VFS, path, want string) {
	t.Helper()
	data, err := v.ReadFile(path)
	must(t, err)
	if string(data) != want {
		t.Fatalf("ReadFile(%q) = %q, want %q", path, data, want)
	}
}

func TestResolvePath(t *testing.T) {
	v := vfs.NewMemVFS()
	for _, tc := range []struct{ cwd, p, want string }{
		{"", "", "/"},
		{"/a//b/..", "", "/a"},
		{"/a/b", "~", "/home/learner"},
		{"/a/b", "~/a", "/home/learner/a"},
		{"/a/b", "~root", "/a/b/~root"},
		{"/a/b", "..", "/a"},
		{"/a/b", "/../../", "/"},
		{"/a/b", "./c///", "/a/b/c"},
		{"a/b", "../../../c", "/c"},
		{"/a/b", "/x//y/.", "/x/y"},
	} {
		t.Run(fmt.Sprintf("%s,%s", tc.cwd, tc.p), func(t *testing.T) {
			if got := v.ResolvePath(tc.cwd, "/home/learner", tc.p); got != tc.want {
				t.Fatalf("ResolvePath = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestSymlinks(t *testing.T) {
	v := vfs.NewMemVFS()
	must(t, v.MkdirAll("/base/dir/sub", 0))
	must(t, v.WriteFile("/base/dir/file", []byte("contents"), 0))
	for _, tc := range []struct{ target, link string }{
		{"/base", "/alias"},
		{"../dir/file", "/base/dir/relative"},
		{"sub", "/base/dir/via"},
		{"via/../file", "/base/dir/parent"},
		{"missing", "/base/dir/dangling"},
		{"/loop2", "/loop1"},
		{"/loop1", "/loop2"},
	} {
		must(t, v.Symlink(tc.target, tc.link))
	}
	for _, tc := range []struct {
		p, wantPath, target string
		lstat               bool
		err                 error
	}{
		{p: "/alias/dir/relative", wantPath: "/base/dir/file"},
		{p: "/base/dir/parent", wantPath: "/base/dir/file"},
		{p: "/alias/dir/relative", wantPath: "/base/dir/relative", lstat: true, target: "../dir/file"},
		{p: "/base/dir/dangling", wantPath: "/base/dir/dangling", lstat: true, target: "missing"},
		{p: "/base/dir/dangling", err: contract.ErrNotExist},
		{p: "/base/dir/absent/file", err: contract.ErrNotExist},
		{p: "/base/dir/relative/child", err: contract.ErrNotDir},
		{p: "/loop1", err: contract.ErrSymlinkLoop},
		{p: "/loop1/child", lstat: true, err: contract.ErrSymlinkLoop},
		{p: "/loop1", wantPath: "/loop1", lstat: true, target: "/loop2"},
	} {
		t.Run(fmt.Sprintf("%s,lstat=%t", tc.p, tc.lstat), func(t *testing.T) {
			stat := v.Stat
			if tc.lstat {
				stat = v.Lstat
			}
			info, err := stat(tc.p)
			wantError(t, err, tc.err)
			if tc.err == nil && (info.Path != tc.wantPath || info.LinkTarget != tc.target ||
				(tc.target != "" && info.Type != contract.TypeSymlink)) {
				t.Fatalf("unexpected metadata: %+v", info)
			}
		})
	}

	for i := 0; i < 41; i++ {
		target := fmt.Sprintf("link-%d", i-1)
		if i == 0 {
			target = "/base/dir/file"
		}
		must(t, v.Symlink(target, fmt.Sprintf("/link-%d", i)))
	}
	wantFile(t, v, "/link-39", "contents")
	_, err := v.ReadFile("/link-40")
	wantError(t, err, contract.ErrSymlinkLoop)

	target, err := v.Readlink("/alias/dir/relative")
	must(t, err)
	if target != "../dir/file" {
		t.Fatalf("Readlink = %q", target)
	}
	_, err = v.Readlink("/base/dir/file")
	wantError(t, err, contract.ErrInvalidPath)
	wantError(t, v.Symlink("file", "/alias/dir/relative"), contract.ErrExist)
	wantError(t, v.Symlink("file", "/missing/link"), contract.ErrNotExist)
	must(t, v.WriteFile("/base/dir/dangling", []byte("created"), 0))
	must(t, v.AppendFile("/alias/dir/relative", []byte("!"), 0))
	wantFile(t, v, "/base/dir/missing", "created")
	wantFile(t, v, "/base/dir/file", "contents!")
}

func TestFileAndDirectoryMetadata(t *testing.T) {
	v := vfs.NewMemVFS()
	root, err := v.Stat("/")
	must(t, err)
	if !root.IsDir() || root.Mode != 0755 || root.Owner != "root" || root.Group != "root" {
		t.Fatalf("root metadata = %+v", root)
	}
	must(t, v.MkdirAll("/home/learner", 0))
	must(t, v.MkdirAll("/home/learner", 0700))
	data := []byte("hello")
	must(t, v.WriteFile("/home/learner/z", data, 0600))
	data[0] = 'A'
	firstRead, err := v.ReadFile("/home/learner/z")
	must(t, err)
	firstRead[0] = 'B'
	wantFile(t, v, "/home/learner/z", "hello")
	must(t, v.WriteFile("/home/learner/z", []byte("world"), 0777))
	must(t, v.AppendFile("/home/learner/a", []byte("appended"), 0))
	must(t, v.Symlink("z", "/home/learner/link"))
	at := time.Unix(123, 456)
	must(t, v.Touch("/home/learner/link", at))
	must(t, v.Touch("/home/learner/empty", at))
	wantFile(t, v, "/home/learner/empty", "")

	infos, err := v.ReadDir("/home/learner")
	must(t, err)
	var names []string
	for _, info := range infos {
		names = append(names, info.Name)
	}
	if !reflect.DeepEqual(names, []string{"a", "empty", "link", "z"}) {
		t.Fatalf("ReadDir names = %v", names)
	}
	link, z := infos[2], infos[3]
	if link.Type != contract.TypeSymlink || link.LinkTarget != "z" {
		t.Fatalf("link metadata = %+v", link)
	}
	if z.Owner != "learner" || z.Group != "learner" || z.Mode != 0600 || z.Size != 5 || !z.ModTime.Equal(at) {
		t.Fatalf("file metadata = %+v", z)
	}
	z.Name, z.Mode = "changed", 0
	info, err := v.Stat("/home/learner/z")
	must(t, err)
	if info.Name != "z" || info.Mode != 0600 {
		t.Fatalf("metadata changed through ReadDir: %+v", info)
	}
	info.Name = "changed"
	info, err = v.Stat("/home/learner/z")
	must(t, err)
	if info.Name != "z" {
		t.Fatalf("metadata changed through Stat: %+v", info)
	}
	must(t, v.Chmod("/home/learner/link", fs.ModeSetuid|0300))
	info, err = v.Stat("/home/learner/z")
	must(t, err)
	if info.Mode != 0300 {
		t.Fatalf("chmod mode = %#o", info.Mode)
	}
	_, err = v.ReadFile("/home/learner/z")
	wantError(t, err, contract.ErrPermission)
	_, err = v.ReadFile("/home")
	wantError(t, err, contract.ErrIsDir)
	_, err = v.ReadDir("/home/learner/link")
	wantError(t, err, contract.ErrNotDir)
	wantError(t, v.Mkdir("/home", 0), contract.ErrExist)
	wantError(t, v.Mkdir("/absent/dir", 0), contract.ErrNotExist)
	wantError(t, v.MkdirAll("/home/learner/z/dir", 0), contract.ErrNotDir)
	wantError(t, v.WriteFile("/home", nil, 0), contract.ErrIsDir)
	wantError(t, v.WriteFile("/absent/file", nil, 0), contract.ErrNotExist)
	wantError(t, v.WriteFile("/home/learner/z/sub", nil, 0), contract.ErrNotDir)
}

func TestOpenFile(t *testing.T) {
	for _, tc := range []struct {
		name, path, write, read, want string
		flag                          int
		err                           error
	}{
		{name: "read only", path: "/existing", read: "abcdef", want: "abcdef", flag: os.O_RDONLY},
		{name: "write only", path: "/existing", write: "xy", want: "xycdef", flag: os.O_WRONLY},
		{name: "create", path: "/new", write: "new", want: "new", flag: os.O_WRONLY | os.O_CREATE | os.O_TRUNC},
		{name: "truncate", path: "/existing", write: "a", want: "a", flag: os.O_WRONLY | os.O_CREATE | os.O_TRUNC},
		{name: "append", path: "/existing", write: "xy", want: "abcdefxy", flag: os.O_WRONLY | os.O_APPEND},
		{name: "read write", path: "/existing", write: "xy", read: "cdef", want: "xycdef", flag: os.O_RDWR},
		{name: "exclusive create", path: "/new", write: "new", want: "new", flag: os.O_WRONLY | os.O_CREATE | os.O_EXCL},
		{name: "exclusive existing", path: "/existing", flag: os.O_WRONLY | os.O_CREATE | os.O_EXCL, err: contract.ErrExist},
		{name: "exclusive dangling symlink", path: "/dangling", flag: os.O_WRONLY | os.O_CREATE | os.O_EXCL, err: contract.ErrExist},
		{name: "create through dangling symlink", path: "/dangling", write: "new", want: "new", flag: os.O_WRONLY | os.O_CREATE},
		{name: "missing", path: "/new", flag: os.O_RDONLY, err: contract.ErrNotExist},
		{name: "directory", path: "/", flag: os.O_RDWR, err: contract.ErrIsDir},
		{name: "permission", path: "/unreadable", flag: os.O_RDONLY, err: contract.ErrPermission},
	} {
		t.Run(tc.name, func(t *testing.T) {
			v := vfs.NewMemVFS()
			must(t, v.WriteFile("/existing", []byte("abcdef"), 0644))
			must(t, v.WriteFile("/unreadable", nil, 0200))
			must(t, v.Symlink("new", "/dangling"))
			f, err := v.OpenFile(tc.path, tc.flag, 0644)
			wantError(t, err, tc.err)
			if tc.err != nil {
				return
			}
			if tc.write != "" {
				n, err := io.WriteString(f, tc.write)
				must(t, err)
				if n != len(tc.write) {
					t.Fatalf("short write: %d", n)
				}
			}
			if tc.read != "" {
				data, err := io.ReadAll(f)
				must(t, err)
				if string(data) != tc.read {
					t.Fatalf("handle read = %q, want %q", data, tc.read)
				}
			}
			if tc.flag == os.O_RDONLY {
				_, err := f.Write([]byte("x"))
				wantError(t, err, contract.ErrPermission)
			}
			if tc.flag&os.O_WRONLY != 0 {
				_, err := f.Read(make([]byte, 1))
				wantError(t, err, contract.ErrPermission)
			}
			wantFile(t, v, tc.path, tc.want)
			must(t, f.Close())
			wantFile(t, v, tc.path, tc.want)
			_, err = f.Write([]byte("closed"))
			wantError(t, err, fs.ErrClosed)
		})
	}
}

func TestShellRedirects(t *testing.T) {
	v := vfs.NewMemVFS()
	var stdout, stderr bytes.Buffer
	runner, err := interp.New(interp.Dir("/"), interp.StdIO(nil, &stdout, &stderr),
		interp.OpenHandler(func(_ context.Context, p string, flag int, perm os.FileMode) (io.ReadWriteCloser, error) {
			return v.OpenFile(p, flag, perm)
		}))
	must(t, err)
	prog, err := syntax.NewParser().Parse(strings.NewReader(
		"printf 'first\\n' >/file; printf 'second\\n' >>/file; { read -r a; read -r b; } </file; printf '%s %s\\n' \"$a\" \"$b\"",
	), "")
	must(t, err)
	if err := runner.Run(t.Context(), prog); err != nil {
		t.Fatalf("shell returned %v: %s", err, stderr.String())
	}
	if stdout.String() != "first second\n" {
		t.Fatalf("stdout = %q", stdout.String())
	}
	wantFile(t, v, "/file", "first\nsecond\n")
}

func TestTreeOperations(t *testing.T) {
	setup := func(t *testing.T) *vfs.MemVFS {
		t.Helper()
		v := vfs.NewMemVFS()
		must(t, v.MkdirAll("/src/sub", 0700))
		must(t, v.WriteFile("/src/sub/file", []byte("source"), 0600))
		must(t, v.Symlink("sub/file", "/src/link"))
		must(t, v.Symlink("/src", "/alias"))
		must(t, v.Symlink("/", "/root-link"))
		return v
	}
	for _, tc := range []struct {
		name string
		op   func(*vfs.MemVFS) error
		want error
	}{
		{"copy without recursion", func(v *vfs.MemVFS) error { return v.Copy("/src", "/dst", false) }, contract.ErrIsDir},
		{"copy into self", func(v *vfs.MemVFS) error { return v.Copy("/src", "/src/sub/new", true) }, contract.ErrInvalidPath},
		{"copy through alias into self", func(v *vfs.MemVFS) error { return v.Copy("/src", "/alias/sub/new", true) }, contract.ErrInvalidPath},
		{"rename into self", func(v *vfs.MemVFS) error { return v.Rename("/src", "/src/absent/new") }, contract.ErrInvalidPath},
		{"rename through alias into self", func(v *vfs.MemVFS) error { return v.Rename("/src", "/alias/sub/new") }, contract.ErrInvalidPath},
		{"rename missing source", func(v *vfs.MemVFS) error { return v.Rename("/absent", "/dst") }, contract.ErrNotExist},
		{"rename missing parent", func(v *vfs.MemVFS) error { return v.Rename("/src", "/absent/dst") }, contract.ErrNotExist},
		{"rename over nonempty directory", func(v *vfs.MemVFS) error { return v.Rename("/src/link", "/src/sub") }, contract.ErrNotEmpty},
		{"rename root", func(v *vfs.MemVFS) error { return v.Rename("/src/..", "/dst") }, contract.ErrDestructiveRoot},
		{"remove missing", func(v *vfs.MemVFS) error { return v.Remove("/absent") }, contract.ErrNotExist},
		{"remove nonempty", func(v *vfs.MemVFS) error { return v.Remove("/src") }, contract.ErrNotEmpty},
		{"remove root", func(v *vfs.MemVFS) error { return v.Remove("/../../") }, contract.ErrDestructiveRoot},
		{"remove all root", func(v *vfs.MemVFS) error { return v.RemoveAll("/src/..//") }, contract.ErrDestructiveRoot},
		{"remove all missing", func(v *vfs.MemVFS) error { return v.RemoveAll("/absent/path") }, nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			v := setup(t)
			wantError(t, tc.op(v), tc.want)
			wantFile(t, v, "/src/link", "source")
		})
	}
	v := setup(t)
	must(t, v.Copy("/src", "/copy", true))
	wantFile(t, v, "/copy/link", "source")
	info, err := v.Stat("/copy/sub/file")
	must(t, err)
	if info.Mode != 0600 {
		t.Fatalf("copy mode = %#o", info.Mode)
	}
	must(t, v.WriteFile("/copy/sub/file", []byte("modified"), 0))
	wantFile(t, v, "/src/sub/file", "source")
	must(t, v.Rename("/copy", "/moved"))
	must(t, v.Rename("/moved/sub/file", "/src/sub/file"))
	info, err = v.Lstat("/moved/link")
	must(t, err)
	if info.Path != "/moved/link" {
		t.Fatalf("renamed link path = %q", info.Path)
	}
	info, err = v.Stat("/moved/sub")
	must(t, err)
	if info.Path != "/moved/sub" {
		t.Fatalf("renamed directory path = %q", info.Path)
	}
	wantFile(t, v, "/src/sub/file", "modified")
	must(t, v.Remove("/src/link"))
	must(t, v.Remove("/root-link"))
	wantFile(t, v, "/src/sub/file", "modified")
	must(t, v.RemoveAll("/alias"))
	must(t, v.RemoveAll("/src"))
	if v.Exists("/copy") || v.Exists("/src") || v.Exists("/root-link") || !v.IsDir("/") {
		t.Fatal("unexpected filesystem state after removals")
	}
}

func TestCloneResetAndVirtualBinary(t *testing.T) {
	v := vfs.NewMemVFS()
	must(t, v.MkdirAll("/home/learner/bin", 0))
	must(t, v.WriteFile("/home/learner/bin/tool", []byte("text"), 0644))
	must(t, v.SetVirtualBinary("/home/learner/bin/tool", 0, "hello", "tool.c"))
	must(t, v.Copy("/home/learner/bin/tool", "/copied", false))
	must(t, v.Copy("/home/learner/bin", "/tree", true))
	clone := v.Clone()
	for _, tc := range []struct {
		fs   contract.VFS
		path string
	}{
		{v, "/home/learner/bin/tool"}, {v, "/copied"}, {v, "/tree/tool"},
		{clone, "/home/learner/bin/tool"}, {clone, "/copied"}, {clone, "/tree/tool"},
	} {
		info, err := tc.fs.Stat(tc.path)
		must(t, err)
		if !info.IsVirtualBinary || info.VirtualBinaryOutput != "hello" ||
			info.VirtualBinarySource != "tool.c" || info.Mode != 0755 || !info.IsExecutable() {
			t.Fatalf("binary metadata = %+v", info)
		}
		wantFile(t, tc.fs, tc.path, "\x7fELFvirtual-binary\n")
	}
	must(t, clone.WriteFile("/copied", []byte("text overwrite"), 0600))
	must(t, clone.RemoveAll("/tree"))
	wantFile(t, v, "/copied", "\x7fELFvirtual-binary\n")
	wantFile(t, v, "/tree/tool", "\x7fELFvirtual-binary\n")
	info, err := clone.Stat("/copied")
	must(t, err)
	if info.IsVirtualBinary || info.VirtualBinaryOutput != "" || info.VirtualBinarySource != "" || info.Mode != 0755 {
		t.Fatalf("overwrite metadata = %+v", info)
	}
	v.Reset(clone)
	wantFile(t, v, "/copied", "text overwrite")
	if v.Exists("/tree") {
		t.Fatal("reset retained tree removed in snapshot")
	}
	must(t, clone.AppendFile("/copied", []byte("!"), 0))
	wantFile(t, v, "/copied", "text overwrite")
	must(t, v.Remove("/copied"))
	wantFile(t, clone, "/copied", "text overwrite!")
	v.Reset(v)
	if !v.IsDir("/") {
		t.Fatal("self-reset lost root")
	}
}

type wrappedVFS struct{ contract.VFS }

func (w wrappedVFS) Clone() contract.VFS {
	return wrappedVFS{w.VFS.Clone()}
}

func TestResetFromVFSInterface(t *testing.T) {
	source := vfs.NewMemVFS()
	must(t, source.Mkdir("/a", 0750))
	must(t, source.WriteFile("/a/file", []byte("secret"), 0200))
	must(t, source.Symlink("file", "/a/link"))
	dest := vfs.NewMemVFS()
	dest.Reset(wrappedVFS{source})

	info, err := dest.Stat("/a/file")
	must(t, err)
	if info.Mode != 0200 {
		t.Fatalf("reset mode = %#o, want 0200", info.Mode)
	}
	must(t, dest.Chmod("/a/file", 0600))
	wantFile(t, dest, "/a/link", "secret")
	_, err = source.ReadFile("/a/file")
	wantError(t, err, contract.ErrPermission)
}

func TestWalk(t *testing.T) {
	v := vfs.NewMemVFS()
	must(t, v.MkdirAll("/b", 0))
	must(t, v.MkdirAll("/a/y", 0))
	must(t, v.WriteFile("/a/z", nil, 0))
	must(t, v.Symlink("/a", "/link"))
	stop := errors.New("stop")
	for _, tc := range []struct {
		name, skip, stopAt string
		want               []string
		err                error
	}{
		{name: "preorder", want: []string{"/", "/a", "/a/y", "/a/z", "/b", "/link"}},
		{name: "skip directory", skip: "/a", want: []string{"/", "/a", "/b", "/link"}},
		{name: "skip root", skip: "/", want: []string{"/"}},
		{name: "skip from file stops", skip: "/a/z", want: []string{"/", "/a", "/a/y", "/a/z"}, err: fs.SkipDir},
		{name: "stop", stopAt: "/a/y", want: []string{"/", "/a", "/a/y"}, err: stop},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var seen []string
			err := v.Walk("/", func(p string, info *contract.NodeInfo) error {
				seen = append(seen, p)
				if p != info.Path {
					t.Fatalf("path %q differs from metadata %+v", p, info)
				}
				// Walk callbacks can call back into the filesystem.
				must(t, v.Touch(p, time.Time{}))
				info.Path = "/changed"
				if p == tc.skip {
					return fs.SkipDir
				}
				if p == tc.stopAt {
					return stop
				}
				return nil
			})
			wantError(t, err, tc.err)
			if !reflect.DeepEqual(seen, tc.want) {
				t.Fatalf("Walk = %v, want %v", seen, tc.want)
			}
		})
	}
}

func TestDefaultLinuxVFS(t *testing.T) {
	v := vfs.NewDefaultLinuxVFS("/home/custom")
	for _, p := range []string{"/bin", "/etc", "/home", "/home/learner", "/home/custom", "/tmp", "/usr/bin", "/var/log"} {
		if !v.IsDir(p) {
			t.Errorf("%s is missing or not a directory", p)
		}
	}
	wantFile(t, v, "/etc/hostname", "linux-virtual\n")
	data, err := v.ReadFile("/etc/passwd")
	must(t, err)
	if !strings.Contains(string(data), "root:x:0:0:") || !strings.Contains(string(data), ":/home/custom:/bin/bash") {
		t.Fatalf("unexpected passwd contents: %q", data)
	}
	wantFile(t, v, "/etc/os-release", "NAME=\"Debian GNU/Linux\"\nID=debian\n")
}

func TestConcurrentAccess(t *testing.T) {
	v := vfs.NewMemVFS()
	must(t, v.WriteFile("/file", nil, 0))
	const workers, iterations = 12, 30
	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Go(func() {
			f, err := v.OpenFile("/file", os.O_WRONLY|os.O_APPEND, 0)
			if err != nil {
				t.Error(err)
				return
			}
			defer f.Close()
			for j := 0; j < iterations; j++ {
				if _, err := io.WriteString(f, "line\n"); err != nil {
					t.Error(err)
					return
				}
				if _, err := v.Clone().ReadFile("/file"); err != nil {
					t.Error(err)
					return
				}
				if _, err := v.ReadDir("/"); err != nil {
					t.Error(err)
					return
				}
			}
		})
	}
	wg.Wait()
	wantFile(t, v, "/file", strings.Repeat("line\n", workers*iterations))
}
