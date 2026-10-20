package lens_test

import (
	"bytes"
	"encoding/base64"
	"encoding/binary"
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/stellar/go-stellar-sdk/xdr"

	"github.com/stellar-optics/stellar-xdr-lens/pkg/lens"
)

// errorsOnRead is a reader that fails with a fixed error after delivering
// its initial bytes, which is how a failing pipe or socket surfaces.
type errorsOnRead struct {
	data []byte
	err  error
	ii   int
}

func (r *errorsOnRead) Read(p []byte) (int, error) {
	if r.ii >= len(r.data) {
		return 0, r.err
	}
	n := copy(p, r.data[r.ii:])
	r.ii += n
	return n, nil
}

// oneAtATimeReader delivers a single byte per Read, the pathological
// pipeline case, to prove the pipe does not depend on read sizes.
type oneAtATimeReader struct{ data []byte }

func (r *oneAtATimeReader) Read(p []byte) (int, error) {
	if len(r.data) == 0 {
		return 0, io.EOF
	}
	p[0] = r.data[0]
	r.data = r.data[1:]
	return 1, nil
}

func TestStdinPipeDetectsFraming(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		input string
		want  lens.StdinFraming
	}{
		{"base64 payload", "AAAAAA==", lens.StdinFramingLines},
		{"base64 with newline", "AAAAAA==\n", lens.StdinFramingLines},
		{"hex with 0x prefix", "0x00000000", lens.StdinFramingLines},
		{"leading whitespace", "\n\n\t AAAAAA==", lens.StdinFramingLines},
		{"comment first", "# a comment\nAAAAAA==\n", lens.StdinFramingLines},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			p := lens.NewStdinPipe(strings.NewReader(tc.input))
			got, err := p.Framing()
			if err != nil {
				t.Fatalf("Framing() error = %v", err)
			}
			if got != tc.want {
				t.Errorf("Framing() = %v, want %v", got, tc.want)
			}
			// The probe is idempotent and never re-reads the source.
			again, err := p.Framing()
			if err != nil || again != tc.want {
				t.Errorf("second Framing() = %v, %v, want %v, nil", again, err, tc.want)
			}
		})
	}
}

func TestStdinPipeDetectsBinaryFraming(t *testing.T) {
	t.Parallel()

	// A raw XDR value can be a legitimate binary stream opening byte, so
	// build input that cannot be text: length-prefixed framing of a value.
	b64, err := lens.SafeMarshalBase64(xdr.Uint32(7))
	if err != nil {
		t.Fatalf("MarshalBase64() error = %v", err)
	}
	rawBytes, err := decodeB64ToBytes(t, b64)
	if err != nil {
		t.Fatalf("decoding marshalled value: %v", err)
	}

	var buf bytes.Buffer
	var head [4]byte
	binary.BigEndian.PutUint32(head[:], uint32(len(rawBytes)))
	buf.Write(head[:])
	buf.Write(rawBytes)

	p := lens.NewStdinPipe(bytes.NewReader(buf.Bytes()))
	// The zero byte from the 32-bit big-endian length is not a text byte, so
	// the pipe must classify the input as binary framing.
	first, err := p.Framing()
	if err != nil {
		t.Fatalf("Framing() error = %v", err)
	}
	if first != lens.StdinFramingBinary {
		t.Errorf("Framing() = %v, want binary", first)
	}
}

func TestStdinPipeEmptyStream(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name  string
		input string
	}{
		{"empty", ""},
		{"whitespace only", "  \n\t\n"},
		{"newline only", "\n\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			p := lens.NewStdinPipe(strings.NewReader(tc.input))
			if _, err := p.Framing(); !errors.Is(err, lens.ErrEmptyInput) {
				t.Errorf("Framing() error = %v, want ErrEmptyInput", err)
			}
			if _, err := p.ReadPayload(); !errors.Is(err, lens.ErrEmptyInput) {
				t.Errorf("ReadPayload() error = %v, want ErrEmptyInput", err)
			}
			if _, err := p.Decode(); !errors.Is(err, lens.ErrEmptyInput) {
				t.Errorf("Decode() error = %v, want ErrEmptyInput", err)
			}
		})
	}
}

