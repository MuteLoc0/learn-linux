package contract

// Lang represents a supported UI and curriculum language.
type Lang string

const (
	LangZH Lang = "zh"
	LangEN Lang = "en"
)

// ManOption describes a single command-line option in a simplified man page.
type ManOption struct {
	Flag string `json:"flag"`
	Desc string `json:"desc"`
}

// ManExample describes a usage example in a simplified man page.
type ManExample struct {
	Cmd  string `json:"cmd"`
	Desc string `json:"desc"`
}

// ManDoc holds a bilingual simplified manual entry for a built-in command.
type ManDoc struct {
	Name        string       `json:"name"`
	Summary     string       `json:"summary"`
	Synopsis    string       `json:"synopsis"`
	Description string       `json:"description"`
	Options     []ManOption  `json:"options"`
	Examples    []ManExample `json:"examples"`
	SeeAlso     []string     `json:"see_also"`
}

// I18N provides localized strings and manual entries.
type I18N interface {
	Lang() Lang
	SetLang(lang Lang) error
	T(key string, args ...any) string
	Has(key string) bool
	ManPage(cmd string) (ManDoc, bool)
	AllManCommands() []string
}
