package curriculum

import (
	"errors"
	"io/fs"
	"path"
	"slices"
	"strings"
	"testing"

	"learn-linux/internal/contract"
)

func TestLoadChapters(t *testing.T) {
	engine := NewEngine()
	tests := []struct {
		slug     string
		keys     [3]string
		commands string
		files    []string
	}{
		{"sandbox", [3]string{contract.KeySandboxMetaTitle, contract.KeySandboxMetaSubtitle, contract.KeySandboxMetaIntro}, "",
			[]string{"notes.txt", ".hidden_note", "data/users.csv", "logs/server.log", "workspace/projects/alpha/status.txt",
				"workspace/projects/beta/status.txt", "hello.c", "Makefile", "hello.sh"}},
		{"navigation", [3]string{contract.KeyCh1Title, contract.KeyCh1Subtitle, contract.KeyCh1Intro}, "pwd ls cd tree clear",
			[]string{"welcome.txt", ".hidden_note", "workspace/projects/alpha/notes.txt", "workspace/projects/beta/status.txt",
				"workspace/long-directory-name-for-tab-practice/secret.txt"}},
		{"file-management", [3]string{contract.KeyCh2Title, contract.KeyCh2Subtitle, contract.KeyCh2Intro}, "mkdir touch cp mv rm ln",
			[]string{"inbox/report_draft.txt", "inbox/junk.tmp", "templates/config.ini", "templates/readme.txt"}},
		{"text-editing", [3]string{contract.KeyCh3Title, contract.KeyCh3Subtitle, contract.KeyCh3Intro}, "cat head tail wc less nano",
			[]string{"logs/server.log", "app.conf"}},
		{"search-processing", [3]string{contract.KeyCh4Title, contract.KeyCh4Subtitle, contract.KeyCh4Intro}, "find grep cut sort uniq",
			[]string{"src/main.go", "src/utils/helper.go", "src/utils/config.yaml", "src/legacy/old.log", "data/users.csv"}},
		{"pipes-redirects", [3]string{contract.KeyCh5Title, contract.KeyCh5Subtitle, contract.KeyCh5Intro}, "| > >> < && || ; *",
			[]string{"events/app.log", "events/notes_a.txt", "events/notes_b.txt", "events/notes_c.txt"}},
		{"permissions-scripts", [3]string{contract.KeyCh6Title, contract.KeyCh6Subtitle, contract.KeyCh6Intro}, "stat chmod env export alias ./compile.sh",
			[]string{"build/compile.sh", "build/README.txt"}},
		{"archives-toolchain", [3]string{contract.KeyCh7Title, contract.KeyCh7Subtitle, contract.KeyCh7Intro}, "tar gzip gcc make",
			[]string{"project/src/main.c", "project/Makefile", "project/assets/readme.txt", "project/assets/version.txt"}},
		{"capstone", [3]string{contract.KeyCh8Title, contract.KeyCh8Subtitle, contract.KeyCh8Intro}, "find grep nano chmod gcc tar ./compile.sh",
			[]string{"release_prep/scripts/compile.sh", "release_prep/src/app.c", "release_prep/config/env.conf", "release_prep/logs/deploy.log"}},
	}

	listed := engine.Chapters()
	if len(listed) != 8 {
		t.Fatalf("Chapters() has %d entries, want 8", len(listed))
	}
	for id, test := range tests {
		t.Run(test.slug, func(t *testing.T) {
			meta, ok := engine.ChapterMeta(id)
			keys := [3]string{meta.TitleKey, meta.SubtitleKey, meta.IntroKey}
			if !ok || meta.ID != id || meta.Slug != test.slug || meta.IsSandbox != (id == 0) ||
				keys != test.keys || strings.Join(meta.Commands, " ") != test.commands {
				t.Fatalf("ChapterMeta(%d) = %+v, ok = %v", id, meta, ok)
			}
			if id > 0 && listed[id-1].ID != id {
				t.Fatalf("chapter listing is out of order: %+v", listed)
			}

			var v contract.VFS
			var cwd string
			var err error
			if id == 0 {
				v, cwd, err = engine.LoadSandbox()
			} else {
				v, cwd, err = engine.LoadChapter(id)
			}
			if err != nil || cwd != "/home/learner" || !v.IsDir(cwd) || !v.Exists("/etc/os-release") {
				t.Fatalf("load = cwd %q, err %v", cwd, err)
			}

			for _, file := range test.files {
				p := "/home/learner/" + file
				info, err := v.Stat(p)
				mode := fs.FileMode(0644)
				if test.slug == "sandbox" && file == "hello.sh" {
					mode = 0755
				}
				if err != nil || info.Type != contract.TypeFile || info.Mode != mode || info.Size == 0 {
					t.Errorf("Stat(%s) = %+v, %v; want non-empty file with mode %o", p, info, err, mode)
				}
			}
		})
	}
}

