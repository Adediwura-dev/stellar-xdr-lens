package format_test

import (
	"bytes"
	"encoding/base64"
	"encoding/csv"
	"errors"
	"strings"
	"testing"

	"github.com/stellar/go-stellar-sdk/xdr"

	"github.com/stellar-optics/stellar-xdr-lens/pkg/lens"
	"github.com/stellar-optics/stellar-xdr-lens/pkg/lens/format"
)

// errSink refuses every write, which is how a closed pipe or a full disk
// looks to the formatter.
type errSink struct{ err error }

func (w errSink) Write(p []byte) (int, error) { return 0, w.err }

var errSinkBoom = errors.New("sink is closed")

// parseCSV reads formatter output back with a standard CSV reader, which is
// the interface downstream tooling uses. Anything the formatter emits must
// survive that round trip.
func parseCSV(t *testing.T, out string) [][]string {
	t.Helper()
	records, err := csv.NewReader(strings.NewReader(out)).ReadAll()
	if err != nil {
		t.Fatalf("output is not valid CSV: %v\n%s", err, out)
	}
	return records
}

// assertRecordWidth enforces the schema invariant: every record, header
// included, carries exactly one field per column.
func assertRecordWidth(t *testing.T, records [][]string) {
	t.Helper()
	want := len(format.CSVColumns())
	for i, rec := range records {
		if len(rec) != want {
			t.Errorf("record %d has %d fields, want %d: %v", i, len(rec), want, rec)
		}
	}
}

func assertRecord(t *testing.T, got, want []string) {
	t.Helper()
	cols := format.CSVColumns()
	if len(got) != len(cols) {
		t.Fatalf("record has %d fields, want %d: %v", len(got), len(cols), got)
	}
	for i := range cols {
		if got[i] != want[i] {
			t.Errorf("column %d %q = %q, want %q\nrow: %v", i, cols[i], got[i], want[i], got)
		}
	}
}

// csvDataRow formats v with a fresh formatter and returns the data row.
func csvDataRow(t *testing.T, v *lens.Value) []string {
	t.Helper()
	var buf bytes.Buffer
	if err := format.NewCSVFormatter().Format(&buf, v); err != nil {
		t.Fatalf("Format() error = %v", err)
	}
	records := parseCSV(t, buf.String())
	if len(records) != 2 {
		t.Fatalf("got %d records, want a header and one data row:\n%s", len(records), buf.String())
	}
	assertRecordWidth(t, records)
	return records[1]
}

// pairSummary explains an envelope and its result together, the way a ledger
// export would.
func pairSummary(t *testing.T, envName, resName string) *lens.Summary {
	t.Helper()
	s, err := lens.ExplainPair(loadValue(t, envName), loadValue(t, resName))
	if err != nil {
		t.Fatalf("ExplainPair(%s, %s) error = %v", envName, resName, err)
	}
	return s
}

// emptyRow is the all-blank row used to build expectations field by field.
func emptyRow() []string { return make([]string, len(format.CSVColumns())) }