func TestStdinPipeSurfacesReaderErrors(t *testing.T) {
	t.Parallel()

	wantErr := errors.New("broken pipe")
	p := lens.NewStdinPipe(&errorsOnRead{data: []byte{' ', '\t'}, err: wantErr})
	_, err := p.Framing()
	if !errors.Is(err, wantErr) {
		t.Errorf("Framing() error = %v, want the underlying reader error", err)
	}
}

func TestStdinPipeDecodeRecord(t *testing.T) {
	t.Parallel()

	payload, err := readFixture("tx_success.env")
	if err != nil {
		t.Fatalf("loadFixture(%q) error = %v", "tx_success.env", err)
	}

	recorder, err := typesOf(lens.NewStdinPipe(strings.NewReader(payload)))
	if err != nil {
		t.Fatalf("typesOf() error = %v", err)
	}
	if len(recorder) != 1 || recorder[0] != "TransactionEnvelope" {
		t.Errorf("records = %v, want [TransactionEnvelope]", recorder)
	}
}

func TestStdinPipeDecodeMultiRecord(t *testing.T) {
	t.Parallel()

	payload, err := readFixture("tx_success.env")
	if err != nil {
		t.Fatalf("loadFixture() error = %v", err)
	}
	other, err := readFixture("tx_failed.res")
	if err != nil {
		t.Fatalf("loadFixture() error = %v", err)
	}

	stream := "# captured mainnet batch\n" +
		payload + "\n" +
		"\n" + // blank lines are not records
		other + "\n"
	recorder, err := typesOf(lens.NewStdinPipe(strings.NewReader(stream)))
	if err != nil {
		t.Fatalf("typesOf() error = %v", err)
	}
	if len(recorder) != 2 || recorder[0] != "TransactionEnvelope" || recorder[1] != "TransactionResult" {
		t.Errorf("records = %v, want [TransactionEnvelope TransactionResult]", recorder)
	}
}

func TestStdinPipeDecodeHexRecord(t *testing.T) {
	t.Parallel()

	b64, err := readFixture("tx_success.env")
	if err != nil {
		t.Fatalf("loadFixture() error = %v", err)
	}
	hex, err := lens.Base64ToHex(b64)
	if err != nil {
		t.Fatalf("Base64ToHex() error = %v", err)
	}

	recorder, err := typesOf(lens.NewStdinPipe(strings.NewReader(hex)))
	if err != nil {
		t.Fatalf("typesOf() error = %v", err)
	}
	if len(recorder) != 1 || recorder[0] != "TransactionEnvelope" {
		t.Errorf("records = %v, want [TransactionEnvelope]", recorder)
	}
}

func TestStdinPipeDecodeRecordByRecord(t *testing.T) {
	t.Parallel()

	a, err := lens.SafeMarshalBase64(xdr.Uint32(1))
	if err != nil {
		t.Fatalf("MarshalBase64() error = %v", err)
	}
	b, err := lens.SafeMarshalBase64(xdr.Uint32(2))
	if err != nil {
		t.Fatalf("MarshalBase64() error = %v", err)
	}

	seen := 0
	err = lens.NewStdinPipe(strings.NewReader(a + "\n" + b + "\n")).
		DecodeEach(func(rec *lens.MultiStreamRecord) error {
			seen++
			if rec.Err != nil {
				t.Errorf("record %d err = %v, want nil", rec.Index, rec.Err)
			}
			if rec.Value == nil {
				t.Fatalf("record %d has no value", rec.Index)
			}
			return nil
		})
	if err != nil {
		t.Fatalf("DecodeEach() error = %v", err)
	}
	if seen != 2 {
		t.Errorf("saw %d records, want 2", seen)
	}
}

func TestStdinPipeDecodeEachStopsAtConsumer(t *testing.T) {
	t.Parallel()

	a, err := lens.SafeMarshalBase64(xdr.Uint32(11))
	if err != nil {
		t.Fatalf("MarshalBase64() error = %v", err)
	}
	b, err := lens.SafeMarshalBase64(xdr.Uint32(22))
	if err != nil {
		t.Fatalf("MarshalBase64() error = %v", err)
	}

	stopAfterFirst := errors.New("stop after first")
	err = lens.NewStdinPipe(strings.NewReader(a + "\n" + b + "\n")).
		DecodeEach(func(rec *lens.MultiStreamRecord) error {
			return stopAfterFirst
		})
	if !errors.Is(err, stopAfterFirst) {
		t.Errorf("DecodeEach() error = %v, want the consumer's error", err)
	}
}

