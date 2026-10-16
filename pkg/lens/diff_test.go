package lens_test

import (
	"strings"
	"testing"

	"github.com/stellar/go-stellar-sdk/xdr"

	"github.com/stellar-optics/stellar-xdr-lens/pkg/lens"
)

// mustMarshal builds a base64 XDR payload from a concrete value. Constructing
// fixtures this way keeps diff tests deterministic and offline while still
// exercising the real encoder.
func mustMarshal(t *testing.T, v any) string {
	t.Helper()
	s, err := xdr.MarshalBase64(v)
	if err != nil {
		t.Fatalf("marshalling %T: %v", v, err)
	}
	return s
}

func TestDiffIdenticalValues(t *testing.T) {
	t.Parallel()

	fixtures := []string{"tx_success.env", "tx_failed.res", "tx_soroban.env"}

	for _, f := range fixtures {
		t.Run(f, func(t *testing.T) {
			t.Parallel()

			payload := loadFixture(t, f)
			got, err := lens.DiffBase64(payload, payload, "")
			if err != nil {
				t.Fatalf("DiffBase64() error = %v", err)
			}
			if !got.Equal() {
				t.Errorf("Diff of a value against itself reported %d change(s): %v",
					len(got.Changes), got.Changes)
			}
			if got.Summary() != "no differences" {
				t.Errorf("Summary() = %q, want %q", got.Summary(), "no differences")
			}
		})
	}
}

func TestDiffDetectsScalarChange(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		left           any
		right          any
		wantPath       string
		wantOp         lens.ChangeOp
		wantBeforeNote string
		wantAfterNote  string
	}{
		{
			name:           "price numerator",
			left:           xdr.Price{N: 1, D: 2},
			right:          xdr.Price{N: 3, D: 2},
			wantPath:       "", // prices now render as a single fraction scalar
			wantOp:         lens.ChangeModified,
			wantBeforeNote: "1/2 (= 0.5000000)",
			wantAfterNote:  "3/2 (= 1.5000000)",
		},
		{
			name:           "price denominator",
			left:           xdr.Price{N: 1, D: 2},
			right:          xdr.Price{N: 1, D: 7},
			wantPath:       "", // prices now render as a single fraction scalar
			wantOp:         lens.ChangeModified,
			wantBeforeNote: "1/2 (= 0.5000000)",
			wantAfterNote:  "1/7 (= 0.1428571)",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got, err := lens.DiffBase64(
				mustMarshal(t, tc.left), mustMarshal(t, tc.right), "Price")
			if err != nil {
				t.Fatalf("DiffBase64() error = %v", err)
			}
			if len(got.Changes) != 1 {
				t.Fatalf("got %d change(s), want 1: %v", len(got.Changes), got.Changes)
			}
			c := got.Changes[0]
			if c.Path != tc.wantPath {
				t.Errorf("Path = %q, want %q", c.Path, tc.wantPath)
			}
			if c.Op != tc.wantOp {
				t.Errorf("Op = %v, want %v", c.Op, tc.wantOp)
			}
			before, ok := c.Before.(string)
			if !ok || before != tc.wantBeforeNote {
				t.Errorf("Before = %v, want %q", c.Before, tc.wantBeforeNote)
			}
			after, ok := c.After.(string)
			if !ok || after != tc.wantAfterNote {
				t.Errorf("After = %v, want %q", c.After, tc.wantAfterNote)
			}
		})
	}
}

