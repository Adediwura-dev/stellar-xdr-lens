package lens

import (
	"bytes"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"math"
	"reflect"
	"strconv"
	"strings"
)

// ChangeOp classifies a single difference between two XDR values.
type ChangeOp uint8

const (
	// ChangeModified means the path exists on both sides with different values.
	ChangeModified ChangeOp = iota
	// ChangeAdded means the path exists only on the right-hand side.
	ChangeAdded
	// ChangeRemoved means the path exists only on the left-hand side.
	ChangeRemoved
	// ChangeTypeChanged means the path exists on both sides but holds a
	// different kind of value, such as a scalar replaced by a struct. This
	// is the usual signal that a union switched arms.
	ChangeTypeChanged
)

func (c ChangeOp) String() string {
	switch c {
	case ChangeModified:
		return "modified"
	case ChangeAdded:
		return "added"
	case ChangeRemoved:
		return "removed"
	case ChangeTypeChanged:
		return "type_changed"
	default:
		return "unknown"
	}
}

// Change is one difference between two decoded XDR values.
type Change struct {
	// Path locates the change, e.g. "V1.Tx.Operations[0].Body.PaymentOp.Amount".
	Path string
	// Op is the kind of change.
	Op ChangeOp
	// Before is the left-hand value; nil when Op is ChangeAdded.
	Before any
	// After is the right-hand value; nil when Op is ChangeRemoved.
	After any
	// BeforeNote and AfterNote carry the enriched renderings, so a diff of
	// two amounts can show "100.0000000" rather than only the stroop count.
	BeforeNote string
	AfterNote  string
}

// String renders a change as a single line.
func (c Change) String() string {
	switch c.Op {
	case ChangeAdded:
		return fmt.Sprintf("+ %s = %s", c.Path, display(c.After, c.AfterNote))
	case ChangeRemoved:
		return fmt.Sprintf("- %s = %s", c.Path, display(c.Before, c.BeforeNote))
	default:
		return fmt.Sprintf("~ %s: %s -> %s", c.Path,
			display(c.Before, c.BeforeNote), display(c.After, c.AfterNote))
	}
}

func display(v any, note string) string {
	if v == nil && note == "" {
		return "(none)"
	}
	if v == nil {
		return note
	}
	s := fmt.Sprintf("%v", v)
	if note != "" {
		return s + " (" + note + ")"
	}
	return s
}

// DiffResult is the outcome of comparing two decoded values.
type DiffResult struct {
	// LeftType and RightType are the XDR types compared. They may differ,
	// which is itself worth reporting.
	LeftType  string
	RightType string
	// Changes lists every difference, in document order.
	Changes []Change
}

// DiffOptions configures structural comparison behavior.
type DiffOptions struct {
	// AlignLists matches unchanged list elements with a longest-common-
	// subsequence pass before comparing them. It is useful when insertions
	// would otherwise shift every later positional diff.
	AlignLists bool
	// Semantic compares supported ScVal integers as exact decimal values and
	// pairs ScVal map entries by canonical key instead of list position.
	Semantic bool
	// UnorderedSigners compares signature and signer collections as unordered
	// multisets, so reordering them is not reported as a difference. Only the
	// fields the protocol defines as sets (Signatures, Signers and
	// ExtraSigners) are affected; every other list stays positional.
	UnorderedSigners bool
	// Filter restricts the reported changes to the paths it selects. A nil
	// filter reports every change. Filtering happens after the walk, so it
	// only narrows the output; it never changes how the values are compared.
	Filter *PathFilter
	// Threshold ignores scalar numeric differences below or equal to the given delta.
	Threshold float64
	// DiffThreshold is an alias for Threshold.
	DiffThreshold float64
}

// Equal reports whether the two values were structurally identical.
func (d *DiffResult) Equal() bool { return len(d.Changes) == 0 }

