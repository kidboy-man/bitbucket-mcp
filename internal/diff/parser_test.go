package diff

import (
	"strings"
	"testing"
)

var sampleDiff = `diff --git a/internal/handler/user.go b/internal/handler/user.go
index abc1234..def5678 100644
--- a/internal/handler/user.go
+++ b/internal/handler/user.go
@@ -10,7 +10,9 @@ import (
 func GetUser(ctx context.Context, id string) (*User, error) {
 	if id == "" {
 		return nil, errors.New("id is required")
+	}
+	if len(id) > 64 {
+		return nil, errors.New("id too long")
 	}
 	return db.Query(ctx, id)
 }
@@ -25,6 +27,5 @@ func DeleteUser(ctx context.Context, id string) error {
 	if err != nil {
 		return err
 	}
-	log.Printf("deleted user %s", id)
 	return nil
 }`

func TestParseFilePath(t *testing.T) {
	pd, err := Parse(sampleDiff)
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}
	if len(pd.Files) != 1 {
		t.Fatalf("expected 1 file, got %d", len(pd.Files))
	}
	if pd.Files[0].Path != "internal/handler/user.go" {
		t.Errorf("unexpected path: %q", pd.Files[0].Path)
	}
}

func TestHunkCount(t *testing.T) {
	pd, err := Parse(sampleDiff)
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}
	if len(pd.Files[0].Hunks) != 2 {
		t.Errorf("expected 2 hunks, got %d", len(pd.Files[0].Hunks))
	}
}

func TestDiffPositionMonotonicallyIncreasing(t *testing.T) {
	pd, err := Parse(sampleDiff)
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}
	last := 0
	for _, f := range pd.Files {
		for _, h := range f.Hunks {
			for _, l := range h.Lines {
				if l.DiffPosition <= last {
					t.Errorf("DiffPosition went backwards: %d after %d", l.DiffPosition, last)
				}
				last = l.DiffPosition
			}
		}
	}
}

func TestAddedLineNewLineNo(t *testing.T) {
	pd, err := Parse(sampleDiff)
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}
	hunk := pd.Files[0].Hunks[0]
	var addedLines []DiffLine
	for _, l := range hunk.Lines {
		if l.Type == LineAdded {
			addedLines = append(addedLines, l)
		}
	}
	if len(addedLines) != 3 {
		t.Fatalf("expected 3 added lines in hunk 0, got %d", len(addedLines))
	}
	expected := []int{13, 14, 15}
	for i, l := range addedLines {
		if l.NewLineNo != expected[i] {
			t.Errorf("added line %d: expected NewLineNo %d, got %d", i, expected[i], l.NewLineNo)
		}
	}
}

func TestRemovedLineOldLineNo(t *testing.T) {
	pd, err := Parse(sampleDiff)
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}
	hunk := pd.Files[0].Hunks[1]
	for _, l := range hunk.Lines {
		if l.Type == LineRemoved {
			if l.OldLineNo == 0 {
				t.Error("removed line has OldLineNo == 0")
			}
			if l.NewLineNo != 0 {
				t.Errorf("removed line should have NewLineNo 0, got %d", l.NewLineNo)
			}
		}
	}
}

func TestFindDiffPosition(t *testing.T) {
	pd, err := Parse(sampleDiff)
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}
	for _, f := range pd.Files {
		for _, h := range f.Hunks {
			for _, l := range h.Lines {
				if l.Type == LineAdded {
					pos, err := pd.FindDiffPosition(f.Path, l.NewLineNo)
					if err != nil {
						t.Errorf("FindDiffPosition failed for new line %d: %v", l.NewLineNo, err)
					}
					if pos != l.DiffPosition {
						t.Errorf("position mismatch: got %d, want %d", pos, l.DiffPosition)
					}
					return
				}
			}
		}
	}
}

func TestFindDiffPositionNotFound(t *testing.T) {
	pd, err := Parse(sampleDiff)
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}
	_, err = pd.FindDiffPosition("internal/handler/user.go", 9999)
	if err == nil {
		t.Error("expected error for out-of-range line, got nil")
	}
}

