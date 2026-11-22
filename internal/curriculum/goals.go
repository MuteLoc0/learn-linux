package curriculum

import (
	"path"
	"slices"
	"strings"

	"learn-linux/internal/contract"
)

type evaluation struct {
	fs       contract.VFS
	env      map[string]string
	commands commandHistory
}

func navigationGoals(e evaluation) []bool {
	return []bool{
		e.commands.ran("pwd", nil),
		e.commands.ran("ls", func(args []string) bool { return hasOption(args, "a", "all") }),
		e.commands.ran("tree", nil),
		e.commands.any(func(record contract.CommandRecord) bool { return record.CWD == atHome(tabDirectory) }),
	}
}

func fileManagementGoals(e evaluation) []bool {
	v := e.fs
	notes := atHome("lab/notes.md")
	report := "lab/docs/report.txt"
	return []bool{
		v.IsDir(atHome("lab/docs/archive")),
		v.Exists(notes) && !v.IsDir(notes),
		!v.Exists(atHome("inbox/report_draft.txt")) && fileContains(v, report, reportDraft),
		v.Exists(atHome("lab/templates_backup/config.ini")) && !v.Exists(atHome("inbox/junk.tmp")),
		linksTo(v, "lab/current_report", report),
	}
}

func textEditingGoals(e evaluation) []bool {
	logArg := func(args []string) bool { return referencesFile(args, "server.log") }
	config := nonEmptyLines(readFile(e.fs, "app.conf"))
	return []bool{
		e.commands.successful(func(record contract.CommandRecord) bool {
			return matchesCommand(record, "head", logArg) || matchesCommand(record, "tail", logArg) ||
				rawCommand(record.Raw, "head", nil) || rawCommand(record.Raw, "tail", nil)
		}),
		e.commands.ran("wc", nil),
		e.commands.ran("less", logArg),
		slices.Contains(config, "debug=false") && slices.Contains(config, "port=9000"),
	}
}

func searchProcessingGoals(e evaluation) []bool {
	return []bool{
		e.commands.successful(func(record contract.CommandRecord) bool {
			return strings.Contains(record.Stdout, "config.yaml") && matchesCommand(record, "find", nil)
		}),
		e.commands.successful(func(record contract.CommandRecord) bool {
			return strings.Contains(record.Stdout, "ERROR") && matchesCommand(record, "grep", recursiveErrorSearch)
		}),
		e.commands.successful(func(record contract.CommandRecord) bool {
			return strings.Contains(record.Stdout, "alice") && strings.Contains(record.Stdout, "charlie") &&
				!strings.Contains(record.Stdout, ",admin") &&
				(matchesCommand(record, "cut", func(args []string) bool { return referencesFile(args, "users.csv") }) ||
					(matchesCommand(record, "cut", nil) && strings.Contains(record.Raw, "users.csv")))
		}),
		e.commands.successful(func(record contract.CommandRecord) bool {
			return deduplicatedUsers(record.Stdout) && (matchesCommand(record, "sort", func(args []string) bool {
				return hasOption(args, "u", "unique")
			}) || (matchesCommand(record, "sort", nil) && matchesCommand(record, "uniq", nil)))
		}),
	}
}

func pipesRedirectsGoals(e evaluation) []bool {
	errors := nonEmptyLines(readFile(e.fs, "errors.txt"))
	const footer = "--- END ---"
	hasFooter := len(errors) > 0 && errors[len(errors)-1] == footer
	if hasFooter {
		errors = errors[:len(errors)-1]
	}
	sortedErrors := slices.Equal(errors, []string{
		"ERROR auth token expired", "ERROR cache miss storm", "ERROR db connection timeout",
	})

	return []bool{
		sortedErrors,
		sortedErrors && hasFooter,
		fileContains(e.fs, "combined.txt", "Part A:", "Part B:", "Part C:"),
		e.fs.IsDir(atHome("done_dir")) && e.fs.Exists(atHome("done_dir/ok.txt")) &&
			e.commands.successful(func(record contract.CommandRecord) bool { return hasAndOperator(record.Raw) }),
	}
}

func permissionsGoals(e evaluation) []bool {
	return []bool{
		e.commands.ran("stat", nil) || e.commands.ran("ls", func(args []string) bool { return hasOption(args, "l") }),
		executable(e.fs, buildScript),
		e.env["BUILD_MODE"] == "release",
		fileContains(e.fs, "build/out/artifact.txt", "BUILD_OK:release"),
	}
}

func archivesGoals(e evaluation) []bool {
	return []bool{
		virtualBinary(e.fs, "project/app"),
		e.commands.produced(appOutput),
		archiveExists(e.fs, "project/assets.tar.gz"),
		fileContains(e.fs, "extracted/assets/version.txt", "v1.0.0") ||
			fileContains(e.fs, "extracted/version.txt", "v1.0.0"),
	}
}

func capstoneGoals(e evaluation) []bool {
	critical := readFile(e.fs, releaseRoot+"critical.txt")
	return []bool{
		strings.Contains(critical, tlsCritical) && strings.Contains(critical, dbCritical) && !strings.Contains(critical, "INFO"),
		fileContains(e.fs, deployConfig, "ENV=prod"),
		executable(e.fs, deployScript) && virtualBinary(e.fs, releaseRoot+"dist/service"),
		e.commands.produced(serviceOutput),
		archiveExists(e.fs, "release_v2.tar.gz"),
	}
}

func atHome(name string) string {
	return home + "/" + name
}

func missing(v contract.VFS, names ...string) bool {
	return slices.ContainsFunc(names, func(name string) bool { return !v.Exists(atHome(name)) })
}

func readFile(v contract.VFS, name string) string {
	data, err := v.ReadFile(atHome(name))
	if err != nil {
		return ""
	}
	return string(data)
}

func fileContains(v contract.VFS, name string, fragments ...string) bool {
	content := readFile(v, name)
	for _, fragment := range fragments {
		if !strings.Contains(content, fragment) {
			return false
		}
	}
	return true
}

func executable(v contract.VFS, name string) bool {
	info, err := v.Stat(atHome(name))
	return err == nil && info.IsExecutable()
}

func virtualBinary(v contract.VFS, name string) bool {
	info, err := v.Stat(atHome(name))
	return err == nil && info.IsVirtualBinary && info.IsExecutable()
}

func archiveExists(v contract.VFS, name string) bool {
	info, err := v.Stat(atHome(name))
	return err == nil && info.Type == contract.TypeFile && info.Size > 20
}

func linksTo(v contract.VFS, link, target string) bool {
	name := atHome(link)
	info, err := v.Lstat(name)
	if err != nil || info.Type != contract.TypeSymlink {
		return false
	}
	resolved, err := v.Stat(name)
	if err != nil {
		return false
	}
	expected, err := v.Stat(atHome(target))
	return err == nil && expected.Type == contract.TypeFile && expected.Path == resolved.Path
}

func nonEmptyLines(text string) []string {
	var lines []string
	for line := range strings.SplitSeq(text, "\n") {
		if trimmed := strings.TrimSpace(line); trimmed != "" {
			lines = append(lines, trimmed)
		}
	}
	return lines
}

func referencesFile(args []string, filename string) bool {
	return slices.ContainsFunc(args, func(arg string) bool { return path.Base(arg) == filename })
}

func deduplicatedUsers(output string) bool {
	lines := nonEmptyLines(output)
	for i, line := range lines {
		lines[i], _, _ = strings.Cut(line, ",")
	}
	slices.Sort(lines)
	return slices.Equal(lines, []string{"alice", "bob", "charlie", "dave"})
}
