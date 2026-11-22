package shell

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"learn-linux/internal/contract"
	"learn-linux/internal/vfs"
)

const home = "/home/learner"

func newTestEngine(t *testing.T, config Config) (*Engine, *vfs.MemVFS) {
	t.Helper()
	fs := vfs.NewDefaultLinuxVFS(home)
	config.VFS = fs
	if config.Registry == nil {
		reg := NewRegistry()
		reg.Register("cat", func(ec *contract.ExecContext, args []string) int {
			if len(args) == 0 {
				if ec.Stdin != nil {
					_, _ = io.Copy(ec.Stdout, ec.Stdin)
				}
				return 0
			}
			for _, arg := range args {
				data, err := ec.VFS.ReadFile(ec.VFS.ResolvePath(ec.CWD, ec.Home, arg))
				if err != nil {
					fmt.Fprintln(ec.Stderr, err)
					return 1
				}
				_, _ = ec.Stdout.Write(data)
			}
			return 0
		})
		reg.Register("mkdir", func(ec *contract.ExecContext, args []string) int {
			for _, arg := range args {
				if arg == "-p" {
					continue
				}
				if err := ec.VFS.MkdirAll(ec.VFS.ResolvePath(ec.CWD, ec.Home, arg), 0755); err != nil {
					fmt.Fprintln(ec.Stderr, err)
					return 1
				}
			}
			return 0
		})
		reg.Register("ls", func(ec *contract.ExecContext, args []string) int {
			fmt.Fprintln(ec.Stdout, strings.Join(args, " "))
			return 0
		})
		config.Registry = reg
	}
	engine, err := NewEngine(config)
	must(t, err)
	return engine, fs
}

func must(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}

func run(t *testing.T, e *Engine, line string, code int, stdout, stderr string) contract.CommandRecord {
	t.Helper()
	record, err := e.Execute(context.Background(), line)
	must(t, err)
	if record.Raw != line || record.ExitCode != code || record.Stdout != stdout || record.Stderr != stderr {
		t.Fatalf("Execute(%q) = %+v, want status %d, stdout %q, stderr %q",
			line, record, code, stdout, stderr)
	}
	return record
}

func TestShellSyntaxAndIO(t *testing.T) {
	e, fs := newTestEngine(t, Config{})
	must(t, fs.Mkdir(home+"/events", 0755))
	must(t, fs.WriteFile(home+"/events/notes_a.txt", nil, 0644))
	must(t, fs.WriteFile(home+"/events/notes_b.txt", nil, 0644))
	must(t, fs.WriteFile(home+"/b.txt", nil, 0644))
	must(t, fs.WriteFile(home+"/.hidden.txt", nil, 0644))
	cases := []struct {
		line   string
		code   int
		output string
	}{
		{"echo hello | cat", 0, "hello\n"},
		{"echo first > a.txt && printf '%s\\n' second >> a.txt; cat < a.txt", 0, "first\nsecond\n"},
		{"printf discarded > /dev/null; cat < /dev/null", 0, ""},
		{"missing-command 2> errors || cat errors", 0, "bash: missing-command: command not found\n"},
		{"missing-command 2>&1 | cat", 0, "bash: missing-command: command not found\n"},
		{"false && echo fail || echo recovered; true && echo chained", 0, "recovered\nchained\n"},
		{"VALUE=expanded; printf '%s\\n' '$VALUE' \"$VALUE\" one\\ two", 0, "$VALUE\nexpanded\none two\n"},
		{"printf '%s\\n' *.txt [a-b].txt ?.txt events/notes_*.txt", 0,
			"a.txt\nb.txt\na.txt\nb.txt\na.txt\nb.txt\nevents/notes_a.txt\nevents/notes_b.txt\n"},
		{"printf '%s\\n' \"$(<a.txt)\"", 0, "first\nsecond\n"},
		{"test -f a.txt && [[ -r a.txt && -c /dev/null ]]", 0, ""},
		{"false", 1, ""},
		{"echo $?", 0, "1\n"},
	}
	for _, tc := range cases {
		t.Run(tc.line, func(t *testing.T) { run(t, e, tc.line, tc.code, tc.output, "") })
	}
	record := run(t, e, "echo 'one two' | cat", 0, "one two\n", "")
	if record.Command != "echo" || !slices.Equal(record.Args, []string{"one two"}) {
		t.Fatalf("primary command = %q %q", record.Command, record.Args)
	}
}

