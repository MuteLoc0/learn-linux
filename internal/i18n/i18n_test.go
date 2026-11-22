package i18n

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"testing"

	"learn-linux/internal/contract"
)

var expectedManCommands = []string{
	"alias", "bash", "cat", "cd", "chapter", "check", "chmod", "clear",
	"cp", "cut", "date", "echo", "env", "exit", "export", "file",
	"find", "gcc", "grep", "gunzip", "gzip", "head", "help", "hint",
	"history", "hostname", "id", "lang", "less", "ln", "ls", "make",
	"man", "mission", "mkdir", "mv", "nano", "printf", "pwd", "reset",
	"rm", "sandbox", "sh", "sort", "stat", "tail", "tar", "touch",
	"tree", "unalias", "uname", "uniq", "unset", "wc", "which", "whoami",
}

func extractContractKeys(t *testing.T) map[string]string {
	t.Helper()
	keysPath := filepath.Join("..", "contract", "keys.go")
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, keysPath, nil, 0)
	if err != nil {
		t.Fatalf("failed to parse %s: %v", keysPath, err)
	}

	keys := make(map[string]string)
	for _, decl := range file.Decls {
		gd, ok := decl.(*ast.GenDecl)
		if !ok || gd.Tok != token.CONST {
			continue
		}
		for _, spec := range gd.Specs {
			vs, ok := spec.(*ast.ValueSpec)
			if !ok {
				continue
			}
			for i, name := range vs.Names {
				if !strings.HasPrefix(name.Name, "Key") {
					continue
				}
				lit, ok := vs.Values[i].(*ast.BasicLit)
				if !ok || lit.Kind != token.STRING {
					t.Fatalf("constant %s is not a string literal", name.Name)
				}
				val, err := strconv.Unquote(lit.Value)
				if err != nil {
					t.Fatalf("failed to unquote %s: %v", name.Name, err)
				}
				keys[name.Name] = val
			}
		}
	}
	if len(keys) == 0 {
		t.Fatal("expected to extract contract key constants, got 0")
	}
	return keys
}

func TestBundleLoadsCleanlyAndDefaults(t *testing.T) {
	bZH, err := NewBundle(contract.LangZH)
	if err != nil {
		t.Fatalf("NewBundle(LangZH) error: %v", err)
	}
	if bZH.Lang() != contract.LangZH {
		t.Fatalf("expected LangZH, got %q", bZH.Lang())
	}

	bEN, err := NewBundle(contract.LangEN)
	if err != nil {
		t.Fatalf("NewBundle(LangEN) error: %v", err)
	}
	if bEN.Lang() != contract.LangEN {
		t.Fatalf("expected LangEN, got %q", bEN.Lang())
	}

	bDefault, err := NewBundle("")
	if err != nil {
		t.Fatalf("NewBundle(\"\") error: %v", err)
	}
	if bDefault.Lang() != contract.LangZH {
		t.Fatalf("expected empty default lang to fall back to LangZH, got %q", bDefault.Lang())
	}

	if _, err := NewBundle(contract.Lang("fr")); err == nil {
		t.Fatal("expected error for unsupported language \"fr\", got nil")
	}
}

func TestStringCatalogParityAndAllContractKeysPresent(t *testing.T) {
	b, err := NewBundle(contract.LangZH)
	if err != nil {
		t.Fatalf("NewBundle error: %v", err)
	}

	zhMap := b.strings[contract.LangZH]
	enMap := b.strings[contract.LangEN]

	// 1. Every key in zh.json must exist in en.json and be non-empty.
	for k, v := range zhMap {
		if strings.TrimSpace(v) == "" {
			t.Errorf("zh.json has empty value for key %q", k)
		}
		if _, ok := enMap[k]; !ok {
			t.Errorf("key %q exists in zh.json but is missing in en.json", k)
		}
	}

	// 2. Every key in en.json must exist in zh.json and be non-empty.
	for k, v := range enMap {
		if strings.TrimSpace(v) == "" {
			t.Errorf("en.json has empty value for key %q", k)
		}
		if _, ok := zhMap[k]; !ok {
			t.Errorf("key %q exists in en.json but is missing in zh.json", k)
		}
	}

	// 3. Every Key* constant in internal/contract/keys.go must exist and be non-empty in BOTH languages.
	contractKeys := extractContractKeys(t)
	for constName, keyVal := range contractKeys {
		if v, ok := zhMap[keyVal]; !ok || strings.TrimSpace(v) == "" {
			t.Errorf("contract constant %s (%q) missing or empty in zh.json", constName, keyVal)
		}
		if v, ok := enMap[keyVal]; !ok || strings.TrimSpace(v) == "" {
			t.Errorf("contract constant %s (%q) missing or empty in en.json", constName, keyVal)
		}
		if !b.Has(keyVal) {
			t.Errorf("Bundle.Has(%q) returned false for contract constant %s", keyVal, constName)
		}
	}
}

