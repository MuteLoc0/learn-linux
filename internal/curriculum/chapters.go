package curriculum

import (
	"fmt"
	"strings"

	"learn-linux/internal/contract"
)

const (
	tabDirectory  = "workspace/long-directory-name-for-tab-practice"
	reportDraft   = "Quarterly Linux Report Draft v1"
	serverLog     = "logs/server.log"
	buildScript   = "build/compile.sh"
	releaseRoot   = "release_prep/"
	deployScript  = releaseRoot + "scripts/compile.sh"
	deployConfig  = releaseRoot + "config/env.conf"
	deployLog     = releaseRoot + "logs/deploy.log"
	tlsCritical   = "2026-11-22 08:00:04 CRITICAL tls certificate expires in 24h"
	dbCritical    = "2026-11-22 08:00:15 CRITICAL database replica lag exceeded threshold"
	appOutput     = "Hello from Virtual Linux C App!"
	serviceOutput = "Service v2.0 ready"
)

var definitions = [...]chapterDefinition{
	0: {
		meta: contract.ChapterMeta{
			ID: 0, Slug: "sandbox", IsSandbox: true,
			TitleKey: contract.KeySandboxMetaTitle, SubtitleKey: contract.KeySandboxMetaSubtitle, IntroKey: contract.KeySandboxMetaIntro,
		},
		files: []seedFile{
			dataFile("notes.txt", "alpha\nbeta\ngamma\nalpha\n"),
			dataFile(".hidden_note", "hidden\n"),
			dataFile("data/users.csv", "alice,admin,90\nbob,dev,85\ncharlie,dev,92\n"),
			dataFile(serverLog, serverLogContents()),
			dataFile("workspace/projects/alpha/status.txt", "status=active\n"),
			dataFile("workspace/projects/beta/status.txt", "status=pending\n"),
			dataFile("hello.c", cProgram("Hello, Linux!")),
			dataFile("Makefile", makefile("hello", "hello.c")),
			{"hello.sh", "#!/bin/sh\necho 'Hello, Linux!'\n", 0755},
		},
	},
	1: {
		meta: contract.ChapterMeta{
			ID: 1, Slug: "navigation",
			TitleKey: contract.KeyCh1Title, SubtitleKey: contract.KeyCh1Subtitle, IntroKey: contract.KeyCh1Intro,
			GoalKeys: []string{contract.KeyCh1Goal1, contract.KeyCh1Goal2, contract.KeyCh1Goal3, contract.KeyCh1Goal4},
			HintKeys: []string{contract.KeyCh1Hint1, contract.KeyCh1Hint2, contract.KeyCh1Hint3},
			Commands: []string{"pwd", "ls", "cd", "tree", "clear"},
		},
		files: []seedFile{
			dataFile("welcome.txt", "Welcome to learn-linux!\nTry ls -la, tree, and Tab completion.\n"),
			dataFile(".hidden_note", "You found the hidden file! Dotfiles start with '.' in Linux.\n"),
			dataFile("workspace/projects/alpha/notes.txt", "Alpha project notes\n"),
			dataFile("workspace/projects/beta/status.txt", "Beta status: active\n"),
			dataFile(tabDirectory+"/secret.txt", "Nice Tab completion!\n"),
		},
		goals:       navigationGoals,
		softLocked:  func(v contract.VFS) bool { return !v.IsDir(atHome(tabDirectory)) },
		softLockKey: contract.KeyCh1SoftLock,
	},
	2: {
		meta: contract.ChapterMeta{
			ID: 2, Slug: "file-management",
			TitleKey: contract.KeyCh2Title, SubtitleKey: contract.KeyCh2Subtitle, IntroKey: contract.KeyCh2Intro,
			GoalKeys: []string{contract.KeyCh2Goal1, contract.KeyCh2Goal2, contract.KeyCh2Goal3, contract.KeyCh2Goal4, contract.KeyCh2Goal5},
			HintKeys: []string{contract.KeyCh2Hint1, contract.KeyCh2Hint2, contract.KeyCh2Hint3},
			Commands: []string{"mkdir", "touch", "cp", "mv", "rm", "ln"},
		},
		files: []seedFile{
			dataFile("inbox/report_draft.txt", reportDraft+"\n"),
			dataFile("inbox/junk.tmp", "obsolete temporary data\n"),
			dataFile("templates/config.ini", "[app]\nmode=dev\nretries=3\n"),
			dataFile("templates/readme.txt", "Standard project template\n"),
		},
		goals: fileManagementGoals,
		softLocked: func(v contract.VFS) bool {
			return (!v.Exists(atHome("inbox/report_draft.txt")) && !v.Exists(atHome("lab/docs/report.txt"))) ||
				(!v.Exists(atHome("templates/config.ini")) && !v.Exists(atHome("lab/templates_backup/config.ini")))
		},
		softLockKey: contract.KeyCh2SoftLock,
	},
	3: {
		meta: contract.ChapterMeta{
			ID: 3, Slug: "text-editing",
			TitleKey: contract.KeyCh3Title, SubtitleKey: contract.KeyCh3Subtitle, IntroKey: contract.KeyCh3Intro,
			GoalKeys: []string{contract.KeyCh3Goal1, contract.KeyCh3Goal2, contract.KeyCh3Goal3, contract.KeyCh3Goal4},
			HintKeys: []string{contract.KeyCh3Hint1, contract.KeyCh3Hint2, contract.KeyCh3Hint3},
			Commands: []string{"cat", "head", "tail", "wc", "less", "nano"},
		},
		files: []seedFile{
			dataFile(serverLog, serverLogContents()),
			dataFile("app.conf", "# Application Configuration\nhost=127.0.0.1\nport=8080\ndebug=true\n"),
		},
		goals:       textEditingGoals,
		softLocked:  func(v contract.VFS) bool { return missing(v, serverLog, "app.conf") },
		softLockKey: contract.KeyCh3SoftLock,
	},
	4: {
		meta: contract.ChapterMeta{
			ID: 4, Slug: "search-processing",
			TitleKey: contract.KeyCh4Title, SubtitleKey: contract.KeyCh4Subtitle, IntroKey: contract.KeyCh4Intro,
			GoalKeys: []string{contract.KeyCh4Goal1, contract.KeyCh4Goal2, contract.KeyCh4Goal3, contract.KeyCh4Goal4},
			HintKeys: []string{contract.KeyCh4Hint1, contract.KeyCh4Hint2, contract.KeyCh4Hint3},
			Commands: []string{"find", "grep", "cut", "sort", "uniq"},
		},
		files: []seedFile{
			dataFile("src/main.go", "package main\n// ERROR: missing timeout handler\nfunc main() {}\n"),
			dataFile("src/utils/helper.go", "package utils\n// TODO: refactor\n// ERROR: nil pointer guard needed\n"),
			dataFile("src/utils/config.yaml", "service: api\nport: 8080\n"),
			dataFile("src/legacy/old.log", "INFO started\nERROR disk full\n"),
			dataFile("data/users.csv", "alice,admin,90\nbob,dev,85\ncharlie,dev,92\nalice,admin,90\ndave,ops,78\nbob,dev,85\n"),
		},
		goals:       searchProcessingGoals,
		softLocked:  func(v contract.VFS) bool { return missing(v, "src/utils/config.yaml", "data/users.csv") },
		softLockKey: contract.KeyCh4SoftLock,
	},
	5: {
		meta: contract.ChapterMeta{
			ID: 5, Slug: "pipes-redirects",
			TitleKey: contract.KeyCh5Title, SubtitleKey: contract.KeyCh5Subtitle, IntroKey: contract.KeyCh5Intro,
			GoalKeys: []string{contract.KeyCh5Goal1, contract.KeyCh5Goal2, contract.KeyCh5Goal3, contract.KeyCh5Goal4},
			HintKeys: []string{contract.KeyCh5Hint1, contract.KeyCh5Hint2, contract.KeyCh5Hint3},
			Commands: []string{"|", ">", ">>", "<", "&&", "||", ";", "*"},
		},
		files: []seedFile{
			dataFile("events/app.log", `INFO boot complete
ERROR db connection timeout
WARN high memory usage
ERROR cache miss storm
ERROR db connection timeout
INFO request served
ERROR auth token expired
ERROR cache miss storm
`),
			dataFile("events/notes_a.txt", "Part A: Pipes connect stdout to stdin.\n"),
			dataFile("events/notes_b.txt", "Part B: > overwrites and >> appends.\n"),
			dataFile("events/notes_c.txt", "Part C: && runs next command only on success.\n"),
		},
		goals: pipesRedirectsGoals,
		softLocked: func(v contract.VFS) bool {
			return missing(v, "events/app.log", "events/notes_a.txt", "events/notes_b.txt", "events/notes_c.txt")
		},
		softLockKey: contract.KeyCh5SoftLock,
	},
	6: {
		meta: contract.ChapterMeta{
			ID: 6, Slug: "permissions-scripts",
			TitleKey: contract.KeyCh6Title, SubtitleKey: contract.KeyCh6Subtitle, IntroKey: contract.KeyCh6Intro,
			GoalKeys: []string{contract.KeyCh6Goal1, contract.KeyCh6Goal2, contract.KeyCh6Goal3, contract.KeyCh6Goal4},
			HintKeys: []string{contract.KeyCh6Hint1, contract.KeyCh6Hint2, contract.KeyCh6Hint3},
			Commands: []string{"stat", "chmod", "env", "export", "alias", "./compile.sh"},
		},
		files: []seedFile{
			dataFile(buildScript, `#!/bin/sh
if [ "$BUILD_MODE" != "release" ]; then
  echo "Error: BUILD_MODE must be set to 'release'"
  exit 1
fi
mkdir -p /home/learner/build/out
echo "BUILD_OK:$BUILD_MODE" > /home/learner/build/out/artifact.txt
echo "Compilation succeeded in $BUILD_MODE mode!"
`),
			dataFile("build/README.txt", "Build instructions:\n1. Ensure compile.sh has execute permission.\n2. Export BUILD_MODE=release.\n3. Run ./compile.sh\n"),
		},
		goals:       permissionsGoals,
		softLocked:  func(v contract.VFS) bool { return missing(v, buildScript) },
		softLockKey: contract.KeyCh6SoftLock,
	},
	7: {
		meta: contract.ChapterMeta{
			ID: 7, Slug: "archives-toolchain",
			TitleKey: contract.KeyCh7Title, SubtitleKey: contract.KeyCh7Subtitle, IntroKey: contract.KeyCh7Intro,
			GoalKeys: []string{contract.KeyCh7Goal1, contract.KeyCh7Goal2, contract.KeyCh7Goal3, contract.KeyCh7Goal4},
			HintKeys: []string{contract.KeyCh7Hint1, contract.KeyCh7Hint2, contract.KeyCh7Hint3},
			Commands: []string{"tar", "gzip", "gcc", "make"},
		},
		files: []seedFile{
			dataFile("project/src/main.c", cProgram(appOutput)),
			dataFile("project/Makefile", makefile("app", "src/main.c")),
			dataFile("project/assets/readme.txt", "Release assets bundle\n"),
			dataFile("project/assets/version.txt", "v1.0.0\n"),
		},
		goals:       archivesGoals,
		softLocked:  func(v contract.VFS) bool { return missing(v, "project/src/main.c", "project/assets/version.txt") },
		softLockKey: contract.KeyCh7SoftLock,
	},
	8: {
		meta: contract.ChapterMeta{
			ID: 8, Slug: "capstone",
			TitleKey: contract.KeyCh8Title, SubtitleKey: contract.KeyCh8Subtitle, IntroKey: contract.KeyCh8Intro,
			GoalKeys: []string{contract.KeyCh8Goal1, contract.KeyCh8Goal2, contract.KeyCh8Goal3, contract.KeyCh8Goal4, contract.KeyCh8Goal5},
			HintKeys: []string{contract.KeyCh8Hint1, contract.KeyCh8Hint2, contract.KeyCh8Hint3, contract.KeyCh8Hint4},
			Commands: []string{"find", "grep", "nano", "chmod", "gcc", "tar", "./compile.sh"},
		},
		files: []seedFile{
			dataFile(deployScript, `#!/bin/sh
if ! grep "ENV=prod" /home/learner/release_prep/config/env.conf > /dev/null; then
  echo "Build blocked: config/env.conf must have ENV=prod"
  exit 1
fi
mkdir -p /home/learner/release_prep/dist
gcc -o /home/learner/release_prep/dist/service /home/learner/release_prep/src/app.c
echo "Service binary built at release_prep/dist/service"
`),
			dataFile(releaseRoot+"src/app.c", cProgram(serviceOutput)),
			dataFile(deployConfig, "ENV=staging\nLOG_LEVEL=debug\n"),
			dataFile(deployLog, "2026-11-22 08:00:01 INFO deployment check started\n"+
				tlsCritical+"\n2026-11-22 08:00:09 INFO healthcheck ok\n"+
				dbCritical+"\n2026-11-22 08:00:22 INFO check finished\n"),
		},
		goals: capstoneGoals,
		softLocked: func(v contract.VFS) bool {
			return missing(v, deployScript, releaseRoot+"src/app.c", deployConfig, deployLog)
		},
		softLockKey: contract.KeyCh8SoftLock,
	},
}