func TestSeedContents(t *testing.T) {
	logs := loadScenario(t, 3)
	log := nonEmptyLines(readFile(logs.v, "logs/server.log"))
	if len(log) != 60 || log[44] != "2026-11-22 10:45:00 INFO auth TOKEN=ALPHA-742 verified" {
		t.Fatalf("unexpected seeded log with %d lines: %q", len(log), log)
	}
	for _, level := range []string{"INFO", "WARN", "ERROR"} {
		if !strings.Contains(strings.Join(log, "\n"), " "+level+" ") {
			t.Errorf("server.log lacks %s entries", level)
		}
	}

	archive := loadScenario(t, 7)
	makefile := readFile(archive.v, "project/Makefile")
	if !strings.Contains(makefile, "\n\t$(CC) -o $(TARGET) src/main.c\n") {
		t.Errorf("project/Makefile lacks a tab-indented gcc recipe: %q", makefile)
	}

	capstone := loadScenario(t, 8)
	script := readFile(capstone.v, "release_prep/scripts/compile.sh")
	if !strings.Contains(script, `if ! grep "ENV=prod" /home/learner/release_prep/config/env.conf > /dev/null; then`) {
		t.Errorf("compile.sh lacks the required grep redirection: %q", script)
	}
}

func TestGoalTransitions(t *testing.T) {
	const uniqueErrors = "ERROR auth token expired\nERROR cache miss storm\nERROR db connection timeout\n"
	tests := []struct {
		id    int
		keys  []string
		steps []func(*scenario)
	}{
		{1, []string{contract.KeyCh1Goal1, contract.KeyCh1Goal2, contract.KeyCh1Goal3, contract.KeyCh1Goal4},
			[]func(*scenario){
				func(s *scenario) { s.record("pwd", "/home/learner\n") },
				func(s *scenario) { s.record("ls -lah", ".hidden_note\n") },
				func(s *scenario) { s.record("tree", "workspace\n") },
				func(s *scenario) {
					s.record("cd workspace/long-directory-name-for-tab-practice", "")
					s.last.CWD = "/home/learner/workspace/long-directory-name-for-tab-practice"
				},
			}},
		{2, []string{contract.KeyCh2Goal1, contract.KeyCh2Goal2, contract.KeyCh2Goal3, contract.KeyCh2Goal4, contract.KeyCh2Goal5},
			[]func(*scenario){
				func(s *scenario) { s.require(s.v.MkdirAll(atHome("lab/docs/archive"), 0755)) },
				func(s *scenario) { s.write("lab/notes.md", "") },
				func(s *scenario) {
					s.require(s.v.Rename(atHome("inbox/report_draft.txt"), atHome("lab/docs/report.txt")))
				},
				func(s *scenario) {
					s.require(s.v.Copy(atHome("templates"), atHome("lab/templates_backup"), true))
					s.require(s.v.Remove(atHome("inbox/junk.tmp")))
				},
				func(s *scenario) { s.require(s.v.Symlink("docs/report.txt", atHome("lab/current_report"))) },
			}},
		{3, []string{contract.KeyCh3Goal1, contract.KeyCh3Goal2, contract.KeyCh3Goal3, contract.KeyCh3Goal4},
			[]func(*scenario){
				func(s *scenario) { s.record("tail -n 3 logs/server.log", "") },
				func(s *scenario) { s.record("wc -l logs/server.log", "60 logs/server.log\n") },
				func(s *scenario) { s.record("less 'logs/server.log'", "") },
				func(s *scenario) {
					s.write("app.conf", "# Application Configuration\nhost=127.0.0.1\n port=9000 \n\tdebug=false\n")
				},
			}},
		{4, []string{contract.KeyCh4Goal1, contract.KeyCh4Goal2, contract.KeyCh4Goal3, contract.KeyCh4Goal4},
			[]func(*scenario){
				func(s *scenario) { s.record("find src -name '*.yaml'", "src/utils/config.yaml\n") },
				func(s *scenario) { s.record("grep -irn 'ERROR' src", "src/legacy/old.log:2:ERROR disk full\n") },
				func(s *scenario) { s.record("cut -d , -f 1 data/users.csv", "alice\nbob\ncharlie\nalice\ndave\nbob\n") },
				func(s *scenario) {
					s.record("sort data/users.csv | uniq", "alice,admin,90\nbob,dev,85\ncharlie,dev,92\ndave,ops,78\n")
				},
			}},
		{5, []string{contract.KeyCh5Goal1, contract.KeyCh5Goal2, contract.KeyCh5Goal3, contract.KeyCh5Goal4},
			[]func(*scenario){
				func(s *scenario) { s.write("errors.txt", uniqueErrors) },
				func(s *scenario) { s.require(s.v.AppendFile(atHome("errors.txt"), []byte("--- END ---\n\n"), 0644)) },
				func(s *scenario) {
					s.write("combined.txt", readFile(s.v, "events/notes_a.txt")+readFile(s.v, "events/notes_b.txt")+readFile(s.v, "events/notes_c.txt"))
				},
				func(s *scenario) {
					s.write("done_dir/ok.txt", "")
					s.record("mkdir -p done_dir && touch done_dir/ok.txt", "")
				},
			}},
		{6, []string{contract.KeyCh6Goal1, contract.KeyCh6Goal2, contract.KeyCh6Goal3, contract.KeyCh6Goal4},
			[]func(*scenario){
				func(s *scenario) { s.record("ls -la build/compile.sh", "") },
				func(s *scenario) { s.require(s.v.Chmod(atHome("build/compile.sh"), 0755)) },
				func(s *scenario) { s.env["BUILD_MODE"] = "release" },
				func(s *scenario) { s.write("build/out/artifact.txt", "BUILD_OK:release\n") },
			}},
		{7, []string{contract.KeyCh7Goal1, contract.KeyCh7Goal2, contract.KeyCh7Goal3, contract.KeyCh7Goal4},
			[]func(*scenario){
				func(s *scenario) { s.binary("project/app", "Hello from Virtual Linux C App!") },
				func(s *scenario) { s.record("./project/app", "Hello from Virtual Linux C App!\n") },
				func(s *scenario) { s.write("project/assets.tar.gz", strings.Repeat("x", 21)) },
				func(s *scenario) { s.write("extracted/assets/version.txt", "v1.0.0\n") },
			}},
		{8, []string{contract.KeyCh8Goal1, contract.KeyCh8Goal2, contract.KeyCh8Goal3, contract.KeyCh8Goal4, contract.KeyCh8Goal5},
			[]func(*scenario){
				func(s *scenario) {
					s.write("release_prep/critical.txt", "2026-11-22 08:00:04 CRITICAL tls certificate expires in 24h\n"+
						"2026-11-22 08:00:15 CRITICAL database replica lag exceeded threshold\n")
				},
				func(s *scenario) { s.write("release_prep/config/env.conf", "ENV=prod\nLOG_LEVEL=debug\n") },
				func(s *scenario) {
					s.require(s.v.Chmod(atHome("release_prep/scripts/compile.sh"), 0755))
					s.binary("release_prep/dist/service", "Service v2.0 ready")
				},
				func(s *scenario) { s.record("./release_prep/dist/service", "Service v2.0 ready\n") },
				func(s *scenario) { s.write("release_v2.tar.gz", strings.Repeat("x", 21)) },
			}},
	}

	engine := NewEngine()
	for _, test := range tests {
		meta, _ := engine.ChapterMeta(test.id)
		t.Run(meta.Slug, func(t *testing.T) {
			s := loadScenario(t, test.id)
			s.assertProgress(test.keys, 0)
			for i, step := range test.steps {
				step(s)
				s.assertProgress(test.keys, i+1)
				if s.last.Raw != "" {
					s.history = append(s.history, s.last)
					s.last = contract.CommandRecord{}
				}
			}
			s.assertProgress(test.keys, len(test.keys))
		})
	}
}

