package corecmds_test

import (
	"bytes"
	"context"
	"errors"
	"slices"
	"strings"
	"testing"
	"time"

	"learn-linux/internal/commands/corecmds"
	"learn-linux/internal/contract"
	"learn-linux/internal/ui"
	"learn-linux/internal/vfs"
)

const home = "/home/learner"

type registry map[string]contract.CommandHandler

func (r registry) Register(name string, fn contract.CommandHandler) { r[name] = fn }
func (r registry) Lookup(name string) (contract.CommandHandler, bool) {
	fn, ok := r[name]
	return fn, ok
}
func (r registry) Names() []string {
	var names []string
	for name := range r {
		names = append(names, name)
	}
	slices.Sort(names)
	return names
}

func TestRegister(t *testing.T) {
	r := registry{}
	corecmds.Register(r)
	want := strings.Fields("cat cp cut echo file find grep head ln ls mkdir mv printf pwd rm sort stat tail touch tree uniq wc")
	if !slices.Equal(r.Names(), want) {
		t.Fatalf("registered %v, want %v", r.Names(), want)
	}
}

type fixture struct {
	t        *testing.T
	ec       contract.ExecContext
	r        registry
	out, err bytes.Buffer
}

func newFixture(t *testing.T) *fixture {
	f := &fixture{t: t, r: registry{}}
	f.ec = contract.ExecContext{
		Ctx: context.Background(), VFS: vfs.NewDefaultLinuxVFS(home),
		Stdout: &f.out, Stderr: &f.err, CWD: home, Home: home, User: "learner",
		UI: ui.NewRenderer(), TermWidth: 80,
	}
	corecmds.Register(f.r)
	return f
}

func (f *fixture) write(name, content string) {
	f.t.Helper()
	if err := f.ec.VFS.WriteFile(f.path(name), []byte(content), 0644); err != nil {
		f.t.Fatal(err)
	}
}

func (f *fixture) mkdir(name string) {
	f.t.Helper()
	if err := f.ec.VFS.MkdirAll(f.path(name), 0755); err != nil {
		f.t.Fatal(err)
	}
}

func (f *fixture) path(name string) string {
	return f.ec.VFS.ResolvePath(f.ec.CWD, f.ec.Home, name)
}

func (f *fixture) run(command, stdin string, args ...string) (int, string, string) {
	f.t.Helper()
	f.out.Reset()
	f.err.Reset()
	f.ec.Stdin = strings.NewReader(stdin)
	handler, ok := f.r.Lookup(command)
	if !ok {
		f.t.Fatalf("%s is not registered", command)
	}
	code := handler(&f.ec, args)
	return code, f.out.String(), f.err.String()
}

func (f *fixture) assertContent(name, want string) {
	f.t.Helper()
	data, err := f.ec.VFS.ReadFile(f.path(name))
	if err != nil || string(data) != want {
		f.t.Fatalf("%s: got %q (%v), want %q", name, data, err, want)
	}
}

func (f *fixture) prepare() {
	f.t.Helper()
	f.mkdir("sample/src/nested")
	f.mkdir("sample/emptydir")
	f.write("sample/src/a.txt", "Red\nblue\nred\n")
	f.write("sample/src/nested/config.YAML", "red\npear\n")
	f.write("sample/src/.hidden", "hidden\n")
	f.write("sample/src/empty", "")
	f.write("sample/partial", "abc")
	f.write("sample/csv", "name,score,rank\nAda,20,1\nLin,4,2\n")
	f.write("sample/script", "#!/bin/bash\necho ok\n")
	f.write("sample/archive", "\x1f\x8b")
	f.write("sample/unicode", "café\n")
	if err := f.ec.VFS.Symlink("src/a.txt", f.path("sample/link")); err != nil {
		f.t.Fatal(err)
	}
	if err := f.ec.VFS.SetVirtualBinary(f.path("sample/binary"), 0755, "", ""); err != nil {
		f.t.Fatal(err)
	}
}

