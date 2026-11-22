package ui

import (
	"fmt"
	"strings"

	"learn-linux/internal/contract"
)

// Adaptive ANSI 16-color SGR codes that remain crisp on both dark and light terminals.
const (
	ansiReset   = "\x1b[0m"
	ansiBold    = "\x1b[1m"
	ansiDim     = "\x1b[2m"
	ansiReverse = "\x1b[7m"

	fgRed     = "\x1b[31m"
	fgGreen   = "\x1b[32m"
	fgYellow  = "\x1b[33m"
	fgBlue    = "\x1b[34m"
	fgMagenta = "\x1b[35m"
	fgCyan    = "\x1b[36m"

	boldRed     = "\x1b[1;31m"
	boldGreen   = "\x1b[1;32m"
	boldYellow  = "\x1b[1;33m"
	boldBlue    = "\x1b[1;34m"
	boldMagenta = "\x1b[1;35m"
	boldCyan    = "\x1b[1;36m"
	dimGreen    = "\x1b[2;32m"
)

func styled(text string, codes ...string) string {
	if text == "" || len(codes) == 0 {
		return text
	}
	var b strings.Builder
	for _, c := range codes {
		b.WriteString(c)
	}
	b.WriteString(text)
	b.WriteString(ansiReset)
	return b.String()
}

func hairline(width int) string {
	if width <= 0 {
		return ""
	}
	return styled(strings.Repeat("─", width), ansiDim)
}

func progressBar(done, total, barWidth int) string {
	if barWidth <= 0 {
		barWidth = 12
	}
	if total <= 0 {
		return styled(strings.Repeat("░", barWidth), ansiDim)
	}
	if done < 0 {
		done = 0
	}
	if done > total {
		done = total
	}
	filled := (done * barWidth) / total
	if done > 0 && filled == 0 {
		filled = 1
	}
	empty := barWidth - filled
	return styled(strings.Repeat("█", filled), fgGreen) + styled(strings.Repeat("░", empty), ansiDim)
}

// tr resolves key via i18n, passing args only if the localized template expects format verbs.
// Returns the resolved string and true if args were consumed by the template.
func tr(i18n contract.I18N, key string, args ...any) (string, bool) {
	if i18n == nil {
		return key, false
	}
	raw := i18n.T(key)
	verbs := countFormatVerbs(raw)
	if verbs > 0 || strings.Contains(raw, "(MISSING)") {
		if len(args) == 0 {
			return raw, false
		}
		if verbs > 0 && verbs < len(args) {
			return i18n.T(key, args[:verbs]...), true
		}
		return i18n.T(key, args...), true
	}
	return raw, false
}

// trText is a convenience wrapper for plain i18n keys without format arguments.
func trText(i18n contract.I18N, key string) string {
	if i18n == nil {
		return key
	}
	return i18n.T(key)
}

func countFormatVerbs(s string) int {
	count := 0
	for i := 0; i < len(s); i++ {
		if s[i] != '%' {
			continue
		}
		if i+1 < len(s) && s[i+1] == '%' {
			i++
			continue
		}
		// Scan flags/width/precision to the conversion verb.
		j := i + 1
		for j < len(s) {
			c := s[j]
			if (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') {
				count++
				i = j
				break
			}
			if c == ' ' || c == '%' {
				break
			}
			j++
		}
	}
	return count
}

func highlightToken(text, token, colorCode string) string {
	if token == "" || !strings.Contains(text, token) {
		return text
	}
	return strings.ReplaceAll(text, token, styled(token, colorCode))
}

func formatChapterTag(ch contract.ChapterMeta) string {
	if ch.IsSandbox {
		return "[sandbox]"
	}
	return fmt.Sprintf("[ch%d]", ch.ID)
}