// Diff structurally compares two decoded XDR values and reports every
// differing path.
//
// Unlike a byte or ordering comparison, this walks both trees together and
// names the fields that actually changed, which is what makes it useful for
// answering "what is different about these two envelopes".
//
// Values of different XDR types can still be compared; the type difference is
// reported and the walk proceeds, since a TransactionV0 and a Transaction
// often share most of their structure.
func Diff(left, right *Value) (*DiffResult, error) {
	return DiffWithOptions(left, right, DiffOptions{})
}

// DiffWithOptions structurally compares two values using the supplied options.
// The default Diff function remains positional for ordered XDR sequences.
func DiffWithOptions(left, right *Value, options DiffOptions) (*DiffResult, error) {
	if left == nil || right == nil {
		return nil, fmt.Errorf("%w: diff requires two decoded values", ErrInvalidArgument)
	}
	leftNode, rightNode := left.Node, right.Node
	if options.Semantic {
		var err error
		leftNode, err = scValNodeForValue(left, true)
		if err != nil {
			return nil, fmt.Errorf("preparing left semantic value: %w", err)
		}
		rightNode, err = scValNodeForValue(right, true)
		if err != nil {
			return nil, fmt.Errorf("preparing right semantic value: %w", err)
		}
	}
	res := &DiffResult{LeftType: left.Type, RightType: right.Type}
	diffNodesWithOptions(leftNode, rightNode, "", &res.Changes, options)
	if !options.Filter.Empty() {
		res.Changes = FilterChanges(res.Changes, options.Filter)
	}
	return res, nil
}

// diffNodesWithOptions walks two subtrees in parallel, appending changes as it goes.
func diffNodesWithOptions(a, b *Node, path string, out *[]Change, options DiffOptions) {
	switch {
	case a == nil && b == nil:
		return
	case a == nil:
		*out = append(*out, Change{Path: path, Op: ChangeAdded, After: leafValue(b), AfterNote: b.Note})
		return
	case b == nil:
		*out = append(*out, Change{Path: path, Op: ChangeRemoved, Before: leafValue(a), BeforeNote: a.Note})
		return
	}

	if a.Kind != b.Kind {
		if nodesBinaryEqual(a, b) {
			return
		}
		*out = append(*out, Change{
			Path:       path,
			Op:         ChangeTypeChanged,
			Before:     leafValue(a),
			After:      leafValue(b),
			BeforeNote: a.Note,
			AfterNote:  b.Note,
		})
		return
	}

	switch a.Kind {
	case KindScalar:
		if !sameScalarWithOptions(a, b, options) {
			*out = append(*out, Change{
				Path:       path,
				Op:         ChangeModified,
				Before:     a.Value,
				After:      b.Value,
				BeforeNote: a.Note,
				AfterNote:  b.Note,
			})
		}

	case KindList:
		if options.UnorderedSigners && isSignerCollection(signerCollectionName(path, a, b)) {
			diffMultisetLists(a, b, path, out)
		} else {
			diffListsWithOptions(a, b, path, out, options)
		}

	case KindStruct:
		diffStructsWithOptions(a, b, path, out, options)
	}
}

// diffStructs compares two records field by field, preserving the field order
// of the left-hand side so output reads top-to-bottom like the source.
func diffStructsWithOptions(a, b *Node, path string, out *[]Change, options DiffOptions) {
	seen := make(map[string]bool, len(a.Children))

	for _, ac := range a.Children {
		seen[ac.Name] = true
		diffNodesWithOptions(ac, b.Child(ac.Name), join(path, ac.Name), out, options)
	}
	// Fields present only on the right: a union that switched arms shows up
	// here as a removal plus an addition.
	for _, bc := range b.Children {
		if seen[bc.Name] {
			continue
		}
		diffNodesWithOptions(nil, bc, join(path, bc.Name), out, options)
	}
}

const maxAlignmentCells = 1 << 20

