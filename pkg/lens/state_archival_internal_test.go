package lens

import (
	"reflect"
	"testing"

	"github.com/stellar/go-stellar-sdk/xdr"
)

func TestLiveUntilLedgerNote(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		value    any
		wantNote string
		wantOK   bool
	}{
		{"live entry", xdr.Uint32(55281000), "live through ledger 55281000", true},
		{"expired entry", xdr.Uint32(0), "already expired (evict at next close)", true},
		{"wrong type declines", xdr.Int64(1), "", false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			note, ok := liveUntilLedgerNote(reflect.ValueOf(tc.value))
			if ok != tc.wantOK || note != tc.wantNote {
				t.Errorf("liveUntilLedgerNote(%v) = (%q, %v), want (%q, %v)",
					tc.value, note, ok, tc.wantNote, tc.wantOK)
			}
		})
	}
}

// TestLiveUntilLedgerNoteReachesTheWalker pins that the note is registered
// under the field name the XDR actually uses, so the registration cannot
// silently drift from the schema.
func TestLiveUntilLedgerNoteReachesTheWalker(t *testing.T) {
	t.Parallel()

	entry := xdr.TtlEntry{KeyHash: xdr.Hash{}, LiveUntilLedgerSeq: 1000}
	node := buildNode("TtlEntry", reflect.ValueOf(entry), 0)
	if node == nil {
		t.Fatal("buildNode(TtlEntry) returned nil")
	}
	live := node.Child("LiveUntilLedgerSeq")
	if live == nil {
		t.Fatal("could not find LiveUntilLedgerSeq on the built node")
	}
	if live.Note != "live through ledger 1000" {
		t.Errorf("LiveUntilLedgerSeq note = %q, want %q", live.Note, "live through ledger 1000")
	}
}
