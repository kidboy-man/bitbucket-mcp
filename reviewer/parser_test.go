package reviewer

import (
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
	// First hunk: @@ -10,7 +10,9 @@
	// context(10), context(11), context(12), added(13), added(14), added(15), context(16), context(17)
	hunk := pd.Files[0].Hunks[0]
	addedLines := make([]DiffLine, 0)
	for _, l := range hunk.Lines {
		if l.Type == LineAdded {
			addedLines = append(addedLines, l)
		}
	}
	if len(addedLines) != 3 {
		t.Fatalf("expected 3 added lines in hunk 0, got %d", len(addedLines))
	}
	// Verify exact new file line numbers
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

	// Find an added line — should succeed.
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
					return // one check is enough
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
	diff := `diff --git a/pkg/cache/redis.go b/pkg/cache/redis.go
new file mode 100644
--- /dev/null
+++ b/pkg/cache/redis.go
@@ -0,0 +1,3 @@
+package cache
+
+// TODO: implement
`
	pd, err := Parse(diff)
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}
	if !pd.Files[0].IsNew {
		t.Error("expected IsNew to be true")
	}
}

func TestRenamedFile(t *testing.T) {
	diff := `diff --git a/old/path.go b/new/path.go
rename from old/path.go
rename to new/path.go
--- a/old/path.go
+++ b/new/path.go
@@ -1,2 +1,2 @@
 package foo
-// old
+// new
`
	pd, err := Parse(diff)
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

func TestSummaryContainsDiffPos(t *testing.T) {
	pd, err := Parse(sampleDiff)
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}
	summary := pd.Summary()
	if !contains(summary, "[diffPos:") {
		t.Error("Summary missing diffPos annotations")
	}
	if !contains(summary, "internal/handler/user.go") {
		t.Error("Summary missing file path")
	}
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || len(s) > 0 && containsStr(s, sub))
}

func containsStr(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