// diffListsWithOptions compares sequences positionally unless alignment was
// requested. Very large products fall back to positional comparison to bound
// memory use on untrusted XDR input.
func diffListsWithOptions(a, b *Node, path string, out *[]Change, options DiffOptions) {
	if options.AlignLists && len(a.Children) > 0 && len(b.Children) > 0 &&
		len(a.Children) <= maxAlignmentCells/len(b.Children) {
		diffAlignedLists(a.Children, b.Children, path, out, options)
		return
	}
	maxLen := len(a.Children)
	if len(b.Children) > maxLen {
		maxLen = len(b.Children)
	}
	for i := range maxLen {
		var ac, bc *Node
		if i < len(a.Children) {
			ac = a.Children[i]
		}
		if i < len(b.Children) {
			bc = b.Children[i]
		}
		diffNodesWithOptions(ac, bc, path+"["+strconv.Itoa(i)+"]", out, options)
	}
}

func diffAlignedLists(left, right []*Node, path string, out *[]Change, options DiffOptions) {
	rows, columns := len(left)+1, len(right)+1
	lcs := make([][]int, rows)
	for i := range rows {
		lcs[i] = make([]int, columns)
	}
	for i := len(left) - 1; i >= 0; i-- {
		for j := len(right) - 1; j >= 0; j-- {
			if sameNodeWithOptions(left[i], right[j], options) {
				lcs[i][j] = 1 + lcs[i+1][j+1]
			} else if lcs[i+1][j] > lcs[i][j+1] {
				lcs[i][j] = lcs[i+1][j]
			} else {
				lcs[i][j] = lcs[i][j+1]
			}
		}
	}

	i, j := 0, 0
	for i < len(left) && j < len(right) {
		if sameNodeWithOptions(left[i], right[j], options) {
			diffNodesWithOptions(left[i], right[j], path+"["+strconv.Itoa(j)+"]", out, options)
			i++
			j++
		} else if lcs[i][j+1] >= lcs[i+1][j] {
			diffNodesWithOptions(nil, right[j], path+"["+strconv.Itoa(j)+"]", out, options)
			j++
		} else {
			diffNodesWithOptions(left[i], nil, path+"["+strconv.Itoa(i)+"]", out, options)
			i++
		}
	}
	for ; i < len(left); i++ {
		diffNodesWithOptions(left[i], nil, path+"["+strconv.Itoa(i)+"]", out, options)
	}
	for ; j < len(right); j++ {
		diffNodesWithOptions(nil, right[j], path+"["+strconv.Itoa(j)+"]", out, options)
	}
}

func sameNodeWithOptions(a, b *Node, options DiffOptions) bool {
	if a == nil || b == nil || a.Name != b.Name {
		return a == b
	}
	if a.Kind != b.Kind {
		return nodesBinaryEqual(a, b)
	}
	if a.Kind == KindScalar {
		return sameScalarWithOptions(a, b, options)
	}
	if len(a.Children) != len(b.Children) {
		return false
	}
	for i := range a.Children {
		if !sameNodeWithOptions(a.Children[i], b.Children[i], options) {
			return false
		}
	}
	return true
}

func sameScalarWithOptions(a, b *Node, options DiffOptions) bool {
	threshold := options.Threshold
	if threshold == 0 && options.DiffThreshold > 0 {
		threshold = options.DiffThreshold
	}

	// 1. Direct scalar value and note match
	if a.Note == b.Note && fmt.Sprintf("%v", a.Value) == fmt.Sprintf("%v", b.Value) {
		return true
	}

	// 2. Numeric delta threshold filter (--diff-threshold)
	if threshold > 0 {
		if numA, okA := asNumber(a.Value); okA {
			if numB, okB := asNumber(b.Value); okB {
				if math.Abs(numA-numB) <= threshold {
					return true
				}
			}
		}
	}

	// 3. Binary representation normalization (Hex vs Base64 vs Bytes)
	if nodesBinaryEqual(a, b) {
		if a.Note == b.Note || a.Note == "" || b.Note == "" || notesCompatible(a.Note, b.Note) {
			return true
		}
	}

	return false
}