func TestCommandOutput(t *testing.T) {
	tests := []struct {
		name     string
		command  string
		args     []string
		stdin    string
		code     int
		exact    string
		contains []string
		excludes []string
		err      string
	}{
		{name: "pwd", command: "pwd", exact: home + "\n"},
		{name: "ls flags and dots", command: "ls", args: []string{"-alh", "sample/src"}, contains: []string{"total ", " .\n", " ..\n", " .hidden\n", " a.txt\n"}},
		{name: "ls files first", command: "ls", args: []string{"-1", "sample/src", "sample/partial"}, exact: "sample/partial\n\nsample/src:\na.txt\nempty\nnested\n"},
		{name: "ls reverse size", command: "ls", args: []string{"-Sr", "sample/src"}, exact: "nested\nempty\na.txt\n"},
		{name: "ls recursive", command: "ls", args: []string{"-R", "sample/src"}, contains: []string{"sample/src:\n", "sample/src/nested:\nconfig.YAML\n"}, excludes: []string{".hidden"}},
		{name: "ls color", command: "ls", args: []string{"--color=always", "sample/src"}, contains: []string{"\x1b[", "nested"}},
		{name: "ls missing", command: "ls", args: []string{"missing"}, code: 1, err: "no such file or directory"},
		{name: "tree depth", command: "tree", args: []string{"-aL1", "sample/src"}, contains: []string{"sample/src\n", ".hidden", "nested", "1 directory, 3 files"}, excludes: []string{"config.YAML"}},
		{name: "tree directories", command: "tree", args: []string{"-d", "sample/src"}, exact: "sample/src\n└── nested\n\n1 directory, 0 files\n"},
		{name: "tree invalid depth", command: "tree", args: []string{"-L", "0"}, code: 1, err: "invalid count"},
		{name: "cat numbered", command: "cat", args: []string{"-n", "sample/partial", "-", "sample/src/a.txt"}, stdin: "def\n\n", exact: "     1\tabcdef\n     2\t\n     3\tRed\n     4\tblue\n     5\tred\n"},
		{name: "cat stdin", command: "cat", stdin: "one\ntwo", exact: "one\ntwo"},
		{name: "cat directory", command: "cat", args: []string{"sample/src"}, code: 1, err: "Is a directory"},
		{name: "head shorthand", command: "head", args: []string{"-2"}, stdin: "a\nb\nc", exact: "a\nb\n"},
		{name: "head file headers and bytes", command: "head", args: []string{"-c2", "sample/partial", "sample/src/a.txt"}, exact: "==> sample/partial <==\nab\n==> sample/src/a.txt <==\nRe"},
		{name: "head missing", command: "head", args: []string{"missing"}, code: 1, err: "no such file or directory"},
		{name: "tail starting line", command: "tail", args: []string{"-n", "+2", "sample/src/a.txt"}, exact: "blue\nred\n"},
		{name: "tail shorthand", command: "tail", args: []string{"-2"}, stdin: "a\nb\nc", exact: "b\nc"},
		{name: "tail bytes", command: "tail", args: []string{"-c", "2"}, stdin: "abcdef", exact: "ef"},
		{name: "wc defaults", command: "wc", stdin: "a b\nc", exact: "1 3 5\n"},
		{name: "wc files and total", command: "wc", args: []string{"-lwcm", "sample/unicode", "sample/partial"}, exact: "1 1 5 6 sample/unicode\n0 1 3 3 sample/partial\n1 2 8 9 total\n"},
		{name: "sort numeric key", command: "sort", args: []string{"-rnk2", "-t,"}, stdin: "x,2\ny,10\nz,-3\n", exact: "y,10\nx,2\nz,-3\n"},
		{name: "sort unique folded", command: "sort", args: []string{"-fu"}, stdin: "Beta\nalpha\nALPHA\n", exact: "alpha\nBeta\n"},
		{name: "uniq counts only duplicates", command: "uniq", args: []string{"-cdi"}, stdin: "Alpha\nalpha\nother\nother\nAlpha\n", exact: "      2 Alpha\n      2 other\n"},
		{name: "uniq only singletons", command: "uniq", args: []string{"-u"}, stdin: "a\nb\nb\na\n", exact: "a\na\n"},
		{name: "cut csv", command: "cut", args: []string{"-d", ",", "-f", "1", "sample/csv"}, exact: "name\nAda\nLin\n"},
		{name: "cut field ranges", command: "cut", args: []string{"-d,", "-f1,3-4,6-"}, stdin: "a,b,c,d,e,f,g\nuntouched\n", exact: "a,c,d,f,g\nuntouched\n"},
		{name: "cut character ranges", command: "cut", args: []string{"-c-2,4"}, stdin: "aébcd\n", exact: "aéc\n"},
		{name: "cut invalid range", command: "cut", args: []string{"-f", "4-2"}, code: 1, err: "invalid field or character range"},
		{name: "echo escapes", command: "echo", args: []string{"-ne", "a\\nb\\tc\\rd\\\\e\\x1b"}, exact: "a\nb\tc\rd\\e\x1b"},
		{name: "echo literal by default", command: "echo", args: []string{"one", "two"}, exact: "one two\n"},
		{name: "printf recycling", command: "printf", args: []string{"%5s %03d %.2f %x %%\\n", "test", "7", "1.25", "16", "a", "1", "2", "3"}, exact: " test 007 1.25 10 %\n    a 001 2.00 3 %\n"},
		{name: "printf escapes and defaults", command: "printf", args: []string{"%s\\t%d\\\\\\n", "a"}, exact: "a\t0\\\n"},
		{name: "printf invalid number", command: "printf", args: []string{"%d", "xx"}, code: 1, exact: "0", err: "invalid number"},
		{name: "stat", command: "stat", args: []string{"sample/partial", "sample/src", "sample/link"}, contains: []string{"File: sample/partial", "Size: 3", "Type: regular file", "Access: (0644/-rw-r--r--)", "Uid: (learner)", "Gid: (learner)", "Modify: ", "Type: directory", "Type: symbolic link"}},
		{name: "file classifications", command: "file", args: []string{"sample/src", "sample/link", "sample/binary", "sample/script", "sample/archive", "sample/src/empty", "sample/partial", "sample/unicode"}, exact: "sample/src: directory\nsample/link: symbolic link to src/a.txt\nsample/binary: ELF 64-bit LSB executable (virtual)\nsample/script: Bourne-Again shell script, ASCII text executable\nsample/archive: gzip compressed data\nsample/src/empty: empty\nsample/partial: ASCII text\nsample/unicode: UTF-8 Unicode text\n"},
		{name: "grep recursive case insensitive", command: "grep", args: []string{"-rn", "-i", "RED", "sample/src"}, exact: "sample/src/a.txt:1:Red\nsample/src/a.txt:3:red\nsample/src/nested/config.YAML:1:red\n"},
		{name: "grep extended and count", command: "grep", args: []string{"-Ec", "^(blue|red)$", "sample/src/a.txt"}, exact: "2\n"},
		{name: "grep filenames", command: "grep", args: []string{"-Fl", "red", "sample/src/a.txt", "sample/partial"}, exact: "sample/src/a.txt\n"},
		{name: "grep invert and fixed", command: "grep", args: []string{"-Fvn", "a.b"}, stdin: "axb\na.b\n", exact: "1:axb\n"},
		{name: "grep basic regex", command: "grep", args: []string{"a\\+"}, stdin: "none\naaa\n", exact: "aaa\n"},
		{name: "grep bare color option", command: "grep", args: []string{"--color", "red"}, stdin: "red\n", contains: []string{"\x1b[", "red"}},
		{name: "grep no match", command: "grep", args: []string{"missing"}, stdin: "other\n", code: 1},
		{name: "grep invalid regex", command: "grep", args: []string{"-E", "["}, code: 2, err: "error parsing regexp"},
		{name: "grep file error with matches", command: "grep", args: []string{"red", "missing", "sample/src/a.txt"}, code: 2, exact: "sample/src/a.txt:red\n", err: "no such file or directory"},
		{name: "find name and type", command: "find", args: []string{"sample/src", "-iname", "*.yaml", "-type", "f"}, exact: "sample/src/nested/config.YAML\n"},
		{name: "find root spelling and maximum", command: "find", args: []string{"./sample/src", "-mindepth", "1", "-maxdepth", "1", "-name", "*.txt", "-type", "f"}, exact: "./sample/src/a.txt\n"},
		{name: "find empty", command: "find", args: []string{"sample", "-empty", "!", "-type", "l"}, exact: "sample/emptydir\nsample/src/empty\n"},
		{name: "find negation", command: "find", args: []string{"sample", "-maxdepth", "1", "-not", "-type", "f", "-not", "-type", "d"}, exact: "sample/link\n"},
		{name: "find missing", command: "find", args: []string{"missing"}, code: 1, err: "no such file or directory"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := newFixture(t)
			f.prepare()
			code, stdout, stderr := f.run(tt.command, tt.stdin, tt.args...)
			if code != tt.code {
				t.Fatalf("exit code %d, want %d; stderr: %q", code, tt.code, stderr)
			}
			if tt.err == "" && stderr != "" || tt.err != "" && !strings.Contains(stderr, tt.err) {
				t.Errorf("stderr = %q, want %q", stderr, tt.err)
			}
			if len(tt.contains) == 0 && stdout != tt.exact {
				t.Errorf("stdout = %q, want %q", stdout, tt.exact)
			}
			for _, fragment := range tt.contains {
				if !strings.Contains(stdout, fragment) {
					t.Errorf("stdout = %q, missing %q", stdout, fragment)
				}
			}
			for _, fragment := range tt.excludes {
				if strings.Contains(stdout, fragment) {
					t.Errorf("stdout = %q, unexpectedly contains %q", stdout, fragment)
				}
			}
		})
	}
}

