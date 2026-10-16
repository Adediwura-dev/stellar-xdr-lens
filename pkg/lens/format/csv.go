package format

import (
	"encoding/csv"
	"fmt"
	"io"
	"slices"
	"strconv"
	"strings"

	"github.com/stellar-optics/stellar-xdr-lens/pkg/lens"
)

// CSVFormatter renders decoded XDR as flat, machine-readable CSV: one header
// line naming the columns, then one row per value carrying the high-level
// fields a batch export needs — who sent it, what it cost, what it did, and
// whether it succeeded.
//
// The schema is fixed (see CSVColumns), so rows produced from thousands of
// transactions can be concatenated and still parse: a column that does not
// apply to a row — the fee bid of a bare TransactionResult, say — is emitted
// as an empty field rather than omitted, so every row has exactly the same
// number of fields.
//
// TransactionEnvelope and TransactionResult values are exported field by
// field. Any other registered type falls back to its XDR type name plus a
// flattened "detail" column of key=value pairs, so no input fails an export.
//
// Output is deterministic and strictly offline: the formatter only reads the
// value it is handed and writes to the supplied writer. Nothing else — no
// network, no clock, no environment — influences the bytes.
type CSVFormatter struct {
	header bool
	comma  rune
}

// CSVOption configures a CSVFormatter.
type CSVOption func(*CSVFormatter)

// WithCSVHeader controls whether the column-name row is emitted before the
// data rows. It defaults to true. Turn it off when appending to a file that
// already carries a header.
func WithCSVHeader(on bool) CSVOption {
	return func(c *CSVFormatter) { c.header = on }
}

// WithCSVDelimiter sets the field delimiter, which defaults to a comma. The
// delimiter must not be a quote, a carriage return or a line feed, because
// RFC 4180 CSV reserves those; an invalid delimiter is reported as an error
// when the first row is written.
func WithCSVDelimiter(r rune) CSVOption {
	return func(c *CSVFormatter) { c.comma = r }
}

// NewCSVFormatter returns a CSVFormatter that emits a header row before its
// data rows.
func NewCSVFormatter(opts ...CSVOption) *CSVFormatter {
	c := &CSVFormatter{header: true, comma: ','}
	for _, o := range opts {
		o(c)
	}
	return c
}

// CSVColumns returns the fixed output schema, in emit order. It is a copy, so
// callers may keep it. Column names are part of the public interface —
// scripts address them positionally and by name — so renaming one is a
// breaking change.
func CSVColumns() []string {
	return slices.Clone(csvColumns)
}

// csvColumns is the fixed CSV schema, in emit order.
//
//	type              registered XDR type name, or the Summary kind for rows
//	                  exported from a lens.Summary
//	variant           envelope shape: v0, v1 or fee_bump
//	source            strkey source account (inner source for fee bumps)
//	fee_bump_source   strkey fee-bump account, empty unless fee-bumped
//	seq_num           sequence number
//	fee               fee bid, in stroops
//	memo              memo rendered as "text: …", "id: …", "hash: …"
//	signatures        number of signatures
//	operation_count   number of operations, or of operation results
//	operation_types   de-duplicated operation types joined with "|"
//	fee_charged       fee actually charged, in stroops; set with a result
//	success           true/false; set with a result
//	result_code       short transaction result code, e.g. tx_failed
//	inner_result_code inner code of a fee bump, e.g. tx_failed
//	failed_operations indexes of failed operations joined with "|"
//	detail            flattened key=value pairs, non-transaction types only
var csvColumns = []string{
	"type",
	"variant",
	"source",
	"fee_bump_source",
	"seq_num",
	"fee",
	"memo",
	"signatures",
	"operation_count",
	"operation_types",
	"fee_charged",
	"success",
	"result_code",
	"inner_result_code",
	"failed_operations",
	"detail",
}

