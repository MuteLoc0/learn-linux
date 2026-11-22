package devcmds

import (
	"archive/tar"
	"bytes"
	"context"
	"errors"
	"io"
	"io/fs"
	"maps"
	"path"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"learn-linux/internal/contract"
	"learn-linux/internal/ui"
	"learn-linux/internal/vfs"
)

type testRegistry map[string]contract.CommandHandler

func (r testRegistry) Register(name string, fn contract.CommandHandler) { r[name] = fn }

func (r testRegistry) Lookup(name string) (contract.CommandHandler, bool) {
	fn, ok := r[name]
	return fn, ok
}

func (r testRegistry) Names() []string { return slices.Sorted(maps.Keys(r)) }

type fixture struct {
	t   *testing.T
	ec  *contract.ExecContext
	reg testRegistry
	out bytes.Buffer
	err bytes.Buffer
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	const home = "/home/learner"
	f := &fixture{t: t, reg: make(testRegistry)}
	Register(f.reg)
	f.ec = &contract.ExecContext{
		Ctx: context.Background(), VFS: vfs.NewDefaultLinuxVFS(home),
		CWD: home, Home: home, User: "learner", Host: "fallback", Stdin: strings.NewReader(""),
		Stdout: &f.out, Stderr: &f.err, UI: ui.NewRenderer(), I18N: testI18N{},
	}
	return f
}

func (f *fixture) run(want int, args ...string) string {
	f.t.Helper()
	f.out.Reset()
	f.err.Reset()
	fn, ok := f.reg.Lookup(args[0])
	if !ok {
		f.t.Fatalf("unregistered command: %s", args[0])
	}
	if got := fn(f.ec, args[1:]); got != want {
		f.t.Fatalf("%q exited %d, want %d; stdout %q, stderr %q", args, got, want, f.out.String(), f.err.String())
	}
	if want == 0 && f.err.Len() > 0 {
		f.t.Fatalf("%q wrote unexpected stderr: %q", args, f.err.String())
	}
	return f.out.String()
}

func (f *fixture) mkdir(p string) {
	f.t.Helper()
	if err := f.ec.VFS.MkdirAll(resolve(f.ec, p), 0755); err != nil {
		f.t.Fatal(err)
	}
}

func (f *fixture) write(p, contents string, mode fs.FileMode) {
	f.t.Helper()
	if err := f.ec.VFS.WriteFile(resolve(f.ec, p), []byte(contents), mode); err != nil {
		f.t.Fatal(err)
	}
}

func (f *fixture) read(p string) string {
	f.t.Helper()
	data, err := f.ec.VFS.ReadFile(resolve(f.ec, p))
	if err != nil {
		f.t.Fatal(err)
	}
	return string(data)
}

func (f *fixture) info(p string) *contract.NodeInfo {
	f.t.Helper()
	info, err := f.ec.VFS.Lstat(resolve(f.ec, p))
	if err != nil {
		f.t.Fatal(err)
	}
	return info
}

func (f *fixture) exists(p string, want bool) {
	f.t.Helper()
	if got := f.ec.VFS.Exists(resolve(f.ec, p)); got != want {
		f.t.Fatalf("exists(%s) = %v, want %v", p, got, want)
	}
}