func TestFileCommands(t *testing.T) {
	tests := []struct {
		name, command string
		args          []string
		code          int
		output, err   string
		verify        func(*fixture)
	}{
		{"mkdir parents and multiple operands", "mkdir", []string{"-p", "sample/src", "~/new/sub", "another"}, 0, "", "", func(f *fixture) {
			for _, name := range []string{"new/sub", "another"} {
				if !f.ec.VFS.IsDir(f.path(name)) {
					f.t.Errorf("directory %s not created", name)
				}
			}
		}},
		{"mkdir missing operand", "mkdir", nil, 1, "", "missing operand", nil},
		{"mkdir existing directory", "mkdir", []string{"sample/src"}, 1, "", "file exists", nil},
		{"touch multiple and double dash", "touch", []string{"sample/partial", "--", "-file"}, 0, "", "", func(f *fixture) {
			f.assertContent("-file", "")
			f.assertContent("sample/partial", "abc")
			info, err := f.ec.VFS.Stat(f.path("sample/partial"))
			if err != nil || time.Since(info.ModTime) > time.Minute {
				f.t.Fatalf("touch did not update time: %v, %v", info, err)
			}
		}},
		{"touch missing parent", "touch", []string{"missing/a"}, 1, "", "no such file or directory", nil},
		{"cp recursive verbose", "cp", []string{"-rv", "sample/src", "newsrc"}, 0, "'sample/src' -> 'newsrc'\n", "", func(f *fixture) {
			f.assertContent("newsrc/nested/config.YAML", "red\npear\n")
		}},
		{"cp recursive directory merge", "cp", []string{"-R", "sample/src", "sample/src/nested", "sample/emptydir"}, 0, "", "", func(f *fixture) {
			f.assertContent("sample/emptydir/src/nested/config.YAML", "red\npear\n")
			f.assertContent("sample/emptydir/nested/config.YAML", "red\npear\n")
			if code, _, stderr := f.run("cp", "", "-R", "sample/src", "sample/emptydir"); code != 0 {
				f.t.Fatalf("copy into populated directory: exit=%d, stderr=%q", code, stderr)
			}
		}},
		{"cp multiple to directory", "cp", []string{"-f", "sample/partial", "sample/csv", "sample/emptydir"}, 0, "", "", func(f *fixture) {
			f.assertContent("sample/emptydir/partial", "abc")
			f.assertContent("sample/emptydir/csv", "name,score,rank\nAda,20,1\nLin,4,2\n")
		}},
		{"cp omits directory", "cp", []string{"sample/src", "newsrc"}, 1, "", "omitting directory", nil},
		{"mv multiple forced and verbose", "mv", []string{"-fv", "sample/partial", "sample/csv", "sample/emptydir"}, 0, "renamed 'sample/partial' -> 'sample/emptydir/partial'\nrenamed 'sample/csv' -> 'sample/emptydir/csv'\n", "", func(f *fixture) {
			f.assertContent("sample/emptydir/partial", "abc")
			if f.ec.VFS.Exists(f.path("sample/partial")) || f.ec.VFS.Exists(f.path("sample/csv")) {
				f.t.Error("sources still present after move")
			}
		}},
		{"mv missing source", "mv", []string{"missing", "new"}, 1, "", "no such file or directory", nil},
		{"rm recursive force verbose", "rm", []string{"-rvf", "sample/src", "missing"}, 0, "removed directory 'sample/src'\n", "", func(f *fixture) {
			if f.ec.VFS.Exists(f.path("sample/src")) {
				f.t.Error("directory still present after removal")
			}
		}},
		{"rm missing without force", "rm", []string{"-ri", "missing"}, 1, "", "no such file or directory", nil},
		{"rm directory without recursion", "rm", []string{"sample/emptydir"}, 1, "", "Is a directory", nil},
		{"ln symbolic", "ln", []string{"-s", "sample/partial", "alias"}, 0, "", "", func(f *fixture) {
			target, err := f.ec.VFS.Readlink(f.path("alias"))
			if err != nil || target != "sample/partial" {
				f.t.Fatalf("link target = %q (%v)", target, err)
			}
			f.assertContent("alias", "abc")
		}},
		{"ln with implied basename", "ln", []string{"-s", fqn("sample/partial")}, 0, "", "", func(f *fixture) {
			f.assertContent("partial", "abc")
		}},
		{"ln into directory", "ln", []string{"-s", "../partial", "sample/emptydir"}, 0, "", "", func(f *fixture) {
			f.assertContent("sample/emptydir/partial", "abc")
		}},
		{"ln forced replacement", "ln", []string{"-sf", "partial", "sample/link"}, 0, "", "", func(f *fixture) {
			f.assertContent("sample/link", "abc")
		}},
		{"ln hard link fallback", "ln", []string{"sample/partial", "copy"}, 0, "", "", func(f *fixture) {
			f.assertContent("copy", "abc")
		}},
		{"ln hard link directory", "ln", []string{"sample/src", "copy"}, 1, "", "hard link not allowed for directory", nil},
		{"uniq writes output file", "uniq", []string{"sample/src/a.txt", "output"}, 0, "", "", func(f *fixture) {
			f.assertContent("output", "Red\nblue\nred\n")
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := newFixture(t)
			f.prepare()
			code, stdout, stderr := f.run(tt.command, "", tt.args...)
			if code != tt.code || stdout != tt.output || (tt.err == "" && stderr != "") || !strings.Contains(stderr, tt.err) {
				t.Fatalf("exit=%d, stdout=%q, stderr=%q; want exit=%d, stdout=%q, stderr containing %q", code, stdout, stderr, tt.code, tt.output, tt.err)
			}
			if tt.verify != nil {
				tt.verify(f)
			}
		})
	}
}

