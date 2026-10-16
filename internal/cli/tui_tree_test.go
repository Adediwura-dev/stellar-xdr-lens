package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stellar-optics/stellar-xdr-lens/pkg/lens"
	"github.com/stellar-optics/stellar-xdr-lens/pkg/lens/format"
)

func loadFixtureValue(t *testing.T, filename string) *lens.Value {
	t.Helper()
	path := filepath.Join("..", "..", "testdata", filename)
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading %s: %v", filename, err)
	}
	val, err := lens.Decode(strings.TrimSpace(string(data)))
	if err != nil {
		t.Fatalf("decoding %s: %v", filename, err)
	}
	return val
}

func TestTUITreeCanonicalFixtures(t *testing.T) {
	fixtures := []string{
		"tx_success.env.txt",
		"tx_failed.env.txt",
		"tx_feebump.env.txt",
		"tx_soroban.env.txt",
		"tx_success.res.txt",
	}

	for _, f := range fixtures {
		t.Run(f, func(t *testing.T) {
			val := loadFixtureValue(t, f)
			in := strings.NewReader("q\n")
			var out bytes.Buffer

			tree := NewTUITree(val, in, &out, format.NoColor)
			tree.SetDimensions(80, 24)

			if err := tree.Run(); err != nil {
				t.Fatalf("tree.Run() error = %v", err)
			}

			rendered := out.String()
			if !strings.Contains(rendered, val.Type) {
				t.Errorf("expected rendered TUI to contain type %s", val.Type)
			}
			if !strings.Contains(rendered, "Explorer") {
				t.Errorf("expected header in TUI output")
			}
		})
	}
}

func TestTUITreeNavigationAndExpand(t *testing.T) {
	val := loadFixtureValue(t, "tx_success.env.txt")
	var out bytes.Buffer
	tree := NewTUITree(val, strings.NewReader(""), &out, format.NoColor)
	tree.SetDimensions(80, 24)

	initialCount := len(tree.rows)
	if initialCount == 0 {
		t.Fatal("expected non-empty initial rows")
	}

	// Move Down
	tree.HandleKey("down")
	if tree.cursor != 1 {
		t.Errorf("expected cursor at 1, got %d", tree.cursor)
	}

	// Move Up
	tree.HandleKey("up")
	if tree.cursor != 0 {
		t.Errorf("expected cursor at 0, got %d", tree.cursor)
	}

	// Move Bottom and Top
	tree.HandleKey("G")
	if tree.cursor != len(tree.rows)-1 {
		t.Errorf("expected cursor at end, got %d", tree.cursor)
	}
	tree.HandleKey("g")
	if tree.cursor != 0 {
		t.Errorf("expected cursor at top, got %d", tree.cursor)
	}

	// Expand All
	tree.HandleKey("e")
	expandedCount := len(tree.rows)
	if expandedCount < initialCount {
		t.Errorf("expected expanded rows >= initial rows, got %d < %d", expandedCount, initialCount)
	}

	// Collapse All
	tree.HandleKey("c")
	collapsedCount := len(tree.rows)
	if collapsedCount > expandedCount {
		t.Errorf("expected collapsed rows <= expanded rows, got %d > %d", collapsedCount, expandedCount)
	}

	// Toggle selected
	tree.HandleKey("enter")
	tree.HandleKey("space")

	// Collapse selected / move to parent
	tree.HandleKey("left")
	tree.HandleKey("h")
}

func TestTUITreeFilter(t *testing.T) {
	val := loadFixtureValue(t, "tx_success.env.txt")
	var out bytes.Buffer
	tree := NewTUITree(val, strings.NewReader(""), &out, format.NoColor)

	tree.HandleKey("/Fee")
	if len(tree.rows) == 0 {
		t.Error("expected matches for filter /Fee")
	}

	// Clear filter with esc
	tree.HandleKey("esc")
	if tree.filter != "" {
		t.Errorf("expected empty filter after esc, got %q", tree.filter)
	}
}

func TestTUITreeKeySequencesAndRun(t *testing.T) {
	val := loadFixtureValue(t, "tx_success.env.txt")

	// Simulated ANSI arrow key sequences: down, enter, up, ctrl-c
	input := "\x1b[B\r\x1b[A\x1b[H\x1b[F\x03"
	var out bytes.Buffer
	tree := NewTUITree(val, strings.NewReader(input), &out, format.ColorPalette)

	if err := tree.Run(); err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	rendered := out.String()
	if !strings.Contains(rendered, "\x1b[") {
		t.Errorf("expected ANSI color codes with ColorPalette")
	}
}

func TestTUITreeEdgeCases(t *testing.T) {
	emptyVal := &lens.Value{
		Type: "Empty",
		Node: nil,
	}
	var out bytes.Buffer
	tree := NewTUITree(emptyVal, strings.NewReader("q\n"), &out, format.NoColor)
	tree.SetDimensions(0, 0) // Should keep default dimensions

	if err := tree.Run(); err != nil {
		t.Fatalf("empty tree run: %v", err)
	}

	tree.MoveUp()
	tree.MoveDown()
	tree.MoveTop()
	tree.MoveBottom()
	tree.ToggleSelected()
	tree.CollapseSelected()

	// Test readKey branches: space, tab, standalone esc, arrow keys
	keysInput := " \t\x1b\x1b[C\x1b[D"
	keyTree := NewTUITree(emptyVal, strings.NewReader(keysInput), &out, format.NoColor)
	_ = keyTree.Run()
}