func TestChmod(t *testing.T) {
	f := newFixture(t)
	f.mkdir("dir/nested")
	f.write("dir/nested/a", "one", 0644)
	f.write("dir/b", "two", 0666)
	for _, test := range []struct {
		mode string
		want fs.FileMode
	}{{"755", 0755}, {"644", 0644}, {"+x", 0755}, {"0755", 0755}, {"666", 0666}, {"u+rwx,go-w", 0744}, {"a=r", 0444}, {"u+x,g-w,o=r", 0544}} {
		f.run(0, "chmod", test.mode, "dir/nested/a", "dir/b")
		for _, file := range []string{"dir/nested/a", "dir/b"} {
			if got := f.info(file).Mode; got != test.want {
				t.Fatalf("chmod %s: %s has %04o, want %04o", test.mode, file, got, test.want)
			}
		}
	}
	out := f.run(0, "chmod", "-Rv", "a+x", "dir")
	if !strings.Contains(out, "dir/nested/a") || f.info("dir/nested/a").Mode != 0555 {
		t.Fatalf("recursive chmod output %q, child mode %04o", out, f.info("dir/nested/a").Mode)
	}
	f.run(1, "chmod", "644")
	f.run(1, "chmod", "bad", "dir")
	f.run(1, "chmod", "644", "missing", "dir/b")
	if f.info("dir/b").Mode != 0644 {
		t.Fatal("chmod stopped at the first missing operand")
	}
}

func TestTarRoundTrip(t *testing.T) {
	f := newFixture(t)
	f.mkdir("assets/sub/empty")
	f.mkdir("out")
	f.write("assets/sub/a", "binary\x00\xff\n", 0751)
	f.write("assets/readme", "start here\n", 0640)
	if err := f.ec.VFS.Chmod(resolve(f.ec, "assets/sub"), 0710); err != nil {
		t.Fatal(err)
	}
	modTime := time.Unix(1700000000, 123456789)
	for _, name := range []string{"assets/sub/a", "assets/sub"} {
		if err := f.ec.VFS.Touch(resolve(f.ec, name), modTime); err != nil {
			t.Fatal(err)
		}
	}
	if err := f.ec.VFS.Symlink("sub/a", resolve(f.ec, "assets/link")); err != nil {
		t.Fatal(err)
	}

	f.run(0, "tar", "-czf", "assets.tar.gz", "assets")
	const listing = "assets/\nassets/link\nassets/readme\nassets/sub/\nassets/sub/a\nassets/sub/empty/\n"
	if got := f.run(0, "tar", "-tzf", "assets.tar.gz"); got != listing {
		t.Fatalf("archive listing %q, want %q", got, listing)
	}
	if got := f.run(0, "tar", "-tvf", "assets.tar.gz"); !strings.Contains(got, "-rwxr-x--x 9 assets/sub/a\n") {
		t.Fatalf("verbose listing %q", got)
	}

	for _, p := range []string{"assets/inner.tar", "plain.tar"} {
		f.exists(p, false)
	}
	if got := f.run(0, "tar", "cvf", "plain.tar", "-C", "assets", "sub"); !strings.HasPrefix(got, "sub/\n") {
		t.Fatalf("traditional creation with -C printed %q", got)
	}
	if got := f.run(0, "tar", "-tf", "plain.tar"); got != "sub/\nsub/a\nsub/empty/\n" {
		t.Fatalf("uncompressed archive listing %q", got)
	}

	f.run(0, "tar", "-xzf", "assets.tar.gz", "-C", "out")
	if err := f.ec.VFS.RemoveAll(resolve(f.ec, "assets")); err != nil {
		t.Fatal(err)
	}
	f.run(0, "tar", "-xzf", "assets.tar.gz")
	for _, prefix := range []string{"assets", "out/assets"} {
		if got := f.read(prefix + "/sub/a"); got != "binary\x00\xff\n" {
			t.Fatalf("extracted bytes %q", got)
		}
		if got := f.read(prefix + "/link"); got != "binary\x00\xff\n" {
			t.Fatalf("symlink read %q", got)
		}
		if link := f.info(prefix + "/link"); link.Type != contract.TypeSymlink || link.LinkTarget != "sub/a" {
			t.Fatalf("extracted link: %+v", link)
		}
		if !f.info(prefix + "/sub/empty").IsDir() {
			t.Fatal("empty directory not restored")
		}
		for name, mode := range map[string]fs.FileMode{"sub/a": 0751, "sub": 0710} {
			info := f.info(path.Join(prefix, name))
			if info.Mode != mode || !info.ModTime.Equal(modTime) {
				t.Fatalf("extracted metadata: %+v", info)
			}
		}
	}
}