func TestStdinPipeDecodeBinaryFramed(t *testing.T) {
	t.Parallel()

	env, err := readFixture("tx_success.env")
	if err != nil {
		t.Fatalf("loadFixture() error = %v", err)
	}
	envBytes, err := lens.HexToBytes(safeHex(t, env))
	if err != nil {
		t.Fatalf("HexToBytes() error = %v", err)
	}

	var buf bytes.Buffer
	var head [4]byte
	binary.BigEndian.PutUint32(head[:], uint32(len(envBytes)))
	buf.Write(head[:])
	buf.Write(envBytes)

	recorder, err := typesOf(lens.NewStdinPipe(bytes.NewReader(buf.Bytes())))
	if err != nil {
		t.Fatalf("typesOf() error = %v", err)
	}
	if len(recorder) != 1 || recorder[0] != "TransactionEnvelope" {
		t.Errorf("records = %v, want [TransactionEnvelope]", recorder)
	}
}

func TestStdinPipeRoundTripsLargePayload(t *testing.T) {
	t.Parallel()

	// A stream longer than the probe limit exercises the growing path and
	// trailing-whitespace handling across many chunk boundaries.
	payload, err := readFixture("tx_success.env")
	if err != nil {
		t.Fatalf("loadFixture() error = %v", err)
	}
	stream := strings.Repeat(payload+"\n", 40)

	recorder, err := typesOf(lens.NewStdinPipe(strings.NewReader(stream)))
	if err != nil {
		t.Fatalf("typesOf() error = %v", err)
	}
	if len(recorder) != 40 {
		t.Errorf("records = %d, want 40", len(recorder))
	}
}

func TestReadStdinPayload(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"plain", "AAAAAA==", "AAAAAA=="},
		{"trimmed", "\n  AAAAAA==  \n", "AAAAAA=="},
		{"internal whitespace kept", "ff ff", "ff ff"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got, err := lens.ReadStdinPayload(strings.NewReader(tc.input))
			if err != nil {
				t.Fatalf("ReadStdinPayload() error = %v", err)
			}
			if got != tc.want {
				t.Errorf("ReadStdinPayload() = %q, want %q", got, tc.want)
			}
		})
	}

	if _, err := lens.ReadStdinPayload(strings.NewReader(" \n")); !errors.Is(err, lens.ErrEmptyInput) {
		t.Errorf("whitespace-only payload error = %v, want ErrEmptyInput", err)
	}
}

func TestReadStdinPayloadHandlesFailingReads(t *testing.T) {
	t.Parallel()

	wantErr := errors.New("disk trouble")
	if _, err := lens.ReadStdinPayload(&errorsOnRead{err: wantErr}); !errors.Is(err, wantErr) {
		t.Errorf("ReadStdinPayload() error = %v, want the underlying reader error", err)
	}
}

func TestReadStdinPayloadHandlesByteFragmentedReads(t *testing.T) {
	t.Parallel()

	payload, err := readFixture("tx_success.env")
	if err != nil {
		t.Fatalf("loadFixture() error = %v", err)
	}

	// One byte per Read must produce exactly the same payload.
	got, err := lens.ReadStdinPayload(&oneAtATimeReader{data: []byte(" " + payload + "\n")})
	if err != nil {
		t.Fatalf("ReadStdinPayload() error = %v", err)
	}
	if got != payload {
		t.Errorf("ReadStdinPayload() = %q, want %q", got, payload)
	}
}

