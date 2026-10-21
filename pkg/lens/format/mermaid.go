package format

import (
	"fmt"
	"io"
	"regexp"
	"strings"

	"github.com/stellar-optics/stellar-xdr-lens/pkg/lens"
)

// MermaidFormatter renders transaction operations and relationships as a
// Mermaid flowchart diagram (graph TD).
type MermaidFormatter struct {
	direction string
}

// MermaidOption configures a MermaidFormatter.
type MermaidOption func(*MermaidFormatter)

// WithMermaidDirection sets the flowchart layout direction ("TD", "LR", etc.).
func WithMermaidDirection(direction string) MermaidOption {
	return func(m *MermaidFormatter) {
		m.direction = direction
	}
}

// NewMermaidFormatter creates a MermaidFormatter with default direction "TD".
func NewMermaidFormatter(opts ...MermaidOption) *MermaidFormatter {
	m := &MermaidFormatter{
		direction: "TD",
	}
	for _, opt := range opts {
		opt(m)
	}
	return m
}

// Format renders the transaction operations or node structure to w as Mermaid flowchart syntax.
func (f *MermaidFormatter) Format(w io.Writer, v *lens.Value) error {
	if v == nil || v.Node == nil {
		return fmt.Errorf("format: nil value")
	}

	direction := f.direction
	if direction == "" {
		direction = "TD"
	}

	var b strings.Builder
	fmt.Fprintf(&b, "graph %s\n", direction)

	// Attempt high-level transaction operation explanation first
	summary, err := lens.Explain(v)
	if err == nil && summary != nil && (summary.Source != "" || len(summary.Operations) > 0) {
		f.formatTransactionGraph(&b, summary)
	} else {
		// Fallback for non-transaction XDR types: render top-level node hierarchy
		f.formatNodeGraph(&b, v.Node)
	}

	_, writeErr := io.WriteString(w, b.String())
	return writeErr
}

var nonAlphanumeric = regexp.MustCompile(`[^a-zA-Z0-9_]`)

func sanitizeID(s string) string {
	res := nonAlphanumeric.ReplaceAllString(s, "_")
	if res == "" || (res[0] >= '0' && res[0] <= '9') {
		res = "node_" + res
	}
	return res
}

func sanitizeLabel(s string) string {
	// Escape quotes and ensure no bracket breaking
	clean := strings.ReplaceAll(s, `"`, `'`)
	clean = strings.ReplaceAll(clean, "\n", " ")
	return clean
}

func (f *MermaidFormatter) formatTransactionGraph(b *strings.Builder, s *lens.Summary) {
	sourceAcc := s.Source
	if sourceAcc == "" {
		sourceAcc = "TransactionSource"
	}
	sourceID := sanitizeID("acc_" + sourceAcc)
	fmt.Fprintf(b, "  %s[\"%s (Source)\"]\n", sourceID, sanitizeLabel(truncateAddr(sourceAcc)))

	if s.FeeBumpSource != "" && s.FeeBumpSource != s.Source {
		feeBumpID := sanitizeID("acc_" + s.FeeBumpSource)
		fmt.Fprintf(b, "  %s[\"%s (Fee-Bump)\"]\n", feeBumpID, sanitizeLabel(truncateAddr(s.FeeBumpSource)))
		fmt.Fprintf(b, "  %s -->|covers fee| %s\n", feeBumpID, sourceID)
	}

	for _, op := range s.Operations {
		opSource := op.Source
		currentSourceID := sourceID
		if opSource != "" && opSource != s.Source {
			currentSourceID = sanitizeID("acc_" + opSource)
			fmt.Fprintf(b, "  %s[\"%s (Op %d Source)\"]\n", currentSourceID, sanitizeLabel(truncateAddr(opSource)), op.Index)
		}

		opNodeID := fmt.Sprintf("op_%d", op.Index)
		opLabel := fmt.Sprintf("Op %d: %s", op.Index, op.Type)
		if op.Result != nil && !op.Result.Success {
			opLabel += fmt.Sprintf(" (FAILED: %s)", op.Result.Code)
		}
		fmt.Fprintf(b, "  %s[\"%s\"]\n", opNodeID, sanitizeLabel(opLabel))

		edgeLabel := op.Type
		if op.Detail != "" {
			edgeLabel = op.Detail
		}
		fmt.Fprintf(b, "  %s -->|\"%s\"| %s\n", currentSourceID, sanitizeLabel(edgeLabel), opNodeID)
	}
}

func (f *MermaidFormatter) formatNodeGraph(b *strings.Builder, root *lens.Node) {
	rootID := "root"
	rootName := root.TypeName
	if rootName == "" {
		rootName = root.Name
	}
	if rootName == "" {
		rootName = "Root"
	}
	fmt.Fprintf(b, "  %s[\"%s\"]\n", rootID, sanitizeLabel(rootName))

	for i, child := range root.Children {
		childID := fmt.Sprintf("child_%d", i)
		childName := child.Name
		if childName == "" && child.TypeName != "" {
			childName = child.TypeName
		}
		if childName == "" {
			childName = fmt.Sprintf("item_%d", i)
		}
		if child.Kind == lens.KindScalar && child.Value != nil {
			childName = fmt.Sprintf("%s: %v", childName, child.Value)
		}
		fmt.Fprintf(b, "  %s[\"%s\"]\n", childID, sanitizeLabel(childName))
		fmt.Fprintf(b, "  %s --> %s\n", rootID, childID)
	}
}

func truncateAddr(addr string) string {
	if len(addr) > 12 {
		return addr[:4] + "..." + addr[len(addr)-4:]
	}
	return addr
}
