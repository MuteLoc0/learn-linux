package app_test

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"learn-linux/internal/app"
	"learn-linux/internal/contract"
	"learn-linux/internal/store"
	"learn-linux/internal/term"
	"learn-linux/internal/ui"
)

func typeCmd(events *[]contract.KeyEvent, cmd string) {
	for _, r := range cmd {
		*events = append(*events, contract.KeyEvent{Kind: contract.KeyRune, Rune: r})
	}
	*events = append(*events, contract.KeyEvent{Kind: contract.KeyEnter})
}

func TestEndToEndAllChaptersAndFeatures(t *testing.T) {
	tmpStore := store.NewFileStore(filepath.Join(t.TempDir(), "progress.json"))
	renderer := ui.NewRenderer()
	renderer.PanicTickDuration = 0

	var events []contract.KeyEvent

	// Chapter 1: Navigation & Listing
	typeCmd(&events, "mission")
	typeCmd(&events, "hint")
	typeCmd(&events, "pwd")
	typeCmd(&events, "ls -la")
	typeCmd(&events, "tree")
	typeCmd(&events, "cd workspace/long-directory-name-for-tab-practice")

	// Chapter 2: File & Directory Management
	typeCmd(&events, "chapter 2")
	typeCmd(&events, "mkdir -p lab/docs/archive")
	typeCmd(&events, "touch lab/notes.md")
	typeCmd(&events, "mv inbox/report_draft.txt lab/docs/report.txt")
	typeCmd(&events, "cp -r templates lab/templates_backup")
	typeCmd(&events, "rm inbox/junk.tmp")
	typeCmd(&events, "ln -s docs/report.txt lab/current_report")

	// Chapter 3: Viewing, Paging & Editing Text (including real interactive less & nano!)
	typeCmd(&events, "chapter 3")
	typeCmd(&events, "head -n 5 logs/server.log")
	typeCmd(&events, "wc -l logs/server.log")
	typeCmd(&events, "less logs/server.log")
	// Inside less: scroll down, quit
	events = append(events,
		contract.KeyEvent{Kind: contract.KeyRune, Rune: 'j'},
		contract.KeyEvent{Kind: contract.KeyRune, Rune: 'q'},
	)
	// Edit app.conf cleanly via nano (also test real nano Ctrl+O / Ctrl+X!)
	typeCmd(&events, "nano app.conf")
	// Move down 2 lines to port=8080, cut line (Ctrl+K), type port=9000 + Enter, cut debug=true (Ctrl+K), type debug=false, Ctrl+O, Ctrl+X
	events = append(events,
		contract.KeyEvent{Kind: contract.KeyDown},
		contract.KeyEvent{Kind: contract.KeyDown},
		contract.KeyEvent{Kind: contract.KeyCtrlK},
	)
	for _, r := range "port=9000" {
		events = append(events, contract.KeyEvent{Kind: contract.KeyRune, Rune: r})
	}
	events = append(events,
		contract.KeyEvent{Kind: contract.KeyEnter},
		contract.KeyEvent{Kind: contract.KeyCtrlK},
	)
	for _, r := range "debug=false" {
		events = append(events, contract.KeyEvent{Kind: contract.KeyRune, Rune: r})
	}
	events = append(events,
		contract.KeyEvent{Kind: contract.KeyCtrlO},
		contract.KeyEvent{Kind: contract.KeyCtrlX},
	)

	// Chapter 4: Search & Text Processing
	typeCmd(&events, "chapter 4")
	typeCmd(&events, "find src -name \"*.yaml\"")
	typeCmd(&events, "grep -rn ERROR src")
	typeCmd(&events, "cut -d , -f 1 data/users.csv")
	typeCmd(&events, "sort data/users.csv | uniq")

	// Chapter 5: Pipes, Redirections & Chaining
	typeCmd(&events, "chapter 5")
	typeCmd(&events, "grep ERROR events/app.log | sort | uniq > errors.txt")
	typeCmd(&events, "echo \"--- END ---\" >> errors.txt")
	typeCmd(&events, "cat events/notes_*.txt > combined.txt")
	typeCmd(&events, "mkdir -p done_dir && touch done_dir/ok.txt")

	// Chapter 6: Permissions, Env & Running Scripts
	typeCmd(&events, "chapter 6")
	typeCmd(&events, "stat build/compile.sh")
	typeCmd(&events, "chmod +x build/compile.sh")
	typeCmd(&events, "export BUILD_MODE=release")
	typeCmd(&events, "cd build && ./compile.sh")

	// Chapter 7: Archives & Simulated Build Toolchain
	typeCmd(&events, "chapter 7")
	typeCmd(&events, "cd project && make")
	typeCmd(&events, "./app")
	typeCmd(&events, "tar -czf assets.tar.gz assets")
	typeCmd(&events, "mkdir -p /home/learner/extracted && tar -xzf assets.tar.gz -C /home/learner/extracted")

	// Chapter 8: Capstone Integration Challenge
	typeCmd(&events, "chapter 8")
	typeCmd(&events, "grep CRITICAL release_prep/logs/deploy.log > release_prep/critical.txt")
	typeCmd(&events, "printf \"ENV=prod\\nLOG_LEVEL=info\\n\" > release_prep/config/env.conf")
	typeCmd(&events, "chmod +x release_prep/scripts/compile.sh && ./release_prep/scripts/compile.sh")
	typeCmd(&events, "./release_prep/dist/service")
	typeCmd(&events, "tar -czf /home/learner/release_v2.tar.gz -C release_prep dist")

	// Extra feature checks: lang toggle, man, help, chapter list, soft-lock + reset, sandbox, rm -rf / panic!
	typeCmd(&events, "lang en")
	typeCmd(&events, "man ls")
	typeCmd(&events, "help")
	typeCmd(&events, "chapter list")
	typeCmd(&events, "rm release_prep/src/app.c")
	typeCmd(&events, "reset")
	typeCmd(&events, "sandbox")
	typeCmd(&events, "rm -rf /")
	typeCmd(&events, "lang zh")
	typeCmd(&events, "exit")

	mockTerm := term.NewMockTerminal(events...)
	application, err := app.New(app.Config{
		Term:            mockTerm,
		Store:           tmpStore,
		UI:              renderer,
		PanicSeconds:    1,
		PanicSecondsSet: true,
		InitialLang:     contract.LangZH,
	})
	if err != nil {
		t.Fatalf("app.New failed: %v", err)
	}

	if err := application.Run(context.Background()); err != nil {
		t.Fatalf("app.Run failed: %v", err)
	}

	// Verify all 8 chapters were persisted as completed!
	saved, err := tmpStore.Load()
	if err != nil {
		t.Fatalf("store.Load failed: %v", err)
	}
	if len(saved.CompletedChapters) != 8 {
		t.Fatalf("expected all 8 chapters completed, got %v\n--- OUTPUT ---\n%s", saved.CompletedChapters, mockTerm.Output.String())
	}
	for i := 1; i <= 8; i++ {
		if saved.CompletedChapters[i-1] != i {
			t.Fatalf("expected chapter %d completed, got %v", i, saved.CompletedChapters)
		}
	}

	out := mockTerm.Output.String()
	for _, mustContain := range []string{
		"LearnLinux v1.0",
		"欢迎来到LearnLinux，一个虚拟化的终端界面，不会影响任何宿主机文件。",
		"\x1b[31mexit (退出)\x1b[0m",
		"tips: ",
		"Kernel panic",
		"Service v2.0 ready",
		"Hello from Virtual Linux C App!",
		"Compilation succeeded in release mode!",
	} {
		if !strings.Contains(out, mustContain) {
			t.Errorf("expected terminal output to contain %q", mustContain)
		}
	}
	for _, mustNotContain := range []string{
		"▸ learn-linux",
		"小技巧：",
		"%!s(",
		"%!d(",
		"Chapter 0",
	} {
		if strings.Contains(out, mustNotContain) {
			t.Errorf("unexpected formatting glitch %q in output", mustNotContain)
		}
	}
}

