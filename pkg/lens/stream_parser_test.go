package lens

import (
	"bytes"
	"encoding/base64"
	"errors"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/stellar/go-stellar-sdk/xdr"
)

func TestStreamFramingString(t *testing.T) {
	cases := []struct {
		f    StreamFraming
		want string
	}{
		{FramingAuto, "auto"},
		{FramingRecordMarking, "record-marking (RFC 5531)"},
		{FramingLengthPrefixed, "length-prefixed"},
		{FramingBase64Lines, "base64-lines"},
		{StreamFraming(99), "unknown"},
	}
	for _, tc := range cases {
		if got := tc.f.String(); got != tc.want {
			t.Errorf("framing %d: got %q, want %q", tc.f, got, tc.want)
		}
	}
}

func TestStreamParserRecordMarking(t *testing.T) {
	rec1 := []byte("first record payload")
	rec2 := []byte("second larger payload to test stream parsing across chunks")

	var buf bytes.Buffer
	// Write rec1 as single fragment
	if err := WriteRecordMarking(&buf, rec1, 0); err != nil {
		t.Fatalf("failed WriteRecordMarking: %v", err)
	}
	// Write rec2 split into 10-byte fragments
	if err := WriteRecordMarking(&buf, rec2, 10); err != nil {
		t.Fatalf("failed WriteRecordMarking chunked: %v", err)
	}

	p := NewStreamParser(&buf, WithFraming(FramingRecordMarking), WithBufferSize(1024))
	if p.Framing() != FramingRecordMarking {
		t.Errorf("expected FramingRecordMarking, got %v", p.Framing())
	}

	// Read rec1
	got1, err := p.Next()
	if err != nil || !bytes.Equal(got1, rec1) {
		t.Fatalf("rec1 mismatch: got %q, err %v", got1, err)
	}
	if p.ChunkIndex() != 1 {
		t.Errorf("expected ChunkIndex 1, got %d", p.ChunkIndex())
	}

	// Read rec2
	got2, err := p.Next()
	if err != nil || !bytes.Equal(got2, rec2) {
		t.Fatalf("rec2 mismatch: got %q, err %v", got2, err)
	}
	if p.ChunkIndex() != 2 {
		t.Errorf("expected ChunkIndex 2, got %d", p.ChunkIndex())
	}

	// Read EOF
	_, err = p.Next()
	if !errors.Is(err, io.EOF) {
		t.Errorf("expected io.EOF, got %v", err)
	}

	// Read after EOF -> ErrStreamClosed
	_, err = p.Next()
	if !errors.Is(err, ErrStreamClosed) {
		t.Errorf("expected ErrStreamClosed, got %v", err)
	}

	if p.BytesRead() == 0 {
		t.Errorf("expected non-zero BytesRead")
	}
}

func TestStreamParserLengthPrefixed(t *testing.T) {
	rec1 := []byte("alpha payload")
	rec2 := []byte("beta payload")

	var buf bytes.Buffer
	if err := WriteLengthPrefixed(&buf, rec1); err != nil {
		t.Fatalf("failed WriteLengthPrefixed: %v", err)
	}
	if err := WriteLengthPrefixed(&buf, rec2); err != nil {
		t.Fatalf("failed WriteLengthPrefixed: %v", err)
	}

	p := NewStreamParser(&buf, WithFraming(FramingLengthPrefixed))

	got1, err := p.Next()
	if err != nil || !bytes.Equal(got1, rec1) {
		t.Fatalf("got1 mismatch: %q, err: %v", got1, err)
	}
	got2, err := p.Next()
	if err != nil || !bytes.Equal(got2, rec2) {
		t.Fatalf("got2 mismatch: %q, err: %v", got2, err)
	}
	_, err = p.Next()
	if !errors.Is(err, io.EOF) {
		t.Errorf("expected io.EOF, got %v", err)
	}
}