func TestFormatPlaceholders(t *testing.T) {
	cases := []struct {
		key  string
		args []any
	}{
		{contract.KeyMOTDProgress, []any{3, 8}},
		{contract.KeyMissionHeader, []any{1, "Navigation"}},
		{contract.KeyHintHeader, []any{2, 3}},
		{contract.KeyClearedBody, []any{"Navigation"}},
		{contract.KeyClearedNextHint, []any{2}},
		{contract.KeyChapterNotFound, []any{"99"}},
		{contract.KeyManNotFound, []any{"foobar"}},
		{contract.KeyLangSwitched, []any{"English"}},
		{contract.KeyResetDone, []any{"Chapter 1"}},
		{contract.KeyPanicCountdown, []any{3}},
		{contract.KeyCmdNotFound, []any{"bogus"}},
		{contract.KeyEditorSaved, []any{42, "app.conf"}},
		{contract.KeyPagerNotFound, []any{"TOKEN"}},
	}

	b, err := NewBundle(contract.LangZH)
	if err != nil {
		t.Fatalf("NewBundle error: %v", err)
	}

	for _, lang := range []contract.Lang{contract.LangZH, contract.LangEN} {
		if err := b.SetLang(lang); err != nil {
			t.Fatalf("SetLang(%q) error: %v", lang, err)
		}
		for _, tc := range cases {
			rendered := b.T(tc.key, tc.args...)
			if strings.Contains(rendered, "%!") {
				t.Errorf("[%s] key %q formatted with fmt error: %q", lang, tc.key, rendered)
			}
			if rendered == tc.key {
				t.Errorf("[%s] key %q fell back to key name", lang, tc.key)
			}
		}
	}
}

func TestManCatalogParityAndCompleteness(t *testing.T) {
	b, err := NewBundle(contract.LangZH)
	if err != nil {
		t.Fatalf("NewBundle error: %v", err)
	}

	if len(expectedManCommands) != 56 {
		t.Fatalf("expectedManCommands should have 56 items, got %d", len(expectedManCommands))
	}

	zhMan := b.manDocs[contract.LangZH]
	enMan := b.manDocs[contract.LangEN]

	// Parity between zh and en.
	for cmd := range zhMan {
		if _, ok := enMan[cmd]; !ok {
			t.Errorf("command %q exists in man_zh.json but is missing in man_en.json", cmd)
		}
	}
	for cmd := range enMan {
		if _, ok := zhMan[cmd]; !ok {
			t.Errorf("command %q exists in man_en.json but is missing in man_zh.json", cmd)
		}
	}

	// Verify all 56 expected commands exist and have non-empty required fields in both languages.
	for _, lang := range []contract.Lang{contract.LangZH, contract.LangEN} {
		if err := b.SetLang(lang); err != nil {
			t.Fatalf("SetLang(%q) error: %v", lang, err)
		}
		for _, cmd := range expectedManCommands {
			doc, ok := b.ManPage(cmd)
			if !ok {
				t.Errorf("[%s] missing man page for %q", lang, cmd)
				continue
			}
			if strings.TrimSpace(doc.Name) == "" {
				t.Errorf("[%s] man %q has empty Name", lang, cmd)
			}
			if strings.TrimSpace(doc.Summary) == "" {
				t.Errorf("[%s] man %q has empty Summary", lang, cmd)
			}
			if strings.TrimSpace(doc.Synopsis) == "" {
				t.Errorf("[%s] man %q has empty Synopsis", lang, cmd)
			}
			if strings.TrimSpace(doc.Description) == "" {
				t.Errorf("[%s] man %q has empty Description", lang, cmd)
			}
			if len(doc.Examples) < 2 {
				t.Errorf("[%s] man %q has %d examples, want >= 2", lang, cmd, len(doc.Examples))
			}
			for i, ex := range doc.Examples {
				if strings.TrimSpace(ex.Cmd) == "" || strings.TrimSpace(ex.Desc) == "" {
					t.Errorf("[%s] man %q example[%d] has empty cmd or desc", lang, cmd, i)
				}
			}
		}
	}

	// AllManCommands should be sorted and match all 56 expected commands.
	gotCmds := b.AllManCommands()
	wantCmds := append([]string(nil), expectedManCommands...)
	sort.Strings(wantCmds)
	if len(gotCmds) != len(wantCmds) {
		t.Fatalf("AllManCommands() returned %d commands, want %d", len(gotCmds), len(wantCmds))
	}
	for i := range wantCmds {
		if gotCmds[i] != wantCmds[i] {
			t.Errorf("AllManCommands()[%d] = %q, want %q", i, gotCmds[i], wantCmds[i])
		}
	}

	// Case-insensitive lookup and missing command handling.
	doc, ok := b.ManPage("  GREP ")
	if !ok || doc.Name != "grep" {
		t.Errorf("expected case-insensitive lookup for GREP to succeed, got ok=%v name=%q", ok, doc.Name)
	}
	if _, ok := b.ManPage("nonexistent_cmd"); ok {
		t.Error("expected ManPage for unknown command to return false")
	}

	// Ensure returned ManDoc slices are defensively cloned.
	doc.Examples[0].Cmd = "mutated"
	doc2, _ := b.ManPage("grep")
	if doc2.Examples[0].Cmd == "mutated" {
		t.Error("ManPage returned shared slice backing array instead of a defensive copy")
	}
}

