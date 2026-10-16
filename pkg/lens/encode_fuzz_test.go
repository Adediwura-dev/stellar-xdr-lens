package lens

import (
	"bytes"
	"encoding/base64"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// RunStreamParserFuzz executes the stream parser against arbitrary byte input without panicking.
func RunStreamParserFuzz(data []byte) error {
	defer func() {
		if r := recover(); r != nil {
			panic(r)
		}
	}()

	p := NewStreamParser(bytes.NewReader(data), WithMaxRecordSize(1024*1024))
	_, err := p.Drain()
	return err
}

// RunCanonicalFuzz executes canonical validation and canonicalization against arbitrary byte input without panicking.
func RunCanonicalFuzz(data []byte) {
	defer func() {
		if r := recover(); r != nil {
			panic(r)
		}
	}()

	_ = ValidateCanonicalXDR(data)
	_ = IsCanonicalXDR(data)
	_, _ = CanonicalizeXDR(data)
}

// RunDiagnosticFuzz executes JSON error diagnostics against arbitrary byte input without panicking.
func RunDiagnosticFuzz(data []byte, path string) {
	defer func() {
		if r := recover(); r != nil {
			panic(r)
		}
	}()

	_, _ = FindJSONPathLocation(data, path)
	_ = FormatDiagnostic(data, errors.New("fuzz test error"))
	_ = WrapWithDiagnostics(data, errors.New("fuzz test error"))
	_, _ = ReadAndValidateJSON(bytes.NewReader(data))
}

// FuzzStreamParser fuzzes the stream parser with arbitrary byte sequences.
func FuzzStreamParser(f *testing.F) {
	// Seed with valid and invalid framing structures
	f.Add([]byte{})
	f.Add([]byte{0x80, 0x00, 0x00, 0x04, 0x01, 0x02, 0x03, 0x04})
	f.Add([]byte{0x00, 0x00, 0x00, 0x04, 0x01, 0x02, 0x03, 0x04})
	f.Add([]byte("AAAAAA==\n"))
	f.Add([]byte("invalid base64!!!\n"))
	f.Add([]byte{0xFF, 0xFF, 0xFF, 0xFF})

	// Add testdata fixture seed
	if fixture, err := loadFuzzFixture("tx_success.env.txt"); err == nil {
		var buf bytes.Buffer
		_ = WriteRecordMarking(&buf, fixture, 0)
		f.Add(buf.Bytes())
	}

	f.Fuzz(func(t *testing.T, data []byte) {
		_ = RunStreamParserFuzz(data)
	})
}

// FuzzCanonicalValidation fuzzes the canonical XDR validation engine.
func FuzzCanonicalValidation(f *testing.F) {
	f.Add([]byte{})
	f.Add([]byte{0x00, 0x00, 0x00, 0x01, 'A', 0x00, 0x00, 0x00})
	f.Add([]byte{0x00, 0x00, 0x00, 0x01, 'A', 0x00, 0xFF, 0x00})
	f.Add([]byte{0x01, 0x02, 0x03}) // misaligned

	if fixture, err := loadFuzzFixture("tx_success.env.txt"); err == nil {
		f.Add(fixture)
	}

	f.Fuzz(func(t *testing.T, data []byte) {
		RunCanonicalFuzz(data)
	})
}

// FuzzDiagnosticLocator fuzzes the JSON diagnostic engine and path locator.
func FuzzDiagnosticLocator(f *testing.F) {
	f.Add([]byte(`{"key": "value"}`), "key")
	f.Add([]byte(`{"arr": [{"sub": 10}]}`), "arr[0].sub")
	f.Add([]byte(`{malformed json`), "path")
	f.Add([]byte(``), "")

	f.Fuzz(func(t *testing.T, data []byte, path string) {
		RunDiagnosticFuzz(data, path)
	})
}

func loadFuzzFixture(filename string) ([]byte, error) {
	paths := []string{
		filepath.Join("../../testdata", filename),
		filepath.Join("../testdata", filename),
		filepath.Join("testdata", filename),
	}
	for _, p := range paths {
		if b, err := os.ReadFile(p); err == nil {
			trimmed := strings.TrimSpace(string(b))
			return base64.StdEncoding.DecodeString(trimmed)
		}
	}
	return nil, os.ErrNotExist
}

// TestFuzzHarnessNeverPanics tests all fuzz targets with representative malformed inputs
// to verify safe degradation and high test coverage.
func TestFuzzHarnessNeverPanics(t *testing.T) {
	// 1. Stream parser inputs
	streamCorpus := [][]byte{
		{},
		{0x80, 0x00},
		{0x80, 0x00, 0x00, 0x05, 'A', 'B', 'C'}, // truncated
		{0x00, 0x00, 0x00, 0x00},
		[]byte("AAAA\nBBBB\n"),
		[]byte("not-base-64\n"),
		make([]byte, 1024),
	}
	for i, input := range streamCorpus {
		err := RunStreamParserFuzz(input)
		t.Logf("stream corpus [%d]: err=%v", i, err)
	}

	// 2. Canonical validation inputs
	canonicalCorpus := [][]byte{
		{},
		{0x01},
		{0x01, 0x02, 0x03, 0x04},
		{0x00, 0x00, 0x00, 0x01, 'A', 0x00, 0x00, 0x00},
		{0x00, 0x00, 0x00, 0x01, 'A', 0xFF, 0x00, 0x00},
		make([]byte, 256),
	}
	for _, input := range canonicalCorpus {
		RunCanonicalFuzz(input)
	}

	// 3. Diagnostic inputs
	diagCorpus := []struct {
		json []byte
		path string
	}{
		{[]byte(`{}`), ""},
		{[]byte(`{"a": 1}`), "a"},
		{[]byte(`{"a": [1, 2]}`), "a[1]"},
		{[]byte(`{"unclosed": `), "unclosed"},
		{[]byte(`invalid`), "missing"},
		{nil, "empty"},
	}
	for _, tc := range diagCorpus {
		RunDiagnosticFuzz(tc.json, tc.path)
	}

	// 4. Test with canonical fixture
	if fixture, err := loadFuzzFixture("tx_success.env.txt"); err == nil {
		RunCanonicalFuzz(fixture)
		var buf bytes.Buffer
		_ = WriteRecordMarking(&buf, fixture, 256)
		_ = RunStreamParserFuzz(buf.Bytes())
	}
}