// Column indexes into a record. They are kept in lockstep with csvColumns and
// the relationship is pinned by a test.
const (
	colType = iota
	colVariant
	colSource
	colFeeBumpSource
	colSeqNum
	colFee
	colMemo
	colSignatures
	colOperationCount
	colOperationTypes
	colFeeCharged
	colSuccess
	colResultCode
	colInnerResultCode
	colFailedOperations
	colDetail
)

// Format writes the header row (unless disabled) followed by one data row for
// v, satisfying the Formatter interface.
//
// The header is written on every call, because a single Format call is the
// common case (one value in, one self-describing CSV document out). Use
// FormatAll when exporting a batch, so the header is written once.
func (c *CSVFormatter) Format(w io.Writer, v *lens.Value) error {
	rec, err := valueRecord(v)
	if err != nil {
		return err
	}
	return c.write(w, [][]string{rec})
}

// FormatAll writes the header row (unless disabled) followed by one data row
// per value. This is the batch entry point: it is how one header covers
// thousands of transactions.
//
// Every row is produced before anything is written, so a failure part-way
// through leaves w untouched rather than truncated mid-export.
func (c *CSVFormatter) FormatAll(w io.Writer, values []*lens.Value) error {
	records := make([][]string, 0, len(values))
	for i, v := range values {
		rec, err := valueRecord(v)
		if err != nil {
			return fmt.Errorf("format: value %d: %w", i, err)
		}
		records = append(records, rec)
	}
	return c.write(w, records)
}

// FormatSummary writes the header row (unless disabled) followed by one data
// row for s.
//
// This is the paired form: a Summary built by lens.ExplainPair carries the
// outcome of the envelope and its result together, which is what populates
// success, result_code and failed_operations from a single transaction.
func (c *CSVFormatter) FormatSummary(w io.Writer, s *lens.Summary) error {
	rec, err := summaryRecord(s)
	if err != nil {
		return err
	}
	return c.write(w, [][]string{rec})
}

// FormatSummaries writes the header row (unless disabled) followed by one
// data row per summary, for batch exports of paired transactions.
func (c *CSVFormatter) FormatSummaries(w io.Writer, summaries []*lens.Summary) error {
	records := make([][]string, 0, len(summaries))
	for i, s := range summaries {
		rec, err := summaryRecord(s)
		if err != nil {
			return fmt.Errorf("format: summary %d: %w", i, err)
		}
		records = append(records, rec)
	}
	return c.write(w, records)
}

// write emits the header (when enabled) and the records as RFC 4180 CSV.
//
// Records are handed to encoding/csv rather than joined by hand, so a memo
// containing the delimiter, a quote or a newline is quoted correctly and the
// result round-trips through any standard CSV reader.
func (c *CSVFormatter) write(w io.Writer, records [][]string) error {
	cw := csv.NewWriter(w)
	cw.Comma = c.comma
	if c.header {
		if err := cw.Write(csvColumns); err != nil {
			return fmt.Errorf("format: write csv header: %w", err)
		}
	}
	for _, rec := range records {
		if err := cw.Write(rec); err != nil {
			return fmt.Errorf("format: write csv row: %w", err)
		}
	}
	// csv.Writer buffers; a short write only surfaces on Flush, so the
	// buffered error must be checked or a full disk would look like success.
	cw.Flush()
	if err := cw.Error(); err != nil {
		return fmt.Errorf("format: flush csv: %w", err)
	}
	return nil
}

// valueRecord builds the record for a decoded value.
//
// Transaction values go through lens.Explain, which is what turns the raw
// tree into the high-level fields this format exists to export — short result
// codes, short operation names, the outcome. Any other type is reported
// honestly as its type name plus a flattened detail column, because a CSV
// row for a bucket entry is not a transaction row and should not pretend to
// be one.
func valueRecord(v *lens.Value) ([]string, error) {
	if v == nil || v.Node == nil {
		return nil, fmt.Errorf("format: nil value")
	}
	rec := make([]string, len(csvColumns))
	rec[colType] = v.Type

	s, err := lens.Explain(v)
	if err != nil {
		if isTransactionType(v.Type) {
			// A transaction that cannot be summarised is a real failure,
			// not something to paper over with a half-filled row.
			return nil, fmt.Errorf("format: explain %s: %w", v.Type, err)
		}
		rec[colDetail] = NewCompactFormatter().FormatNode(v.Node)
		return rec, nil
	}

	rec[colVariant] = envelopeVariant(v.Node)
	fillTransaction(rec, s)
	return rec, nil
}

