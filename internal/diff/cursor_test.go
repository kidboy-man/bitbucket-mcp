package diff

import (
	"testing"
)

func TestCursorRoundtrip(t *testing.T) {
	c := Cursor{FileIndex: 1, HunkIndex: 2, LineIndex: 3, FilterHash: "abc123"}
	enc, err := EncodeCursor(c)
	if err != nil {
		t.Fatalf("EncodeCursor: %v", err)
	}
	got, err := DecodeCursor(enc)
	if err != nil {
		t.Fatalf("DecodeCursor: %v", err)
	}
	if got != c {
		t.Errorf("got %+v, want %+v", got, c)
	}
}

func TestGetPageFirstPage(t *testing.T) {
	pd, err := Parse(sampleDiff)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	page, err := pd.GetPage(PageInput{PRURL: "https://bitbucket.org/ws/repo/pull-requests/1"})
	if err != nil {
		t.Fatalf("GetPage: %v", err)
	}
	if len(page.Files) == 0 {
		t.Fatal("expected files in page")
	}
	if page.NextCursor != "" {
		t.Error("first full page should have no NextCursor for small diff")
	}
}

func TestGetPageMaxLines(t *testing.T) {
	pd, err := Parse(sampleDiff)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	prURL := "https://bitbucket.org/ws/repo/pull-requests/1"
	page1, err := pd.GetPage(PageInput{PRURL: prURL, MaxLines: 3})
	if err != nil {
		t.Fatalf("GetPage page1: %v", err)
	}
	if page1.NextCursor == "" {
		t.Fatal("expected NextCursor when MaxLines reached")
	}

	// Count lines in page1.
	lines1 := countPageLines(page1)
	if lines1 != 3 {
		t.Errorf("page1 lines = %d, want 3", lines1)
	}

	// Page 2.
	page2, err := pd.GetPage(PageInput{PRURL: prURL, MaxLines: 3, Cursor: page1.NextCursor})
	if err != nil {
		t.Fatalf("GetPage page2: %v", err)
	}
	lines2 := countPageLines(page2)
	if lines2 == 0 {
		t.Error("page2 should have lines")
	}

	// No overlap: collect all line DiffPositions.
	seen := map[int]bool{}
	for _, f := range page1.Files {
		for _, h := range f.Hunks {
			for _, l := range h.Lines {
				seen[l.DiffPosition] = true
			}
		}
	}
	for _, f := range page2.Files {
		for _, h := range f.Hunks {
			for _, l := range h.Lines {
				if seen[l.DiffPosition] {
					t.Errorf("duplicate DiffPosition %d across pages", l.DiffPosition)
				}
			}
		}
	}
}

func TestGetPageFilterMismatch(t *testing.T) {
	pd, err := Parse(sampleDiff)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	prURL := "https://bitbucket.org/ws/repo/pull-requests/1"
	page1, err := pd.GetPage(PageInput{PRURL: prURL, MaxLines: 2})
	if err != nil {
		t.Fatalf("GetPage: %v", err)
	}

	// Use cursor with different PR URL — should fail.
	_, err = pd.GetPage(PageInput{PRURL: "https://bitbucket.org/ws/repo/pull-requests/99", Cursor: page1.NextCursor})
	if err == nil {
		t.Error("expected filter mismatch error")
	}
}

func TestGetPageFilterMismatchWhenMaxLinesChanges(t *testing.T) {
	pd, err := Parse(sampleDiff)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	prURL := "https://bitbucket.org/ws/repo/pull-requests/1"
	page1, err := pd.GetPage(PageInput{PRURL: prURL, MaxLines: 2})
	if err != nil {
		t.Fatalf("GetPage: %v", err)
	}

	_, err = pd.GetPage(PageInput{PRURL: prURL, MaxLines: 3, Cursor: page1.NextCursor})
	if err == nil {
		t.Fatal("expected filter mismatch error")
	}
}

func TestGetPageFileFilter(t *testing.T) {
	pd, err := Parse(sampleDiff)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	page, err := pd.GetPage(PageInput{
		PRURL:      "https://bitbucket.org/ws/repo/pull-requests/1",
		FileFilter: "internal/handler/user.go",
	})
	if err != nil {
		t.Fatalf("GetPage: %v", err)
	}
	for _, f := range page.Files {
		if f.Path != "internal/handler/user.go" {
			t.Errorf("unexpected file in filtered page: %q", f.Path)
		}
	}
}

func TestGetPageInvalidCursor(t *testing.T) {
	pd, err := Parse(sampleDiff)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	_, err = pd.GetPage(PageInput{PRURL: "u", Cursor: "notvalid!!"})
	if err == nil {
		t.Error("expected error for invalid cursor")
	}
}

func countPageLines(p *DiffPage) int {
	n := 0
	for _, f := range p.Files {
		for _, h := range f.Hunks {
			n += len(h.Lines)
		}
	}
	return n
}
