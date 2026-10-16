package lens

import (
	"reflect"
	"testing"

	"github.com/stellar/go-stellar-sdk/xdr"
)

func TestProtocolVersionNote(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		value    any
		wantNote string
		wantOK   bool
	}{
		{"released version", xdr.Uint32(22), "protocol 22", true},
		{"zero version declines", xdr.Uint32(0), "", false},
		{"wrong type declines", xdr.Int64(22), "", false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			note, ok := protocolVersionNote(reflect.ValueOf(tc.value))
			if ok != tc.wantOK || note != tc.wantNote {
				t.Errorf("protocolVersionNote(%v) = (%q, %v), want (%q, %v)",
					tc.value, note, ok, tc.wantNote, tc.wantOK)
			}
		})
	}
}

// TestEnrichAmountFieldAcceptShapes pins the three shapes a stroop amount
// arrives in: a plain Int64, a Uint32 protocol parameter, and a pointer-
// wrapped union arm.
func TestEnrichAmountFieldAcceptShapes(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		field    string
		value    any
		wantNote string
		wantOK   bool
	}{
		{"int64 amount", "TotalCoins", xdr.Int64(1045912973109999952), "104591297310.9999952 (stroops: 1045912973109999952)", true},
		{"uint32 parameter", "BaseReserve", xdr.Uint32(5000000), "0.5000000 (stroops: 5000000)", true},
		{"pointer parameter", "BaseFee", ptrTo(xdr.Uint32(100)), "0.0000100 (stroops: 100)", true},
		{"unknown field declines", "NotAnAmount", xdr.Int64(1), "", false},
		{"wrong type declines", "TotalCoins", "not an amount", "", false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			note, ok := enrichAmountField(tc.field, reflect.ValueOf(tc.value))
			if ok != tc.wantOK || note != tc.wantNote {
				t.Errorf("enrichAmountField(%q, %T) = (%q, %v), want (%q, %v)",
					tc.field, tc.value, note, ok, tc.wantNote, tc.wantOK)
			}
		})
	}
}

func TestEnrichAmountFieldDeclinesNilPointer(t *testing.T) {
	t.Parallel()

	var p *xdr.Uint32
	if note, ok := enrichAmountField("BaseFee", reflect.ValueOf(p)); ok {
		t.Errorf("enrichAmountField(nil pointer) = (%q, true), want ok=false", note)
	}
}

// TestEnrichFieldAppliesRegisteredNotes covers the fieldNotes seam itself:
// registered fields get their note, unregistered fields decline.
func TestEnrichFieldAppliesRegisteredNotes(t *testing.T) {
	t.Parallel()

	note, ok := enrichField("LedgerVersion", reflect.ValueOf(xdr.Uint32(22)))
	if !ok || note != "protocol 22" {
		t.Errorf("enrichField(LedgerVersion) = (%q, %v), want (protocol 22, true)", note, ok)
	}

	if note, ok := enrichField("Unregistered", reflect.ValueOf(xdr.Uint32(1))); ok {
		t.Errorf("enrichField(unregistered) = (%q, true), want ok=false", note)
	}
}

// TestEnrichFieldRecoversFromPanickingNote proves a note function that panics
// on hostile input degrades to no note instead of taking the process down.
//
// It deliberately does not run in parallel: it registers a note in the shared
// fieldNotes map, and parallel tests are sequenced after sequential ones so
// the registry is never written while another test reads it.
func TestEnrichFieldRecoversFromPanickingNote(t *testing.T) {
	const field = "TestPanickingNote"
	fieldNotes[field] = func(reflect.Value) (string, bool) { panic("boom") }
	t.Cleanup(func() { delete(fieldNotes, field) })

	if note, ok := enrichField(field, reflect.ValueOf(1)); ok {
		t.Errorf("enrichField(panicking note) = (%q, true), want ok=false", note)
	}
}

func ptrTo[T any](v T) *T { return &v }