// TestDiffDetectsUnionArmSwitch covers the case that motivates a structural
// diff: when a union changes arms, the useful answer is which field appeared
// and which disappeared, not that the bytes differ.
func TestDiffDetectsUnionArmSwitch(t *testing.T) {
	t.Parallel()

	text := "hello"
	id := xdr.Uint64(42)

	left := xdr.Memo{Type: xdr.MemoTypeMemoText, Text: &text}
	right := xdr.Memo{Type: xdr.MemoTypeMemoId, Id: &id}

	got, err := lens.DiffBase64(mustMarshal(t, left), mustMarshal(t, right), "Memo")
	if err != nil {
		t.Fatalf("DiffBase64() error = %v", err)
	}
	if got.Equal() {
		t.Fatal("Diff reported no changes for a union that switched arms")
	}

	byPath := make(map[string]lens.Change, len(got.Changes))
	for _, c := range got.Changes {
		byPath[c.Path] = c
	}

	if c, ok := byPath["Text"]; !ok || c.Op != lens.ChangeRemoved {
		t.Errorf("want Text removed, got %+v (present=%v)", c, ok)
	}
	if c, ok := byPath["Id"]; !ok || c.Op != lens.ChangeAdded {
		t.Errorf("want Id added, got %+v (present=%v)", c, ok)
	}
	if c, ok := byPath["Type"]; !ok || c.Op != lens.ChangeModified {
		t.Errorf("want Type modified, got %+v (present=%v)", c, ok)
	}
}

// TestDiffRealTransactionsNamesChangedPaths asserts the diff is specific
// enough to be useful on production data, not merely non-empty.
func TestDiffRealTransactionsNamesChangedPaths(t *testing.T) {
	t.Parallel()

	got, err := lens.DiffBase64(
		loadFixture(t, "tx_soroban.res"),
		loadFixture(t, "tx_failed.res"),
		"",
	)
	if err != nil {
		t.Fatalf("DiffBase64() error = %v", err)
	}
	if got.Equal() {
		t.Fatal("Diff of a successful and a failed result reported no changes")
	}
	if got.LeftType != "TransactionResult" || got.RightType != "TransactionResult" {
		t.Errorf("types = %s/%s, want TransactionResult on both sides",
			got.LeftType, got.RightType)
	}

	// The transaction-level result code must be among the reported changes:
	// that is the single most important difference between these two.
	var foundCode bool
	for _, c := range got.Changes {
		if c.Path == "Result.Code" {
			foundCode = true
			before, _ := c.Before.(string)
			after, _ := c.After.(string)
			if !strings.Contains(before, "Success") {
				t.Errorf("Result.Code before = %q, want a success code", before)
			}
			if !strings.Contains(after, "Failed") {
				t.Errorf("Result.Code after = %q, want a failure code", after)
			}
		}
	}
	if !foundCode {
		t.Errorf("Diff did not report a change at Result.Code; paths were: %v", paths(got))
	}

	// Paths must be addressable, not opaque blobs.
	for _, c := range got.Changes {
		if c.Path == "" {
			t.Error("a change has an empty path")
		}
	}
}

// TestDiffAcrossDifferentTypes documents that comparing unlike types is
// allowed and reports the mismatch rather than failing.
func TestDiffAcrossDifferentTypes(t *testing.T) {
	t.Parallel()

	got, err := lens.DiffBase64(
		loadFixture(t, "tx_success.env"),
		loadFixture(t, "tx_success.res"),
		"",
	)
	if err != nil {
		t.Fatalf("DiffBase64() error = %v", err)
	}
	if got.LeftType == got.RightType {
		t.Fatalf("expected differing types, both were %s", got.LeftType)
	}
	if got.Equal() {
		t.Error("Diff of two different types reported no changes")
	}
}