// TestCSVColumnsAreStable pins the exported schema. Scripts address these
// columns positionally, so renaming or reordering one is a breaking change.
func TestCSVColumnsAreStable(t *testing.T) {
	t.Parallel()

	want := []string{
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
	got := format.CSVColumns()
	if len(got) != len(want) {
		t.Fatalf("CSVColumns() has %d columns, want %d: %v", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("column %d = %q, want %q", i, got[i], want[i])
		}
	}
}

// TestCSVColumnsReturnsACopy guards callers that keep and mutate the slice.
func TestCSVColumnsReturnsACopy(t *testing.T) {
	t.Parallel()

	first := format.CSVColumns()
	first[0] = "mutated"

	if format.CSVColumns()[0] != "type" {
		t.Fatalf("mutating the returned slice changed the schema: %v", format.CSVColumns())
	}
}

// TestCSVFormatterEnvelopeV1 pins the full row for a canonical v1 envelope
// fixture, column by column.
func TestCSVFormatterEnvelopeV1(t *testing.T) {
	t.Parallel()

	got := csvDataRow(t, loadValue(t, "tx_success.env"))

	want := emptyRow()
	want[0] = "TransactionEnvelope"
	want[1] = "v1"
	want[2] = "GCJNHFXJEQEY54NPKU4DBMT5WOBJUHMYSFBDAQF4OMTEGJN4BQU7ZST6"
	want[4] = "259021488982594615"
	want[5] = "58002"
	want[7] = "1"
	want[8] = "1"
	want[9] = "invoke_host_function"

	assertRecord(t, got, want)
}

// TestCSVFormatterFeeBumpEnvelope covers the fee-bump shape, where the source
// account is the fee-bump account and the inner source is preserved.
func TestCSVFormatterFeeBumpEnvelope(t *testing.T) {
	t.Parallel()

	got := csvDataRow(t, loadValue(t, "tx_feebump.env"))

	want := emptyRow()
	want[0] = "TransactionEnvelope"
	want[1] = "fee_bump"
	want[2] = "GAKXCN4JEJP653SG7PZMOKCLR6ASO6BCC7IPSHGQGB2DCRFBPZNSXTDG"
	want[3] = "GA2JRQOF6EA3HQWDCEDBPPMLYPJCFLDDGYZLEQGMS5SOBQIB3BAFHVAW"
	want[4] = "261917804473868099"
	want[5] = "45105"
	want[7] = "1"
	want[8] = "1"
	want[9] = "invoke_host_function"

	assertRecord(t, got, want)
}

// TestCSVFormatterSorobanFeeBumpEnvelope is the second real fee-bump fixture,
// which pins that variant detection does not depend on one payload.
func TestCSVFormatterSorobanFeeBumpEnvelope(t *testing.T) {
	t.Parallel()

	got := csvDataRow(t, loadValue(t, "tx_soroban.env"))

	want := emptyRow()
	want[0] = "TransactionEnvelope"
	want[1] = "fee_bump"
	want[2] = "GARP5EFFRIN545CGIL6G62YWVZGQPUB367QSZ64O7S2L443FIUKJ5SQ4"
	want[3] = "GA2JRQOF6EA3HQWDCEDBPPMLYPJCFLDDGYZLEQGMS5SOBQIB3BAFHVAW"
	want[4] = "265409522691058404"
	want[5] = "45762"
	want[7] = "1"
	want[8] = "1"
	want[9] = "invoke_host_function"

	assertRecord(t, got, want)
}

// TestCSVFormatterV0Envelope covers the third envelope shape. No captured
// fixture uses it, so the envelope is built and marshalled here.
func TestCSVFormatterV0Envelope(t *testing.T) {
	t.Parallel()

	got := csvDataRow(t, v0Envelope(t, "hello"))

	want := emptyRow()
	want[0] = "TransactionEnvelope"
	want[1] = "v0"
	// The source is a strkey of the raw ed25519 key; assert the shape and
	// leave the exact encoding to pkg/lens's own tests.
	if !strings.HasPrefix(got[2], "G") || len(got[2]) != 56 {
		t.Errorf("source = %q, want a 56-character G… strkey", got[2])
	}
	want[2] = got[2]
	want[4] = "7"
	want[5] = "100"
	want[6] = "text: hello"
	want[7] = "0"
	want[8] = "0"

	assertRecord(t, got, want)
}

// TestCSVFormatterQuotesMemoText proves the format is RFC 4180 rather than
// delimiter-joined: a memo carrying a comma, a quote and a newline must come
// back byte for byte from a standard reader.
func TestCSVFormatterQuotesMemoText(t *testing.T) {
	t.Parallel()

	memo := `a,b "quoted"` + "\n" + `second line`
	v := v0Envelope(t, memo)

	var buf bytes.Buffer
	if err := format.NewCSVFormatter().Format(&buf, v); err != nil {
		t.Fatalf("Format() error = %v", err)
	}

	records := parseCSV(t, buf.String())
	assertRecordWidth(t, records)
	memoIdx := columnIndex(t, "memo")
	if got, want := records[1][memoIdx], "text: "+memo; got != want {
		t.Errorf("memo round trip = %q, want %q", got, want)
	}
}

// TestCSVFormatterTransactionResult covers a bare result: outcome columns are
// set, envelope-only columns stay blank so the row still lines up.
func TestCSVFormatterTransactionResult(t *testing.T) {
	t.Parallel()

	got := csvDataRow(t, loadValue(t, "tx_failed.res"))

	want := emptyRow()
	want[0] = "TransactionResult"
	want[8] = "1" // operation results
	want[10] = "13355"
	want[11] = "false"
	want[12] = "tx_fee_bump_inner_failed"
	want[13] = "tx_failed"
	want[14] = "0"

	assertRecord(t, got, want)
}

func TestCSVFormatterTransactionResultFixtures(t *testing.T) {
	t.Parallel()

	tests := []struct {
		fixture   string
		fee       string
		success   string
		code      string
		innerCode string
		failed    string
	}{
		{"tx_success.res", "37939", "true", "tx_success", "", ""},
		{"tx_feebump.res", "34354", "true", "tx_fee_bump_inner_success", "tx_success", ""},
		{"tx_soroban.res", "35438", "true", "tx_fee_bump_inner_success", "tx_success", ""},
		{"tx_failed.res", "13355", "false", "tx_fee_bump_inner_failed", "tx_failed", "0"},
	}

	for _, tc := range tests {
		t.Run(tc.fixture, func(t *testing.T) {
			t.Parallel()

			got := csvDataRow(t, loadValue(t, tc.fixture))

			// A result never carries envelope fields.
			for _, col := range []string{"variant", "source", "fee_bump_source", "seq_num", "fee", "signatures"} {
				if got[columnIndex(t, col)] != "" {
					t.Errorf("%s = %q for a bare result, want empty", col, got[columnIndex(t, col)])
				}
			}
			for col, want := range map[string]string{
				"fee_charged":       tc.fee,
				"success":           tc.success,
				"result_code":       tc.code,
				"inner_result_code": tc.innerCode,
				"failed_operations": tc.failed,
			} {
				if got[columnIndex(t, col)] != want {
					t.Errorf("%s = %q, want %q", col, got[columnIndex(t, col)], want)
				}
			}
		})
	}
}

// TestCSVFormatterNonTransactionFallsBackToDetail shows that a bucket entry,
// ledger header or overlay message still exports: the row names its type and
// flattens the tree into detail rather than pretending to be a transaction.
func TestCSVFormatterNonTransactionFallsBackToDetail(t *testing.T) {
	t.Parallel()

	for _, fixture := range []string{"ledger_header", "scp_envelope", "trustline_entry_v1", "overlay_hello"} {
		t.Run(fixture, func(t *testing.T) {
			t.Parallel()

			v := loadValue(t, fixture)
			got := csvDataRow(t, v)

			if got[0] != v.Type {
				t.Errorf("type = %q, want %q", got[0], v.Type)
			}
			if got[columnIndex(t, "detail")] == "" {
				t.Errorf("detail column is empty for a non-transaction type:\n%v", got)
			}
			if strings.Contains(got[columnIndex(t, "detail")], "\x1b[") {
				t.Errorf("detail carries ANSI escapes into CSV output: %q", got[columnIndex(t, "detail")])
			}
			// Every transaction-specific column stays blank.
			for _, col := range []string{"variant", "source", "fee", "success", "result_code"} {
				if got[columnIndex(t, col)] != "" {
					t.Errorf("%s = %q for a non-transaction type, want empty", col, got[columnIndex(t, col)])
				}
			}
		})
	}
}

// TestCSVFormatterPairedSummary covers the export a batch ledger envelope job
// actually runs: an envelope and its result explained together.
func TestCSVFormatterPairedSummary(t *testing.T) {
	t.Parallel()

	s := pairSummary(t, "tx_failed.env", "tx_failed.res")

	var buf bytes.Buffer
	if err := format.NewCSVFormatter().FormatSummary(&buf, s); err != nil {
		t.Fatalf("FormatSummary() error = %v", err)
	}
	records := parseCSV(t, buf.String())
	if len(records) != 2 {
		t.Fatalf("got %d records, want a header and one data row:\n%s", len(records), buf.String())
	}
	assertRecordWidth(t, records)

	want := emptyRow()
	want[0] = "transaction"
	want[1] = "fee_bump"
	want[2] = "GAPB7YMUQ6MPQ23PTFDNBZP7HGGTZPI2DT5JJURUOIXNPLGHW2O3WYXZ"
	want[3] = "GA2JRQOF6EA3HQWDCEDBPPMLYPJCFLDDGYZLEQGMS5SOBQIB3BAFHVAW"
	want[4] = "264125967419700139"
	want[5] = "24167"
	want[7] = "1"
	want[8] = "1"
	want[9] = "invoke_host_function"
	want[10] = "13355"
	want[11] = "false"
	want[12] = "tx_fee_bump_inner_failed"
	want[13] = "tx_failed"
	want[14] = "0"

	assertRecord(t, records[1], want)
}

// TestCSVFormatterSummaryOmitsEnvelopeOnlyColumnsForResults covers the summary
// a bare result explains to: seq, fee and signature count were never known,
// so exporting them as zero would be a lie.
func TestCSVFormatterSummaryOmitsEnvelopeOnlyColumnsForResults(t *testing.T) {
	t.Parallel()

	s, err := lens.Explain(loadValue(t, "tx_success.res"))
	if err != nil {
		t.Fatalf("Explain() error = %v", err)
	}

	var buf bytes.Buffer
	if err := format.NewCSVFormatter().FormatSummary(&buf, s); err != nil {
		t.Fatalf("FormatSummary() error = %v", err)
	}
	row := parseCSV(t, buf.String())[1]
	assertRecordWidth(t, parseCSV(t, buf.String()))

	if row[columnIndex(t, "type")] != "transaction_result" {
		t.Errorf("type = %q, want transaction_result", row[columnIndex(t, "type")])
	}
	for _, col := range []string{"seq_num", "fee", "signatures", "variant"} {
		if row[columnIndex(t, col)] != "" {
			t.Errorf("%s = %q, want empty for a result-only summary", col, row[columnIndex(t, col)])
		}
	}
	if row[columnIndex(t, "success")] != "true" {
		t.Errorf("success = %q, want true", row[columnIndex(t, "success")])
	}
}

// TestCSVFormatterSummaryDeduplicatesOperationTypes pins the operation_types
// contract: first-seen order, de-duplicated, placeholders dropped.
func TestCSVFormatterSummaryDeduplicatesOperationTypes(t *testing.T) {
	t.Parallel()

	s := &lens.Summary{
		Kind: "transaction",
		Operations: []lens.OpSummary{
			{Index: 0, Type: "payment"},
			{Index: 1, Type: "payment"},
			{Index: 2, Type: "unknown"},
			{Index: 3, Type: ""},
			{Index: 4, Type: "create_account"},
		},
	}

	var buf bytes.Buffer
	if err := format.NewCSVFormatter().FormatSummary(&buf, s); err != nil {
		t.Fatalf("FormatSummary() error = %v", err)
	}
	row := parseCSV(t, buf.String())[1]

	if got, want := row[columnIndex(t, "operation_types")], "payment|create_account"; got != want {
		t.Errorf("operation_types = %q, want %q", got, want)
	}
	if got, want := row[columnIndex(t, "operation_count")], "5"; got != want {
		t.Errorf("operation_count = %q, want %q (every operation is counted)", got, want)
	}
}

// TestCSVFormatterSummaryFeeBumpVariant covers variant inference for a summary
// with no node tree behind it.
func TestCSVFormatterSummaryFeeBumpVariant(t *testing.T) {
	t.Parallel()

	s := &lens.Summary{Kind: "transaction", FeeBumpSource: "GA2JRQOF6EA3HQWDCEDBPPMLYPJCFLDDGYZLEQGMS5SOBQIB3BAFHVAW"}

	var buf bytes.Buffer
	if err := format.NewCSVFormatter().FormatSummary(&buf, s); err != nil {
		t.Fatalf("FormatSummary() error = %v", err)
	}
	row := parseCSV(t, buf.String())[1]

	if got, want := row[columnIndex(t, "variant")], "fee_bump"; got != want {
		t.Errorf("variant = %q, want %q", got, want)
	}
	if got, want := row[columnIndex(t, "fee_bump_source")], s.FeeBumpSource; got != want {
		t.Errorf("fee_bump_source = %q, want %q", got, want)
	}
}

// TestCSVFormatterFormatAllIsABatchExport is the core batch contract: one
// header for thousands of transactions, one row each, fixed width.
func TestCSVFormatterFormatAllIsABatchExport(t *testing.T) {
	t.Parallel()

	values := []*lens.Value{
		loadValue(t, "tx_success.env"),
		loadValue(t, "tx_failed.res"),
		loadValue(t, "ledger_header"),
	}

	var buf bytes.Buffer
	if err := format.NewCSVFormatter().FormatAll(&buf, values); err != nil {
		t.Fatalf("FormatAll() error = %v", err)
	}
	records := parseCSV(t, buf.String())

	if len(records) != len(values)+1 {
		t.Fatalf("got %d records, want %d", len(records), len(values)+1)
	}
	assertRecordWidth(t, records)

	// The header appears exactly once, at the top.
	header := strings.Join(records[0], ",")
	if header != strings.Join(format.CSVColumns(), ",") {
		t.Errorf("header = %v, want %v", records[0], format.CSVColumns())
	}
	for i, v := range values {
		if got, want := strings.Join(records[i+1], "|"), strings.Join(csvDataRow(t, v), "|"); got != want {
			t.Errorf("batch row %d differs from single-value export:\n got %s\nwant %s", i, got, want)
		}
	}
}

// TestCSVFormatterHeaderCanBeDisabled proves appending works: headerless runs
// concatenated equal one headed run.
func TestCSVFormatterHeaderCanBeDisabled(t *testing.T) {
	t.Parallel()

	values := []*lens.Value{loadValue(t, "tx_success.env"), loadValue(t, "tx_failed.res")}

	bare := format.NewCSVFormatter(format.WithCSVHeader(false))

	var appended bytes.Buffer
	for _, v := range values {
		if err := bare.Format(&appended, v); err != nil {
			t.Fatalf("Format() error = %v", err)
		}
	}
	if strings.Contains(appended.String(), "type,variant") {
		t.Errorf("header was emitted with WithCSVHeader(false):\n%s", appended.String())
	}

	var batch bytes.Buffer
	headed := format.NewCSVFormatter()
	if err := headed.FormatAll(&batch, values); err != nil {
		t.Fatalf("FormatAll() error = %v", err)
	}

	// Strip the single header line the headed run emitted.
	_, body, ok := strings.Cut(batch.String(), "\n")
	if !ok {
		t.Fatalf("batch output has no header line:\n%s", batch.String())
	}
	if appended.String() != body {
		t.Errorf("headerless append differs from a headed batch:\n got %q\nwant %q", appended.String(), body)
	}
}

func TestCSVFormatterCustomDelimiter(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	f := format.NewCSVFormatter(format.WithCSVDelimiter(';'))
	if err := f.Format(&buf, loadValue(t, "tx_success.env")); err != nil {
		t.Fatalf("Format() error = %v", err)
	}
	if strings.Contains(buf.String(), ",") {
		t.Errorf("output still contains commas with a ';' delimiter:\n%s", buf.String())
	}

	records, err := csv.NewReader(strings.NewReader(buf.String())).ReadAll()
	if err != nil {
		t.Fatalf("output is not valid ';'-delimited CSV: %v", err)
	}
	_ = records
}

// TestCSVFormatterInvalidDelimiter covers the documented failure: RFC 4180
// reserves the quote, carriage return and line feed, so they are rejected
// rather than silently producing unparseable CSV.
func TestCSVFormatterInvalidDelimiter(t *testing.T) {
	t.Parallel()

	v := loadValue(t, "tx_success.env")

	for _, r := range []rune{'"', '\r', '\n', 0} {
		t.Run(string(r), func(t *testing.T) {
			t.Parallel()

			var buf bytes.Buffer
			err := format.NewCSVFormatter(format.WithCSVDelimiter(r)).Format(&buf, v)
			if err == nil {
				t.Fatalf("Format() with delimiter %q error = nil, want an error", r)
			}
			if !strings.Contains(err.Error(), "csv") {
				t.Errorf("error %q does not identify the CSV writer", err)
			}
			if buf.Len() != 0 {
				t.Errorf("invalid delimiter still wrote %q", buf.String())
			}

			// The header is the first row written, so the error is reported
			// there when a header is emitted.
			if !strings.Contains(err.Error(), "header") {
				t.Errorf("error = %v, want it reported while writing the header", err)
			}

			// With the header disabled the first data row is where it fails.
			buf.Reset()
			err = format.NewCSVFormatter(format.WithCSVHeader(false), format.WithCSVDelimiter(r)).Format(&buf, v)
			if err == nil {
				t.Fatalf("Format() without header, delimiter %q: error = nil, want an error", r)
			}
			if !strings.Contains(err.Error(), "row") {
				t.Errorf("error = %v, want it reported while writing a row", err)
			}
		})
	}
}

func TestCSVFormatterNilInputs(t *testing.T) {
	t.Parallel()

	f := format.NewCSVFormatter()

	var valueOut bytes.Buffer
	if err := f.Format(&valueOut, nil); err == nil {
		t.Error("Format(nil) error = nil, want an error")
	}
	if valueOut.Len() != 0 {
		t.Errorf("Format(nil) still produced output: %q", valueOut.String())
	}

	var summaryOut bytes.Buffer
	if err := f.FormatSummary(&summaryOut, nil); err == nil {
		t.Error("FormatSummary(nil) error = nil, want an error")
	}
	if summaryOut.Len() != 0 {
		t.Errorf("FormatSummary(nil) still produced output: %q", summaryOut.String())
	}

	// An empty batch is not an error: it is a header with no rows, which is
	// what an export of zero transactions should produce.
	var batchOut bytes.Buffer
	if err := f.FormatAll(&batchOut, nil); err != nil {
		t.Errorf("FormatAll(nil) error = %v, want nil for an empty batch", err)
	}
	if got, want := batchOut.String(), strings.Join(format.CSVColumns(), ",")+"\n"; got != want {
		t.Errorf("FormatAll(nil) output = %q, want just the header %q", got, want)
	}
}

// TestCSVFormatterNilNodeIsRejected covers a Value that decoded to nothing.
func TestCSVFormatterNilNodeIsRejected(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	err := format.NewCSVFormatter().Format(&buf, &lens.Value{Type: "TransactionEnvelope"})
	if err == nil {
		t.Fatal("Format() with a nil node error = nil, want an error")
	}
	if buf.Len() != 0 {
		t.Errorf("a rejected value still wrote %q", buf.String())
	}
}

// TestCSVFormatterUnexplainableTransactionFails covers the deliberate
// asymmetry: a non-transaction type degrades to the detail column, but a
// transaction that cannot be summarised is a real failure.
func TestCSVFormatterUnexplainableTransactionFails(t *testing.T) {
	t.Parallel()

	node := &lens.Node{Name: "Root", Kind: lens.KindStruct}
	for _, typeName := range []string{"TransactionEnvelope", "TransactionResult"} {
		t.Run(typeName, func(t *testing.T) {
			t.Parallel()

			var buf bytes.Buffer
			err := format.NewCSVFormatter().Format(&buf, &lens.Value{Type: typeName, Node: node})
			if err == nil {
				t.Fatalf("Format() of an unexplainable %s error = nil, want an error", typeName)
			}
			if !strings.Contains(err.Error(), "explain") {
				t.Errorf("error = %v, want it to say the value could not be explained", err)
			}
			if buf.Len() != 0 {
				t.Errorf("a failed export still wrote %q", buf.String())
			}
		})
	}
}

// TestCSVFormatterBatchErrorsAreAttributed pins that a bad value names its
// position in the batch, and that nothing is written when a batch fails.
func TestCSVFormatterBatchErrorsAreAttributed(t *testing.T) {
	t.Parallel()

	env := loadValue(t, "tx_success.env")

	t.Run("FormatAll", func(t *testing.T) {
		t.Parallel()

		var buf bytes.Buffer
		err := format.NewCSVFormatter().FormatAll(&buf, []*lens.Value{env, nil})
		if err == nil {
			t.Fatal("FormatAll() with a nil value error = nil, want an error")
		}
		if !strings.Contains(err.Error(), "value 1") {
			t.Errorf("error = %v, want it to name value 1", err)
		}
		if buf.Len() != 0 {
			t.Errorf("a failed batch wrote %q; the writer must be left untouched", buf.String())
		}
	})

	t.Run("FormatSummaries", func(t *testing.T) {
		t.Parallel()

		summaries := []*lens.Summary{pairSummary(t, "tx_failed.env", "tx_failed.res"), nil}

		var buf bytes.Buffer
		err := format.NewCSVFormatter().FormatSummaries(&buf, summaries)
		if err == nil {
			t.Fatal("FormatSummaries() with a nil summary error = nil, want an error")
		}
		if !strings.Contains(err.Error(), "summary 1") {
			t.Errorf("error = %v, want it to name summary 1", err)
		}
		if buf.Len() != 0 {
			t.Errorf("a failed batch wrote %q; the writer must be left untouched", buf.String())
		}
	})
}

// TestCSVFormatterFormatSummariesBatch covers the successful batch path.
func TestCSVFormatterFormatSummariesBatch(t *testing.T) {
	t.Parallel()

	summaries := []*lens.Summary{
		pairSummary(t, "tx_failed.env", "tx_failed.res"),
		pairSummary(t, "tx_soroban.env", "tx_soroban.res"),
	}

	var buf bytes.Buffer
	if err := format.NewCSVFormatter().FormatSummaries(&buf, summaries); err != nil {
		t.Fatalf("FormatSummaries() error = %v", err)
	}
	records := parseCSV(t, buf.String())

	if len(records) != len(summaries)+1 {
		t.Fatalf("got %d records, want %d", len(records), len(summaries)+1)
	}
	assertRecordWidth(t, records)
	for i, s := range summaries {
		if got, want := records[i+1][columnIndex(t, "result_code")], s.Outcome.Reason.Code; got != want {
			t.Errorf("row %d result_code = %q, want %q", i, got, want)
		}
	}
}

// TestCSVFormatterWriteErrorIsReported covers the buffered-writer trap: a
// short write only surfaces on Flush, so the formatter must check it.
func TestCSVFormatterWriteErrorIsReported(t *testing.T) {
	t.Parallel()

	v := loadValue(t, "tx_success.env")

	for _, tc := range []struct {
		name   string
		values []*lens.Value
	}{
		{"Format", []*lens.Value{v}},
		{"FormatAll", []*lens.Value{v, v}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			f := format.NewCSVFormatter()
			var err error
			if tc.name == "Format" {
				err = f.Format(errSink{errSinkBoom}, tc.values[0])
			} else {
				err = f.FormatAll(errSink{errSinkBoom}, tc.values)
			}
			if err == nil {
				t.Fatal("write error was swallowed, want an error")
			}
			if !errors.Is(err, errSinkBoom) {
				t.Errorf("error = %v, want it to wrap the writer's error", err)
			}
		})
	}
}

