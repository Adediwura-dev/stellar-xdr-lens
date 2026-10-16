package lens

import (
	"reflect"
	"testing"

	"github.com/stellar/go-stellar-sdk/xdr"
)

func TestLedgerSeqNote(t *testing.T) {
	t.Parallel()

	note, ok := ledgerSeqNote(reflect.ValueOf(xdr.Uint32(55278432)))
	if !ok || note != "ledger 55278432" {
		t.Errorf("ledgerSeqNote(55278432) = (%q, %v), want (ledger 55278432, true)", note, ok)
	}

	// Ledger zero is the genesis placeholder, not a sequence worth naming.
	if note, ok := ledgerSeqNote(reflect.ValueOf(xdr.Uint32(0))); ok {
		t.Errorf("ledgerSeqNote(0) = (%q, true), want ok=false", note)
	}
	if _, ok := ledgerSeqNote(reflect.ValueOf(xdr.Int64(55278432))); ok {
		t.Error("ledgerSeqNote(Int64) succeeded, want ok=false")
	}
}

func TestBatchBoundaryNotes(t *testing.T) {
	t.Parallel()

	first, ok := firstLedgerNote(reflect.ValueOf(xdr.Uint32(55278432)))
	if !ok || first != "first ledger in batch 55278432" {
		t.Errorf("firstLedgerNote() = (%q, %v), want a first-ledger note", first, ok)
	}
	last, ok := lastLedgerNote(reflect.ValueOf(xdr.Uint32(55278432)))
	if !ok || last != "last ledger in batch 55278432" {
		t.Errorf("lastLedgerNote() = (%q, %v), want a last-ledger note", last, ok)
	}

	// Unlike LedgerSeq, a zero batch boundary is still worth naming: an
	// unset bound is exactly what a caller needs to see.
	if note, ok := firstLedgerNote(reflect.ValueOf(xdr.Uint32(0))); !ok || note != "first ledger in batch 0" {
		t.Errorf("firstLedgerNote(0) = (%q, %v), want the zero bound named", note, ok)
	}
	if note, ok := lastLedgerNote(reflect.ValueOf(xdr.Uint32(0))); !ok || note != "last ledger in batch 0" {
		t.Errorf("lastLedgerNote(0) = (%q, %v), want the zero bound named", note, ok)
	}

	for name, fn := range map[string]func(reflect.Value) (string, bool){
		"firstLedgerNote": firstLedgerNote,
		"lastLedgerNote":  lastLedgerNote,
	} {
		if _, ok := fn(reflect.ValueOf(xdr.Int64(1))); ok {
			t.Errorf("%s(Int64) succeeded, want ok=false", name)
		}
	}
}
