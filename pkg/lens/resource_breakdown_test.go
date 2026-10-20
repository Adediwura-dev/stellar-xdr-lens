package lens_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/stellar/go-stellar-sdk/xdr"

	"github.com/stellar-optics/stellar-xdr-lens/pkg/lens"
)

func sorobanDataValue() *lens.Value {
	data := &xdr.SorobanTransactionData{
		Resources: xdr.SorobanResources{
			Footprint: xdr.LedgerFootprint{
				ReadOnly:  []xdr.LedgerKey{{}, {}},
				ReadWrite: []xdr.LedgerKey{{}},
			},
			Instructions:  100_000_000,
			DiskReadBytes: 150_000,
			WriteBytes:    100_000,
		},
		ResourceFee: 12_345,
	}
	return &lens.Value{Type: "SorobanTransactionData", Raw: data}
}

func TestDefaultResourceLimits(t *testing.T) {
	t.Parallel()

	limits := lens.DefaultResourceLimits()
	if limits.Instructions != 400_000_000 {
		t.Errorf("Instructions = %d, want 400000000", limits.Instructions)
	}
	if limits.MemoryBytes != 40*1024*1024 {
		t.Errorf("MemoryBytes = %d, want %d", limits.MemoryBytes, 40*1024*1024)
	}
	if limits.WriteBytes != 132_096 {
		t.Errorf("WriteBytes = %d, want 132096", limits.WriteBytes)
	}
}

func TestExplainResourcesComparesUsageWithLimits(t *testing.T) {
	t.Parallel()

	got, err := lens.ExplainResources(sorobanDataValue(), lens.DefaultResourceLimits())
	if err != nil {
		t.Fatalf("ExplainResources() error = %v", err)
	}
	if got.Source != "SorobanTransactionData" {
		t.Fatalf("Source = %q, want SorobanTransactionData", got.Source)
	}
	if got.HasExceeded() {
		t.Fatalf("HasExceeded() = true, exceeded %v", got.Exceeded)
	}

	rows := make(map[string]lens.ResourceUsageRow, len(got.Rows))
	for _, row := range got.Rows {
		rows[row.Resource] = row
	}
	if row := rows["Instructions (CPU)"]; row.Used != 100_000_000 || row.Limit != 400_000_000 {
		t.Errorf("instructions row = %+v", row)
	}
	if row := rows["Disk read entries"]; row.Used != 2 || row.Limit != 200 {
		t.Errorf("disk read entries row = %+v", row)
	}
	if row := rows["Footprint entries"]; row.Used != 3 {
		t.Errorf("footprint entries row = %+v", row)
	}
	if row := rows["Memory"]; row.Declared {
		t.Errorf("memory row should be marked undeclared: %+v", row)
	}
	if row := rows["Resource fee (declared)"]; row.Used != 12_345 {
		t.Errorf("resource fee row = %+v", row)
	}
	report := got.String()
	if !strings.Contains(report, "100,000,000") {
		t.Errorf("report does not format large values:\n%s", report)
	}
	if !strings.Contains(report, "All declared resources are within") {
		t.Errorf("report missing within-limits conclusion:\n%s", report)
	}
}

func TestExplainResourcesFlagsExceededResources(t *testing.T) {
	t.Parallel()

	limits := lens.DefaultResourceLimits()
	limits.WriteBytes = 1
	limits.FootprintEntries = 2

	got, err := lens.ExplainResources(sorobanDataValue(), limits)
	if err != nil {
		t.Fatalf("ExplainResources() error = %v", err)
	}
	if !got.HasExceeded() {
		t.Fatal("HasExceeded() = false, want true")
	}
	if len(got.Exceeded) != 2 || got.Exceeded[0] != "Write bytes" || got.Exceeded[1] != "Footprint entries" {
		t.Errorf("Exceeded = %v, want [Write bytes, Footprint entries]", got.Exceeded)
	}
	report := got.String()
	if !strings.Contains(report, "EXCEEDED") {
		t.Errorf("report does not flag exceeded resources:\n%s", report)
	}
}

func TestExplainResourcesWithChargedFees(t *testing.T) {
	t.Parallel()

	v1 := xdr.SorobanTransactionMetaExtV1{
		TotalNonRefundableResourceFeeCharged: 7_000,
		TotalRefundableResourceFeeCharged:    5_000,
		RentFeeCharged:                       2_000,
	}
	meta := &lens.Value{
		Type: "TransactionMeta",
		Raw: &xdr.TransactionMeta{
			V: 3,
			V3: &xdr.TransactionMetaV3{
				SorobanMeta: &xdr.SorobanTransactionMeta{
					Ext: xdr.SorobanTransactionMetaExt{V: 1, V1: &v1},
				},
			},
		},
	}

	got, err := lens.ExplainResourcesWithMeta(sorobanDataValue(), meta, lens.DefaultResourceLimits())
	if err != nil {
		t.Fatalf("ExplainResourcesWithMeta() error = %v", err)
	}
	rows := make(map[string]lens.ResourceUsageRow, len(got.Rows))
	for _, row := range got.Rows {
		rows[row.Resource] = row
	}
	for name, want := range map[string]int64{
		"Non-refundable fee (charged)": 7_000,
		"Refundable fee (charged)":     5_000,
		"Rent fee (charged)":           2_000,
	} {
		if row, ok := rows[name]; !ok || row.Used != want {
			t.Errorf("%s = %+v, want used %d", name, row, want)
		}
	}
}

func TestExplainResourcesErrors(t *testing.T) {
	t.Parallel()

	if _, err := lens.ExplainResources(nil, lens.DefaultResourceLimits()); !errors.Is(err, lens.ErrInvalidArgument) {
		t.Errorf("nil value error = %v, want ErrInvalidArgument", err)
	}

	locked := &lens.Value{Type: "TransactionEnvelope", Raw: &xdr.TransactionEnvelope{
		Type: xdr.EnvelopeTypeEnvelopeTypeTx,
		V1: &xdr.TransactionV1Envelope{
			Tx: xdr.Transaction{Operations: []xdr.Operation{}},
		},
	}}
	if _, err := lens.ExplainResources(locked, lens.DefaultResourceLimits()); !errors.Is(err, lens.ErrUnsupportedType) {
		t.Errorf("non-Soroban envelope error = %v, want ErrUnsupportedType", err)
	}

	result := &lens.Value{Type: "TransactionResult", Raw: &xdr.TransactionResult{}}
	if _, err := lens.ExplainResources(result, lens.DefaultResourceLimits()); !errors.Is(err, lens.ErrUnsupportedType) {
		t.Errorf("result error = %v, want ErrUnsupportedType", err)
	}
}
