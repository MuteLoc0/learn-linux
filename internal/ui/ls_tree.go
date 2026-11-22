package ui

import (
	"fmt"
	"io/fs"
	"sort"
	"strconv"
	"strings"
	"time"

	"learn-linux/internal/contract"
)

type gridEntry struct {
	plainWidth int
	rendered   string
}

// formatColumnGrid lays out entries in column-major order matching GNU ls / bash Tab completion.
func formatColumnGrid(entries []gridEntry, termWidth int) string {
	n := len(entries)
	if n == 0 {
		return ""
	}
	maxW := 1
	for _, e := range entries {
		if e.plainWidth > maxW {
			maxW = e.plainWidth
		}
	}
	colW := maxW + 2
	cols := termWidth / colW
	if cols < 1 {
		cols = 1
	}
	if cols > n {
		cols = n
	}
	rows := (n + cols - 1) / cols
	// Recalculate cols so we don't allocate empty trailing columns.
	cols = (n + rows - 1) / rows

	// Compute tight per-column widths like GNU ls.
	colWidths := make([]int, cols)
	for c := 0; c < cols; c++ {
		w := 0
		for r := 0; r < rows; r++ {
			idx := c*rows + r
			if idx >= n {
				break
			}
			if entries[idx].plainWidth > w {
				w = entries[idx].plainWidth
			}
		}
		colWidths[c] = w + 2
	}

	var b strings.Builder
	for r := 0; r < rows; r++ {
		for c := 0; c < cols; c++ {
			idx := c*rows + r
			if idx >= n {
				continue
			}
			e := entries[idx]
			hasNextInRow := (c+1)*rows+r < n
			b.WriteString(e.rendered)
			if hasNextInRow {
				pad := colWidths[c] - e.plainWidth
				if pad < 2 {
					pad = 2
				}
				b.WriteString(strings.Repeat(" ", pad))
			}
		}
		if r < rows-1 {
			b.WriteByte('\n')
		}
	}
	return b.String()
}

func isArchiveName(name string) bool {
	lower := strings.ToLower(name)
	for _, ext := range []string{".tar.gz", ".tgz", ".tar.bz2", ".tar.xz", ".tar", ".gz", ".zip", ".bz2", ".xz"} {
		if strings.HasSuffix(lower, ext) {
			return true
		}
	}
	return false
}

func colorizeNodeName(name string, info *contract.NodeInfo, isExec bool, color bool) string {
	if !color || info == nil {
		return name
	}
	switch {
	case info.Type == contract.TypeDir:
		return styled(name, boldBlue)
	case info.Type == contract.TypeSymlink:
		return styled(name, boldCyan)
	case isExec || info.IsExecutable():
		return styled(name, boldGreen)
	case isArchiveName(info.Name) || isArchiveName(name):
		return styled(name, boldRed)
	default:
		return name
	}
}

// RenderLSGrid renders a GNU ls multi-column grid respecting CJK display widths and ANSI colors.
func (r *Renderer) RenderLSGrid(items []contract.LSItem, termWidth int, color bool) string {
	if len(items) == 0 {
		return ""
	}
	w := clampGridWidth(termWidth)
	entries := make([]gridEntry, len(items))
	for i, it := range items {
		name := it.DisplayName
		if name == "" && it.Info != nil {
			name = it.Info.Name
		}
		entries[i] = gridEntry{
			plainWidth: displayWidth(name),
			rendered:   colorizeNodeName(name, it.Info, it.IsExecutable, color),
		}
	}
	return formatColumnGrid(entries, w)
}