func TestHistoryGoalVariants(t *testing.T) {
	const uniqueUsers = "alice,admin,90\nbob,dev,85\ncharlie,dev,92\ndave,ops,78\n"
	tests := []struct {
		name   string
		id     int
		goal   int
		record contract.CommandRecord
		want   bool
	}{
		{"failed pwd", 1, 1, contract.CommandRecord{Raw: "pwd", ExitCode: 1}, false},
		{"clustered ls", 1, 2, contract.CommandRecord{Command: "ls", Args: []string{"-al"}}, true},
		{"long ls option", 1, 2, contract.CommandRecord{Raw: `ls "--all"`}, true},
		{"ls operand", 1, 2, contract.CommandRecord{Raw: "ls -- -a"}, false},
		{"ls almost all", 1, 2, contract.CommandRecord{Raw: "ls -A"}, false},
		{"echoed tree", 1, 3, contract.CommandRecord{Raw: "echo tree"}, false},
		{"head from stdin", 3, 1, contract.CommandRecord{Raw: "cat logs/server.log | head"}, true},
		{"head on log", 3, 1, contract.CommandRecord{Command: "head", Args: []string{"logs/server.log"}}, true},
		{"less on wrong file", 3, 3, contract.CommandRecord{Raw: "less app.conf"}, false},
		{"recursive grep", 4, 2, contract.CommandRecord{Raw: "grep -R -- ERROR src", Stdout: "ERROR disk full"}, true},
		{"plain grep", 4, 2, contract.CommandRecord{Raw: "grep ERROR src/legacy/old.log", Stdout: "ERROR disk full"}, false},
		{"grep pattern option", 4, 2, contract.CommandRecord{Raw: "grep -eERROR src/legacy/old.log", Stdout: "ERROR disk full"}, false},
		{"cut with unwanted columns", 4, 3, contract.CommandRecord{Raw: "cut -d , -f 1,2 data/users.csv", Stdout: uniqueUsers}, false},
		{"sort unique", 4, 4, contract.CommandRecord{Command: "sort", Args: []string{"-u", "data/users.csv"}, Stdout: uniqueUsers}, true},
		{"duplicate users", 4, 4, contract.CommandRecord{Raw: "sort -u data/users.csv", Stdout: uniqueUsers + "alice,other,90\n"}, false},
		{"echoed sort and uniq", 4, 4, contract.CommandRecord{Raw: `echo "sort | uniq"`, Stdout: uniqueUsers}, false},
		{"stat", 6, 1, contract.CommandRecord{Command: "stat", Args: []string{"build/compile.sh"}}, true},
		{"failed app", 7, 2, contract.CommandRecord{Stdout: "Hello from Virtual Linux C App!", ExitCode: 1}, false},
		{"failed service", 8, 4, contract.CommandRecord{Stdout: "Service v2.0 ready", ExitCode: 1}, false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			s := loadScenario(t, test.id)
			s.history = []contract.CommandRecord{test.record}
			if goal := s.evaluate().Goals[test.goal-1]; goal.Completed != test.want {
				t.Fatalf("%s completion = %v, want %v", goal.ID, goal.Completed, test.want)
			}
		})
	}
}

