package vfs

import (
	"path"
	"strings"

	"learn-linux/internal/contract"
)

// FilePath anchors a relative path without cleaning dot components.
func FilePath(cwd, name string) string {
	if name == "" || path.IsAbs(name) {
		return name
	}
	return strings.TrimSuffix(cwd, "/") + "/" + name
}

// LookPath searches the virtual PATH for a regular file.
func LookPath(v contract.VFS, cwd, searchPath, name string, executable bool) string {
	if name == "" || strings.ContainsRune(name, '/') {
		return ""
	}
	for dir := range strings.SplitSeq(searchPath, ":") {
		if dir == "" {
			dir = cwd
		}
		candidate := FilePath(FilePath(cwd, dir), name)
		if info, err := v.Stat(candidate); err == nil && info.Type == contract.TypeFile &&
			(!executable || info.IsExecutable()) {
			return candidate
		}
	}
	return ""
}