func TestCDInVFS(t *testing.T) {
	e, fs := newTestEngine(t, Config{})
	must(t, fs.Mkdir(home+"/build", 0755))
	must(t, fs.MkdirAll(home+"/workspace/a/b", 0755))
	run(t, e, "cd /home/learner/build && pwd", 0, home+"/build\n", "")
	run(t, e, "cd ..", 0, "", "")
	run(t, e, "cd workspace/a/b; cd ..; printf '%s|%s' \"$PWD\" \"$OLDPWD\"", 0,
		home+"/workspace/a|"+home+"/workspace/a/b", "")
	run(t, e, "cd ~ && pwd", 0, home+"\n", "")
	record := run(t, e, "cd -", 0, home+"/workspace/a\n", "")
	if record.CWD != home+"/workspace/a" || e.CWD() != record.CWD ||
		e.Env()["PWD"] != record.CWD || e.Env()["OLDPWD"] != home {
		t.Fatalf("cwd did not persist: %+v, %v", record, e.Env())
	}
	run(t, e, "mkdir -p child && cd child && pwd -P", 0, home+"/workspace/a/child\n", "")
	must(t, fs.Symlink(home+"/build", home+"/build-link"))
	run(t, e, "cd ~/build-link; pwd; builtin pwd -P", 0, home+"/build-link\n"+home+"/build\n", "")
	e.SetCWD(home)
	run(t, e, "cd -; pwd", 0, home+"/build-link\n"+home+"/build-link\n", "")
}

func TestPOSIXPathsAfterCD(t *testing.T) {
	e, fs := newTestEngine(t, Config{})
	events := home + "/events"
	must(t, fs.MkdirAll(events+"/sub", 0755))
	must(t, fs.Mkdir(events+"/[notes]", 0755))
	must(t, fs.WriteFile(events+"/notes_a.txt", []byte("one\n"), 0644))
	must(t, fs.WriteFile(events+"/notes_b.txt", []byte("two\n"), 0644))
	must(t, fs.WriteFile(events+"/[notes]/read.txt", nil, 0644))
	must(t, fs.WriteFile(events+"/C:\\notes.txt", []byte("virtual\n"), 0644))
	must(t, fs.Symlink(".", events+"/loop"))
	must(t, fs.Symlink(events+"/sub", home+"/sub-link"))
	must(t, fs.Symlink(events+"/notes_a.txt", events+"/first.txt"))

	run(t, e, "cd && cd events && cat notes_*.txt > combined; cat combined", 0, "one\ntwo\n", "")
	run(t, e, "printf '%s\\n' ../events/notes_*.txt /home/learner/events/notes_*.txt '[notes]'/*.txt",
		0, "../events/notes_a.txt\n../events/notes_b.txt\n"+events+"/notes_a.txt\n"+events+"/notes_b.txt\n[notes]/read.txt\n", "")
	run(t, e, "cat C:*.txt; test -f 'C:\\notes.txt' && [[ -f 'C:\\notes.txt' ]]", 0, "virtual\n", "")
	run(t, e, "printf '%s\\n' nonexistent/*; shopt -s nullglob; printf '<%s>\\n' nonexistent/*", 0,
		"nonexistent/*\n<>\n", "")
	run(t, e, "shopt -s globstar; printf '%s\\n' **/read.txt", 0, "[notes]/read.txt\n", "")
	run(t, e, "[ -f combined ] && [[ -e /home/learner/events/combined && -L first.txt ]] && test -O notes_a.txt -a -G notes_a.txt",
		0, "", "")
	run(t, e, "[[ first.txt -ef notes_a.txt && ! first.txt -ef notes_b.txt ]] && [ ! -e '' ]", 0, "", "")
	run(t, e, "(cd /tmp && pwd && echo child > check); pwd; test ! -e check", 0, "/tmp\n"+events+"\n", "")
	run(t, e, "PWD=/tmp; printf '%s\\n' notes_*.txt; echo actual > check", 0, "notes_a.txt\nnotes_b.txt\n", "")
	run(t, e, "pwd; echo \"$PWD\"; cat check; cat /tmp/check", 0, events+"\n/tmp\nactual\nchild\n", "")
	run(t, e, "cd -P ../sub-link/.. && pwd && echo \"$OLDPWD\"", 0, events+"\n"+events+"\n", "")
	run(t, e, "cd -L ../sub-link/.. && pwd && cd -", 0, home+"\n"+events+"\n", "")
	run(t, e, "test -f ../sub-link/../combined && cat < ../sub-link/../combined", 0, "one\ntwo\n", "")
	run(t, e, "PATH=lower path=upper; export PATH path; printf '%s|%s\\n' \"$PATH\" \"$path\"", 0, "lower|upper\n", "")
	run(t, e, "(printf '%s|%s\\n' \"$PATH\" \"$path\")", 0, "lower|upper\n", "")
}