func TestVFSGoalVariants(t *testing.T) {
	t.Run("absolute symlink", func(t *testing.T) {
		s := loadScenario(t, 2)
		s.write("lab/docs/report.txt", "Quarterly Linux Report Draft v1\n")
		s.require(s.v.Symlink(atHome("lab/docs/report.txt"), atHome("lab/current_report")))
		if !s.evaluate().Goals[4].Completed {
			t.Fatal("absolute report symlink did not pass")
		}
		s.require(s.v.Remove(atHome("lab/docs/report.txt")))
		if s.evaluate().Goals[4].Completed {
			t.Fatal("dangling report symlink passed")
		}
	})

	t.Run("footer and chaining", func(t *testing.T) {
		s := loadScenario(t, 5)
		s.write("errors.txt", "--- END ---\n")
		s.write("done_dir/ok.txt", "")
		s.record(`echo '&&'`, "&&\n")
		goals := s.evaluate().Goals
		if goals[0].Completed || goals[1].Completed || goals[3].Completed {
			t.Fatalf("footer-only output or quoted && passed: %+v", goals)
		}
		s.write("errors.txt", "ERROR db connection timeout\nERROR auth token expired\nERROR cache miss storm\n")
		if s.evaluate().Goals[0].Completed {
			t.Fatal("unsorted errors passed")
		}
		s.write("errors.txt", "ERROR auth token expired\nERROR cache miss storm\nERROR cache miss storm\nERROR db connection timeout\n")
		if s.evaluate().Goals[0].Completed {
			t.Fatal("duplicate errors passed")
		}
	})

	t.Run("flat archive extraction", func(t *testing.T) {
		s := loadScenario(t, 7)
		s.write("extracted/version.txt", "v1.0.0\n")
		s.write("project/assets.tar.gz", strings.Repeat("x", 20))
		goals := s.evaluate().Goals
		if goals[2].Completed || !goals[3].Completed {
			t.Fatalf("archive size or flat extraction misclassified: %+v", goals)
		}
	})
}

