package lens

import (
	"reflect"
	"testing"

	"github.com/stellar/go-stellar-sdk/xdr"
)

func TestAccountExtensionNote(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		value    any
		wantNote string
		wantOK   bool
	}{
		{"account ext v0 is the quiet default", xdr.AccountEntryExt{V: 0}, "", false},
		{"account ext v1 adds liabilities", xdr.AccountEntryExt{V: 1}, "liabilities", true},
		{"v1 ext v0 stays quiet", xdr.AccountEntryExtensionV1Ext{V: 0}, "", false},
		{"v1 ext v2 adds sponsorship", xdr.AccountEntryExtensionV1Ext{V: 2}, "sponsorship counters", true},
		{"v2 ext v0 stays quiet", xdr.AccountEntryExtensionV2Ext{V: 0}, "", false},
		{"v2 ext v3 adds sequence ledger", xdr.AccountEntryExtensionV2Ext{V: 3}, "sequence ledger and time", true},
		{"other unions decline", xdr.DataEntryExt{V: 0}, "", false},
		{"non-union types decline", xdr.Uint32(1), "", false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			note, ok := accountExtensionNote(reflect.ValueOf(tc.value))
			if ok != tc.wantOK || note != tc.wantNote {
				t.Errorf("accountExtensionNote(%v) = (%q, %v), want (%q, %v)",
					tc.value, note, ok, tc.wantNote, tc.wantOK)
			}
		})
	}
}

// TestAccountExtensionNoteReachesTheWalker pins the hook registration: the
// note lives on the discriminant child, which is where the formatters render
// it.
func TestAccountExtensionNoteReachesTheWalker(t *testing.T) {
	t.Parallel()

	entry := xdr.AccountEntryExt{
		V: 1,
		V1: &xdr.AccountEntryExtensionV1{
			Ext: xdr.AccountEntryExtensionV1Ext{V: 0},
		},
	}
	node := buildNode("Ext", reflect.ValueOf(entry), 0)
	if node == nil {
		t.Fatal("buildNode(AccountEntryExt) returned nil")
	}
	if v := node.Child("V"); v == nil || v.Note != "liabilities" {
		t.Errorf("Ext.V = %v, want a liabilities note", v)
	}
	if v := node.Child("V1").Child("Ext").Child("V"); v == nil || v.Note != "" {
		t.Errorf("V1.Ext.V = %v, want no note for the zero arm", v)
	}
}

func TestNumSponsoringNote(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		value    any
		wantNote string
		wantOK   bool
	}{
		{"zero is unremarkable", xdr.Uint32(0), "", false},
		{"one entry", xdr.Uint32(1), "sponsoring 1 ledger entry", true},
		{"many entries", xdr.Uint32(3), "sponsoring 3 ledger entries", true},
		{"wrong type declines", xdr.Int64(1), "", false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			note, ok := numSponsoringNote(reflect.ValueOf(tc.value))
			if ok != tc.wantOK || note != tc.wantNote {
				t.Errorf("numSponsoringNote(%v) = (%q, %v), want (%q, %v)",
					tc.value, note, ok, tc.wantNote, tc.wantOK)
			}
		})
	}
}

func TestNumSponsoredNote(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		value    any
		wantNote string
		wantOK   bool
	}{
		{"zero is unremarkable", xdr.Uint32(0), "", false},
		{"one entry", xdr.Uint32(1), "1 ledger entry sponsored by another account", true},
		{"many entries", xdr.Uint32(4), "4 ledger entries sponsored by another account", true},
		{"wrong type declines", xdr.Int64(1), "", false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			note, ok := numSponsoredNote(reflect.ValueOf(tc.value))
			if ok != tc.wantOK || note != tc.wantNote {
				t.Errorf("numSponsoredNote(%v) = (%q, %v), want (%q, %v)",
					tc.value, note, ok, tc.wantNote, tc.wantOK)
			}
		})
	}
}

func TestSeqLedgerNote(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		value    any
		wantNote string
		wantOK   bool
	}{
		{"ledger number", xdr.Uint32(55278432), "seqNum set in ledger 55278432", true},
		{"wrong type declines", xdr.Int64(1), "", false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			note, ok := seqLedgerNote(reflect.ValueOf(tc.value))
			if ok != tc.wantOK || note != tc.wantNote {
				t.Errorf("seqLedgerNote(%v) = (%q, %v), want (%q, %v)",
					tc.value, note, ok, tc.wantNote, tc.wantOK)
			}
		})
	}
}

func TestPluralSuffix(t *testing.T) {
	t.Parallel()

	if got := pluralSuffix(1); got != "y" {
		t.Errorf("pluralSuffix(1) = %q, want y", got)
	}
	if got := pluralSuffix(0); got != "ies" {
		t.Errorf("pluralSuffix(0) = %q, want ies", got)
	}
	if got := pluralSuffix(2); got != "ies" {
		t.Errorf("pluralSuffix(2) = %q, want ies", got)
	}
}
