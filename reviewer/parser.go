package reviewer

import (
	"fmt"
	"strconv"
	"strings"
)

// LineType represents the type of a diff line.
type LineType int

const (
	LineContext  LineType = iota // unchanged context line
	LineAdded                   // line prefixed with '+'
	LineRemoved                 // line prefixed with '-'
)

// DiffLine is a single line in a hunk, with its full position tracking.
type DiffLine struct {
	// DiffPosition is the 1-based position within the file's diff
	// (counting from the first '@@' line). This is what the Bitbucket
	// inline comment API expects for the "to" field.
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

// ParsedDiff is the top-level result returned to Claude.
type ParsedDiff struct {
	Files []FileDiff
}

// Parse parses a unified diff string into a structured ParsedDiff.
// It correctly tracks DiffPosition (the Bitbucket API's notion of line position)
// independently of OldLineNo / NewLineNo.
func Parse(raw string) (*ParsedDiff, error) {
	lines := strings.Split(raw, "\n")
	pd := &ParsedDiff{}

	var cur *FileDiff
	var curHunk *Hunk
	diffPos := 0 // resets per file, counts from first @@ line

	for i := 0; i < len(lines); i++ {
		line := lines[i]

		switch {
		// ----------------------------------------------------------------
		// File header: "diff --git a/foo b/foo"
		// ----------------------------------------------------------------
		case strings.HasPrefix(line, "diff --git "):
			if cur != nil {
				pd.Files = append(pd.Files, *cur)
			}
			cur = &FileDiff{}
			diffPos = 0
			curHunk = nil

			// Extract paths from "diff --git a/path b/path"
			parts := strings.Fields(line)
			if len(parts) >= 4 {
				cur.OldPath = strings.TrimPrefix(parts[2], "a/")
				cur.Path = strings.TrimPrefix(parts[3], "b/")
			}

		// ----------------------------------------------------------------
		// Extended headers
		// ----------------------------------------------------------------
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

		// Explicit new path from +++ line (handles /dev/null for new files)
		case strings.HasPrefix(line, "+++ "):
			if cur != nil {
				p := strings.TrimPrefix(line, "+++ ")
				p = strings.TrimPrefix(p, "b/")
				if p != "/dev/null" {
					cur.Path = p
				}
			}

		// ----------------------------------------------------------------
		// Hunk header: "@@ -old,count +new,count @@ optional context"
		// ----------------------------------------------------------------
		case strings.HasPrefix(line, "@@ "):
			if cur == nil {
				continue
			}
			diffPos++ // the @@ line itself counts as position 1 for this hunk
			hunk, err := parseHunkHeader(line)
			if err != nil {
				return nil, fmt.Errorf("file %q: %w", cur.Path, err)
			}
			hunk.Header = line
			cur.Hunks = append(cur.Hunks, *hunk)
			curHunk = &cur.Hunks[len(cur.Hunks)-1]

		// ----------------------------------------------------------------
		// Diff content lines
		// ----------------------------------------------------------------
		case curHunk != nil && len(line) > 0:
			diffPos++
			dl := DiffLine{DiffPosition: diffPos, Content: line[1:]}

			switch line[0] {
			case '+':
				dl.Type = LineAdded
				curHunk.NewStart++ // track running new line counter
				dl.NewLineNo = curHunk.NewStart - 1
			case '-':
				dl.Type = LineRemoved
				curHunk.OldStart++
				dl.OldLineNo = curHunk.OldStart - 1
			case ' ':
				dl.Type = LineContext
				curHunk.OldStart++
				curHunk.NewStart++
				dl.OldLineNo = curHunk.OldStart - 1
				dl.NewLineNo = curHunk.NewStart - 1
			default:
				// No-newline marker or unknown — skip position bump
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

// parseHunkHeader parses "@@ -oldStart,oldCount +newStart,newCount @@"
// into a Hunk with the numeric fields populated.
// The Start fields are set to (headerValue - 1) so the content loop can
// use pre-increment to produce 1-based line numbers.
func parseHunkHeader(header string) (*Hunk, error) {
	// Extract the part between the @@ markers: "-3,7 +3,6"
	inner := header
	inner = strings.TrimPrefix(inner, "@@ ")
	if idx := strings.Index(inner, " @@"); idx >= 0 {
		inner = inner[:idx]
	}

	parts := strings.Fields(inner) // ["-3,7", "+3,6"]
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
		// Subtract 1 so the loop's pre-increment gives the correct 1-based value.
		OldStart: oldStart - 1,
		OldCount: oldCount,
		NewStart: newStart - 1,
		NewCount: newCount,
	}, nil
}

// parseRange parses "-3,7" or "+10" into (start, count).
// A missing count defaults to 1 (per the unified diff spec).
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
// new-file line number. This is the value to pass to the Bitbucket
// inline comment API as "to".
//
// Returns (0, error) if the line is not found in the diff (e.g. it is
// an unchanged line outside any hunk — Bitbucket cannot anchor a comment
// there).
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

// Summary returns a compact, Claude-readable description of the diff:
// file paths, hunk ranges, and each changed line with its position metadata.
// This is what gets sent to Claude for review.
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