func notesCompatible(aNote, bNote string) bool {
	if aNote == bNote {
		return true
	}
	aClean := strings.Trim(aNote, "\"")
	bClean := strings.Trim(bNote, "\"")
	return aClean == bClean
}

func asNumber(v any) (float64, bool) {
	if v == nil {
		return 0, false
	}
	switch val := v.(type) {
	case int:
		return float64(val), true
	case int8:
		return float64(val), true
	case int16:
		return float64(val), true
	case int32:
		return float64(val), true
	case int64:
		return float64(val), true
	case uint:
		return float64(val), true
	case uint8:
		return float64(val), true
	case uint16:
		return float64(val), true
	case uint32:
		return float64(val), true
	case uint64:
		return float64(val), true
	case float32:
		return float64(val), true
	case float64:
		return val, true
	case string:
		f, err := strconv.ParseFloat(val, 64)
		if err == nil {
			return f, true
		}
		return 0, false
	default:
		return 0, false
	}
}

func bytesFromHex(s string) ([]byte, bool) {
	s = strings.TrimSpace(s)
	s = strings.TrimPrefix(s, "0x")
	s = strings.TrimPrefix(s, "0X")
	if len(s) == 0 || len(s)%2 != 0 {
		return nil, false
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')) {
			return nil, false
		}
	}
	b, err := hex.DecodeString(s)
	if err != nil {
		return nil, false
	}
	return b, true
}

func bytesFromBase64(s string) ([]byte, bool) {
	s = strings.TrimSpace(s)
	if len(s) == 0 {
		return nil, false
	}
	if b, err := base64.StdEncoding.DecodeString(s); err == nil && len(b) > 0 {
		return b, true
	}
	if b, err := base64.RawStdEncoding.DecodeString(s); err == nil && len(b) > 0 {
		return b, true
	}
	return nil, false
}

func extractDirectBytes(v any) ([]byte, bool) {
	if v == nil {
		return nil, false
	}
	switch val := v.(type) {
	case []byte:
		if len(val) == 0 {
			return nil, false
		}
		return val, true
	default:
		rv := reflect.ValueOf(v)
		if rv.IsValid() && rv.Kind() == reflect.Array && rv.Type().Elem().Kind() == reflect.Uint8 {
			if rv.Len() == 0 {
				return nil, false
			}
			b := make([]byte, rv.Len())
			for i := 0; i < rv.Len(); i++ {
				b[i] = byte(rv.Index(i).Uint())
			}
			return b, true
		}
		return nil, false
	}
}

func extractListBytes(n *Node) ([]byte, bool) {
	if n == nil || n.Kind != KindList || len(n.Children) == 0 {
		return nil, false
	}
	b := make([]byte, len(n.Children))
	for i, child := range n.Children {
		if child.Kind != KindScalar {
			return nil, false
		}
		num, ok := asNumber(child.Value)
		if !ok || num < 0 || num > 255 || math.Floor(num) != num {
			return nil, false
		}
		b[i] = byte(num)
	}
	return b, true
}