func TestFindDiffPositionUnknownFile(t *testing.T) {
	pd, err := Parse(sampleDiff)
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}
	_, err = pd.FindDiffPosition("does/not/exist.go", 1)
	if err == nil {
		t.Error("expected error for unknown file, got nil")
	}
}

func TestNewFile(t *testing.T) {
	d := `diff --git a/pkg/cache/redis.go b/pkg/cache/redis.go
new file mode 100644
--- /dev/null
+++ b/pkg/cache/redis.go
@@ -0,0 +1,3 @@
+package cache
+
+// TODO: implement
`
	pd, err := Parse(d)
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}
	if !pd.Files[0].IsNew {
		t.Error("expected IsNew to be true")
	}
}

func TestRenamedFile(t *testing.T) {
	d := `diff --git a/old/path.go b/new/path.go
rename from old/path.go
rename to new/path.go
--- a/old/path.go
+++ b/new/path.go
@@ -1,2 +1,2 @@
 package foo
-// old
+// new
`
	pd, err := Parse(d)
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}
	f := pd.Files[0]
	if !f.IsRenamed {
		t.Error("expected IsRenamed to be true")
	}
	if f.OldPath != "old/path.go" {
		t.Errorf("unexpected OldPath: %q", f.OldPath)
	}
	if f.Path != "new/path.go" {
		t.Errorf("unexpected Path: %q", f.Path)
	}
}

func TestGetContextForLine(t *testing.T) {
	pd, err := Parse(sampleDiff)
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}
	file := "internal/handler/user.go"

	tests := []struct {
		name         string
		filePath     string
		newLineNo    int
		contextLines int
		wantFound    bool
		wantLen      int
	}{
		{
			name:         "middle of hunk returns symmetric window",
			filePath:     file,
			newLineNo:    14,
			contextLines: 2,
			wantFound:    true,
			wantLen:      5,
		},
		{
			name:         "near start of hunk clips to beginning",
			filePath:     file,
			newLineNo:    10,
			contextLines: 5,
			wantFound:    true,
			wantLen:      6,
		},
		{
			name:         "near end of hunk clips to end",
			filePath:     file,
			newLineNo:    18,
			contextLines: 5,
			wantFound:    true,
			wantLen:      6,
		},
		{
			name:         "contextLines=1 returns exactly 3 lines",
			filePath:     file,
			newLineNo:    14,
			contextLines: 1,
			wantFound:    true,
			wantLen:      3,
		},
		{
			name:         "unknown file returns not found",
			filePath:     "does/not/exist.go",
			newLineNo:    14,
			contextLines: 5,
			wantFound:    false,
		},
		{
			name:         "line not in diff returns not found",
			filePath:     file,
			newLineNo:    9999,
			contextLines: 5,
			wantFound:    false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			lines, found := pd.GetContextForLine(tc.filePath, tc.newLineNo, tc.contextLines)
			if found != tc.wantFound {
				t.Fatalf("found=%v, want %v", found, tc.wantFound)
			}
			if !tc.wantFound {
				if lines != nil {
					t.Error("expected nil slice when not found")
				}
				return
			}
			if len(lines) != tc.wantLen {
				t.Errorf("len=%d, want %d", len(lines), tc.wantLen)
			}
			hasAnchor := false
			for _, l := range lines {
				if l.NewLineNo == tc.newLineNo {
					hasAnchor = true
					break
				}
			}
			if !hasAnchor {
				t.Errorf("anchor line %d not found in result window", tc.newLineNo)
			}
			for _, l := range lines {
				if l.DiffPosition <= 0 {
					t.Errorf("line with DiffPosition %d — expected > 0", l.DiffPosition)
				}
			}
		})
	}
}

func TestSummaryContainsDiffPos(t *testing.T) {
	pd, err := Parse(sampleDiff)
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}
	summary := pd.Summary()
	if !strings.Contains(summary, "[diffPos:") {
		t.Error("Summary missing diffPos annotations")
	}
	if !strings.Contains(summary, "internal/handler/user.go") {
		t.Error("Summary missing file path")
	}
}
