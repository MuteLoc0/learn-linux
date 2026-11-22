package store_test

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"sync"
	"testing"

	"learn-linux/internal/contract"
	"learn-linux/internal/store"
)

func must(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}

func defaults() contract.Progress {
	return contract.Progress{
		CurrentChapter: 1, CompletedChapters: []int{},
		Language: contract.LangZH, CommandHistory: []string{},
	}
}

func TestStorePaths(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	if got, want := store.NewFileStore("").Path(), filepath.Join(home, ".learn-linux-cli", "progress.json"); got != want {
		t.Fatalf("default Path = %q, want %q", got, want)
	}
	custom := filepath.Join(home, "custom.json")
	if got := store.NewFileStore(custom).Path(); got != custom {
		t.Fatalf("custom Path = %q, want %q", got, custom)
	}
}

func TestLoad(t *testing.T) {
	for _, tc := range []struct {
		name, json string
		missing    bool
		want       contract.Progress
	}{
		{name: "missing", missing: true, want: defaults()},
		{name: "empty file", want: defaults()},
		{name: "malformed", json: `{"current_chapter": 7,`, want: defaults()},
		{name: "wrong type", json: `{"current_chapter": "two"}`, want: defaults()},
		{name: "omitted fields", json: `{}`, want: defaults()},
		{name: "null arrays", json: `{"completed_chapters": null, "command_history": null}`, want: defaults()},
		{name: "normalize", json: `{"current_chapter": 99, "completed_chapters": [3,1,3,2,1], "language": "invalid", "command_history": ["ls"]}`,
			want: contract.Progress{CurrentChapter: 8, CompletedChapters: []int{1, 2, 3}, Language: contract.LangZH, CommandHistory: []string{"ls"}}},
		{name: "clamp below zero", json: `{"current_chapter":-1,"language":"en"}`,
			want: contract.Progress{CurrentChapter: 0, CompletedChapters: []int{}, Language: contract.LangEN, CommandHistory: []string{}}},
		{name: "preserve sandbox chapter", json: `{"current_chapter":0,"language":"zh"}`,
			want: contract.Progress{CurrentChapter: 0, CompletedChapters: []int{}, Language: contract.LangZH, CommandHistory: []string{}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := store.NewFileStore(filepath.Join(t.TempDir(), "progress.json"))
			if !tc.missing {
				must(t, os.WriteFile(s.Path(), []byte(tc.json), 0600))
			}
			p, err := s.Load()
			must(t, err)
			if !reflect.DeepEqual(*p, tc.want) {
				t.Fatalf("Load = %+v, want %+v", *p, tc.want)
			}
		})
	}
}

func TestSaveLoadAndReset(t *testing.T) {
	for _, tc := range []struct {
		name          string
		historyLength int
	}{
		{"short history", 2},
		{"exact limit", 200},
		{"over limit", 253},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := store.NewFileStore(filepath.Join(t.TempDir(), "new", "nested", "progress.json"))
			history := make([]string, tc.historyLength)
			for i := range history {
				history[i] = fmt.Sprintf("command %d", i)
			}
			p := &contract.Progress{
				CurrentChapter: 4, CompletedChapters: []int{1, 2, 3},
				Language: contract.LangEN, CommandHistory: history,
			}
			must(t, s.Save(p))
			if !reflect.DeepEqual(p.CommandHistory, history) {
				t.Fatal("Save mutated its input history")
			}
			want := *p
			want.CommandHistory = history[max(0, len(history)-200):]
			got, err := s.Load()
			must(t, err)
			if !reflect.DeepEqual(*got, want) {
				t.Fatalf("round trip returned %d history entries, want %+v", len(got.CommandHistory), want)
			}
			p.CurrentChapter = 5
			must(t, s.Save(p))
			got, err = s.Load()
			must(t, err)
			if got.CurrentChapter != 5 {
				t.Fatalf("replacement chapter = %d", got.CurrentChapter)
			}
			entries, err := os.ReadDir(filepath.Dir(s.Path()))
			must(t, err)
			if len(entries) != 1 || entries[0].Name() != filepath.Base(s.Path()) {
				t.Fatalf("unexpected files after save: %v", entries)
			}
			must(t, s.Reset())
			must(t, s.Reset())
			if _, err := os.Stat(s.Path()); !os.IsNotExist(err) {
				t.Fatalf("progress file still exists after Reset: %v", err)
			}
			got, err = s.Load()
			must(t, err)
			if !reflect.DeepEqual(*got, defaults()) {
				t.Fatalf("Load after Reset = %+v", *got)
			}
		})
	}
}

func TestConcurrentSavesAndLoads(t *testing.T) {
	s := store.NewFileStore(filepath.Join(t.TempDir(), "progress.json"))
	progress := func(chapter int) *contract.Progress {
		return &contract.Progress{
			CurrentChapter: chapter, CompletedChapters: []int{chapter},
			Language: contract.LangEN, CommandHistory: []string{fmt.Sprint(chapter)},
		}
	}
	must(t, s.Save(progress(1)))
	var wg sync.WaitGroup
	for chapter := 1; chapter <= 8; chapter++ {
		wg.Go(func() {
			p := progress(chapter)
			for i := 0; i < 12; i++ {
				if err := s.Save(p); err != nil {
					t.Error(err)
					return
				}
				loaded, err := s.Load()
				if err != nil {
					t.Error(err)
					return
				}
				if !reflect.DeepEqual(loaded, progress(loaded.CurrentChapter)) {
					t.Errorf("Load observed a partial write: %+v", loaded)
					return
				}
			}
		})
	}
	wg.Wait()
}
