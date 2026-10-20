package lens

import (
	"bufio"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
)

// StdinFraming describes how a piped payload is framed, as detected from the
// leading bytes of the stream. The detection exists because a shell pipeline
// hands the tool an opaque reader: it cannot know whether the bytes are a
// base64 paste, a hexadecimal dump or a framed binary archive without
// peeking, and choosing the wrong parser either fails or buffers everything.
type StdinFraming int

const (
	// StdinFramingUnknown means the stream is empty or exhausted: nothing
	// was read during detection.
	StdinFramingUnknown StdinFraming = iota
	// StdinFramingLines is newline-delimited text: one or more base64 or
	// hexadecimal XDR payloads, tolerating blank and comment (#, //) lines.
	StdinFramingLines
	// StdinFramingBinary is framed raw XDR: RFC 5531 record marking or a
	// 4-byte length prefix, as accepted by the stream parser.
	StdinFramingBinary
)

// String returns a stable, lowercase name for the framing, suitable for
// diagnostics and test output.
func (f StdinFraming) String() string {
	switch f {
	case StdinFramingLines:
		return "lines"
	case StdinFramingBinary:
		return "binary"
	default:
		return "unknown"
	}
}

// Stdin tuning constants. The read-ahead buffer is sized for transaction
// envelopes (a mainnet envelope is a few hundred bytes; a large Soroban one
// is a few kilobytes), so a typical piped payload is read with a single
// large read instead of the 4-KiB dribble that naive io.ReadAll produces on
// pipes.
const (
	// DefaultStdinPipeBuffer is the default read-ahead size (128 KiB).
	DefaultStdinPipeBuffer = 128 * 1024
	// stdinPeekLimit bounds how many bytes of whitespace the framing probe
	// will skip before giving up; beyond that the stream is not a payload.
	stdinPeekLimit = 8 * 1024
)

// StdinPipeOption configures a StdinPipe.
type StdinPipeOption func(*StdinPipe)

// WithStdinPipeBuffer sets the read-ahead buffer size in bytes. Values below
// the framing-probe limit are clamped to it, because detection needs room to
// look ahead.
func WithStdinPipeBuffer(n int) StdinPipeOption {
	return func(p *StdinPipe) {
		if n > stdinPeekLimit {
			p.bufSize = n
		}
	}
}

// StdinPipe is a buffered reader at the front of a piped XDR payload.
//
// It exists so that `cat txs.txt | lens explain` works on a large stream the
// same way `lens explain "$XDR"` works on an argument: one probe consumes a
// handful of leading bytes to pick the parser, and every byte after that goes
// straight to the parser that was picked — the decode path, not a second
// full buffering pass. It performs no IO beyond the supplied reader, so it
// stays as offline and deterministic as the rest of the library.
type StdinPipe struct {
	rdr      *bufio.Reader
	bufSize  int
	kind     StdinFraming
	probed   bool
	probeErr error
}

// NewStdinPipe wraps r in the buffered stdin front-end.
func NewStdinPipe(r io.Reader, opts ...StdinPipeOption) *StdinPipe {
	p := &StdinPipe{
		bufSize: DefaultStdinPipeBuffer,
	}
	for _, opt := range opts {
		opt(p)
	}
	p.rdr = bufio.NewReaderSize(r, p.bufSize)
	return p
}

// stdinTextBytes is every byte that can open a text payload: base64
// standard and URL alphabets, hex digits, the 0x prefix, and the comment
// prefixes (#, //) that a line-framed stream may open with.
var stdinTextBytes = func() [256]bool {
	var ok [256]bool
	for _, r := range "0123456789" +
		"ABCDEFGHIJKLMNOPQRSTUVWXYZ" +
		"abcdefghijklmnopqrstuvwxyz" +
		"+/=_-" +
		"#" {
		ok[r] = true
	}
	return ok
}()

// Framing probes the leading bytes of the stream and reports how the
// remaining input is framed. The probe is idempotent: the first call reads
// and buffers the probe bytes; later calls report the same answer without
// touching the reader again, so nothing decoded later loses its place.
//
// An empty stream reports StdinFramingUnknown. Reader failures during the
// probe surface as they were raised from the underlying reader.
func (p *StdinPipe) Framing() (StdinFraming, error) {
	if !p.probed {
		p.probed = true
		p.kind, p.probeErr = p.probe()
	}
	if p.kind != StdinFramingUnknown {
		return p.kind, nil
	}
	// An empty-or-all-whitespace stream is empty input; a surviving reader
	// error from the probe is the real cause and is surfaced verbatim.
	if p.probeErr != nil && !errors.Is(p.probeErr, ErrEmptyInput) {
		return p.kind, p.probeErr
	}
	return p.kind, ErrEmptyInput
}