// RenderLSLong renders authentic `ls -l` output with optional human-readable sizes and colors.
func (r *Renderer) RenderLSLong(items []contract.LSItem, humanReadable bool, color bool) string {
	if len(items) == 0 {
		return "total 0"
	}

	type row struct {
		mode  string
		links string
		owner string
		group string
		size  string
		mtime string
		name  string
	}

	var totalBlocks int64
	rows := make([]row, 0, len(items))
	maxOwner, maxGroup, maxSize := 1, 1, 1

	for _, it := range items {
		info := it.Info
		if info == nil {
			info = &contract.NodeInfo{Name: it.DisplayName, Mode: 0o644}
		}
		totalBlocks += (info.Size + 1023) / 1024

		name := it.DisplayName
		if name == "" {
			name = info.Name
		}
		coloredName := colorizeNodeName(name, info, it.IsExecutable, color)
		if info.Type == contract.TypeSymlink && info.LinkTarget != "" {
			coloredName += " -> " + info.LinkTarget
		}

		owner := info.Owner
		if owner == "" {
			owner = "learner"
		}
		group := info.Group
		if group == "" {
			group = "learner"
		}

		var sizeStr string
		if humanReadable {
			sizeStr = formatHumanSize(info.Size)
		} else {
			sizeStr = strconv.FormatInt(info.Size, 10)
		}

		mt := info.ModTime
		if mt.IsZero() {
			mt = time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
		}

		maxOwner = maxInt(maxOwner, len(owner))
		maxGroup = maxInt(maxGroup, len(group))
		maxSize = maxInt(maxSize, len(sizeStr))

		rows = append(rows, row{
			mode:  formatMode(info.Type, info.Mode),
			links: "1",
			owner: owner,
			group: group,
			size:  sizeStr,
			mtime: mt.Format("Jan _2 15:04"),
			name:  coloredName,
		})
	}

	var b strings.Builder
	b.WriteString(fmt.Sprintf("total %d\n", totalBlocks))
	for i, rw := range rows {
		b.WriteString(fmt.Sprintf("%s %s %s %s %s %s %s",
			rw.mode,
			rw.links,
			padRight(rw.owner, maxOwner),
			padRight(rw.group, maxGroup),
			padLeft(rw.size, maxSize),
			rw.mtime,
			rw.name,
		))
		if i < len(rows)-1 {
			b.WriteByte('\n')
		}
	}
	return b.String()
}

func formatMode(ft contract.FileType, mode fs.FileMode) string {
	var buf [10]byte
	switch ft {
	case contract.TypeDir:
		buf[0] = 'd'
	case contract.TypeSymlink:
		buf[0] = 'l'
	default:
		buf[0] = '-'
	}

	const rwx = "rwxrwxrwx"
	perm := mode.Perm()
	for i := 0; i < 9; i++ {
		if perm&(1<<uint(8-i)) != 0 {
			buf[1+i] = rwx[i]
		} else {
			buf[1+i] = '-'
		}
	}
	return string(buf[:])
}

func formatHumanSize(bytes int64) string {
	if bytes < 0 {
		bytes = 0
	}
	if bytes < 1024 {
		return strconv.FormatInt(bytes, 10)
	}
	units := []string{"K", "M", "G", "T"}
	val := float64(bytes)
	for _, u := range units {
		val /= 1024.0
		if val < 1024.0 || u == "T" {
			if val >= 10.0 {
				return fmt.Sprintf("%.0f%s", val, u)
			}
			return fmt.Sprintf("%.1f%s", val, u)
		}
	}
	return strconv.FormatInt(bytes, 10)
}

// RenderTree renders classic `tree` output with box-drawing connectors and summary line.
func (r *Renderer) RenderTree(root *contract.TreeNode, dirCount, fileCount int, color bool) string {
	var b strings.Builder
	if root == nil {
		b.WriteString(formatTreeSummary(dirCount, fileCount))
		return b.String()
	}

	rootName := "."
	if root.Info != nil && root.Info.Name != "" {
		rootName = colorizeNodeName(root.Info.Name, root.Info, false, color)
	} else if color {
		rootName = styled(".", boldBlue)
	}
	b.WriteString(rootName)
	b.WriteByte('\n')

	renderTreeChildren(&b, root.Children, "", color)
	b.WriteByte('\n')
	b.WriteString(formatTreeSummary(dirCount, fileCount))
	return b.String()
}

