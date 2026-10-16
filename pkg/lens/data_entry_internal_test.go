package lens

import (
	"reflect"
	"testing"

	"github.com/stellar/go-stellar-sdk/xdr"
)

func TestEnrichDataValue(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		value    xdr.DataValue
		wantNote string
		wantRaw  any
	}{
		{"empty value", xdr.DataValue{}, "empty", ""},
		{"text value", xdr.DataValue("hello"), "UTF-8 text, 5 byte(s)", "hello"},
		{"binary value", xdr.DataValue{0x00, 0xff}, "binary, 2 byte(s)", "00ff"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			note, raw, ok := enrichDataValue(reflect.ValueOf(tc.value))
			if !ok || note != tc.wantNote || raw != tc.wantRaw {
				t.Errorf("enrichDataValue(%v) = (%q, %v, %v), want (%q, %v, true)",
					tc.value, note, raw, ok, tc.wantNote, tc.wantRaw)
			}
		})
	}
}

func TestEnrichDataValueIgnoresOtherTypes(t *testing.T) {
	t.Parallel()

	if _, _, ok := enrichDataValue(reflect.ValueOf(xdr.String64("x"))); ok {
		t.Error("enrichDataValue(String64) ok = true, want false")
	}
}

// TestEnrichDataValueReachesTheWalker pins the type-based registration: the
// payload must be rendered through the enricher, not as a byte list.
func TestEnrichDataValueReachesTheWalker(t *testing.T) {
	t.Parallel()

	entry := xdr.DataEntry{DataName: "k", DataValue: xdr.DataValue("v")}
	node := buildNode("DataEntry", reflect.ValueOf(entry), 0)
	if node == nil {
		t.Fatal("buildNode(DataEntry) returned nil")
	}
	value := node.Child("DataValue")
	if value == nil || value.Note != "UTF-8 text, 1 byte(s)" || value.Value != "v" {
		t.Errorf("DataValue = %v, want the text rendering", value)
	}
}