func TestSoftLocks(t *testing.T) {
	tests := []struct {
		id    int
		key   string
		paths []string
	}{
		{1, contract.KeyCh1SoftLock, []string{"workspace/long-directory-name-for-tab-practice"}},
		{2, contract.KeyCh2SoftLock, []string{"inbox/report_draft.txt", "templates/config.ini"}},
		{3, contract.KeyCh3SoftLock, []string{"logs/server.log", "app.conf"}},
		{4, contract.KeyCh4SoftLock, []string{"src/utils/config.yaml", "data/users.csv"}},
		{5, contract.KeyCh5SoftLock, []string{"events/app.log", "events/notes_a.txt", "events/notes_b.txt", "events/notes_c.txt"}},
		{6, contract.KeyCh6SoftLock, []string{"build/compile.sh"}},
		{7, contract.KeyCh7SoftLock, []string{"project/src/main.c", "project/assets/version.txt"}},
		{8, contract.KeyCh8SoftLock, []string{"release_prep/scripts/compile.sh", "release_prep/src/app.c",
			"release_prep/config/env.conf", "release_prep/logs/deploy.log"}},
	}
	for _, test := range tests {
		for _, name := range test.paths {
			t.Run(name, func(t *testing.T) {
				s := loadScenario(t, test.id)
				s.require(s.v.RemoveAll(atHome(name)))
				got := s.evaluate()
				if !got.SoftLocked || got.SoftLockReasonKey != test.key {
					t.Fatalf("removing %s produced soft lock %v with key %q, want %q",
						name, got.SoftLocked, got.SoftLockReasonKey, test.key)
				}

				fresh := loadScenario(t, test.id)
				if state := fresh.evaluate(); state.SoftLocked || state.SoftLockReasonKey != "" {
					t.Fatalf("reload is soft locked: %+v", state)
				}
			})
		}
	}

	t.Run("moved-source-and-backup", func(t *testing.T) {
		s := loadScenario(t, 2)
		s.write("lab/docs/report.txt", "Quarterly Linux Report Draft v1\n")
		s.write("lab/templates_backup/config.ini", "[app]\n")
		s.require(s.v.Remove(atHome("inbox/report_draft.txt")))
		s.require(s.v.Remove(atHome("templates/config.ini")))
		if got := s.evaluate(); got.SoftLocked {
			t.Fatalf("relocated files produced a soft lock: %+v", got)
		}
	})
}

func TestNextHintKey(t *testing.T) {
	hints := [][]string{
		nil,
		{contract.KeyCh1Hint1, contract.KeyCh1Hint2, contract.KeyCh1Hint3},
		{contract.KeyCh2Hint1, contract.KeyCh2Hint2, contract.KeyCh2Hint3},
		{contract.KeyCh3Hint1, contract.KeyCh3Hint2, contract.KeyCh3Hint3},
		{contract.KeyCh4Hint1, contract.KeyCh4Hint2, contract.KeyCh4Hint3},
		{contract.KeyCh5Hint1, contract.KeyCh5Hint2, contract.KeyCh5Hint3},
		{contract.KeyCh6Hint1, contract.KeyCh6Hint2, contract.KeyCh6Hint3},
		{contract.KeyCh7Hint1, contract.KeyCh7Hint2, contract.KeyCh7Hint3},
		{contract.KeyCh8Hint1, contract.KeyCh8Hint2, contract.KeyCh8Hint3, contract.KeyCh8Hint4},
	}
	engine := NewEngine()
	for id, keys := range hints {
		meta, _ := engine.ChapterMeta(id)
		t.Run(meta.Slug, func(t *testing.T) {
			if !slices.Equal(meta.HintKeys, keys) {
				t.Fatalf("metadata hint keys = %v, want %v", meta.HintKeys, keys)
			}
			if len(keys) == 0 {
				key, next, total := engine.NextHintKey(id, 1)
				if key != "" || next != 0 || total != 0 {
					t.Fatalf("sandbox hint = %q, %d, %d", key, next, total)
				}
				return
			}

			idx := -3
			for _, expected := range append(slices.Clone(keys), contract.KeyHintExhausted, keys[0]) {
				key, next, total := engine.NextHintKey(id, idx)
				expectedNext := max(0, idx) + 1
				if expected == contract.KeyHintExhausted {
					expectedNext = 0
				}
				if key != expected || next != expectedNext || total != len(keys) {
					t.Fatalf("hint index %d = %q, %d, %d; want %q, %d, %d",
						idx, key, next, total, expected, expectedNext, len(keys))
				}
				idx = next
			}

			key, next, total := engine.NextHintKey(id, 100)
			if key != contract.KeyHintExhausted || next != 0 || total != len(keys) {
				t.Fatalf("out-of-range hint = %q, %d, %d", key, next, total)
			}
		})
	}
}