func TestTarRejectsTraversal(t *testing.T) {
	for _, test := range []struct {
		name    string
		headers []tar.Header
	}{
		{"parent", []tar.Header{{Name: "dir/../../escaped", Typeflag: tar.TypeReg, Size: 1}}},
		{"absolute", []tar.Header{{Name: "/tmp/escaped", Typeflag: tar.TypeReg, Size: 1}}},
		{"symlink parent", []tar.Header{
			{Name: "link", Typeflag: tar.TypeSymlink, Linkname: "/tmp"},
			{Name: "link/escaped", Typeflag: tar.TypeReg, Size: 1},
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			f := newFixture(t)
			f.mkdir("out")
			var buf bytes.Buffer
			w := tar.NewWriter(&buf)
			for _, header := range test.headers {
				if err := w.WriteHeader(&header); err != nil {
					t.Fatal(err)
				}
				if header.Size != 0 {
					if _, err := w.Write([]byte("x")); err != nil {
						t.Fatal(err)
					}
				}
			}
			if err := w.Close(); err != nil {
				t.Fatal(err)
			}
			f.write("bad.tar", buf.String(), 0644)
			f.run(1, "tar", "-xf", "bad.tar", "-C", "out")
			f.exists("escaped", false)
			f.exists("/tmp/escaped", false)
		})
	}
}

func TestGzip(t *testing.T) {
	f := newFixture(t)
	const data = "hello\x00\xff\n"
	f.write("sample", data, 0644)
	f.run(0, "gzip", "-k", "sample")
	f.exists("sample", true)
	f.run(1, "gzip", "sample")
	f.exists("sample", true)
	f.run(0, "gzip", "-f", "sample")
	f.exists("sample", false)
	f.run(0, "gunzip", "-k", "sample.gz")
	f.exists("sample.gz", true)
	if got := f.read("sample"); got != data {
		t.Fatalf("decompressed %q", got)
	}
	f.run(0, "gunzip", "-f", "sample.gz")
	f.exists("sample.gz", false)
	if got := f.read("sample"); got != data {
		t.Fatalf("decompressed %q", got)
	}
	f.ec.Stdin = strings.NewReader(data)
	encoded := f.run(0, "gzip", "-c")
	f.ec.Stdin = strings.NewReader(encoded)
	if got := f.run(0, "gzip", "-dc"); got != data {
		t.Fatalf("stdin gzip round trip: %q", got)
	}
	encoded = f.run(0, "gzip", "-c", "sample")
	f.exists("sample", true)
	f.write("named.gz", encoded, 0644)
	if got := f.run(0, "gunzip", "-c", "named.gz"); got != data {
		t.Fatalf("gunzip stdout: %q", got)
	}
	f.exists("named.gz", true)
}

func TestGCC(t *testing.T) {
	f := newFixture(t)
	f.mkdir("src")
	f.write("src/main.c", `#include <stdio.h>
int main(void) {
  // puts("ignored"); }
  printf("hello\n\t\"C\"\\");
  puts(" {world}");
  return 0;
}
`, 0644)
	f.write("src/extra.c", "int helper(void) { return 1; }\n", 0644)
	f.run(0, "gcc", "-o", "app", "-Wall", "-Wextra", "-O3", "-g", "-std=c17", "-I", "src", "src/main.c", "src/extra.c")
	info := f.info("app")
	if !info.IsVirtualBinary || !info.IsExecutable() || info.Mode != 0755 || info.VirtualBinaryOutput != "hello\n\t\"C\"\\ {world}\n" {
		t.Fatalf("gcc executable metadata %+v", info)
	}
	if info.VirtualBinarySource != resolve(f.ec, "src/main.c")+" "+resolve(f.ec, "src/extra.c") {
		t.Fatalf("gcc source paths %q", info.VirtualBinarySource)
	}
	f.run(1, "gcc", "missing.c")
	if got := f.err.String(); got != "gcc: error: missing.c: No such file or directory\n" {
		t.Fatalf("missing operand error %q", got)
	}
	for _, source := range []string{"int main(void) {\n", "int main(void) { return 0 }", "int main(void) {} /* comment"} {
		f.write("src/broken.c", source, 0644)
		f.run(1, "gcc", "src/broken.c")
		f.exists("a.out", false)
	}
	f.run(1, "gcc", "src/extra.c")
	f.write("src/silent.c", "int main(void) { return 0; }\n", 0644)
	f.run(0, "gcc", "src/silent.c")
	if info := f.info("a.out"); !info.IsVirtualBinary || info.VirtualBinaryOutput != "" {
		t.Fatalf("silent gcc executable metadata %+v", info)
	}
}