func TestStreamParserBase64Lines(t *testing.T) {
	rec1 := []byte("line 1 binary")
	rec2 := []byte("line 2 binary")

	var buf bytes.Buffer
	if err := WriteBase64Line(&buf, rec1); err != nil {
		t.Fatalf("failed WriteBase64Line: %v", err)
	}
	buf.WriteString("\n\n") // empty lines should be ignored
	if err := WriteBase64Line(&buf, rec2); err != nil {
		t.Fatalf("failed WriteBase64Line: %v", err)
	}

	p := NewStreamParser(&buf, WithFraming(FramingBase64Lines))

	got1, err := p.Next()
	if err != nil || !bytes.Equal(got1, rec1) {
		t.Fatalf("got1 mismatch: %q, err: %v", got1, err)
	}
	got2, err := p.Next()
	if err != nil || !bytes.Equal(got2, rec2) {
		t.Fatalf("got2 mismatch: %q, err: %v", got2, err)
	}
	_, err = p.Next()
	if !errors.Is(err, io.EOF) {
		t.Errorf("expected io.EOF, got %v", err)
	}
}

func TestStreamParserAutoDetect(t *testing.T) {
	rec := []byte("test auto detect record")

	// 1. Auto-detect base64 lines
	var b64Buf bytes.Buffer
	_ = WriteBase64Line(&b64Buf, rec)
	pB64 := NewStreamParser(&b64Buf, WithFraming(FramingAuto))
	gotB64, err := pB64.Next()
	if err != nil || !bytes.Equal(gotB64, rec) {
		t.Fatalf("failed auto-detect base64: %v", err)
	}
	if pB64.Framing() != FramingBase64Lines {
		t.Errorf("expected FramingBase64Lines, got %v", pB64.Framing())
	}

	// 2. Auto-detect RFC 5531 Record Marking
	var rmBuf bytes.Buffer
	_ = WriteRecordMarking(&rmBuf, rec, 0)
	pRM := NewStreamParser(&rmBuf, WithFraming(FramingAuto))
	gotRM, err := pRM.Next()
	if err != nil || !bytes.Equal(gotRM, rec) {
		t.Fatalf("failed auto-detect record marking: %v", err)
	}
	if pRM.Framing() != FramingRecordMarking {
		t.Errorf("expected FramingRecordMarking, got %v", pRM.Framing())
	}

	// 3. Auto-detect Length Prefixed
	var lpBuf bytes.Buffer
	_ = WriteLengthPrefixed(&lpBuf, rec)
	pLP := NewStreamParser(&lpBuf, WithFraming(FramingAuto))
	gotLP, err := pLP.Next()
	if err != nil || !bytes.Equal(gotLP, rec) {
		t.Fatalf("failed auto-detect length prefixed: %v", err)
	}
	if pLP.Framing() != FramingLengthPrefixed {
		t.Errorf("expected FramingLengthPrefixed, got %v", pLP.Framing())
	}
}

func TestStreamParserNextNodeAndNextInto(t *testing.T) {
	// Read real fixture testdata/tx_success.env.txt
	rawB64, err := os.ReadFile("../../testdata/tx_success.env.txt")
	if err != nil {
		rawB64, err = os.ReadFile("testdata/tx_success.env.txt")
	}
	if err != nil {
		rawB64, err = os.ReadFile("../testdata/tx_success.env.txt")
	}
	if err != nil {
		t.Fatalf("failed reading fixture: %v", err)
	}

	rawBytes, err := base64.StdEncoding.DecodeString(strings.TrimSpace(string(rawB64)))
	if err != nil {
		t.Fatalf("failed decoding fixture base64: %v", err)
	}

	// Stream with length-prefixed framing
	var buf bytes.Buffer
	_ = WriteLengthPrefixed(&buf, rawBytes)

	p := NewStreamParser(&buf, WithFraming(FramingLengthPrefixed))
	node, err := p.NextNode()
	if err != nil || node == nil {
		t.Fatalf("failed NextNode: %v", err)
	}
	if node.TypeName != "TransactionEnvelope" {
		t.Errorf("expected type TransactionEnvelope, got %s", node.TypeName)
	}

	// Test NextInto
	var buf2 bytes.Buffer
	_ = WriteLengthPrefixed(&buf2, rawBytes)
	p2 := NewStreamParser(&buf2, WithFraming(FramingLengthPrefixed))
	var env xdr.TransactionEnvelope
	if err := p2.NextInto(&env); err != nil {
		t.Fatalf("failed NextInto: %v", err)
	}
	if env.Type != xdr.EnvelopeTypeEnvelopeTypeTx {
		t.Errorf("expected EnvelopeTypeTx, got %v", env.Type)
	}
}