func renderTreeChildren(b *strings.Builder, children []*contract.TreeNode, prefix string, color bool) {
	for i, child := range children {
		if child == nil {
			continue
		}
		isLast := i == len(children)-1
		connector := "├── "
		nextPrefix := prefix + "│   "
		if isLast {
			connector = "└── "
			nextPrefix = prefix + "    "
		}

		name := ""
		if child.Info != nil {
			name = colorizeNodeName(child.Info.Name, child.Info, false, color)
			if child.Info.Type == contract.TypeSymlink && child.Info.LinkTarget != "" {
				name += " -> " + child.Info.LinkTarget
			}
		}

		b.WriteString(prefix)
		b.WriteString(connector)
		b.WriteString(name)
		b.WriteByte('\n')

		if len(child.Children) > 0 {
			renderTreeChildren(b, child.Children, nextPrefix, color)
		}
	}
}

func formatTreeSummary(dirCount, fileCount int) string {
	dirWord := "directories"
	if dirCount == 1 {
		dirWord = "directory"
	}
	fileWord := "files"
	if fileCount == 1 {
		fileWord = "file"
	}
	return fmt.Sprintf("%d %s, %d %s", dirCount, dirWord, fileCount, fileWord)
}

// RenderGrepMatch formats a matched line with GNU grep --color semantics, safely handling edge ranges.
func (r *Renderer) RenderGrepMatch(m contract.GrepMatch, color bool) string {
	var b strings.Builder
	colon := ":"
	if color {
		colon = styled(":", fgCyan)
	}

	if m.ShowFile {
		if color {
			b.WriteString(styled(m.FilePath, fgMagenta))
		} else {
			b.WriteString(m.FilePath)
		}
		b.WriteString(colon)
	}
	if m.ShowLineNum {
		numStr := strconv.Itoa(m.LineNum)
		if color {
			b.WriteString(styled(numStr, fgGreen))
		} else {
			b.WriteString(numStr)
		}
		b.WriteString(colon)
	}

	if !color || len(m.MatchRanges) == 0 {
		b.WriteString(m.Line)
		return b.String()
	}

	merged := normalizeRanges(m.MatchRanges, len(m.Line))
	if len(merged) == 0 {
		b.WriteString(m.Line)
		return b.String()
	}

	pos := 0
	for _, rg := range merged {
		if rg[0] > pos {
			b.WriteString(m.Line[pos:rg[0]])
		}
		b.WriteString(styled(m.Line[rg[0]:rg[1]], boldRed))
		pos = rg[1]
	}
	if pos < len(m.Line) {
		b.WriteString(m.Line[pos:])
	}
	return b.String()
}

func normalizeRanges(ranges [][2]int, maxLen int) [][2]int {
	if maxLen <= 0 || len(ranges) == 0 {
		return nil
	}
	clean := make([][2]int, 0, len(ranges))
	for _, rg := range ranges {
		start, end := rg[0], rg[1]
		if start < 0 {
			start = 0
		}
		if end > maxLen {
			end = maxLen
		}
		if start >= end {
			continue
		}
		clean = append(clean, [2]int{start, end})
	}
	if len(clean) == 0 {
		return nil
	}
	sort.Slice(clean, func(i, j int) bool {
		if clean[i][0] == clean[j][0] {
			return clean[i][1] < clean[j][1]
		}
		return clean[i][0] < clean[j][0]
	})
	merged := make([][2]int, 0, len(clean))
	cur := clean[0]
	for _, rg := range clean[1:] {
		if rg[0] <= cur[1] {
			if rg[1] > cur[1] {
				cur[1] = rg[1]
			}
		} else {
			merged = append(merged, cur)
			cur = rg
		}
	}
	merged = append(merged, cur)
	return merged
}