func fqn(name string) string { return home + "/" + name }

func TestDestructiveRemove(t *testing.T) {
	for _, args := range [][]string{
		{"-rf", "/"}, {"-fr", "/*"}, {"-rf", "--no-preserve-root", "/"},
		{"-R", "../../"}, {"-rf", "/bin"}, {"-rf", "/etc", "/usr", "/bin"},
		{"-r", "/bin", "/etc", "/home", "/tmp", "/usr", "/var"},
		{"-rf", "sample/partial", "/"},
	} {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			f := newFixture(t)
			f.prepare()
			var calls []string
			f.ec.OnDestructive = func(cmd string) { calls = append(calls, cmd) }
			code, stdout, stderr := f.run("rm", "", args...)
			if code != 0 || stdout != "" || stderr != "" || !slices.Equal(calls, []string{"rm " + strings.Join(args, " ")}) {
				t.Fatalf("exit=%d, stdout=%q, stderr=%q, callbacks=%q", code, stdout, stderr, calls)
			}
			f.assertContent("sample/partial", "abc")
			if !f.ec.VFS.IsDir("/bin") {
				t.Fatal("root filesystem was modified")
			}
		})
	}
	f := newFixture(t)
	f.ec.UI = nil
	if code, _, stderr := f.run("rm", "", "-rf", "/"); code != 1 || stderr != "rm: it is dangerous to operate recursively on '/'\n" {
		t.Fatalf("without callback: exit=%d, stderr=%q", code, stderr)
	}
}