// summaryRecord builds the record for a lens.Summary. There is no node tree
// behind a summary, so variant is inferred from the fee-bump source and the
// type column carries the summary kind.
func summaryRecord(s *lens.Summary) ([]string, error) {
	if s == nil {
		return nil, fmt.Errorf("format: nil summary")
	}
	rec := make([]string, len(csvColumns))
	rec[colType] = s.Kind
	if s.FeeBumpSource != "" {
		rec[colVariant] = "fee_bump"
	}
	fillTransaction(rec, s)
	return rec, nil
}

// fillTransaction copies the transaction-level fields of a Summary into rec.
//
// Fields that only an envelope can know — sequence, fee bid, signatures — are
// left empty for a bare result, and fields that only a result can know are
// left empty until one was supplied. That is what makes the rows of mixed
// batches line up column for column.
func fillTransaction(rec []string, s *lens.Summary) {
	if s.Kind != "transaction_result" {
		rec[colSeqNum] = strconv.FormatInt(s.SeqNum, 10)
		rec[colFee] = strconv.FormatInt(s.Fee, 10)
		rec[colSignatures] = strconv.Itoa(s.SignatureCt)
	}
	rec[colSource] = s.Source
	rec[colFeeBumpSource] = s.FeeBumpSource
	rec[colMemo] = s.Memo
	rec[colOperationCount] = strconv.Itoa(len(s.Operations))
	rec[colOperationTypes] = joinOperationTypes(s.Operations)

	if s.Outcome == nil {
		return
	}
	rec[colFeeCharged] = strconv.FormatInt(s.FeeCharged, 10)
	rec[colSuccess] = strconv.FormatBool(s.Outcome.Success)
	rec[colResultCode] = s.Outcome.Reason.Code
	if inner := s.Outcome.InnerReason; inner != nil {
		rec[colInnerResultCode] = inner.Code
	}
	if len(s.Outcome.FailedOps) > 0 {
		idx := make([]string, 0, len(s.Outcome.FailedOps))
		for _, i := range s.Outcome.FailedOps {
			idx = append(idx, strconv.Itoa(i))
		}
		rec[colFailedOperations] = strings.Join(idx, "|")
	}
}

// envelopeVariant names the envelope shape from the node tree. It returns an
// empty string for values that are not envelopes — a bare TransactionResult,
// for instance — rather than guessing.
func envelopeVariant(n *lens.Node) string {
	switch {
	case n.Child("FeeBump") != nil:
		return "fee_bump"
	case n.Child("V1") != nil:
		return "v1"
	case n.Child("V0") != nil:
		return "v0"
	default:
		return ""
	}
}

// isTransactionType reports whether typeName is a type whose summary is a
// transaction. Anything else falls back to the detail column.
func isTransactionType(typeName string) bool {
	return typeName == "TransactionEnvelope" || typeName == "TransactionResult"
}

// joinOperationTypes renders operation types as one pipe-separated field,
// de-duplicated in first-seen order. Operation results without a paired
// envelope carry the placeholder "unknown", which is dropped rather than
// exported as if it meant something.
func joinOperationTypes(ops []lens.OpSummary) string {
	out := make([]string, 0, len(ops))
	seen := make(map[string]bool, len(ops))
	for _, op := range ops {
		if op.Type == "" || op.Type == "unknown" || seen[op.Type] {
			continue
		}
		seen[op.Type] = true
		out = append(out, op.Type)
	}
	return strings.Join(out, "|")
}