func TestSetLangFallbackAndInvalidLang(t *testing.T) {
	b, err := NewBundle(contract.LangZH)
	if err != nil {
		t.Fatalf("NewBundle error: %v", err)
	}

	zhTitle := b.T(contract.KeyCh1Title)
	if err := b.SetLang(contract.LangEN); err != nil {
		t.Fatalf("SetLang(LangEN) error: %v", err)
	}
	if b.Lang() != contract.LangEN {
		t.Fatalf("expected LangEN, got %q", b.Lang())
	}
	enTitle := b.T(contract.KeyCh1Title)
	if zhTitle == enTitle {
		t.Errorf("expected zh and en titles to differ, both were %q", zhTitle)
	}

	// Invalid language should error and leave active language unchanged.
	for _, invalid := range []contract.Lang{"", "ja", "zh-CN", "EN"} {
		if err := b.SetLang(invalid); err == nil {
			t.Errorf("SetLang(%q) expected error, got nil", invalid)
		}
	}
	if b.Lang() != contract.LangEN {
		t.Errorf("active language changed after invalid SetLang: got %q", b.Lang())
	}

	// Unknown key falls back to key itself.
	const missing = "does.not.exist"
	if b.Has(missing) {
		t.Errorf("Has(%q) = true, want false", missing)
	}
	if got := b.T(missing); got != missing {
		t.Errorf("T(%q) = %q, want %q", missing, got, missing)
	}

	// English fallback when key is missing only in zh.
	b.mu.Lock()
	b.strings[contract.LangEN]["only.in.en"] = "English only: %s"
	b.mu.Unlock()
	if err := b.SetLang(contract.LangZH); err != nil {
		t.Fatalf("SetLang(LangZH) error: %v", err)
	}
	if got := b.T("only.in.en", "ok"); got != "English only: ok" {
		t.Errorf("expected English fallback, got %q", got)
	}
}

func TestBundleConcurrency(t *testing.T) {
	b, err := NewBundle(contract.LangZH)
	if err != nil {
		t.Fatalf("NewBundle error: %v", err)
	}

	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				if (id+j)%2 == 0 {
					_ = b.SetLang(contract.LangZH)
				} else {
					_ = b.SetLang(contract.LangEN)
				}
				_ = b.Lang()
				_ = b.T(contract.KeyMOTDProgress, j, 8)
				_ = b.Has(contract.KeyCh1Title)
				_, _ = b.ManPage("tar")
				_ = b.AllManCommands()
			}
		}(i)
	}
	wg.Wait()
}