func TestVirtualDirectoryStack(t *testing.T) {
	e, _ := newTestEngine(t, Config{})
	run(t, e, "pushd /tmp", 0, "/tmp "+home+"\n", "")
	run(t, e, "cd /; dirs", 0, "/ "+home+"\n", "")
	run(t, e, "(popd && pwd); pwd", 0, home+"\n"+home+"\n/\n", "")
	run(t, e, "popd; printf '%s|%s\\n' \"$PWD\" \"$OLDPWD\"", 0, home+"\n"+home+"|/\n", "")
	run(t, e, "pushd -n /missing", 0, home+" /missing\n", "")
	run(t, e, "popd", 1, "", "popd: no such file or directory: \"/missing\"\n")
	run(t, e, "dirs; popd -n", 0, home+" /missing\n"+home+"\n", "")
}

func TestSourceAndExecutablePATH(t *testing.T) {
	e, fs := newTestEngine(t, Config{})
	must(t, fs.WriteFile("/bin/task", []byte("printf 'task:%s:%s\\n' \"$PWD\" \"$1\"\n"), 0755))
	must(t, fs.WriteFile("/bin/settings", []byte("export LOADED=$1\ncd /tmp\nreturn 3\necho unreachable\n"), 0644))
	must(t, fs.WriteFile(home+"/local-task", []byte("printf 'local:%s\\n' \"$1\"\n"), 0755))
	run(t, e, "cd /tmp && task argument && command -v task && type -t task", 0, "task:/tmp:argument\n/bin/task\nfile\n", "")
	run(t, e, "cd; set -- original; source settings 'one two'; printf '%s:%s:%s:%s\\n' \"$?\" \"$LOADED\" \"$1\" \"$PWD\"",
		0, "3:one two:original:/tmp\n", "")
	run(t, e, "cd; PATH=:/bin; command -v local-task; local-task data; exec task last; echo unreachable",
		0, home+"/local-task\nlocal:data\ntask:"+home+":last\n", "")
	run(t, e, "builtin export KEY='~literal $(echo untouched)'; command export SECOND=value; printf '%s|%s\\n' \"$KEY\" \"$SECOND\"",
		0, "~literal $(echo untouched)|value\n", "")
	run(t, e, "command -v umask", 1, "", "")
}

func TestSetVFSFromRegisteredAction(t *testing.T) {
	e, oldFS := newTestEngine(t, Config{})
	newFS := vfs.NewDefaultLinuxVFS(home)
	must(t, newFS.Mkdir(home+"/chapter", 0755))
	e.Registry().Register("next-chapter", func(_ *contract.ExecContext, _ []string) int {
		e.SetVFS(newFS, home+"/chapter")
		e.SetEnv("CHAPTER", "next")
		return 0
	})
	record := run(t, e, "next-chapter", 0, "", "")
	if record.CWD != home+"/chapter" || e.VFS() != newFS {
		t.Fatalf("chapter switch did not persist: %+v", record)
	}
	run(t, e, "echo \"$CHAPTER\" > lesson.txt; pwd", 0, home+"/chapter\n", "")
	data, err := newFS.ReadFile(home + "/chapter/lesson.txt")
	must(t, err)
	if string(data) != "next\n" || oldFS.Exists(home+"/chapter/lesson.txt") {
		t.Fatalf("lesson file = %q", data)
	}
}

