package contract

// TaskGoal describes a single verifiable objective within a chapter.
type TaskGoal struct {
	ID        string
	DescKey   string
	Completed bool
}

// ChapterMeta holds static metadata and i18n keys for a chapter or sandbox.
// All user-facing prose is resolved through I18N using these keys.
type ChapterMeta struct {
	ID          int
	Slug        string
	TitleKey    string
	SubtitleKey string
	IntroKey    string
	GoalKeys    []string
	HintKeys    []string
	Commands    []string
	IsSandbox   bool
}

// ChapterState captures the live evaluation status of a chapter.
type ChapterState struct {
	Meta              ChapterMeta
	Goals             []TaskGoal
	AllPassed         bool
	SoftLocked        bool
	SoftLockReasonKey string
	TipKey            string
}

// CommandRecord logs a single executed command line and its outcome.
type CommandRecord struct {
	Raw      string
	Command  string
	Args     []string
	ExitCode int
	Stdout   string
	Stderr   string
	CWD      string
}

// Progress holds persisted user progress and preferences.
type Progress struct {
	CurrentChapter    int      `json:"current_chapter"`
	CompletedChapters []int    `json:"completed_chapters"`
	Language          Lang     `json:"language"`
	CommandHistory    []string `json:"command_history,omitempty"`
}

// ProgressStore persists Progress to the local user config directory.
type ProgressStore interface {
	Load() (*Progress, error)
	Save(p *Progress) error
	Reset() error
	Path() string
}

// Curriculum manages all learning chapters, sandbox initialization, and goal evaluation.
type Curriculum interface {
	Chapters() []ChapterMeta
	ChapterMeta(chapterID int) (ChapterMeta, bool)
	LoadChapter(chapterID int) (vfs VFS, startCWD string, err error)
	LoadSandbox() (vfs VFS, startCWD string, err error)
	Evaluate(chapterID int, vfs VFS, env map[string]string, history []CommandRecord, lastCmd CommandRecord) ChapterState
	NextHintKey(chapterID int, currentIdx int) (key string, nextIdx int, total int)
}
