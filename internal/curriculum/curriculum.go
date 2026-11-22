package curriculum

import (
	"fmt"
	"io/fs"
	"path"
	"slices"

	"learn-linux/internal/contract"
	"learn-linux/internal/vfs"
)

const home = "/home/learner"

var _ contract.Curriculum = (*Engine)(nil)

// Engine loads fresh chapter filesystems and evaluates progress from the supplied session state.
type Engine struct{}

func NewEngine() *Engine {
	return &Engine{}
}

func (*Engine) Chapters() []contract.ChapterMeta {
	chapters := make([]contract.ChapterMeta, 0, len(definitions)-1)
	for _, chapter := range definitions[1:] {
		chapters = append(chapters, chapter.metadata())
	}
	return chapters
}

func (*Engine) ChapterMeta(chapterID int) (contract.ChapterMeta, bool) {
	chapter, ok := lookup(chapterID)
	if !ok {
		return contract.ChapterMeta{}, false
	}
	return chapter.metadata(), true
}

func (*Engine) LoadChapter(chapterID int) (contract.VFS, string, error) {
	chapter, ok := lookup(chapterID)
	if !ok {
		return nil, "", fmt.Errorf("chapter %d: %w", chapterID, contract.ErrNotExist)
	}

	filesystem := vfs.NewDefaultLinuxVFS(home)
	for _, file := range chapter.files {
		name := atHome(file.name)
		if err := filesystem.MkdirAll(path.Dir(name), 0755); err != nil {
			return nil, "", fmt.Errorf("seed %s: %w", name, err)
		}
		if err := filesystem.WriteFile(name, []byte(file.contents), file.mode); err != nil {
			return nil, "", fmt.Errorf("seed %s: %w", name, err)
		}
	}
	return filesystem, home, nil
}

func (e *Engine) LoadSandbox() (contract.VFS, string, error) {
	return e.LoadChapter(0)
}

func (*Engine) Evaluate(chapterID int, filesystem contract.VFS, env map[string]string, history []contract.CommandRecord, lastCmd contract.CommandRecord) contract.ChapterState {
	chapter, ok := lookup(chapterID)
	if !ok {
		return contract.ChapterState{}
	}

	state := contract.ChapterState{Meta: chapter.metadata()}
	if state.Meta.IsSandbox {
		return state
	}

	session := evaluation{filesystem, env, commandHistory{history, lastCmd}}
	passed := chapter.goals(session)
	state.Goals = make([]contract.TaskGoal, len(state.Meta.GoalKeys))
	state.AllPassed = true
	for i, key := range state.Meta.GoalKeys {
		state.Goals[i] = contract.TaskGoal{ID: key, DescKey: key, Completed: passed[i]}
		state.AllPassed = state.AllPassed && passed[i]
	}
	if chapter.softLocked(filesystem) {
		state.SoftLocked = true
		state.SoftLockReasonKey = chapter.softLockKey
	}
	return state
}

func (*Engine) NextHintKey(chapterID int, currentIdx int) (string, int, int) {
	chapter, ok := lookup(chapterID)
	if !ok || len(chapter.meta.HintKeys) == 0 {
		return "", 0, 0
	}
	hints := chapter.meta.HintKeys
	currentIdx = max(0, currentIdx)
	if currentIdx >= len(hints) {
		return contract.KeyHintExhausted, 0, len(hints)
	}
	return hints[currentIdx], currentIdx + 1, len(hints)
}

type seedFile struct {
	name     string
	contents string
	mode     fs.FileMode
}

func dataFile(name, contents string) seedFile {
	return seedFile{name, contents, 0644}
}

type chapterDefinition struct {
	meta        contract.ChapterMeta
	files       []seedFile
	goals       func(evaluation) []bool
	softLocked  func(contract.VFS) bool
	softLockKey string
}

func (c chapterDefinition) metadata() contract.ChapterMeta {
	meta := c.meta
	meta.GoalKeys = slices.Clone(meta.GoalKeys)
	meta.HintKeys = slices.Clone(meta.HintKeys)
	meta.Commands = slices.Clone(meta.Commands)
	return meta
}

func lookup(id int) (chapterDefinition, bool) {
	if id < 0 || id >= len(definitions) {
		return chapterDefinition{}, false
	}
	return definitions[id], true
}
