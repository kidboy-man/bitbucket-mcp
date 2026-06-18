package diff

import (
	"fmt"
	"strconv"
	"strings"
)

// LineType represents the type of a diff line.
type LineType int

const (
	LineContext LineType = iota // unchanged context line
	LineAdded                   // line prefixed with '+'
	LineRemoved                 // line prefixed with '-'
)

// DiffLine is a single line in a hunk, with its full position tracking.
type DiffLine struct {
	// DiffPosition is the 1-based position within the file's diff
	// (counting from the first @@ line). Useful metadata for tooling;
	// Bitbucket inline comment anchors use inline.to (new_line_no), not diff_position.
	DiffPosition int

	// OldLineNo is the line number in the base (left) file.
	// 0 means this line does not exist in the old file (added line).
	OldLineNo int

	// NewLineNo is the line number in the head (right) file.
	// 0 means this line does not exist in the new file (removed line).
	NewLineNo int

	Type    LineType
	Content string // raw content without the leading +/-/space
}

// Hunk represents one contiguous changed block in a file.
type Hunk struct {
	// Header is the raw @@ -a,b +c,d @@ ... string.
	Header string

	// OldStart / OldCount / NewStart / NewCount come from the @@ header.
	OldStart int
	OldCount int
	NewStart int
	NewCount int

	Lines []DiffLine
}

// FileDiff holds all hunks for a single file.
type FileDiff struct {
	// Path is the file path relative to the repo root (new path on renames).
	Path string

	// OldPath is set only on renames/copies.
	OldPath string

	IsNew     bool
	IsDeleted bool
	IsRenamed bool

	Hunks []Hunk
}

// ParsedDiff is the top-level parsed result.
type ParsedDiff struct {
	Files []FileDiff
}

// Parse parses a unified diff string into a structured ParsedDiff.
// DiffPosition increments once per @@ line and once per content line, resets per file.
func Parse(raw string) (*ParsedDiff, error) {
	lines := strings.Split(raw, "\n")
	pd := &ParsedDiff{}

	var cur *FileDiff
	var curHunk *Hunk
	diffPos := 0 // resets per file

	for i := 0; i < len(lines); i++ {
		line := lines[i]

		switch {
		case strings.HasPrefix(line, "diff --git "):
			if cur != nil {
				pd.Files = append(pd.Files, *cur)
			}
			cur = &FileDiff{}
			diffPos = 0
			curHunk = nil

			parts := strings.Fields(line)
			if len(parts) >= 4 {
				cur.OldPath = strings.TrimPrefix(parts[2], "a/")
				cur.Path = strings.TrimPrefix(parts[3], "b/")
			}

		case strings.HasPrefix(line, "new file mode"):
			if cur != nil {
				cur.IsNew = true
			}

		case strings.HasPrefix(line, "deleted file mode"):
			if cur != nil {
				cur.IsDeleted = true
			}

		case strings.HasPrefix(line, "rename to "):
			if cur != nil {
				cur.IsRenamed = true
				cur.Path = strings.TrimPrefix(line, "rename to ")
			}

		case strings.HasPrefix(line, "rename from "):
			if cur != nil {
				cur.OldPath = strings.TrimPrefix(line, "rename from ")
			}

		case strings.HasPrefix(line, "+++ "):
			if cur != nil {
				p := strings.TrimPrefix(line, "+++ ")
				p = strings.TrimPrefix(p, "b/")
				if p != "/dev/null" {
					cur.Path = p
				}
			}

		case strings.HasPrefix(line, "@@ "):
			if cur == nil {
				continue
			}
			diffPos++
			hunk, err := parseHunkHeader(line)
			if err != nil {
				return nil, fmt.Errorf("file %q: %w", cur.Path, err)
			}
			hunk.Header = line
			cur.Hunks = append(cur.Hunks, *hunk)
			curHunk = &cur.Hunks[len(cur.Hunks)-1]

		case curHunk != nil && len(line) > 0:
			diffPos++
			dl := DiffLine{DiffPosition: diffPos, Content: line[1:]}

			switch line[0] {
			case '+':
				dl.Type = LineAdded
				curHunk.NewStart++
				dl.NewLineNo = curHunk.NewStart
			case '-':
				dl.Type = LineRemoved
				curHunk.OldStart++
				dl.OldLineNo = curHunk.OldStart
			case ' ':
				dl.Type = LineContext
				curHunk.OldStart++
				curHunk.NewStart++
				dl.OldLineNo = curHunk.OldStart
				dl.NewLineNo = curHunk.NewStart
			default:
				// No-newline marker or unknown — skip position bump.
				diffPos--
				continue
			}

			curHunk.Lines = append(curHunk.Lines, dl)
		}
	}

	if cur != nil {
		pd.Files = append(pd.Files, *cur)
	}

	return pd, nil
}