func nodesBinaryEqual(a, b *Node) bool {
	if a == nil || b == nil {
		return false
	}

	// Case 1: Both are scalar strings (one hex, one base64, or hex case diff)
	sA, okSA := a.Value.(string)
	sB, okSB := b.Value.(string)
	if okSA && okSB && a.Kind == KindScalar && b.Kind == KindScalar {
		bHexA, isHexA := bytesFromHex(sA)
		bHexB, isHexB := bytesFromHex(sB)
		bB64A, isB64A := bytesFromBase64(sA)
		bB64B, isB64B := bytesFromBase64(sB)

		// One is Hex, the other is Base64:
		if isHexA && isB64B && bytes.Equal(bHexA, bB64B) {
			return true
		}
		if isB64A && isHexB && bytes.Equal(bB64A, bHexB) {
			return true
		}
		// Both are Hex (case differences):
		if isHexA && isHexB && bytes.Equal(bHexA, bHexB) {
			return true
		}
		return false
	}

	// Case 2: One is raw bytes or byte array or byte list, the other is scalar string (hex or base64)
	var rawBytesA []byte
	if bytesA, ok := extractDirectBytes(a.Value); ok {
		rawBytesA = bytesA
	} else if bytesA, ok := extractListBytes(a); ok {
		rawBytesA = bytesA
	}

	var rawBytesB []byte
	if bytesB, ok := extractDirectBytes(b.Value); ok {
		rawBytesB = bytesB
	} else if bytesB, ok := extractListBytes(b); ok {
		rawBytesB = bytesB
	}

	if rawBytesA != nil && rawBytesB != nil {
		return bytes.Equal(rawBytesA, rawBytesB)
	}

	if rawBytesA != nil && okSB {
		if bHex, ok := bytesFromHex(sB); ok && bytes.Equal(rawBytesA, bHex) {
			return true
		}
		if bB64, ok := bytesFromBase64(sB); ok && bytes.Equal(rawBytesA, bB64) {
			return true
		}
	}

	if rawBytesB != nil && okSA {
		if bHex, ok := bytesFromHex(sA); ok && bytes.Equal(rawBytesB, bHex) {
			return true
		}
		if bB64, ok := bytesFromBase64(sA); ok && bytes.Equal(rawBytesB, bB64) {
			return true
		}
	}

	return false
}

// leafValue summarises a node for reporting in a change. Struct and list
// nodes are described rather than expanded, since the change line names the
// path and the detail lives in the tree output.
func leafValue(n *Node) any {
	if n == nil {
		return nil
	}
	switch n.Kind {
	case KindScalar:
		return n.Value
	case KindList:
		return fmt.Sprintf("[%d item(s)]", len(n.Children))
	default:
		if n.TypeName != "" {
			return "{" + n.TypeName + "}"
		}
		return "{...}"
	}
}

func join(path, name string) string {
	if path == "" {
		return name
	}
	if name == "" {
		return path
	}
	return path + "." + name
}

// DiffBase64 is a convenience wrapper that decodes both payloads and diffs
// them. When typeName is empty the type is auto-detected for each side
// independently.
func DiffBase64(leftPayload, rightPayload, typeName string) (*DiffResult, error) {
	return DiffBase64WithOptions(leftPayload, rightPayload, typeName, DiffOptions{})
}

// DiffBase64WithOptions decodes and compares two payloads using the supplied
// structural diff options. When typeName is empty, each side is detected
// independently.
func DiffBase64WithOptions(leftPayload, rightPayload, typeName string, options DiffOptions) (*DiffResult, error) {
	decode := func(p string) (*Value, error) {
		if typeName != "" {
			return DecodeAs(p, typeName)
		}
		return Decode(p)
	}
	left, err := decode(leftPayload)
	if err != nil {
		return nil, fmt.Errorf("decoding left value: %w", err)
	}
	right, err := decode(rightPayload)
	if err != nil {
		return nil, fmt.Errorf("decoding right value: %w", err)
	}
	return DiffWithOptions(left, right, options)
}

// Summary renders a one-line description of the diff, suitable for a CI log.
func (d *DiffResult) Summary() string {
	if d.Equal() {
		return "no differences"
	}
	var added, removed, modified int
	for _, c := range d.Changes {
		switch c.Op {
		case ChangeAdded:
			added++
		case ChangeRemoved:
			removed++
		default:
			modified++
		}
	}
	var parts []string
	if modified > 0 {
		parts = append(parts, strconv.Itoa(modified)+" changed")
	}
	if added > 0 {
		parts = append(parts, strconv.Itoa(added)+" added")
	}
	if removed > 0 {
		parts = append(parts, strconv.Itoa(removed)+" removed")
	}
	return strings.Join(parts, ", ")
}
