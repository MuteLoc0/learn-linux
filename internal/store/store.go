package store

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"slices"

	"learn-linux/internal/contract"
)

const historyLimit = 200

// FileStore persists progress as a JSON file on the host filesystem.
type FileStore struct {
	path    string
	pathErr error
}

var _ contract.ProgressStore = (*FileStore)(nil)

func NewFileStore(customPath string) *FileStore {
	if customPath != "" {
		return &FileStore{path: customPath}
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return &FileStore{pathErr: err}
	}
	return &FileStore{path: filepath.Join(home, ".learn-linux-cli", "progress.json")}
}

func defaultProgress() *contract.Progress {
	return &contract.Progress{
		CurrentChapter: 1, CompletedChapters: []int{},
		Language: contract.LangZH, CommandHistory: []string{},
	}
}

func (s *FileStore) Path() string {
	return s.path
}

func (s *FileStore) Load() (*contract.Progress, error) {
	if s.pathErr != nil {
		return nil, s.pathErr
	}
	data, err := os.ReadFile(s.path)
	if errors.Is(err, os.ErrNotExist) {
		return defaultProgress(), nil
	}
	if err != nil {
		return nil, err
	}
	p := defaultProgress()
	if err := json.Unmarshal(data, p); err != nil {
		return defaultProgress(), nil
	}
	p.CurrentChapter = max(0, min(p.CurrentChapter, 8))
	if p.Language != contract.LangEN && p.Language != contract.LangZH {
		p.Language = contract.LangZH
	}
	if p.CompletedChapters == nil {
		p.CompletedChapters = []int{}
	}
	slices.Sort(p.CompletedChapters)
	p.CompletedChapters = slices.Compact(p.CompletedChapters)
	if p.CommandHistory == nil {
		p.CommandHistory = []string{}
	}
	return p, nil
}

func (s *FileStore) Save(p *contract.Progress) error {
	if s.pathErr != nil {
		return s.pathErr
	}
	if p == nil {
		return errors.New("cannot save nil progress")
	}
	saved := *p
	if len(saved.CommandHistory) > historyLimit {
		saved.CommandHistory = saved.CommandHistory[len(saved.CommandHistory)-historyLimit:]
	}
	dir := filepath.Dir(s.path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, ".progress-*.tmp")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	defer tmp.Close()
	if err := json.NewEncoder(tmp).Encode(&saved); err != nil {
		return err
	}
	if err := tmp.Sync(); err != nil {
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), s.path)
}

func (s *FileStore) Reset() error {
	if s.pathErr != nil {
		return s.pathErr
	}
	err := os.Remove(s.path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return err
}