func TestEnvironmentAndAliases(t *testing.T) {
	e, _ := newTestEngine(t, Config{})
	record := run(t, e, "export BUILD_MODE=release", 0, "", "")
	if record.Command != "export" || !slices.Equal(record.Args, []string{"BUILD_MODE=release"}) {
		t.Fatalf("export record = %+v", record)
	}
	run(t, e, "LOCAL=local", 0, "", "")
	run(t, e, "printf '%s %s' \"$BUILD_MODE\" \"$LOCAL\"", 0, "release local", "")
	if e.Env()["BUILD_MODE"] != "release" || e.Env()["LOCAL"] != "local" {
		t.Fatalf("environment = %v", e.Env())
	}
	run(t, e, "unset BUILD_MODE LOCAL", 0, "", "")
	run(t, e, "echo \"${BUILD_MODE-unset} ${LOCAL-unset}\"", 0, "unset unset\n", "")
	if _, ok := e.Env()["BUILD_MODE"]; ok {
		t.Fatal("BUILD_MODE is still set")
	}
	e.SetEnv("RUNTIME", "external")
	run(t, e, "echo \"$RUNTIME\"", 0, "external\n", "")

	run(t, e, "alias ll='ls -la'; alias ls='ls --color'", 0, "", "")
	run(t, e, "ll workspace", 0, "-la workspace\n", "")
	run(t, e, "ls workspace", 0, "--color workspace\n", "")
	run(t, e, "echo ll; 'ls' workspace", 0, "ll\nworkspace\n", "")
	run(t, e, "(unalias ll; alias onlychild='echo child')", 0, "", "")
	if e.Aliases()["ll"] != "ls -la" || e.Aliases()["onlychild"] != "" {
		t.Fatalf("aliases = %v", e.Aliases())
	}
	run(t, e, "unalias ll; ll", 127, "", "bash: ll: command not found\n")
	run(t, e, "unalias -a; ls workspace", 0, "workspace\n", "")
	if len(e.Aliases()) != 0 {
		t.Fatalf("aliases = %v", e.Aliases())
	}
}

func TestScriptsAndVirtualBinaries(t *testing.T) {
	e, fs := newTestEngine(t, Config{})
	must(t, fs.Mkdir(home+"/build", 0755))
	must(t, fs.WriteFile(home+"/build/compile.sh", []byte(
		"#!/usr/bin/env bash\nprintf '%s:%s:%s:%s\\n' \"$0\" \"$1\" \"$#\" \"$BUILD_MODE\" > output.txt\n"+
			"printf '[%s]\\n' \"$@\"\ncd /tmp\nCHILD=ignored\n"), 0644))
	run(t, e, "export BUILD_MODE=release", 0, "", "")
	run(t, e, "cd build && ./compile.sh", 126, "", "bash: ./compile.sh: Permission denied\n")
	run(t, e, "sh compile.sh readable", 0, "[readable]\n", "")
	data, err := fs.ReadFile(home + "/build/output.txt")
	must(t, err)
	if string(data) != "compile.sh:readable:1:release\n" {
		t.Fatalf("script output file = %q", data)
	}
	must(t, fs.Chmod(home+"/build/compile.sh", 0755))
	run(t, e, "cd .. && cd build && ./compile.sh 'one two' three", 0, "[one two]\n[three]\n", "")
	data, err = fs.ReadFile(home + "/build/output.txt")
	must(t, err)
	if string(data) != "./compile.sh:one two:2:release\n" {
		t.Fatalf("script output file = %q", data)
	}
	if e.Env()["CHILD"] != "" || e.CWD() != home+"/build" {
		t.Fatalf("script changed parent environment: cwd=%q, env=%v", e.CWD(), e.Env())
	}
	must(t, fs.SetVirtualBinary(home+"/build/app", 0755, "virtual app\n", "app.c"))
	run(t, e, "./app", 0, "virtual app\n", "")
	run(t, e, "./", 126, "", "bash: ./: Is a directory\n")
	run(t, e, "./nonexistent", 127, "", "bash: ./nonexistent: No such file or directory\n")
	run(t, e, `bash -c 'echo ok; printf "%s:%s:%s\n" "$0" "$1" "$#"' name 'arg value'`,
		0, "ok\nname:arg value:1\n", "")
	run(t, e, `bash -c 'exit 7' || echo continued`, 0, "continued\n", "")
}

