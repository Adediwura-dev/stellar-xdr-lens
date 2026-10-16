package format_test

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stellar-optics/stellar-xdr-lens/pkg/lens"
	"github.com/stellar-optics/stellar-xdr-lens/pkg/lens/format"
)

func TestCompactFormatterCanonicalFixtures(t *testing.T) {
	fixtures := []string{
		"tx_success.env.txt",
		"tx_failed.env.txt",
		"tx_feebump.env.txt",
		"tx_soroban.env.txt",
		"tx_success.res.txt",
		"tx_failed.res.txt",
		"tx_soroban.res.txt",
	}

	f := format.NewCompactFormatter()

	for _, name := range fixtures {
		t.Run(name, func(t *testing.T) {
			path := filepath.Join("..", "..", "..", "testdata", name)
			content, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("reading %s: %v", path, err)
			}
			b64 := strings.TrimSpace(string(content))
			val, err := lens.Decode(b64)
			if err != nil {
				t.Fatalf("decoding %s: %v", name, err)
			}

			var buf bytes.Buffer
			if err := f.Format(&buf, val); err != nil {
				t.Fatalf("formatting %s: %v", name, err)
			}

			out := buf.String()
			// Must end with newline
			if !strings.HasSuffix(out, "\n") {
				t.Errorf("output must end with newline: %q", out)
			}
			// Must be exactly one line
			lines := strings.Split(strings.TrimSuffix(out, "\n"), "\n")
			if len(lines) != 1 {
				t.Errorf("expected 1 line, got %d lines: %v", len(lines), lines)
			}
			// Must contain type
			if !strings.HasPrefix(lines[0], val.Type) {
				t.Errorf("expected output to start with type %s, got: %s", val.Type, lines[0])
			}
		})
	}
}

func TestCompactFormatterOptions(t *testing.T) {
	path := filepath.Join("..", "..", "..", "testdata", "tx_success.env.txt")
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading fixture: %v", err)
	}
	val, err := lens.Decode(strings.TrimSpace(string(content)))
	if err != nil {
		t.Fatalf("decoding fixture: %v", err)
	}

	t.Run("Separator", func(t *testing.T) {
		f := format.NewCompactFormatter(
			format.WithCompactSeparator(" | "),
		)
		var buf bytes.Buffer
		if err := f.Format(&buf, val); err != nil {
			t.Fatalf("format error: %v", err)
		}
		out := buf.String()
		if !strings.Contains(out, " | ") {
			t.Errorf("expected separator ' | ' in output: %s", out)
		}
	})

	t.Run("ExcludeType", func(t *testing.T) {
		f := format.NewCompactFormatter(
			format.WithCompactIncludeType(false),
		)
		var buf bytes.Buffer
		if err := f.Format(&buf, val); err != nil {
			t.Fatalf("format error: %v", err)
		}
		out := buf.String()
		if strings.HasPrefix(out, val.Type) {
			t.Errorf("expected output without type prefix: %s", out)
		}
	})

	t.Run("WithPalette", func(t *testing.T) {
		f := format.NewCompactFormatter(
			format.WithCompactPalette(format.ColorPalette),
		)
		var buf bytes.Buffer
		if err := f.Format(&buf, val); err != nil {
			t.Fatalf("format error: %v", err)
		}
		out := buf.String()
		if !strings.Contains(out, "\x1b[") {
			t.Errorf("expected ansi escapes with color palette: %s", out)
		}
	})

	t.Run("Aliases", func(t *testing.T) {
		f1 := format.NewSingleLineFormatter()
		var buf1 bytes.Buffer
		if err := f1.Format(&buf1, val); err != nil {
			t.Fatalf("f1 format: %v", err)
		}

		f2 := format.NewCompactSummaryFormatter()
		var buf2 bytes.Buffer
		if err := f2.Format(&buf2, val); err != nil {
			t.Fatalf("f2 format: %v", err)
		}

		if buf1.String() != buf2.String() {
			t.Errorf("expected identical output from aliases: %s vs %s", buf1.String(), buf2.String())
		}
	})
}