func TestStdinPipeDecodeErrorsPropagate(t *testing.T) {
	t.Parallel()

	// A text stream whose line is not valid base64 reports the failure from
	// the multi-decoder, still matching the library sentinel. The line
	// starts with a text byte so the probe classifies it as line framing.
	p := lens.NewStdinPipe(strings.NewReader("AAAA\n"))
	err := p.DecodeEach(func(*lens.MultiStreamRecord) error { return nil })
	if !errors.Is(err, lens.ErrCorruptMultiStreamRecord) {
		t.Errorf("text DecodeEach() error = %v, want wrapped ErrCorruptMultiStreamRecord", err)
	}
	if !strings.Contains(err.Error(), "line 1") {
		t.Errorf("text DecodeEach() error = %v, want the line context", err)
	}

	// Decode surfaces the same classification through its slice return.
	if _, err := lens.NewStdinPipe(strings.NewReader("AAAA\n")).Decode(); err == nil || !errors.Is(err, lens.ErrCorruptMultiStreamRecord) {
		t.Errorf("Decode() error = %v, want wrapped ErrCorruptMultiStreamRecord", err)
	}

	// A binary stream whose chunk is not valid XDR names the chunk. Two
	// payload bytes cannot be a valid XDR record, so no type matches.
	var buf bytes.Buffer
	buf.Write([]byte{0x80, 0x00, 0x00, 0x02})
	buf.Write([]byte{0xFF, 0xFE})
	_, err = lens.NewStdinPipe(bytes.NewReader(buf.Bytes())).Decode()
	if err == nil || !errors.Is(err, lens.ErrNoMatch) || !strings.Contains(err.Error(), "chunk 0") {
		t.Errorf("binary Decode() error = %v, want a chunk-0 failure matching ErrNoMatch", err)
	}
}

func TestStdinPipeDecodeCollectsValues(t *testing.T) {
	t.Parallel()

	a, err := lens.SafeMarshalBase64(xdr.Uint32(5))
	if err != nil {
		t.Fatalf("MarshalBase64() error = %v", err)
	}
	b, err := lens.SafeMarshalBase64(xdr.Uint32(9))
	if err != nil {
		t.Fatalf("MarshalBase64() error = %v", err)
	}

	values, err := lens.NewStdinPipe(strings.NewReader(a + "\n" + b + "\n")).Decode()
	if err != nil {
		t.Fatalf("Decode() error = %v", err)
	}
	if len(values) != 2 {
		t.Fatalf("Decode() = %d values, want 2", len(values))
	}
	// Four bytes are genuinely ambiguous in XDR, so pin the count and that
	// each record produced a real decoded value rather than skipping one.
	for i, v := range values {
		if v == nil || v.Type != "Auth" {
			t.Errorf("values[%d] = %+v, want a decoded 4-byte record", i, v)
		}
	}
}

func TestStdinPipeFramingNamesAreStable(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		f    lens.StdinFraming
		want string
	}{
		{lens.StdinFramingUnknown, "unknown"},
		{lens.StdinFramingLines, "lines"},
		{lens.StdinFramingBinary, "binary"},
		{lens.StdinFraming(99), "unknown"},
	} {
		if got := tc.f.String(); got != tc.want {
			t.Errorf("String() = %q, want %q", got, tc.want)
		}
	}
}

func TestStdinPipeBufferOptionClampsShortValues(t *testing.T) {
	t.Parallel()

	// A buffer smaller than the probe limit must not break construction or
	// detection; it is clamped so detection always has room to look ahead.
	payload, err := readFixture("tx_success.env")
	if err != nil {
		t.Fatalf("loadFixture() error = %v", err)
	}
	p := lens.NewStdinPipe(strings.NewReader(payload), lens.WithStdinPipeBuffer(16))
	kind, err := p.Framing()
	if err != nil {
		t.Fatalf("Framing() error = %v", err)
	}
	if kind != lens.StdinFramingLines {
		t.Errorf("Framing() = %v, want lines", kind)
	}
	recorder, err := typesOf(p)
	if err != nil {
		t.Fatalf("typesOf() error = %v", err)
	}
	if len(recorder) != 1 || recorder[0] != "TransactionEnvelope" {
		t.Errorf("records = %v, want [TransactionEnvelope]", recorder)
	}
}

func TestStdinPipeWhitespaceFloodingIsEmpty(t *testing.T) {
	t.Parallel()

	// Whitespace beyond the probe limit is not a payload: the framing probe
	// rejects it rather than reading forever.
	flood := strings.Repeat(" ", lens.DefaultStdinPipeBuffer)
	p := lens.NewStdinPipe(strings.NewReader(flood))
	if _, err := p.Framing(); !errors.Is(err, lens.ErrEmptyInput) {
		t.Errorf("Framing() error = %v, want ErrEmptyInput", err)
	}
}

