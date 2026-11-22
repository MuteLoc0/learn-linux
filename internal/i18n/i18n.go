package i18n

import (
	"embed"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"sync"

	"learn-linux/internal/contract"
)

//go:embed zh.json en.json man_zh.json man_en.json
var embeddedFS embed.FS

// Compile-time interface verification.
var _ contract.I18N = (*Bundle)(nil)

// Bundle implements contract.I18N using embedded bilingual JSON catalogs.
type Bundle struct {
	mu      sync.RWMutex
	lang    contract.Lang
	strings map[contract.Lang]map[string]string
	manDocs map[contract.Lang]map[string]contract.ManDoc
}

// NewBundle loads all embedded language catalogs and initializes the bundle
// with defaultLang. If defaultLang is empty, it defaults to contract.LangZH.
func NewBundle(defaultLang contract.Lang) (*Bundle, error) {
	if defaultLang == "" {
		defaultLang = contract.LangZH
	}
	if !isValidLang(defaultLang) {
		return nil, fmt.Errorf("unsupported language %q: must be %q or %q", defaultLang, contract.LangZH, contract.LangEN)
	}

	zhStrings, err := loadStringCatalog("zh.json")
	if err != nil {
		return nil, err
	}
	enStrings, err := loadStringCatalog("en.json")
	if err != nil {
		return nil, err
	}
	zhMan, err := loadManCatalog("man_zh.json")
	if err != nil {
		return nil, err
	}
	enMan, err := loadManCatalog("man_en.json")
	if err != nil {
		return nil, err
	}

	return &Bundle{
		lang: defaultLang,
		strings: map[contract.Lang]map[string]string{
			contract.LangZH: zhStrings,
			contract.LangEN: enStrings,
		},
		manDocs: map[contract.Lang]map[string]contract.ManDoc{
			contract.LangZH: zhMan,
			contract.LangEN: enMan,
		},
	}, nil
}

// Lang returns the currently active language.
func (b *Bundle) Lang() contract.Lang {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.lang
}

// SetLang switches the active language to contract.LangZH or contract.LangEN.
func (b *Bundle) SetLang(lang contract.Lang) error {
	if !isValidLang(lang) {
		return fmt.Errorf("unsupported language %q: must be %q or %q", lang, contract.LangZH, contract.LangEN)
	}
	b.mu.Lock()
	b.lang = lang
	b.mu.Unlock()
	return nil
}

// T looks up key in the active language, falling back to English and then to
// the key itself. If args are provided, the resolved template is formatted with
// fmt.Sprintf.
func (b *Bundle) T(key string, args ...any) string {
	b.mu.RLock()
	tmpl, ok := b.strings[b.lang][key]
	if !ok {
		tmpl, ok = b.strings[contract.LangEN][key]
	}
	b.mu.RUnlock()

	if !ok {
		tmpl = key
	}
	if len(args) > 0 {
		return fmt.Sprintf(tmpl, args...)
	}
	return tmpl
}

// Has reports whether key exists in the active language or English fallback.
func (b *Bundle) Has(key string) bool {
	b.mu.RLock()
	defer b.mu.RUnlock()
	if _, ok := b.strings[b.lang][key]; ok {
		return true
	}
	_, ok := b.strings[contract.LangEN][key]
	return ok
}

// ManPage looks up a simplified manual entry by command name (case-insensitive).
func (b *Bundle) ManPage(cmd string) (contract.ManDoc, bool) {
	key := strings.ToLower(strings.TrimSpace(cmd))
	b.mu.RLock()
	doc, ok := b.manDocs[b.lang][key]
	if !ok {
		doc, ok = b.manDocs[contract.LangEN][key]
	}
	b.mu.RUnlock()
	if !ok {
		return contract.ManDoc{}, false
	}
	return cloneManDoc(doc), true
}

// AllManCommands returns a sorted list of all documented command names.
func (b *Bundle) AllManCommands() []string {
	b.mu.RLock()
	seen := make(map[string]struct{}, len(b.manDocs[b.lang]))
	for k := range b.manDocs[b.lang] {
		seen[k] = struct{}{}
	}
	for k := range b.manDocs[contract.LangEN] {
		seen[k] = struct{}{}
	}
	b.mu.RUnlock()

	cmds := make([]string, 0, len(seen))
	for k := range seen {
		cmds = append(cmds, k)
	}
	sort.Strings(cmds)
	return cmds
}

func isValidLang(lang contract.Lang) bool {
	return lang == contract.LangZH || lang == contract.LangEN
}

func loadStringCatalog(filename string) (map[string]string, error) {
	data, err := embeddedFS.ReadFile(filename)
	if err != nil {
		return nil, fmt.Errorf("read embedded catalog %s: %w", filename, err)
	}
	var out map[string]string
	if err := json.Unmarshal(data, &out); err != nil {
		return nil, fmt.Errorf("parse embedded catalog %s: %w", filename, err)
	}
	return out, nil
}

func loadManCatalog(filename string) (map[string]contract.ManDoc, error) {
	data, err := embeddedFS.ReadFile(filename)
	if err != nil {
		return nil, fmt.Errorf("read embedded man catalog %s: %w", filename, err)
	}
	var raw map[string]contract.ManDoc
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("parse embedded man catalog %s: %w", filename, err)
	}
	out := make(map[string]contract.ManDoc, len(raw))
	for k, doc := range raw {
		norm := strings.ToLower(strings.TrimSpace(k))
		if doc.Name == "" {
			doc.Name = norm
		}
		out[norm] = doc
	}
	return out, nil
}

func cloneManDoc(doc contract.ManDoc) contract.ManDoc {
	if doc.Options != nil {
		opts := make([]contract.ManOption, len(doc.Options))
		copy(opts, doc.Options)
		doc.Options = opts
	}
	if doc.Examples != nil {
		ex := make([]contract.ManExample, len(doc.Examples))
		copy(ex, doc.Examples)
		doc.Examples = ex
	}
	if doc.SeeAlso != nil {
		see := make([]string, len(doc.SeeAlso))
		copy(see, doc.SeeAlso)
		doc.SeeAlso = see
	}
	return doc
}
