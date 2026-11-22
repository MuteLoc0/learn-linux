package ui

import (
	"strings"
	"unicode"
)

// stripANSI removes ANSI CSI and two-byte escape sequences from s.
func stripANSI(s string) string {
	if !strings.ContainsRune(s, '\x1b') {
		return s
	}
	var b strings.Builder
	b.Grow(len(s))
	i := 0
	for i < len(s) {
		if s[i] == '\x1b' {
			if i+1 < len(s) && s[i+1] == '[' {
				j := i + 2
				for j < len(s) && (s[j] < 0x40 || s[j] > 0x7e) {
					j++
				}
				if j < len(s) {
					j++
				}
				i = j
				continue
			}
			if i+1 < len(s) {
				i += 2
				continue
			}
			i++
			continue
		}
		b.WriteByte(s[i])
		i++
	}
	return b.String()
}

// runeWidth returns the terminal cell width (0, 1, or 2) of a Unicode rune.
func runeWidth(r rune) int {
	if r == 0 || r < 0x20 || (r >= 0x7f && r < 0xa0) {
		return 0
	}
	if unicode.Is(unicode.Mn, r) || unicode.Is(unicode.Me, r) || unicode.Is(unicode.Cf, r) {
		return 0
	}
	if isWideRune(r) {
		return 2
	}
	return 1
}

func isWideRune(r rune) bool {
	switch {
	case r >= 0x1100 && r <= 0x115f: // Hangul Jamo init. consonants
		return true
	case r == 0x2329 || r == 0x232a:
		return true
	case r >= 0x2e80 && r <= 0x303e: // CJK Radicals, Kangxi, Symbols & Punctuation
		return true
	case r >= 0x3040 && r <= 0x33ff: // Hiragana, Katakana, Bopomofo, CJK Compat
		return true
	case r >= 0x3400 && r <= 0x4dbf: // CJK Unified Ideographs Extension A
		return true
	case r >= 0x4e00 && r <= 0xa4cf: // CJK Unified Ideographs, Yi
		return true
	case r >= 0xa960 && r <= 0xa97f: // Hangul Jamo Extended-A
		return true
	case r >= 0xac00 && r <= 0xd7a3: // Hangul Syllables
		return true
	case r >= 0xf900 && r <= 0xfaff: // CJK Compatibility Ideographs
		return true
	case r >= 0xfe10 && r <= 0xfe19: // Vertical forms
		return true
	case r >= 0xfe30 && r <= 0xfe6f: // CJK Compatibility Forms & Small Form Variants
		return true
	case r >= 0xff00 && r <= 0xff60: // Fullwidth ASCII & Punctuation
		return true
	case r >= 0xffe0 && r <= 0xffe6: // Fullwidth Symbol Variants
		return true
	case r >= 0x1f300 && r <= 0x1faff: // Emoji & Pictographs
		return true
	case r >= 0x20000 && r <= 0x3ffff: // CJK Extensions B..H
		return true
	}
	return false
}

// displayWidth returns the visible terminal column width of s, ignoring ANSI escapes.
func displayWidth(s string) int {
	clean := stripANSI(s)
	w := 0
	for _, r := range clean {
		w += runeWidth(r)
	}
	return w
}

// padRight pads s with trailing spaces so its visible width is at least targetWidth.
func padRight(s string, targetWidth int) string {
	w := displayWidth(s)
	if w >= targetWidth {
		return s
	}
	return s + strings.Repeat(" ", targetWidth-w)
}

// padLeft pads s with leading spaces so its visible width is at least targetWidth.
func padLeft(s string, targetWidth int) string {
	w := displayWidth(s)
	if w >= targetWidth {
		return s
	}
	return strings.Repeat(" ", targetWidth-w) + s
}

// truncatePlain truncates plain text (without ANSI escapes) to fit within maxWidth cells.
func truncatePlain(s string, maxWidth int, ellipsis string) string {
	if maxWidth <= 0 {
		return ""
	}
	if displayWidth(s) <= maxWidth {
		return s
	}
	ellW := displayWidth(ellipsis)
	if ellW >= maxWidth {
		ellipsis = ""
		ellW = 0
	}
	limit := maxWidth - ellW
	var b strings.Builder
	cur := 0
	for _, r := range s {
		rw := runeWidth(r)
		if cur+rw > limit {
			break
		}
		b.WriteRune(r)
		cur += rw
	}
	b.WriteString(ellipsis)
	return b.String()
}

// slicePlainByCols returns the substring of plain text s starting at startCol
// up to maxCols visible terminal cells, plus the actual starting column of the slice.
func slicePlainByCols(s string, startCol, maxCols int) string {
	if maxCols <= 0 {
		return ""
	}
	var b strings.Builder
	col := 0
	taken := 0
	for _, r := range s {
		rw := runeWidth(r)
		if col < startCol {
			col += rw
			continue
		}
		if taken+rw > maxCols {
			break
		}
		b.WriteRune(r)
		taken += rw
		col += rw
	}
	return b.String()
}

// wrapText wraps plain text to fit within width terminal columns, handling CJK runes cleanly.
func wrapText(s string, width int) []string {
	if width <= 0 {
		width = 80
	}
	paragraphs := strings.Split(s, "\n")
	var out []string
	for _, para := range paragraphs {
		if strings.TrimSpace(para) == "" {
			out = append(out, "")
			continue
		}
		out = append(out, wrapParagraph(para, width)...)
	}
	return out
}

func wrapParagraph(para string, width int) []string {
	var lines []string
	var cur strings.Builder
	curW := 0

	flush := func() {
		line := strings.TrimRight(cur.String(), " \t")
		lines = append(lines, line)
		cur.Reset()
		curW = 0
	}

	runes := []rune(para)
	i := 0
	for i < len(runes) {
		r := runes[i]
		if r == ' ' || r == '\t' {
			if curW == 0 {
				i++
				continue
			}
			if curW+1 > width {
				flush()
				i++
				continue
			}
			cur.WriteRune(' ')
			curW++
			i++
			continue
		}

		rw := runeWidth(r)
		// Wide characters (CJK) and punctuation can break directly without waiting for spaces.
		if rw == 2 {
			if curW+rw > width && curW > 0 {
				flush()
			}
			cur.WriteRune(r)
			curW += rw
			i++
			continue
		}

		// Collect an ASCII/narrow non-space token so words stay intact when possible.
		j := i
		tokenW := 0
		for j < len(runes) {
			rj := runes[j]
			if rj == ' ' || rj == '\t' || runeWidth(rj) == 2 {
				break
			}
			tokenW += runeWidth(rj)
			j++
		}

		if curW > 0 && curW+tokenW > width {
			flush()
		}

		// If a single token exceeds width, split it across lines cleanly.
		for k := i; k < j; k++ {
			rk := runes[k]
			rkw := runeWidth(rk)
			if curW > 0 && curW+rkw > width {
				flush()
			}
			cur.WriteRune(rk)
			curW += rkw
		}
		i = j
	}

	if curW > 0 || len(lines) == 0 {
		lines = append(lines, strings.TrimRight(cur.String(), " \t"))
	}
	return lines
}

// clampProseWidth returns a comfortable prose/layout width given termWidth.
func clampProseWidth(termWidth int) int {
	if termWidth <= 0 {
		return 80
	}
	if termWidth < 60 {
		return 60
	}
	if termWidth > 84 {
		return 84
	}
	return termWidth
}

// clampGridWidth returns an effective terminal width for multi-column grids.
func clampGridWidth(termWidth int) int {
	if termWidth <= 0 {
		return 80
	}
	if termWidth < 20 {
		return 20
	}
	return termWidth
}