func parseHunkHeader(header string) (*Hunk, error) {
	inner := header
	inner = strings.TrimPrefix(inner, "@@ ")
	if idx := strings.Index(inner, " @@"); idx >= 0 {
		inner = inner[:idx]
	}

	parts := strings.Fields(inner)
	if len(parts) < 2 {
		return nil, fmt.Errorf("malformed hunk header: %q", header)
	}

	oldStart, oldCount, err := parseRange(parts[0])
	if err != nil {
		return nil, err
	}
	newStart, newCount, err := parseRange(parts[1])
	if err != nil {
		return nil, err
	}

	return &Hunk{
		OldStart: oldStart - 1,
		OldCount: oldCount,
		NewStart: newStart - 1,
		NewCount: newCount,
	}, nil
}

func parseRange(s string) (start, count int, err error) {
	s = strings.TrimLeft(s, "+-")
	parts := strings.SplitN(s, ",", 2)
	start, err = strconv.Atoi(parts[0])
	if err != nil {
		return 0, 0, fmt.Errorf("bad range start %q: %w", s, err)
	}
	if len(parts) == 2 {
		count, err = strconv.Atoi(parts[1])
		if err != nil {
			return 0, 0, fmt.Errorf("bad range count %q: %w", s, err)
		}
	} else {
		count = 1
	}
	return start, count, nil
}

// FindDiffPosition returns the DiffPosition for a given file path and
// new-file line number.
func (pd *ParsedDiff) FindDiffPosition(filePath string, newLineNo int) (int, error) {
	for _, f := range pd.Files {
		if f.Path != filePath {
			continue
		}
		for _, h := range f.Hunks {
			for _, l := range h.Lines {
				if l.NewLineNo == newLineNo {
					return l.DiffPosition, nil
				}
			}
		}
		return 0, fmt.Errorf(
			"line %d in %q is not part of any diff hunk (unchanged line — cannot anchor inline comment)",
			newLineNo, filePath,
		)
	}
	return 0, fmt.Errorf("file %q not found in diff", filePath)
}

// GetContextForLine returns DiffLines surrounding the given new-file line number,
// up to contextLines lines on each side, clamped to hunk boundaries.
// Returns (nil, false) when the file or line is not in the diff.
func (pd *ParsedDiff) GetContextForLine(filePath string, newLineNo int, contextLines int) ([]DiffLine, bool) {
	for _, f := range pd.Files {
		if f.Path != filePath {
			continue
		}
		for _, h := range f.Hunks {
			anchorIdx := -1
			for i, l := range h.Lines {
				if l.NewLineNo == newLineNo && l.Type != LineRemoved {
					anchorIdx = i
					break
				}
			}
			if anchorIdx == -1 {
				continue
			}
			start := anchorIdx - contextLines
			if start < 0 {
				start = 0
			}
			end := anchorIdx + contextLines + 1
			if end > len(h.Lines) {
				end = len(h.Lines)
			}
			result := make([]DiffLine, end-start)
			copy(result, h.Lines[start:end])
			return result, true
		}
		return nil, false
	}
	return nil, false
}

// Summary returns a compact text description of the diff: file paths, hunk ranges,
// and each changed line with position metadata.
func (pd *ParsedDiff) Summary() string {
	var sb strings.Builder
	for _, f := range pd.Files {
		sb.WriteString(fmt.Sprintf("### %s", f.Path))
		if f.IsNew {
			sb.WriteString(" [new file]")
		}
		if f.IsDeleted {
			sb.WriteString(" [deleted]")
		}
		if f.IsRenamed {
			sb.WriteString(fmt.Sprintf(" [renamed from %s]", f.OldPath))
		}
		sb.WriteString("\n")

		for _, h := range f.Hunks {
			sb.WriteString(fmt.Sprintf("  %s\n", h.Header))
			for _, l := range h.Lines {
				prefix := ' '
				switch l.Type {
				case LineAdded:
					prefix = '+'
				case LineRemoved:
					prefix = '-'
				}
				sb.WriteString(fmt.Sprintf(
					"  [diffPos:%d old:%d new:%d] %c %s\n",
					l.DiffPosition, l.OldLineNo, l.NewLineNo, prefix, l.Content,
				))
			}
		}
		sb.WriteString("\n")
	}
	return sb.String()
}
