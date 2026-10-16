package lens_test

import (
	"strings"
	"testing"

	"github.com/stellar/go-stellar-sdk/xdr"

	"github.com/stellar-optics/stellar-xdr-lens/pkg/lens"
)

const historyHashNote = "505152535455565758595a5b5c5d5e5f606162636465666768696a6b6c6d6e6f"

func TestDecodeTransactionHistoryEntryV0(t *testing.T) {
	t.Parallel()

	v, err := lens.DecodeAs(loadFixture(t, "tx_history_entry_v0"), "TransactionHistoryEntry")
	if err != nil {
		t.Fatalf("DecodeAs() error = %v", err)
	}
	raw, ok := v.Raw.(*xdr.TransactionHistoryEntry)
	if !ok {
		t.Fatalf("Raw is %T, want *xdr.TransactionHistoryEntry", v.Raw)
	}
	if raw.LedgerSeq != 55278432 || raw.Ext.V != 0 {
		t.Errorf("entry = ledger %d ext %d, want ledger 55278432 ext 0", raw.LedgerSeq, raw.Ext.V)
	}

	// The leading integer is a ledger sequence, and saying so is the point.
	seq := v.Node.Find("LedgerSeq")
	if seq == nil || seq.Note != "ledger 55278432" {
		t.Errorf("LedgerSeq = %v, want a ledger note", seq)
	}
	if hash := v.Node.Find("TxSet.PreviousLedgerHash"); hash == nil || hash.Note != historyHashNote {
		t.Errorf("TxSet.PreviousLedgerHash = %v, want note %q", hash, historyHashNote)
	}
	txs := v.Node.Find("TxSet.Txs")
	if txs == nil || len(txs.Children) != 1 {
		t.Fatalf("TxSet.Txs = %v, want one envelope", txs)
	}
	if typ := v.Node.Find("TxSet.Txs[0].Type"); typ == nil || typ.Value != "EnvelopeTypeEnvelopeTypeTxV0" {
		t.Errorf("Txs[0].Type = %v, want EnvelopeTypeEnvelopeTypeTxV0", typ)
	}
}

func TestDecodeTransactionHistoryEntryV1(t *testing.T) {
	t.Parallel()

	v, err := lens.DecodeAs(loadFixture(t, "tx_history_entry_v1"), "TransactionHistoryEntry")
	if err != nil {
		t.Fatalf("DecodeAs() error = %v", err)
	}
	raw, ok := v.Raw.(*xdr.TransactionHistoryEntry)
	if !ok {
		t.Fatalf("Raw is %T, want *xdr.TransactionHistoryEntry", v.Raw)
	}
	if raw.LedgerSeq != 55278433 || raw.Ext.V != 1 {
		t.Errorf("entry = ledger %d ext %d, want ledger 55278433 ext 1", raw.LedgerSeq, raw.Ext.V)
	}
	if raw.Ext.GeneralizedTxSet == nil || raw.Ext.GeneralizedTxSet.V1TxSet == nil {
		t.Fatal("Ext.GeneralizedTxSet.V1TxSet is nil, want the generalized set")
	}
	if seq := v.Node.Find("LedgerSeq"); seq == nil || seq.Note != "ledger 55278433" {
		t.Errorf("LedgerSeq = %v, want a ledger note", seq)
	}
	phases := v.Node.Find("Ext.GeneralizedTxSet.V1TxSet.Phases")
	if phases == nil || len(phases.Children) != 2 {
		t.Errorf("V1TxSet.Phases = %v, want two phases", phases)
	}
}

func TestDecodeGeneralizedTransactionSetFixture(t *testing.T) {
	t.Parallel()

	v, err := lens.DecodeAs(loadFixture(t, "generalized_tx_set"), "GeneralizedTransactionSet")
	if err != nil {
		t.Fatalf("DecodeAs() error = %v", err)
	}
	raw, ok := v.Raw.(*xdr.GeneralizedTransactionSet)
	if !ok {
		t.Fatalf("Raw is %T, want *xdr.GeneralizedTransactionSet", v.Raw)
	}
	if raw.V != 1 || raw.V1TxSet == nil || len(raw.V1TxSet.Phases) != 2 {
		t.Fatalf("set = %+v, want a v1 set with two phases", raw)
	}

	baseFee := v.Node.Find("V1TxSet.Phases[0].V0Components[0].TxsMaybeDiscountedFee.BaseFee")
	if baseFee == nil || baseFee.Note != "0.0000100 (stroops: 100)" {
		t.Errorf("classic phase BaseFee = %v, want a stroop note", baseFee)
	}
	parallel := v.Node.Find("V1TxSet.Phases[1].ParallelTxsComponent.ExecutionStages")
	if parallel == nil || len(parallel.Children) != 1 {
		t.Errorf("parallel phase stages = %v, want one stage", parallel)
	}
}