// TestCSVFormatterIsDeterministic guards CI diffing: the same input must
// produce byte-identical output, with nothing from the clock or environment.
func TestCSVFormatterIsDeterministic(t *testing.T) {
	t.Parallel()

	values := []*lens.Value{loadValue(t, "tx_soroban.env"), loadValue(t, "scp_envelope")}

	var first, second bytes.Buffer
	f := format.NewCSVFormatter()
	if err := f.FormatAll(&first, values); err != nil {
		t.Fatalf("FormatAll() error = %v", err)
	}
	if err := f.FormatAll(&second, values); err != nil {
		t.Fatalf("FormatAll() error = %v", err)
	}
	if first.String() != second.String() {
		t.Error("CSV output differs between runs; it must be deterministic")
	}
}

// TestCSVFormatterSatisfiesFormatter keeps the interface contract explicit.
func TestCSVFormatterSatisfiesFormatter(t *testing.T) {
	t.Parallel()

	var f format.Formatter = format.NewCSVFormatter()
	if err := f.Format(&bytes.Buffer{}, loadValue(t, "tx_success.env")); err != nil {
		t.Fatalf("Format() through the Formatter interface error = %v", err)
	}
}

// columnIndex returns the position of a column, resolved from the exported
// schema so the tests do not hard-code indexes.
func columnIndex(t *testing.T, name string) int {
	t.Helper()
	for i, col := range format.CSVColumns() {
		if col == name {
			return i
		}
	}
	t.Fatalf("no %q column in %v", name, format.CSVColumns())
	return -1
}