func TestStreamParserDrain(t *testing.T) {
	var buf bytes.Buffer
	for i := 0; i < 5; i++ {
		_ = WriteLengthPrefixed(&buf, []byte("item"))
	}

	p := NewStreamParser(&buf, WithFraming(FramingLengthPrefixed))
	count, err := p.Drain()
	if err != nil || count != 5 {
		t.Errorf("expected count 5, got %d, err: %v", count, err)
	}
}

func TestStreamParserErrors(t *testing.T) {
	// 1. Record too large (Record Marking)
	var rmBuf bytes.Buffer
	_ = WriteRecordMarking(&rmBuf, make([]byte, 100), 0)
	pRM := NewStreamParser(&rmBuf, WithFraming(FramingRecordMarking), WithMaxRecordSize(50))
	if _, err := pRM.Next(); !errors.Is(err, ErrRecordTooLarge) {
		t.Errorf("expected ErrRecordTooLarge, got %v", err)
	}

	// 2. Record too large (Length Prefixed)
	var lpBuf bytes.Buffer
	_ = WriteLengthPrefixed(&lpBuf, make([]byte, 100))
	pLP := NewStreamParser(&lpBuf, WithFraming(FramingLengthPrefixed), WithMaxRecordSize(50))
	if _, err := pLP.Next(); !errors.Is(err, ErrRecordTooLarge) {
		t.Errorf("expected ErrRecordTooLarge, got %v", err)
	}

	// 3. Record too large (Base64)
	var b64Buf bytes.Buffer
	_ = WriteBase64Line(&b64Buf, make([]byte, 100))
	pB64 := NewStreamParser(&b64Buf, WithFraming(FramingBase64Lines), WithMaxRecordSize(50))
	if _, err := pB64.Next(); !errors.Is(err, ErrRecordTooLarge) {
		t.Errorf("expected ErrRecordTooLarge, got %v", err)
	}

	// 4. Premature EOF (Record Marking header truncated)
	pTrunc := NewStreamParser(bytes.NewReader([]byte{0x80, 0x00}))
	if _, err := pTrunc.Next(); !errors.Is(err, ErrPrematureEOF) {
		t.Errorf("expected ErrPrematureEOF on truncated header, got %v", err)
	}

	// 5. Premature EOF (Record Marking body truncated)
	var truncBody bytes.Buffer
	_ = WriteRecordMarking(&truncBody, []byte("1234567890"), 0)
	pTruncBody := NewStreamParser(bytes.NewReader(truncBody.Bytes()[:6]), WithFraming(FramingRecordMarking))
	if _, err := pTruncBody.Next(); !errors.Is(err, ErrPrematureEOF) {
		t.Errorf("expected ErrPrematureEOF on truncated body, got %v", err)
	}

	// 6. Premature EOF (Length Prefixed body truncated)
	var truncLP bytes.Buffer
	_ = WriteLengthPrefixed(&truncLP, []byte("1234567890"))
	pTruncLP := NewStreamParser(bytes.NewReader(truncLP.Bytes()[:6]), WithFraming(FramingLengthPrefixed))
	if _, err := pTruncLP.Next(); !errors.Is(err, ErrPrematureEOF) {
		t.Errorf("expected ErrPrematureEOF on truncated LP, got %v", err)
	}

	// 7. Malformed base64 line
	pBadB64 := NewStreamParser(strings.NewReader("not_valid_base64!!!\n"), WithFraming(FramingBase64Lines))
	if _, err := pBadB64.Next(); !errors.Is(err, ErrInvalidFraming) {
		t.Errorf("expected ErrInvalidFraming on bad base64, got %v", err)
	}

	// 8. WriteRecordMarking empty record
	var emptyBuf bytes.Buffer
	if err := WriteRecordMarking(&emptyBuf, nil, 0); err != nil {
		t.Errorf("expected success for empty WriteRecordMarking: %v", err)
	}
	pEmpty := NewStreamParser(&emptyBuf, WithFraming(FramingRecordMarking))
	rec, err := pEmpty.Next()
	if err != nil || len(rec) != 0 {
		t.Errorf("failed reading empty record marking: rec=%v, err=%v", rec, err)
	}
}