func TestMake(t *testing.T) {
	f := newFixture(t)
	f.mkdir("project/src")
	f.write("project/src/main.c", `int main(void) { puts("built"); return 0; }`, 0644)
	f.write("project/makefile", `# the default target
CC = gcc
TARGET := app
SRC = src/main.c
all: announce ${TARGET}
${TARGET}: $(SRC) announce
	$(CC) -o $@ $<
announce:
  @echo building
clean:
	rm $(TARGET)
fail:
	fail-recipe
	echo should-not-run
cycle: loop
loop: cycle
`, 0644)

	var seen []string
	f.ec.Env = map[string]string{"PWD": f.ec.CWD, "OLDPWD": "/tmp"}
	f.ec.SetCWD = func(dir string) { t.Fatalf("make changed the caller's directory to %q", dir) }
	f.ec.RunSubcommand = func(ctx context.Context, line string, stdin io.Reader, stdout, stderr io.Writer) int {
		if ctx != f.ec.Ctx || stdin != f.ec.Stdin || stdout != f.ec.Stdout || stderr != f.ec.Stderr {
			t.Fatal("make changed subcommand context or streams")
		}
		cwd := f.ec.CWD
		if cwd != path.Join(f.ec.Home, "project") || f.ec.Env["PWD"] != cwd {
			t.Fatalf("recipe %q ran in %q with PWD=%q", line, cwd, f.ec.Env["PWD"])
		}
		seen = append(seen, line)
		args := strings.Fields(line)
		switch args[0] {
		case "echo":
			_, _ = io.WriteString(stdout, strings.Join(args[1:], " ")+"\n")
			return 0
		case "rm":
			if err := f.ec.VFS.Remove(path.Join(cwd, args[1])); err != nil {
				t.Fatal(err)
			}
			return 0
		case "fail-recipe":
			return 7
		}
		fn, ok := f.reg.Lookup(args[0])
		if !ok {
			t.Fatalf("unhandled recipe: %s", line)
		}
		child := *f.ec
		child.CWD = cwd
		return fn(&child, args[1:])
	}

	if got := f.run(0, "make", "-C", "project", "-n"); got != "echo building\ngcc -o app src/main.c\n" || len(seen) != 0 {
		t.Fatalf("make -n stdout %q, executed %q", got, seen)
	}
	f.exists("project/app", false)
	if got := f.run(0, "make", "-C", "project"); got != "building\ngcc -o app src/main.c\n" ||
		!slices.Equal(seen, []string{"echo building", "gcc -o app src/main.c"}) {
		t.Fatalf("make stdout %q, executed %q", got, seen)
	}
	if f.ec.CWD != f.ec.Home || f.ec.Env["PWD"] != f.ec.Home || f.ec.Env["OLDPWD"] != "/tmp" || f.info("project/app").VirtualBinaryOutput != "built\n" {
		t.Fatal("make did not restore the shell directory or build app")
	}
	f.run(0, "make", "-C", "project", "-f", "makefile", "clean")
	f.exists("project/app", false)
	f.run(7, "make", "-C", "project", "fail")
	if slices.Contains(seen, "echo should-not-run") || f.ec.CWD != f.ec.Home {
		t.Fatalf("make did not stop and restore cwd after a failed recipe: %q, cwd %q", seen, f.ec.CWD)
	}
	f.run(1, "make", "-C", "project", "cycle")
	if !strings.Contains(f.err.String(), "Circular") {
		t.Fatalf("cycle diagnostic %q", f.err.String())
	}
}