func TestDiffListLengthChange(t *testing.T) {
	t.Parallel()

	// A claim predicate list is a convenient ordered collection to grow.
	left := xdr.ClaimPredicate{
		Type: xdr.ClaimPredicateTypeClaimPredicateAnd,
		AndPredicates: &[]xdr.ClaimPredicate{
			{Type: xdr.ClaimPredicateTypeClaimPredicateUnconditional},
			{Type: xdr.ClaimPredicateTypeClaimPredicateUnconditional},
		},
	}
	right := xdr.ClaimPredicate{
		Type: xdr.ClaimPredicateTypeClaimPredicateAnd,
		AndPredicates: &[]xdr.ClaimPredicate{
			{Type: xdr.ClaimPredicateTypeClaimPredicateUnconditional},
		},
	}

	l, err := lens.DecodeAs(mustMarshal(t, left), "ClaimPredicate")
	if err != nil {
		t.Skipf("ClaimPredicate is not in the registry: %v", err)
	}
	r, err := lens.DecodeAs(mustMarshal(t, right), "ClaimPredicate")
	if err != nil {
		t.Skipf("ClaimPredicate is not in the registry: %v", err)
	}

	got, err := lens.Diff(l, r)
	if err != nil {
		t.Fatalf("Diff() error = %v", err)
	}
	if got.Equal() {
		t.Error("Diff of lists with different lengths reported no changes")
	}
}

func TestDiffWithOptionsAlignsInsertedListItem(t *testing.T) {
	t.Parallel()

	value := func(items ...string) *lens.Value {
		children := make([]*lens.Node, 0, len(items))
		for _, item := range items {
			children = append(children, &lens.Node{Kind: lens.KindScalar, Value: item})
		}
		return &lens.Value{Type: "List", Node: &lens.Node{Kind: lens.KindList, Children: children}}
	}

	left, right := value("a", "b"), value("new", "a", "b")
	defaultDiff, err := lens.Diff(left, right)
	if err != nil {
		t.Fatalf("Diff() error = %v", err)
	}
	aligned, err := lens.DiffWithOptions(left, right, lens.DiffOptions{AlignLists: true})
	if err != nil {
		t.Fatalf("DiffWithOptions() error = %v", err)
	}
	if len(defaultDiff.Changes) <= 1 {
		t.Fatalf("positional diff got %d change(s), want cascading changes", len(defaultDiff.Changes))
	}
	if len(aligned.Changes) != 1 {
		t.Fatalf("aligned diff got %d change(s), want one: %+v", len(aligned.Changes), aligned.Changes)
	}
	if aligned.Changes[0].Op != lens.ChangeAdded || aligned.Changes[0].Path != "[0]" {
		t.Errorf("aligned change = %+v, want added item at [0]", aligned.Changes[0])
	}
}

func TestDiffWithOptionsAlignsRemovedListItem(t *testing.T) {
	t.Parallel()

	value := func(items ...string) *lens.Value {
		children := make([]*lens.Node, 0, len(items))
		for _, item := range items {
			children = append(children, &lens.Node{Kind: lens.KindScalar, Value: item})
		}
		return &lens.Value{Type: "List", Node: &lens.Node{Kind: lens.KindList, Children: children}}
	}

	got, err := lens.DiffWithOptions(value("a", "remove", "b"), value("a", "b"), lens.DiffOptions{AlignLists: true})
	if err != nil {
		t.Fatalf("DiffWithOptions() error = %v", err)
	}
	if len(got.Changes) != 1 || got.Changes[0].Op != lens.ChangeRemoved {
		t.Fatalf("aligned diff = %+v, want one removal", got.Changes)
	}
}

func TestDiffRejectsNilValues(t *testing.T) {
	t.Parallel()

	v, err := lens.Decode(loadFixture(t, "tx_success.env"))
	if err != nil {
		t.Fatalf("Decode() error = %v", err)
	}

	if _, err := lens.Diff(nil, v); err == nil {
		t.Error("Diff(nil, v) error = nil, want an error")
	}
	if _, err := lens.Diff(v, nil); err == nil {
		t.Error("Diff(v, nil) error = nil, want an error")
	}
}

func TestDiffBase64ReportsDecodeErrors(t *testing.T) {
	t.Parallel()

	valid := loadFixture(t, "tx_success.env")

	tests := []struct {
		name        string
		left, right string
	}{
		{"bad left", "!!!", valid},
		{"bad right", valid, "!!!"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			if _, err := lens.DiffBase64(tc.left, tc.right, ""); err == nil {
				t.Error("DiffBase64() error = nil, want an error")
			}
		})
	}
}

