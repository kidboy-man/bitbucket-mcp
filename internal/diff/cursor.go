package diff

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
)

// Cursor is the internal (decoded) representation of a pagination cursor.
// Callers treat the serialized form as opaque.
type Cursor struct {
	FileIndex  int    `json:"file_index"`
	HunkIndex  int    `json:"hunk_index"`
	LineIndex  int    `json:"line_index"`
	FilterHash string `json:"filter_hash"`
}

// DiffPage is a page of structured diff output.
type DiffPage struct {
	Files            []FileDiffPage `json:"files"`
	NextCursor       string         `json:"next_cursor,omitempty"`
	Truncated        bool           `json:"truncated"`
	TruncationReason string         `json:"truncation_reason,omitempty"`
}

// FileDiffPage mirrors FileDiff but only carries the hunks/lines in this page.
type FileDiffPage struct {
	Path      string     `json:"path"`
	OldPath   string     `json:"old_path,omitempty"`
	IsNew     bool       `json:"is_new,omitempty"`
	IsDeleted bool       `json:"is_deleted,omitempty"`
	IsRenamed bool       `json:"is_renamed,omitempty"`
	Hunks     []HunkPage `json:"hunks"`
}

// HunkPage mirrors Hunk but carries only the lines in this page.
type HunkPage struct {
	Header string     `json:"header"`
	Lines  []LinePage `json:"lines"`
}

// LinePage is the JSON-serialisable form of DiffLine.
type LinePage struct {
	DiffPosition int    `json:"diff_position"`
	OldLineNo    int    `json:"old_line_no,omitempty"`
	NewLineNo    int    `json:"new_line_no,omitempty"`
	Type         string `json:"type"` // "added" | "removed" | "context"
	Content      string `json:"content"`
}

// FilterHash computes a stable hash for parameters that lock a cursor to a request context.
func FilterHash(prURL, fileFilter string, maxLines int) string {
	h := sha256.Sum256([]byte(fmt.Sprintf("%s\x00%s\x00%d", prURL, fileFilter, maxLines)))
	return fmt.Sprintf("%x", h[:8])
}

// EncodeCursor serialises a Cursor to an opaque base64 string.
func EncodeCursor(c Cursor) (string, error) {
	b, err := json.Marshal(c)
	if err != nil {
		return "", fmt.Errorf("encoding cursor: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

// DecodeCursor parses an opaque cursor string back to a Cursor.
func DecodeCursor(s string) (Cursor, error) {
	b, err := base64.RawURLEncoding.DecodeString(s)
	if err != nil {
		return Cursor{}, fmt.Errorf("decoding cursor: %w", err)
	}
	var c Cursor
	if err := json.Unmarshal(b, &c); err != nil {
		return Cursor{}, fmt.Errorf("parsing cursor: %w", err)
	}
	return c, nil
}

// PageInput controls a single GetPage call.
type PageInput struct {
	// PRURL and FileFilter are used to validate cursor hash.
	PRURL      string
	FileFilter string

	// Cursor is empty for the first page; subsequent pages pass the value
	// returned in DiffPage.NextCursor.
	Cursor string

	// MaxLines is the max number of content lines to return per page.
	// 0 means no limit.
	MaxLines int
}

// GetPage returns a DiffPage from pd starting at the position encoded in input.Cursor.
// When Cursor is empty, paging starts from the beginning.
// File filter (input.FileFilter) restricts output to a single file when non-empty.
func (pd *ParsedDiff) GetPage(input PageInput) (*DiffPage, error) {
	fh := FilterHash(input.PRURL, input.FileFilter, input.MaxLines)

	// Decode or build start cursor.
	var start Cursor
	if input.Cursor != "" {
		var err error
		start, err = DecodeCursor(input.Cursor)
		if err != nil {
			return nil, fmt.Errorf("invalid cursor: %w", err)
		}
		if start.FilterHash != fh {
			return nil, fmt.Errorf("cursor filter mismatch: cursor was generated for different request parameters")
		}
	} else {
		start = Cursor{FilterHash: fh}
	}

	page := &DiffPage{}
	lineCount := 0
	done := false

	for fi, f := range pd.Files {
		if fi < start.FileIndex {
			continue
		}
		if input.FileFilter != "" && f.Path != input.FileFilter {
			continue
		}

		var fp *FileDiffPage

		for hi, h := range f.Hunks {
			if fi == start.FileIndex && hi < start.HunkIndex {
				continue
			}

			var hp *HunkPage

			for li, l := range h.Lines {
				if fi == start.FileIndex && hi == start.HunkIndex && li < start.LineIndex {
					continue
				}

				if input.MaxLines > 0 && lineCount >= input.MaxLines {
					// Encode next cursor at this position.
					next, err := EncodeCursor(Cursor{
						FileIndex:  fi,
						HunkIndex:  hi,
						LineIndex:  li,
						FilterHash: fh,
					})
					if err != nil {
						return nil, err
					}
					page.NextCursor = next
					done = true
					break
				}

				// Lazy-init file page.
				if fp == nil {
					fp = &FileDiffPage{
						Path:      f.Path,
						OldPath:   f.OldPath,
						IsNew:     f.IsNew,
						IsDeleted: f.IsDeleted,
						IsRenamed: f.IsRenamed,
					}
				}
				// Lazy-init hunk page.
				if hp == nil {
					hp = &HunkPage{Header: h.Header}
				}

				hp.Lines = append(hp.Lines, toLinePage(l))
				lineCount++
			}

			if hp != nil {
				fp.Hunks = append(fp.Hunks, *hp)
			}
			if done {
				break
			}
		}

		if fp != nil {
			page.Files = append(page.Files, *fp)
		}
		if done {
			break
		}
	}

	return page, nil
}

func toLinePage(l DiffLine) LinePage {
	t := "context"
	switch l.Type {
	case LineAdded:
		t = "added"
	case LineRemoved:
		t = "removed"
	}
	return LinePage{
		DiffPosition: l.DiffPosition,
		OldLineNo:    l.OldLineNo,
		NewLineNo:    l.NewLineNo,
		Type:         t,
		Content:      l.Content,
	}
}