func TestCompactFormatterFormatSummary(t *testing.T) {
	envPath := filepath.Join("..", "..", "..", "testdata", "tx_failed.env.txt")
	resPath := filepath.Join("..", "..", "..", "testdata", "tx_failed.res.txt")
	envContent, _ := os.ReadFile(envPath)
	resContent, _ := os.ReadFile(resPath)

	envVal, err := lens.Decode(strings.TrimSpace(string(envContent)))
	if err != nil {
		t.Fatalf("decoding env: %v", err)
	}
	resVal, err := lens.Decode(strings.TrimSpace(string(resContent)))
	if err != nil {
		t.Fatalf("decoding res: %v", err)
	}

	summary, err := lens.ExplainPair(envVal, resVal)
	if err != nil {
		t.Fatalf("explain pair: %v", err)
	}

	f := format.NewCompactFormatter()
	var buf bytes.Buffer
	if err := f.FormatSummary(&buf, summary); err != nil {
		t.Fatalf("format summary: %v", err)
	}

	out := buf.String()
	if !strings.HasSuffix(out, "\n") {
		t.Errorf("output must end with newline: %q", out)
	}
	lines := strings.Split(strings.TrimSuffix(out, "\n"), "\n")
	if len(lines) != 1 {
		t.Errorf("expected 1 line, got %d", len(lines))
	}
	if !strings.Contains(out, "status=FAILED") {
		t.Errorf("expected status=FAILED in summary: %s", out)
	}
}

func TestCompactFormatterFormatNode(t *testing.T) {
	node := &lens.Node{
		Name: "Root",
		Kind: lens.KindStruct,
		Children: []*lens.Node{
			{Name: "FieldA", Kind: lens.KindScalar, Value: "hello world"},
			{Name: "FieldB", Kind: lens.KindScalar, Value: int64(42)},
			{
				Name: "Sub",
				Kind: lens.KindStruct,
				Children: []*lens.Node{
					{Name: "Item", Kind: lens.KindScalar, Value: true},
				},
			},
		},
	}

	f := format.NewCompactFormatter()
	s := f.FormatNode(node)
	if !strings.Contains(s, `FieldA="hello world"`) {
		t.Errorf("expected quoted string for FieldA: %s", s)
	}
	if !strings.Contains(s, "FieldB=42") {
		t.Errorf("expected FieldB=42: %s", s)
	}
	if !strings.Contains(s, "Sub.Item=true") {
		t.Errorf("expected Sub.Item=true: %s", s)
	}

	listNode := &lens.Node{
		Name: "Items",
		Kind: lens.KindList,
		Children: []*lens.Node{
			{Name: "[0]", Kind: lens.KindScalar, Value: "alpha"},
			{Name: "[1]", Kind: lens.KindScalar, Value: "beta"},
		},
	}
	listStr := f.FormatNode(listNode)
	if !strings.Contains(listStr, "Items_len=2") || !strings.Contains(listStr, "alpha") {
		t.Errorf("expected list formatting, got: %s", listStr)
	}
}

func TestCompactFormatterV0Envelope(t *testing.T) {
	v0Node := &lens.Node{
		TypeName: "TransactionEnvelope",
		Kind:     lens.KindStruct,
		Children: []*lens.Node{
			{
				Name: "V0",
				Kind: lens.KindStruct,
				Children: []*lens.Node{
					{
						Name: "Tx",
						Kind: lens.KindStruct,
						Children: []*lens.Node{
							{Name: "SourceAccount", Kind: lens.KindScalar, Note: "GAA..."},
							{Name: "SeqNum", Kind: lens.KindScalar, Value: int64(1)},
							{Name: "Fee", Kind: lens.KindScalar, Value: uint32(100)},
						},
					},
					{
						Name: "Signatures",
						Kind: lens.KindList,
						Children: []*lens.Node{
							{Name: "[0]", Kind: lens.KindStruct},
						},
					},
				},
			},
		},
	}
	val := &lens.Value{
		Type: "TransactionEnvelope",
		Node: v0Node,
	}
	f := format.NewCompactFormatter()
	var buf bytes.Buffer
	if err := f.Format(&buf, val); err != nil {
		t.Fatalf("format v0 error: %v", err)
	}
	out := buf.String()
	if !strings.Contains(out, "type=v0") || !strings.Contains(out, "source=GAA...") {
		t.Errorf("unexpected v0 summary: %s", out)
	}
}

func TestCompactFormatterErrors(t *testing.T) {
	f := format.NewCompactFormatter()
	if err := f.Format(&bytes.Buffer{}, nil); err == nil {
		t.Error("expected error formatting nil value")
	}
	if err := f.FormatSummary(&bytes.Buffer{}, nil); err == nil {
		t.Error("expected error formatting nil summary")
	}
	if s := f.FormatNode(nil); s != "" {
		t.Errorf("expected empty string for nil node, got: %s", s)
	}
}
