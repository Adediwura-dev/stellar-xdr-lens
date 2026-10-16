package format

import "testing"

// TestCSVColumnIndexesMatchSchema pins the link the column constants rely on:
// each constant names the position of its column in csvColumns. The constants
// are unexported iota values, so without this test a reorder of csvColumns
// would silently misalign every exported row.
func TestCSVColumnIndexesMatchSchema(t *testing.T) {
	t.Parallel()

	byName := make(map[string]int, len(csvColumns))
	for i, name := range csvColumns {
		byName[name] = i
	}

	checks := map[int]string{
		colType:             "type",
		colVariant:          "variant",
		colSource:           "source",
		colFeeBumpSource:    "fee_bump_source",
		colSeqNum:           "seq_num",
		colFee:              "fee",
		colMemo:             "memo",
		colSignatures:       "signatures",
		colOperationCount:   "operation_count",
		colOperationTypes:   "operation_types",
		colFeeCharged:       "fee_charged",
		colSuccess:          "success",
		colResultCode:       "result_code",
		colInnerResultCode:  "inner_result_code",
		colFailedOperations: "failed_operations",
		colDetail:           "detail",
	}

	if len(checks) != len(csvColumns) {
		t.Fatalf("the test covers %d of %d columns", len(checks), len(csvColumns))
	}
	for index, name := range checks {
		got, ok := byName[name]
		if !ok {
			t.Errorf("column %q is missing from csvColumns", name)
			continue
		}
		if got != index {
			t.Errorf("col%s is %d, but %q is at index %d", name, index, name, got)
		}
	}

	// The constants are contiguous, which is what keeps a record built with
	// make([]string, len(csvColumns)) indexed safely.
	if colDetail-colType+1 != len(csvColumns) {
		t.Errorf("column constants span %d slots, want %d", colDetail-colType+1, len(csvColumns))
	}
}

// TestCSVFormatterImplementsFormatter keeps the extension seam explicit at
// compile time.
func TestCSVFormatterImplementsFormatter(t *testing.T) {
	t.Parallel()

	var _ Formatter = (*CSVFormatter)(nil)
}