func TestDecodeTransactionHistoryResultEntryFixture(t *testing.T) {
	t.Parallel()

	v, err := lens.DecodeAs(loadFixture(t, "tx_history_result_entry"), "TransactionHistoryResultEntry")
	if err != nil {
		t.Fatalf("DecodeAs() error = %v", err)
	}
	raw, ok := v.Raw.(*xdr.TransactionHistoryResultEntry)
	if !ok {
		t.Fatalf("Raw is %T, want *xdr.TransactionHistoryResultEntry", v.Raw)
	}
	if raw.LedgerSeq != 55278432 || raw.Ext.V != 0 {
		t.Errorf("entry = ledger %d ext %d, want ledger 55278432 ext 0", raw.LedgerSeq, raw.Ext.V)
	}

	if seq := v.Node.Find("LedgerSeq"); seq == nil || seq.Note != "ledger 55278432" {
		t.Errorf("LedgerSeq = %v, want a ledger note", seq)
	}
	if code := v.Node.Find("TxResultSet.Results[0].Result.Result.Code"); code == nil ||
		code.Value != "TransactionResultCodeTxSuccess" {
		t.Errorf("result code = %v, want TransactionResultCodeTxSuccess", code)
	}
	if fee := v.Node.Find("TxResultSet.Results[0].Result.FeeCharged"); fee == nil ||
		fee.Note != "0.0000100 (stroops: 100)" {
		t.Errorf("FeeCharged = %v, want a stroop note", fee)
	}
}

func TestDecodeLedgerCloseMetaBatchFixture(t *testing.T) {
	t.Parallel()

	v, err := lens.DecodeAs(loadFixture(t, "ledger_close_meta_batch"), "LedgerCloseMetaBatch")
	if err != nil {
		t.Fatalf("DecodeAs() error = %v", err)
	}
	raw, ok := v.Raw.(*xdr.LedgerCloseMetaBatch)
	if !ok {
		t.Fatalf("Raw is %T, want *xdr.LedgerCloseMetaBatch", v.Raw)
	}
	if raw.StartSequence != 55278432 || raw.EndSequence != 55278432 {
		t.Errorf("batch = [%d, %d], want [55278432, 55278432]", raw.StartSequence, raw.EndSequence)
	}

	start := v.Node.Find("StartSequence")
	if start == nil || start.Note != "first ledger in batch 55278432" {
		t.Errorf("StartSequence = %v, want a first-ledger note", start)
	}
	end := v.Node.Find("EndSequence")
	if end == nil || end.Note != "last ledger in batch 55278432" {
		t.Errorf("EndSequence = %v, want a last-ledger note", end)
	}
}

func TestDecodeTransactionHistoryRejectsTruncatedInput(t *testing.T) {
	t.Parallel()

	tests := []struct {
		fixture  string
		typeName string
	}{
		{"tx_history_entry_v0", "TransactionHistoryEntry"},
		{"tx_history_entry_v1", "TransactionHistoryEntry"},
		{"tx_history_result_entry", "TransactionHistoryResultEntry"},
		{"generalized_tx_set", "GeneralizedTransactionSet"},
		{"ledger_close_meta_batch", "LedgerCloseMetaBatch"},
	}

	for _, tc := range tests {
		t.Run(tc.fixture, func(t *testing.T) {
			t.Parallel()

			raw := loadFixtureRaw(tc.fixture)
			if _, err := lens.DecodeAs(raw[:len(raw)/2], tc.typeName); err == nil {
				t.Errorf("DecodeAs(%s truncated) error = nil, want an error", tc.fixture)
			}
		})
	}
}

func TestTransactionHistoryDetectionIsUnambiguous(t *testing.T) {
	t.Parallel()

	tests := []struct {
		fixture  string
		wantType string
	}{
		{"tx_history_entry_v0", "TransactionHistoryEntry"},
		{"tx_history_entry_v1", "TransactionHistoryEntry"},
		{"tx_history_result_entry", "TransactionHistoryResultEntry"},
		{"generalized_tx_set", "GeneralizedTransactionSet"},
		{"ledger_close_meta_batch", "LedgerCloseMetaBatch"},
	}

	for _, tc := range tests {
		t.Run(tc.fixture, func(t *testing.T) {
			t.Parallel()

			cands, err := lens.Detect(loadFixture(t, tc.fixture))
			if err != nil {
				t.Fatalf("Detect() error = %v", err)
			}
			if len(cands) != 1 || cands[0].Type != tc.wantType {
				names := make([]string, 0, len(cands))
				for _, c := range cands {
					names = append(names, c.Type)
				}
				t.Errorf("Detect() = [%s], want exactly [%s]", strings.Join(names, ", "), tc.wantType)
			}
		})
	}
}