func TestSystemAndTutorialCommands(t *testing.T) {
	f := newFixture(t)
	f.ec.History = []string{"pwd", "ls", "history"}
	f.ec.Env = map[string]string{"Z": "two", "A": "one"}
	f.write("/bin/custom", "", 0755)
	for _, test := range []struct {
		args []string
		want string
		code int
	}{
		{[]string{"which", "gcc", "custom", "cd", "missing"}, "/usr/bin/gcc\n/usr/bin/custom\ncd: shell built-in command\n", 1},
		{[]string{"whoami"}, "learner\n", 0},
		{[]string{"hostname"}, "linux-virtual\n", 0},
		{[]string{"uname"}, "Linux\n", 0},
		{[]string{"uname", "-a"}, "Linux linux-virtual 6.8.0-virtual x86_64 GNU/Linux\n", 0},
		{[]string{"uname", "-onm"}, "linux-virtual x86_64 GNU/Linux\n", 0},
		{[]string{"id"}, "uid=1000(learner) gid=1000(learner) groups=1000(learner),27(sudo)\n", 0},
		{[]string{"history"}, "  1  pwd\n  2  ls\n  3  history\n", 0},
		{[]string{"history", "2"}, "  2  ls\n  3  history\n", 0},
		{[]string{"env"}, "A=one\nZ=two\n", 0},
		{[]string{"clear"}, "\x1b[2J\x1b[H", 0},
		{[]string{"true"}, "", 0},
		{[]string{"false"}, "", 1},
	} {
		if got := f.run(test.code, test.args...); got != test.want {
			t.Errorf("%v: got %q, want %q", test.args, got, test.want)
		}
	}
	f.ec.User = ""
	if got := f.run(0, "whoami"); got != "learner\n" {
		t.Errorf("default username %q", got)
	}
	if err := f.ec.VFS.Remove("/etc/hostname"); err != nil {
		t.Fatal(err)
	}
	if got := f.run(0, "hostname"); got != "fallback\n" {
		t.Errorf("fallback hostname %q", got)
	}
	before := time.Now().Unix()
	seconds, err := strconv.ParseInt(strings.TrimSpace(f.run(0, "date", "+%s")), 10, 64)
	if err != nil || seconds < before || seconds > time.Now().Unix() {
		t.Errorf("date +%%s returned %d, %v", seconds, err)
	}
	utc := strings.TrimSpace(f.run(0, "date", "-u", "+%Y-%m-%d %H:%M:%S %Z"))
	if _, err := time.Parse("2006-01-02 15:04:05 UTC", utc); err != nil {
		t.Errorf("date -u returned %q: %v", utc, err)
	}

	var got contract.TutorialAction
	f.ec.OnTutorialAction = func(action contract.TutorialAction) (int, error) {
		got = action
		return 9, errors.New("bridge error")
	}
	for _, name := range []string{"mission", "hint", "check", "chapter", "reset", "sandbox", "man", "lang", "help", "exit"} {
		f.run(9, name, "argument")
		if got.Name != name || !slices.Equal(got.Args, []string{"argument"}) || f.err.String() != name+": bridge error\n" {
			t.Errorf("tutorial bridge %s: got %+v, stderr %q", name, got, f.err.String())
		}
	}
}

type testTerminal struct {
	contract.Terminal
	keys    []contract.KeyEvent
	raw     bool
	cleared int
	frames  []string
}

