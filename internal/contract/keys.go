package contract

// Canonical i18n keys shared across curriculum, UI, commands, and i18n bundles.
// All user-facing prose lives strictly in internal/i18n/{zh,en}.json.
const (
	// MOTD & Shell lifecycle
	KeyMOTDOSName    = "motd.os_name"
	KeyMOTDWelcome   = "motd.welcome"
	KeyMOTDProgress  = "motd.progress"
	KeyMOTDQuickCmds = "motd.quick_cmds"
	KeyMOTDTipTab    = "motd.tip_tab"
	KeyMOTDTipLang   = "motd.tip_lang"
	KeyExitGoodbye   = "ui.exit.goodbye"
	KeyTermFallback  = "ui.term.fallback_notice"
	KeyCmdNotFound   = "ui.cmd.not_found_tip"

	// Mission
	KeyMissionHeader      = "ui.mission.header"
	KeyMissionCommands    = "ui.mission.commands"
	KeyMissionGoals       = "ui.mission.goals"
	KeyMissionDone        = "ui.mission.done"
	KeyMissionTodo        = "ui.mission.todo"
	KeyMissionAllPassed   = "ui.mission.all_passed"
	KeyMissionSandboxDesc = "ui.mission.sandbox_desc"

	// Hint
	KeyHintHeader    = "ui.hint.header"
	KeyHintNone      = "ui.hint.none"
	KeyHintExhausted = "ui.hint.exhausted"

	// Check
	KeyCheckPassed     = "ui.check.passed"
	KeyCheckIncomplete = "ui.check.incomplete"

	// Chapter list
	KeyChapterHeader        = "ui.chapter.header"
	KeyChapterColID         = "ui.chapter.col_id"
	KeyChapterColTitle      = "ui.chapter.col_title"
	KeyChapterColStatus     = "ui.chapter.col_status"
	KeyChapterStatusDone    = "ui.chapter.status_done"
	KeyChapterStatusCurrent = "ui.chapter.status_current"
	KeyChapterStatusOpen    = "ui.chapter.status_open"
	KeyChapterUsage         = "ui.chapter.usage"
	KeyChapterNotFound      = "ui.chapter.not_found"

	// Chapter cleared
	KeyClearedTitle    = "ui.cleared.title"
	KeyClearedBody     = "ui.cleared.body"
	KeyClearedNextHint = "ui.cleared.next_hint"
	KeyClearedAllDone  = "ui.cleared.all_done"

	// Soft-lock
	KeySoftLockPrefix = "ui.softlock.prefix"
	KeySoftLockAction = "ui.softlock.action"

	// Man page
	KeyManSectionName     = "ui.man.section_name"
	KeyManSectionSynopsis = "ui.man.section_synopsis"
	KeyManSectionDesc     = "ui.man.section_desc"
	KeyManSectionOptions  = "ui.man.section_options"
	KeyManSectionExamples = "ui.man.section_examples"
	KeyManSectionSeeAlso  = "ui.man.section_seealso"
	KeyManNotFound        = "ui.man.not_found"
	KeyManUsage           = "ui.man.usage"

	// Help
	KeyHelpTitle        = "ui.help.title"
	KeyHelpSubtitle     = "ui.help.subtitle"
	KeyHelpTutorialCmds = "ui.help.tutorial_cmds"
	KeyHelpShortcuts    = "ui.help.shortcuts"
	KeyHelpTip          = "ui.help.tip"

	// Lang / Reset / Sandbox
	KeyLangSwitched = "ui.lang.switched"
	KeyLangUsage    = "ui.lang.usage"
	KeyResetDone    = "ui.reset.done"
	KeySandboxTitle = "ui.sandbox.title"
	KeySandboxDesc  = "ui.sandbox.desc"

	// Destructive panic simulation (rm -rf /)
	KeyPanicHeader      = "ui.panic.header"
	KeyPanicDeleting    = "ui.panic.deleting"
	KeyPanicKernelPanic = "ui.panic.kernel_panic"
	KeyPanicJoke        = "ui.panic.joke"
	KeyPanicCountdown   = "ui.panic.countdown"
	KeyPanicRestored    = "ui.panic.restored"

	// Interactive pager (less) & editor (nano)
	KeyPagerHelp         = "ui.pager.help"
	KeyPagerSearchPrompt = "ui.pager.search_prompt"
	KeyPagerNotFound     = "ui.pager.not_found"
	KeyPagerEOF          = "ui.pager.eof"
	KeyEditorHelp        = "ui.editor.help"
	KeyEditorSaved       = "ui.editor.saved"
	KeyEditorModified    = "ui.editor.modified"
	KeyEditorConfirmExit = "ui.editor.confirm_exit"

	// Chapter 1: Navigation & Listing
	KeyCh1Title    = "ch1.title"
	KeyCh1Subtitle = "ch1.subtitle"
	KeyCh1Intro    = "ch1.intro"
	KeyCh1Goal1    = "ch1.goal.1"
	KeyCh1Goal2    = "ch1.goal.2"
	KeyCh1Goal3    = "ch1.goal.3"
	KeyCh1Goal4    = "ch1.goal.4"
	KeyCh1Hint1    = "ch1.hint.1"
	KeyCh1Hint2    = "ch1.hint.2"
	KeyCh1Hint3    = "ch1.hint.3"
	KeyCh1SoftLock = "ch1.softlock"

	// Chapter 2: File & Directory Management
	KeyCh2Title    = "ch2.title"
	KeyCh2Subtitle = "ch2.subtitle"
	KeyCh2Intro    = "ch2.intro"
	KeyCh2Goal1    = "ch2.goal.1"
	KeyCh2Goal2    = "ch2.goal.2"
	KeyCh2Goal3    = "ch2.goal.3"
	KeyCh2Goal4    = "ch2.goal.4"
	KeyCh2Goal5    = "ch2.goal.5"
	KeyCh2Hint1    = "ch2.hint.1"
	KeyCh2Hint2    = "ch2.hint.2"
	KeyCh2Hint3    = "ch2.hint.3"
	KeyCh2SoftLock = "ch2.softlock"

	// Chapter 3: Viewing, Paging & Editing Text
	KeyCh3Title    = "ch3.title"
	KeyCh3Subtitle = "ch3.subtitle"
	KeyCh3Intro    = "ch3.intro"
	KeyCh3Goal1    = "ch3.goal.1"
	KeyCh3Goal2    = "ch3.goal.2"
	KeyCh3Goal3    = "ch3.goal.3"
	KeyCh3Goal4    = "ch3.goal.4"
	KeyCh3Hint1    = "ch3.hint.1"
	KeyCh3Hint2    = "ch3.hint.2"
	KeyCh3Hint3    = "ch3.hint.3"
	KeyCh3SoftLock = "ch3.softlock"

	// Chapter 4: Search & Text Processing
	KeyCh4Title    = "ch4.title"
	KeyCh4Subtitle = "ch4.subtitle"
	KeyCh4Intro    = "ch4.intro"
	KeyCh4Goal1    = "ch4.goal.1"
	KeyCh4Goal2    = "ch4.goal.2"
	KeyCh4Goal3    = "ch4.goal.3"
	KeyCh4Goal4    = "ch4.goal.4"
	KeyCh4Hint1    = "ch4.hint.1"
	KeyCh4Hint2    = "ch4.hint.2"
	KeyCh4Hint3    = "ch4.hint.3"
	KeyCh4SoftLock = "ch4.softlock"

	// Chapter 5: Pipes, Redirections & Chaining
	KeyCh5Title    = "ch5.title"
	KeyCh5Subtitle = "ch5.subtitle"
	KeyCh5Intro    = "ch5.intro"
	KeyCh5Goal1    = "ch5.goal.1"
	KeyCh5Goal2    = "ch5.goal.2"
	KeyCh5Goal3    = "ch5.goal.3"
	KeyCh5Goal4    = "ch5.goal.4"
	KeyCh5Hint1    = "ch5.hint.1"
	KeyCh5Hint2    = "ch5.hint.2"
	KeyCh5Hint3    = "ch5.hint.3"
	KeyCh5SoftLock = "ch5.softlock"

	// Chapter 6: Permissions, Env & Running Scripts
	KeyCh6Title    = "ch6.title"
	KeyCh6Subtitle = "ch6.subtitle"
	KeyCh6Intro    = "ch6.intro"
	KeyCh6Goal1    = "ch6.goal.1"
	KeyCh6Goal2    = "ch6.goal.2"
	KeyCh6Goal3    = "ch6.goal.3"
	KeyCh6Goal4    = "ch6.goal.4"
	KeyCh6Hint1    = "ch6.hint.1"
	KeyCh6Hint2    = "ch6.hint.2"
	KeyCh6Hint3    = "ch6.hint.3"
	KeyCh6SoftLock = "ch6.softlock"

	// Chapter 7: Archives & Simulated Build Toolchain
	KeyCh7Title    = "ch7.title"
	KeyCh7Subtitle = "ch7.subtitle"
	KeyCh7Intro    = "ch7.intro"
	KeyCh7Goal1    = "ch7.goal.1"
	KeyCh7Goal2    = "ch7.goal.2"
	KeyCh7Goal3    = "ch7.goal.3"
	KeyCh7Goal4    = "ch7.goal.4"
	KeyCh7Hint1    = "ch7.hint.1"
	KeyCh7Hint2    = "ch7.hint.2"
	KeyCh7Hint3    = "ch7.hint.3"
	KeyCh7SoftLock = "ch7.softlock"

	// Chapter 8: Capstone Integration Challenge
	KeyCh8Title    = "ch8.title"
	KeyCh8Subtitle = "ch8.subtitle"
	KeyCh8Intro    = "ch8.intro"
	KeyCh8Goal1    = "ch8.goal.1"
	KeyCh8Goal2    = "ch8.goal.2"
	KeyCh8Goal3    = "ch8.goal.3"
	KeyCh8Goal4    = "ch8.goal.4"
	KeyCh8Goal5    = "ch8.goal.5"
	KeyCh8Hint1    = "ch8.hint.1"
	KeyCh8Hint2    = "ch8.hint.2"
	KeyCh8Hint3    = "ch8.hint.3"
	KeyCh8Hint4    = "ch8.hint.4"
	KeyCh8SoftLock = "ch8.softlock"

	// Sandbox
	KeySandboxMetaTitle    = "sandbox.title"
	KeySandboxMetaSubtitle = "sandbox.subtitle"
	KeySandboxMetaIntro    = "sandbox.intro"
)