func TestRegistryStreamsHistoryAndExit(t *testing.T) {
	var out, errOut bytes.Buffer
	var actions []contract.TutorialAction
	e, fs := newTestEngine(t, Config{
		Stdout: &out, Stderr: &errOut,
		OnTutorialAction: func(action contract.TutorialAction) (int, error) {
			actions = append(actions, action)
			return 0, nil
		},
	})
	must(t, fs.Mkdir(home+"/work", 0755))
	e.Registry().Register("switch-dir", func(ec *contract.ExecContext, args []string) int {
		ec.SetCWD(args[0])
		return 0
	})
	e.Registry().Register("recipe", func(ec *contract.ExecContext, _ []string) int {
		fmt.Fprintf(ec.Stdout, "%t:%s:%s\n", ec.IsTTY, ec.CWD, ec.Env["BUILD_MODE"])
		fmt.Fprintln(ec.Stderr, "diagnostic")
		return ec.RunSubcommand(ec.Ctx, "read value; printf '%s:%s' \"$PWD\" \"$value\"",
			strings.NewReader("recipe-input\n"), ec.Stdout, ec.Stderr)
	})
	e.SetHistory([]string{"help"})
	e.AddHistory("lesson")
	run(t, e, "history 1", 0, "  2  lesson\n", "")
	out.Reset()
	errOut.Reset()
	record := run(t, e, "BUILD_MODE=debug recipe; switch-dir work; pwd", 0,
		"true:"+home+":debug\n"+home+":recipe-input"+home+"/work\n", "diagnostic\n")
	if out.String() != record.Stdout || errOut.String() != record.Stderr {
		t.Fatal("streamed output differs from the command record")
	}
	run(t, e, "recipe > out 2>/dev/null", 0, "", "")
	data, err := fs.ReadFile(home + "/work/out")
	must(t, err)
	if !strings.HasPrefix(string(data), "false:"+home+"/work:") {
		t.Fatalf("redirected command considered a TTY: %q", data)
	}
	run(t, e, "(exit); bash -c 'exit'; echo still", 0, "still\n", "")
	if len(actions) != 0 {
		t.Fatalf("subshell exit signaled the tutorial: %v", actions)
	}
	run(t, e, "exit 3; echo unreachable", 3, "", "")
	if len(actions) != 1 || actions[0].Name != "exit" {
		t.Fatalf("tutorial actions = %v", actions)
	}
}

func TestSyntaxErrorAndHostIsolation(t *testing.T) {
	e, fs := newTestEngine(t, Config{})
	hostDir := t.TempDir()
	hostFile := filepath.Join(hostDir, "sh-interp-host-marker")
	must(t, os.WriteFile(hostFile, []byte("host only"), 0755))

	for _, line := range []string{"echo 'unterminated", "cat <(echo ignored)", `eval 'cat <(echo ignored)'`,
		`builtin eval 'cat <(echo ignored)'`, `alias sneaky='cat <(echo ignored)'`,
		`trap 'cat <(echo ignored)' EXIT`, "echo ~root"} {
		record, err := e.Execute(context.Background(), line)
		must(t, err)
		if record.ExitCode != 2 || record.Stderr == "" {
			t.Fatalf("Execute(%q) = %+v", line, record)
		}
	}
	must(t, fs.WriteFile(home+"/source.sh", []byte("export FROM_SOURCE=virtual\n"), 0644))
	run(t, e, "source source.sh; echo \"$FROM_SOURCE\"", 0, "virtual\n", "")

	virtualName := filepath.ToSlash(hostFile)
	run(t, e, fmt.Sprintf("command -v %q", virtualName), 1, "", "")
	run(t, e, fmt.Sprintf("test -e %q", virtualName), 1, "", "")
	run(t, e, fmt.Sprintf("%q", virtualName), 127, "", "bash: "+virtualName+": No such file or directory\n")
	run(t, e, fmt.Sprintf("TMPDIR=%q; echo leak > %q 2>/dev/null || true", filepath.ToSlash(hostDir), virtualName), 0, "",
		"open "+virtualName+": no such file or directory\n")
	run(t, e, fmt.Sprintf("exec %q", virtualName), 127, "", "bash: "+virtualName+": No such file or directory\n")
	data, err := os.ReadFile(hostFile)
	must(t, err)
	if string(data) != "host only" {
		t.Fatalf("host path modified: %q", data)
	}
}