func (t *testTerminal) IsRawMode() bool { return t.raw }

func (t *testTerminal) Size() (int, int) { return 80, 5 }

func (t *testTerminal) ClearScreen() error {
	t.cleared++
	return nil
}

func (t *testTerminal) ReadKey() (contract.KeyEvent, error) {
	if len(t.keys) == 0 {
		return contract.KeyEvent{Kind: contract.KeyEOF}, nil
	}
	key := t.keys[0]
	t.keys = t.keys[1:]
	return key, nil
}

func (t *testTerminal) WriteString(frame string) error {
	t.frames = append(t.frames, frame)
	return nil
}

type testI18N struct {
	contract.I18N
}

func (testI18N) T(key string, args ...any) string {
	switch key {
	case contract.KeyPagerNotFound:
		return "No match for " + args[0].(string)
	case contract.KeyEditorSaved:
		return "Saved " + strconv.Itoa(args[0].(int)) + " lines to " + args[1].(string)
	default:
		return key
	}
}

type recordingUI struct {
	contract.UIRenderer
	pager  []contract.PagerState
	editor []contract.EditorState
}

func (r *recordingUI) RenderPager(i18n contract.I18N, state contract.PagerState) string {
	r.pager = append(r.pager, state)
	return r.UIRenderer.RenderPager(i18n, state)
}

func (r *recordingUI) RenderEditor(i18n contract.I18N, state contract.EditorState) string {
	record := state
	record.Lines = slices.Clone(state.Lines)
	r.editor = append(r.editor, record)
	return r.UIRenderer.RenderEditor(i18n, state)
}

func key(kind contract.KeyKind) contract.KeyEvent { return contract.KeyEvent{Kind: kind} }
func runeKey(r rune) contract.KeyEvent            { return contract.KeyEvent{Kind: contract.KeyRune, Rune: r} }

func (f *fixture) interact(keys ...contract.KeyEvent) (*testTerminal, *recordingUI) {
	term := &testTerminal{raw: true, keys: keys}
	renderer := &recordingUI{UIRenderer: ui.NewRenderer()}
	f.ec.Term, f.ec.UI, f.ec.IsTTY = term, renderer, true
	return term, renderer
}

func TestLess(t *testing.T) {
	f := newFixture(t)
	const content = "one\r\ntwo\r\nSEARCH first\r\nfour\r\nfive\r\nsearch second\r\nseven\r\n"
	f.write("data.txt", content, 0644)
	if got := f.run(0, "less", "data.txt"); got != content {
		t.Fatalf("non-TTY file output %q", got)
	}
	f.ec.Stdin = strings.NewReader("stdin\n")
	if got := f.run(0, "less"); got != "stdin\n" {
		t.Fatalf("non-TTY stdin output %q", got)
	}
	f.run(1, "less", "missing.txt")
	if got := f.err.String(); !strings.Contains(got, "No such file or directory") {
		t.Fatalf("missing file stderr %q", got)
	}

	term, renderer := f.interact(
		runeKey('j'), runeKey('k'), runeKey('/'),
		runeKey('s'), runeKey('e'), runeKey('a'), runeKey('r'), runeKey('x'), key(contract.KeyBackspace), runeKey('c'), runeKey('h'),
		key(contract.KeyEnter), runeKey('n'), runeKey('N'),
		contract.KeyEvent{Kind: contract.KeyResize, Width: 40, Height: 4},
		runeKey('/'), runeKey('z'), key(contract.KeyEnter),
		runeKey('/'), key(contract.KeyEscape), runeKey('q'),
	)
	f.run(0, "less", "data.txt")
	states := renderer.pager
	if term.cleared != 1 || len(term.frames) != len(states) || len(states) != 21 {
		t.Fatalf("less rendered %d states and %d frames, cleared %d times", len(states), len(term.frames), term.cleared)
	}
	if states[1].TopLine != 1 || states[2].TopLine != 0 || !states[3].SearchMode || states[11].SearchInput != "search" {
		t.Fatalf("unexpected scrolling or search input: %+v", states[:12])
	}
	if states[12].SearchMode || !slices.Equal(states[12].MatchLines, []int{2, 5}) ||
		states[12].TopLine != 2 || states[13].TopLine != 5 || states[14].TopLine != 2 {
		t.Fatalf("unexpected search results: %+v", states[12:15])
	}
	if states[15].Width != 40 || states[15].Height != 4 || states[18].StatusMsg != "No match for z" || states[20].SearchMode {
		t.Fatalf("unexpected resize or search status: %+v", states[15:])
	}

	term, renderer = f.interact()
	term.raw = false
	if got := f.run(0, "less", "data.txt"); got != content || len(renderer.pager) != 0 {
		t.Fatalf("cooked terminal output %q, rendered %d frames", got, len(renderer.pager))
	}
	f.run(0, "clear")
	if term.cleared != 1 {
		t.Fatal("clear did not use the terminal")
	}
}