func serverLogContents() string {
	messages := []string{
		"INFO http GET /health returned 200",
		"INFO worker completed scheduled cache refresh",
		"WARN api request exceeded 250ms latency target",
		"INFO db connection pool allocated idle connection",
		"INFO http GET /api/users returned 200",
		"ERROR upstream payment API returned 503",
		"WARN upstream request queued for retry",
		"INFO upstream retry returned 200",
		"INFO metrics snapshot sent to collector",
		"WARN cache evicted entries near memory limit",
		"INFO worker completed background sync",
		"INFO http GET /api/status returned 200",
	}
	var log strings.Builder
	for line := 1; line <= 60; line++ {
		message := messages[(line-1)%len(messages)]
		if line == 45 {
			message = "INFO auth TOKEN=ALPHA-742 verified"
		}
		fmt.Fprintf(&log, "2026-11-22 %02d:%02d:00 %s\n", 10+line/60, line%60, message)
	}
	return log.String()
}

func cProgram(output string) string {
	return fmt.Sprintf("#include <stdio.h>\n\nint main(void) {\n    printf(%q);\n    return 0;\n}\n", output+"\n")
}

func makefile(target, source string) string {
	return fmt.Sprintf("CC = gcc\nTARGET = %s\n\nall: $(TARGET)\n\n$(TARGET): %[2]s\n\t$(CC) -o $(TARGET) %[2]s\n\nclean:\n\trm -f $(TARGET)\n", target, source)
}