func TestCompletion(t *testing.T) {
	e, fs := newTestEngine(t, Config{})
	e.Registry().Register("compile", func(*contract.ExecContext, []string) int { return 0 })
	e.Registry().Register("compiler", func(*contract.ExecContext, []string) int { return 0 })
	must(t, fs.MkdirAll(home+"/workspace/long-directory-name-for-tab-practice", 0755))
	must(t, fs.WriteFile(home+"/workspace/note file.txt", nil, 0644))
	must(t, fs.WriteFile(home+"/.profile", nil, 0644))
	must(t, fs.WriteFile(home+"/compile.sh", nil, 0755))
	must(t, fs.WriteFile(home+"/compile.txt", nil, 0644))
	run(t, e, "alias cproj='cd workspace'", 0, "", "")
	cases := []struct {
		line, prefix string
		start, end   int
		path         bool
		matches      []string
	}{
		{"ec", "echo", 0, 2, false, []string{"echo"}},
		{"compi", "compile", 0, 5, false, []string{"compile", "compiler"}},
		{"cpr", "cproj", 0, 3, false, []string{"cproj"}},
		{"cd workspace/long", "workspace/long-directory-name-for-tab-practice/", 3, 17, true,
			[]string{"workspace/long-directory-name-for-tab-practice/"}},
		{"cat .p", ".profile", 4, 6, true, []string{".profile"}},
		{"./comp", "./compile.sh", 0, 6, true, []string{"./compile.sh"}},
		{"echo hello | ca", "ca", 13, 15, false, []string{"case", "cat"}},
		{"cd ~/work", "~/workspace/", 3, 9, true, []string{"~/workspace/"}},
		{`cat "workspace/note f`, `workspace/note\ file.txt`, 4, 21, true, []string{`workspace/note\ file.txt`}},
		{"cat missing", "", 4, 11, true, nil},
		{"echo hi&&ec", "echo", 9, 11, false, []string{"echo"}},
	}
	for _, tc := range cases {
		t.Run(tc.line, func(t *testing.T) {
			got := e.Complete(tc.line, len(tc.line))
			if got.ReplaceStart != tc.start || got.ReplaceEnd != tc.end || got.IsPath != tc.path ||
				got.CommonPrefix != tc.prefix || !slices.Equal(got.Matches, tc.matches) {
				t.Fatalf("Complete(%q) = %+v, want %+v", tc.line, got, tc)
			}
		})
	}
	line := "echo | cax extra"
	got := e.Complete(line, len("echo | ca"))
	if got.ReplaceStart != 7 || got.ReplaceEnd != 10 || !slices.Equal(got.Matches, []string{"case", "cat"}) {
		t.Fatalf("mid-token completion = %+v", got)
	}
}

func TestRegistry(t *testing.T) {
	reg := NewRegistry()
	fn := func(*contract.ExecContext, []string) int { return 3 }
	reg.Register("z", fn)
	reg.Register("a", fn)
	if !slices.Equal(reg.Names(), []string{"a", "z"}) {
		t.Fatalf("registry names = %v", reg.Names())
	}
	if got, ok := reg.Lookup("a"); !ok || got(nil, nil) != 3 {
		t.Fatal("registered handler was not retrieved")
	}
}