func TestStdinPipeBinaryConsumerStopIsHonoured(t *testing.T) {
	t.Parallel()

	b64, err := lens.SafeMarshalBase64(xdr.Uint32(3))
	if err != nil {
		t.Fatalf("MarshalBase64() error = %v", err)
	}
	raw, err := decodeB64ToBytes(t, b64)
	if err != nil {
		t.Fatalf("decodeB64ToBytes() error = %v", err)
	}

	var buf bytes.Buffer
	buf.Write([]byte{0x80, 0x00, 0x00, byte(len(raw))})
	buf.Write(raw)

	stop := errors.New("stop now")
	err = lens.NewStdinPipe(bytes.NewReader(buf.Bytes())).DecodeEach(func(*lens.MultiStreamRecord) error {
		return stop
	})
	if !errors.Is(err, stop) {
		t.Errorf("DecodeEach() error = %v, want the consumer's error", err)
	}
}

func TestStdinPipeCommentOnlyStreamIsEmpty(t *testing.T) {
	t.Parallel()

	// A line-framed stream that contains no records after comment filtering
	// carries nothing: Decode reports empty input rather than a nil slice.
	stream := "# nothing here\n// nothing here either\n"
	p := lens.NewStdinPipe(strings.NewReader(stream))
	kind, err := p.Framing()
	if err != nil || kind != lens.StdinFramingLines {
		t.Fatalf("Framing() = %v, %v, want lines, nil", kind, err)
	}
	if _, err := p.Decode(); !errors.Is(err, lens.ErrEmptyInput) {
		t.Errorf("Decode() error = %v, want ErrEmptyInput", err)
	}
}

func TestStdinPipeBadBinaryFramingSurfacesParserError(t *testing.T) {
	t.Parallel()

	// A length prefix beyond the parser's limit is a framing failure, not a
	// silent truncation.
	var buf bytes.Buffer
	buf.Write([]byte{0xFF, 0xFF, 0xFF, 0xFF})
	_, err := lens.NewStdinPipe(bytes.NewReader(buf.Bytes())).Decode()
	if err == nil {
		t.Fatal("Decode() error = nil, want a framing failure")
	}
	if !errors.Is(err, lens.ErrRecordTooLarge) && !errors.Is(err, lens.ErrInvalidFraming) {
		t.Errorf("Decode() error = %v, want a framing or limit error", err)
	}
}

func TestStdinPipeLargeBufferOptionIsHonoured(t *testing.T) {
	t.Parallel()

	payload, err := readFixture("tx_success.env")
	if err != nil {
		t.Fatalf("loadFixture() error = %v", err)
	}
	recorder, err := typesOf(lens.NewStdinPipe(strings.NewReader(payload), lens.WithStdinPipeBuffer(lens.DefaultStdinPipeBuffer*4)))
	if err != nil {
		t.Fatalf("typesOf() error = %v", err)
	}
	if len(recorder) != 1 || recorder[0] != "TransactionEnvelope" {
		t.Errorf("records = %v, want [TransactionEnvelope]", recorder)
	}
}

// --- helpers ---

// decodeB64ToBytes decodes a base64 string to raw bytes.
func decodeB64ToBytes(t *testing.T, base64Input string) ([]byte, error) {
	t.Helper()
	return base64.StdEncoding.DecodeString(strings.TrimSpace(base64Input))
}

// typesOf runs the pipe's full decode path and returns the record types it
// produced, given the pipe records they arrived in.
func typesOf(p *lens.StdinPipe) ([]string, error) {
	var types []string
	err := p.DecodeEach(func(rec *lens.MultiStreamRecord) error {
		if rec.Err != nil {
			return rec.Err
		}
		if rec.Value == nil {
			return errors.New("record has no value and no error")
		}
		types = append(types, rec.Value.Type)
		return nil
	})
	return types, err
}

// safeHex encodes a base64 fixture to hex for binary framing tests.
func safeHex(t *testing.T, base64Input string) string {
	t.Helper()
	hex, err := lens.Base64ToHex(base64Input)
	if err != nil {
		t.Fatalf("Base64ToHex() error = %v", err)
	}
	return "0x" + hex
}