// probe consumes whitespace up to stdinPeekLimit and classifies the first
// payload byte. Reading (not just peeking) is deliberate: the bytes we
// therefore keep buffered can be handed straight to the downstream parser.
func (p *StdinPipe) probe() (StdinFraming, error) {
	skipped := 0
	for {
		n, err := p.rdr.Peek(1)
		if errors.Is(err, io.EOF) {
			return StdinFramingUnknown, ErrEmptyInput
		}
		if err != nil {
			// Peek's ErrBufferFull-style failures cannot occur at this
			// width (the buffer is always larger than the probe), so any
			// other error is from the underlying reader.
			return StdinFramingUnknown, err
		}
		b := n[0]
		switch {
		case b == ' ' || b == '\t' || b == '\r' || b == '\n':
			skipped++
			if skipped >= stdinPeekLimit {
				return StdinFramingUnknown, ErrEmptyInput
			}
			// The byte is buffered after a successful Peek, so the discard
			// only fails on a reader error, which Peek catches on the next
			// iteration; the check exists for completeness, as the branch
			// is defensive.
			if _, derr := p.rdr.Discard(1); derr != nil {
				return StdinFramingUnknown, derr
			}
		case stdinTextBytes[b]:
			return StdinFramingLines, nil
		default:
			return StdinFramingBinary, nil
		}
	}
}

// ReadPayload consumes the whole remaining stream into a text payload with a
// single trimmed copy, which is what a one-value command needs. The empty
// case is ErrEmptyInput, not a silent success.
func (p *StdinPipe) ReadPayload() (string, error) {
	b, err := readTrimmed(p.rdr)
	if err != nil {
		return "", err
	}
	if len(b) == 0 {
		return "", ErrEmptyInput
	}
	return b, nil
}

// ReadStdinPayload reads a whole piped payload as text in one pass and is the
// helper behind the CLI's argument/--file/stdin resolution. It is the
// buffered equivalent of `strings.TrimSpace(string(io.ReadAll(stdin)))` with a
// pre-sized read-ahead so typical pipelines (an envelope, a batch of one)
// avoid the doubling-growth copies of io.ReadAll's 512-byte bootstrap.
func ReadStdinPayload(r io.Reader) (string, error) {
	return NewStdinPipe(r).ReadPayload()
}

// Decode consumes the piped stream and returns every XDR record it can
// decode.
//
// Text framing is decoded line by line (base64 or hex, blank lines and
// comment prefixes ignored, the same rules `decode --multi` applies); binary
// framing is handed to the stream parser with automatic framing detection,
// which recognises RFC 5531 record marking and 4-byte length prefixes.
//
// All decoding runs locally: the pipe surfaces whatever decode error the
// underlying format produces, never inventing values for records it cannot
// read. An empty stream is ErrEmptyInput.
func (p *StdinPipe) Decode() ([]*Value, error) {
	var out []*Value
	err := p.DecodeEach(func(rec *MultiStreamRecord) error {
		if rec.Value != nil {
			out = append(out, rec.Value)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	if out == nil {
		return nil, ErrEmptyInput
	}
	return out, nil
}

// DecodeEach streams every decoded record through fn as it is read, instead
// of accumulating the whole input first. It is the "cat big.txt |" path: a
// consumer may aggregate, filter or emit without holding every record.
//
// fn returning an error stops the stream and returns that error wrapped with
// the record's line or chunk position; io.EOF at the end of input is
// translated into a clean stop.
func (p *StdinPipe) DecodeEach(fn func(rec *MultiStreamRecord) error) error {
	kind, err := p.Framing()
	if err != nil {
		return err
	}

	switch kind {
	case StdinFramingBinary:
		parser := NewStreamParser(p.rdr)
		for {
			raw, err := parser.Next()
			if errors.Is(err, io.EOF) {
				return nil
			}
			if err != nil {
				return err
			}
			chunk := parser.ChunkIndex() - 1
			v, err := decodeRawValue(raw)
			if err != nil {
				return fmt.Errorf("chunk %d: %w", chunk, err)
			}
			rec := &MultiStreamRecord{Index: int(chunk), RawLine: "", Value: v}
			if err := fn(rec); err != nil {
				return err
			}
		}

	case StdinFramingLines:
		decoder := NewMultiStreamDecoder(p.rdr)
		for {
			rec, err := decoder.Next()
			if errors.Is(err, io.EOF) {
				return nil
			}
			if err != nil {
				return err
			}
			if err := fn(rec); err != nil {
				return err
			}
		}
	}
	return nil
}

// decodeRawValue decodes already-framed raw XDR bytes through the detection
// path, base64 internally, so callers get the same enrichment a pasted
// payload would.
func decodeRawValue(raw []byte) (*Value, error) {
	return Decode(base64.StdEncoding.EncodeToString(raw))
}

// readTrimmed reads everything from r, strips surrounding whitespace, and
// returns the remaining bytes as text. The payload itself is never validated
// here: decoding belongs to the decode path, so a caller can read binary
// framing markers or hex digits through the same front-end.
func readTrimmed(r io.Reader) (string, error) {
	var start, end int
	buf := make([]byte, 0, DefaultStdinPipeBuffer)
	tmp := make([]byte, 32*1024)
	for {
		n, err := r.Read(tmp)
		if n > 0 {
			buf = append(buf, tmp[:n]...)
		}
		if err != nil && !errors.Is(err, io.EOF) {
			return "", err
		}
		if n == 0 && errors.Is(err, io.EOF) {
			break
		}
	}
	start, end = 0, len(buf)
	for start < end && isSpaceByte(buf[start]) {
		start++
	}
	for end > start && isSpaceByte(buf[end-1]) {
		end--
	}
	return string(buf[start:end]), nil
}

// isSpaceByte covers ASCII whitespace, which is the only whitespace a
// base64, hex or TxRep-style payload carries between records.
func isSpaceByte(b byte) bool {
	return b == ' ' || b == '\t' || b == '\n' || b == '\r' || b == '\v' || b == '\f'
}