func TestSortUniqPipeline(t *testing.T) {
	f := newFixture(t)
	code, sorted, stderr := f.run("sort", "pear\napple\npear\nbanana\napple\n")
	if code != 0 || stderr != "" {
		t.Fatalf("sort: exit=%d, stderr=%q", code, stderr)
	}
	code, counted, stderr := f.run("uniq", sorted, "-c")
	if code != 0 || counted != "      2 apple\n      1 banana\n      2 pear\n" || stderr != "" {
		t.Fatalf("uniq: exit=%d, stdout=%q, stderr=%q", code, counted, stderr)
	}
}

func TestTTYRenderingAndFallback(t *testing.T) {
	f := newFixture(t)
	f.prepare()
	f.ec.IsTTY = true
	for _, tt := range []struct {
		command string
		args    []string
		color   bool
	}{
		{"ls", []string{"sample/src"}, true},
		{"ls", []string{"-1", "--color=auto", "sample/src"}, true},
		{"ls", []string{"--color=never", "sample/src"}, false},
		{"grep", []string{"-in", "red", "sample/src/a.txt"}, true},
		{"grep", []string{"--color=never", "red", "sample/src/a.txt"}, false},
		{"tree", []string{"-L1", "sample/src"}, true},
	} {
		code, stdout, stderr := f.run(tt.command, "", tt.args...)
		if code != 0 || stderr != "" || strings.Contains(stdout, "\x1b[") != tt.color {
			t.Errorf("%s %v: exit=%d, stdout=%q, stderr=%q, want color=%t", tt.command, tt.args, code, stdout, stderr, tt.color)
		}
	}
	f.ec.UI = nil
	for _, tt := range []struct {
		command, want string
		args          []string
	}{
		{"grep", "1:Red\n3:red\n", []string{"-in", "red", "sample/src/a.txt"}},
		{"tree", "sample/src\n└── nested\n\n1 directory, 0 files\n", []string{"-d", "sample/src"}},
		{"ls", "a.txt\nempty\nnested\n", []string{"-1", "sample/src"}},
	} {
		code, stdout, stderr := f.run(tt.command, "", tt.args...)
		if code != 0 || stdout != tt.want || stderr != "" {
			t.Errorf("%s %v: exit=%d, stdout=%q, stderr=%q", tt.command, tt.args, code, stdout, stderr)
		}
	}
	if err := f.ec.VFS.Touch(f.path("sample/src/a.txt"), time.Unix(1, 0)); err != nil {
		t.Fatal(err)
	}
	if code, stdout, _ := f.run("ls", "", "-ltr", "sample/src"); code != 0 || strings.Index(stdout, "a.txt") > strings.Index(stdout, "empty") {
		t.Fatalf("ls -ltr: exit=%d, stdout=%q", code, stdout)
	}
	if code, _, _ := f.run("rm", "", "sample/link"); code != 0 {
		t.Fatalf("rm link exit=%d", code)
	}
	if _, err := f.ec.VFS.Lstat(f.path("sample/link")); !errors.Is(err, contract.ErrNotExist) {
		t.Fatal("symlink still exists")
	}
	f.assertContent("sample/src/a.txt", "Red\nblue\nred\n")
}