func TestReviewRegressions(t *testing.T) {
	tmpStore := store.NewFileStore(filepath.Join(t.TempDir(), "progress.json"))
	renderer := ui.NewRenderer()
	renderer.PanicTickDuration = 0

	var events []contract.KeyEvent
	// Chapter 1: ls --all works
	typeCmd(&events, "ls --all")
	// Unexported shell variable must not appear in `env`; exported variable must appear
	typeCmd(&events, "LOCAL_SECRET=hidden")
	typeCmd(&events, "export EXPORTED_OK=visible")
	typeCmd(&events, "env | grep -q LOCAL_SECRET || echo LOCAL_NOT_EXPORTED")
	typeCmd(&events, "env | grep EXPORTED_OK")
	// which bash/sh
	typeCmd(&events, "which bash sh")
	// Switch to chapter 4: piped cut + grep -rn with default "." + grep -q
	typeCmd(&events, "chapter 4")
	typeCmd(&events, "cat data/users.csv | cut -d , -f 1")
	typeCmd(&events, "cd src && grep -rn ERROR")
	typeCmd(&events, "grep -q service utils/config.yaml && echo GREP_Q_OK")
	// mv with trailing slash to non-existent dir must fail (not create file)
	typeCmd(&events, "mv utils/config.yaml /home/learner/missing_dir/ || echo MV_SLASH_BLOCKED")
	// tar attached -f flag
	typeCmd(&events, "tar -czf/home/learner/attached.tar.gz utils && ls /home/learner/attached.tar.gz")
	// sandbox mission must not say Chapter 0
	typeCmd(&events, "sandbox")
	typeCmd(&events, "mission")
	typeCmd(&events, "reset")
	typeCmd(&events, "exit")

	mockTerm := term.NewMockTerminal(events...)
	application, err := app.New(app.Config{
		Term:            mockTerm,
		Store:           tmpStore,
		UI:              renderer,
		PanicSeconds:    0,
		PanicSecondsSet: true,
		InitialLang:     contract.LangEN,
	})
	if err != nil {
		t.Fatalf("app.New failed: %v", err)
	}
	if err := application.Run(context.Background()); err != nil {
		t.Fatalf("app.Run failed: %v", err)
	}

	out := mockTerm.Output.String()
	for _, want := range []string{
		".hidden_note",
		"LOCAL_NOT_EXPORTED",
		"EXPORTED_OK=visible",
		"/bin/bash",
		"/bin/sh",
		"GREP_Q_OK",
		"MV_SLASH_BLOCKED",
		"attached.tar.gz",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("expected output to contain %q\n--- OUTPUT ---\n%s", want, out)
		}
	}
	for _, forbid := range []string{
		"Chapter 0",
		"%!s(",
	} {
		if strings.Contains(out, forbid) {
			t.Errorf("expected output NOT to contain %q", forbid)
		}
	}
}