func TestChangeOpString(t *testing.T) {
	t.Parallel()

	tests := []struct {
		op   lens.ChangeOp
		want string
	}{
		{lens.ChangeModified, "modified"},
		{lens.ChangeAdded, "added"},
		{lens.ChangeRemoved, "removed"},
		{lens.ChangeTypeChanged, "type_changed"},
	}

	for _, tc := range tests {
		if got := tc.op.String(); got != tc.want {
			t.Errorf("ChangeOp(%d).String() = %q, want %q", tc.op, got, tc.want)
		}
	}
}

func TestChangeStringAndDiffSummary(t *testing.T) {
	t.Parallel()

	tests := []struct {
		change lens.Change
		want   string
	}{
		{lens.Change{Path: "Field", Op: lens.ChangeAdded, After: "new"}, "+ Field = new"},
		{lens.Change{Path: "Field", Op: lens.ChangeRemoved, Before: "old"}, "- Field = old"},
		{lens.Change{Path: "Field", Op: lens.ChangeModified, Before: 1, After: 2}, "~ Field: 1 -> 2"},
		{lens.Change{Path: "Field", Op: lens.ChangeTypeChanged, BeforeNote: "before", AfterNote: "after"}, "~ Field: before -> after"},
	}
	for _, tc := range tests {
		if got := tc.change.String(); got != tc.want {
			t.Errorf("Change.String() = %q, want %q", got, tc.want)
		}
	}

	result := &lens.DiffResult{Changes: []lens.Change{
		{Op: lens.ChangeModified},
		{Op: lens.ChangeTypeChanged},
		{Op: lens.ChangeAdded},
		{Op: lens.ChangeRemoved},
	}}
	if got, want := result.Summary(), "2 changed, 1 added, 1 removed"; got != want {
		t.Errorf("Summary() = %q, want %q", got, want)
	}
}

func TestDiffChangeSummariesForAddedContainers(t *testing.T) {
	t.Parallel()

	left := &lens.Value{Type: "Root", Node: &lens.Node{Kind: lens.KindStruct}}
	right := &lens.Value{Type: "Root", Node: &lens.Node{Kind: lens.KindStruct, Children: []*lens.Node{
		{Name: "Record", Kind: lens.KindStruct, TypeName: "RecordType"},
		{Name: "Items", Kind: lens.KindList, Children: []*lens.Node{{Kind: lens.KindScalar, Value: "x"}}},
	}}}
	got, err := lens.Diff(left, right)
	if err != nil {
		t.Fatalf("Diff() error = %v", err)
	}
	if len(got.Changes) != 2 || got.Changes[0].After != "{RecordType}" || got.Changes[1].After != "[1 item(s)]" {
		t.Errorf("container addition values = %+v", got.Changes)
	}
}

func TestDiffAlignmentComparesNestedStructItems(t *testing.T) {
	t.Parallel()

	item := func(id string) *lens.Node {
		return &lens.Node{Kind: lens.KindStruct, TypeName: "Item", Children: []*lens.Node{
			{Name: "ID", Kind: lens.KindScalar, Value: id},
		}}
	}
	list := func(items ...*lens.Node) *lens.Value {
		return &lens.Value{Type: "Items", Node: &lens.Node{Kind: lens.KindList, Children: items}}
	}
	got, err := lens.DiffWithOptions(list(item("a"), item("b")), list(item("inserted"), item("a"), item("b")), lens.DiffOptions{AlignLists: true})
	if err != nil {
		t.Fatalf("DiffWithOptions() error = %v", err)
	}
	if len(got.Changes) != 1 || got.Changes[0].Op != lens.ChangeAdded || got.Changes[0].After != "{Item}" {
		t.Errorf("aligned struct diff = %+v, want one inserted Item", got.Changes)
	}
}

func paths(d *lens.DiffResult) []string {
	out := make([]string, 0, len(d.Changes))
	for _, c := range d.Changes {
		out = append(out, c.Path)
	}
	return out
}