func TestNanoEditing(t *testing.T) {
	f := newFixture(t)
	f.write("edit.txt", "ab\r\néx\r\n", 0644)
	term, renderer := f.interact(
		key(contract.KeyEnd), key(contract.KeyEnter), runeKey('Z'), key(contract.KeyBackspace), key(contract.KeyBackspace),
		key(contract.KeyRight), key(contract.KeyRight), key(contract.KeyDelete), runeKey('λ'),
		key(contract.KeyUp), key(contract.KeyLeft), runeKey('β'), key(contract.KeyDown),
		contract.KeyEvent{Kind: contract.KeyResize, Width: 60, Height: 5},
		key(contract.KeyCtrlO), key(contract.KeyCtrlX),
	)
	f.run(0, "nano", "edit.txt")
	if got := f.read("edit.txt"); got != "aβb\néλ\n" {
		t.Fatalf("edited content %q", got)
	}
	states := renderer.editor
	last := states[len(states)-1]
	if last.StatusMsg != "Saved 2 lines to edit.txt" || last.Modified || last.ConfirmExit || term.cleared != 1 {
		t.Fatalf("saved editor state %+v, clear count %d", last, term.cleared)
	}
	if last.CursorRow != 1 || last.CursorCol != 2 || last.ScrollTop != 1 || last.Width != 60 || last.Height != 5 {
		t.Fatalf("unexpected cursor or viewport %+v", last)
	}
}

func TestNanoConfirmExit(t *testing.T) {
	for _, answer := range []rune{'y', 'n'} {
		t.Run(string(answer), func(t *testing.T) {
			f := newFixture(t)
			term, renderer := f.interact(
				runeKey('A'), key(contract.KeyEnter), runeKey('B'), key(contract.KeyCtrlK),
				key(contract.KeyCtrlX), key(contract.KeyEscape), runeKey('C'), key(contract.KeyCtrlX), runeKey(answer),
			)
			f.run(0, "nano")
			states := renderer.editor
			if !states[5].ConfirmExit || states[6].ConfirmExit || !states[len(states)-1].ConfirmExit || term.cleared != 1 {
				t.Fatalf("unexpected confirmation states %+v, clear count %d", states, term.cleared)
			}
			f.exists("untitled.txt", answer == 'y')
			if answer == 'y' && f.read("untitled.txt") != "A\nC\n" {
				t.Fatalf("saved confirmation content %q", f.read("untitled.txt"))
			}
		})
	}
	f := newFixture(t)
	f.run(1, "nano", "missing.txt")
	if f.err.String() != "nano: interactive terminal required\n" {
		t.Fatalf("missing terminal stderr %q", f.err.String())
	}
	f.interact()
	f.run(1, "nano", ".")
	if !strings.Contains(f.err.String(), "Is a directory") {
		t.Fatalf("directory stderr %q", f.err.String())
	}
}