// v0Envelope builds, marshals and decodes a v0 envelope carrying memoText.
// The testdata corpus is all v1 and fee-bump, and a decode of generated XDR
// is the next best thing to a captured fixture.
func v0Envelope(t *testing.T, memoText string) *lens.Value {
	t.Helper()

	env := xdr.TransactionEnvelope{
		Type: xdr.EnvelopeTypeEnvelopeTypeTxV0,
		V0: &xdr.TransactionV0Envelope{
			Tx: xdr.TransactionV0{
				SourceAccountEd25519: xdr.Uint256{0x01, 0x02, 0x03},
				Fee:                  100,
				SeqNum:               7,
				Memo:                 xdr.Memo{Type: xdr.MemoTypeMemoText, Text: &memoText},
				Operations:           []xdr.Operation{},
				Ext:                  xdr.TransactionV0Ext{V: 0},
			},
			Signatures: []xdr.DecoratedSignature{},
		},
	}

	raw, err := env.MarshalBinary()
	if err != nil {
		t.Fatalf("marshalling v0 envelope: %v", err)
	}

	v, err := lens.DecodeAs(base64.StdEncoding.EncodeToString(raw), "TransactionEnvelope")
	if err != nil {
		t.Fatalf("decoding v0 envelope: %v", err)
	}
	return v
}