func TestEngineBoundaries(t *testing.T) {
	engine := NewEngine()
	for _, id := range []int{-1, 9} {
		if _, ok := engine.ChapterMeta(id); ok {
			t.Errorf("unknown chapter %d returned metadata", id)
		}
		if _, _, err := engine.LoadChapter(id); !errors.Is(err, contract.ErrNotExist) {
			t.Errorf("LoadChapter(%d) error = %v, want ErrNotExist", id, err)
		}
		if state := engine.Evaluate(id, nil, nil, nil, contract.CommandRecord{}); state.Meta.Slug != "" || state.AllPassed {
			t.Errorf("unknown chapter %d returned state %+v", id, state)
		}
		if key, next, total := engine.NextHintKey(id, 0); key != "" || next != 0 || total != 0 {
			t.Errorf("NextHintKey(%d, 0) = %q, %d, %d", id, key, next, total)
		}
	}
	if state := engine.Evaluate(0, nil, nil, nil, contract.CommandRecord{}); !state.Meta.IsSandbox ||
		len(state.Goals) != 0 || state.AllPassed || state.SoftLocked {
		t.Errorf("sandbox state = %+v", state)
	}

	meta := engine.Chapters()[0]
	meta.GoalKeys[0], meta.HintKeys[0], meta.Commands[0] = "", "", ""
	fresh, _ := engine.ChapterMeta(1)
	if fresh.GoalKeys[0] != contract.KeyCh1Goal1 || fresh.HintKeys[0] != contract.KeyCh1Hint1 || fresh.Commands[0] != "pwd" {
		t.Errorf("mutating returned metadata changed the engine: %+v", fresh)
	}
}

type scenario struct {
	t       *testing.T
	engine  *Engine
	id      int
	v       contract.VFS
	env     map[string]string
	history []contract.CommandRecord
	last    contract.CommandRecord
}

func loadScenario(t *testing.T, id int) *scenario {
	t.Helper()
	engine := NewEngine()
	v, _, err := engine.LoadChapter(id)
	if err != nil {
		t.Fatal(err)
	}
	return &scenario{t: t, engine: engine, id: id, v: v, env: make(map[string]string)}
}

func (s *scenario) require(err error) {
	s.t.Helper()
	if err != nil {
		s.t.Fatal(err)
	}
}

func (s *scenario) write(name, text string) {
	s.t.Helper()
	p := atHome(name)
	s.require(s.v.MkdirAll(path.Dir(p), 0755))
	s.require(s.v.WriteFile(p, []byte(text), 0644))
}

func (s *scenario) binary(name, output string) {
	s.t.Helper()
	p := atHome(name)
	s.require(s.v.MkdirAll(path.Dir(p), 0755))
	s.require(s.v.SetVirtualBinary(p, 0755, output, "test.c"))
}

func (s *scenario) record(raw, stdout string) {
	s.last = contract.CommandRecord{Raw: raw, Stdout: stdout}
}

func (s *scenario) evaluate() contract.ChapterState {
	return s.engine.Evaluate(s.id, s.v, s.env, s.history, s.last)
}

func (s *scenario) assertProgress(keys []string, passed int) {
	s.t.Helper()
	state := s.evaluate()
	if state.SoftLocked || state.SoftLockReasonKey != "" {
		s.t.Errorf("unexpected soft lock: %+v", state)
	}
	if state.AllPassed != (passed == len(keys)) {
		s.t.Errorf("AllPassed = %v with %d/%d expected goals complete", state.AllPassed, passed, len(keys))
	}
	if len(state.Goals) != len(keys) || !slices.Equal(state.Meta.GoalKeys, keys) {
		s.t.Fatalf("unexpected goals or keys: %+v", state)
	}
	for i, goal := range state.Goals {
		if goal.ID != keys[i] || goal.DescKey != keys[i] || goal.Completed != (i < passed) {
			s.t.Errorf("after step %d, goal %d = %+v; want key %q and completion %v", passed, i+1, goal, keys[i], i < passed)
		}
	}
}
