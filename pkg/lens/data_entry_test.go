package lens_test

import (
	"testing"

	"github.com/stellar/go-stellar-sdk/xdr"

	"github.com/stellar-optics/stellar-xdr-lens/pkg/lens"
)

const dataEntryText = `{"network":"mainnet","protocol":22}`

func TestDecodeDataEntryText(t *testing.T) {
	t.Parallel()

	v, err := lens.DecodeAs(loadFixture(t, "data_entry_text"), "DataEntry")
	if err != nil {
		t.Fatalf("DecodeAs() error = %v", err)
	}
	if v.Type != "DataEntry" {
		t.Errorf("type = %q, want DataEntry", v.Type)
	}
	raw, ok := v.Raw.(*xdr.DataEntry)
	if !ok {
		t.Fatalf("Raw is %T, want *xdr.DataEntry", v.Raw)
	}
	if string(raw.DataValue) != dataEntryText {
		t.Errorf("DataValue = %q, want %q", raw.DataValue, dataEntryText)
	}
	if name := v.Node.Find("DataName"); name == nil || name.Value != "config.json" {
		t.Errorf("DataName = %v, want config.json", name)
	}

	// The whole point of the issue: a JSON payload must come out as text,
	// with the size stated so the note can be trusted.
	value := v.Node.Find("DataValue")
	if value == nil {
		t.Fatal("DataValue node is missing")
	}
	if value.Value != dataEntryText {
		t.Errorf("DataValue value = %v, want the decoded text", value.Value)
	}
	if value.Note != "UTF-8 text, 35 byte(s)" {
		t.Errorf("DataValue note = %q, want %q", value.Note, "UTF-8 text, 35 byte(s)")
	}
}

func TestDecodeDataEntryBinary(t *testing.T) {
	t.Parallel()

	v, err := lens.DecodeAs(loadFixture(t, "data_entry_binary"), "DataEntry")
	if err != nil {
		t.Fatalf("DecodeAs() error = %v", err)
	}
	raw, ok := v.Raw.(*xdr.DataEntry)
	if !ok {
		t.Fatalf("Raw is %T, want *xdr.DataEntry", v.Raw)
	}
	if raw.DataName != "avatar.png" {
		t.Errorf("DataName = %q, want avatar.png", raw.DataName)
	}

	// The PNG signature starts with 0x89, which is not valid UTF-8: the
	// value must stay hex and must not be mangled by a lossy conversion.
	value := v.Node.Find("DataValue")
	if value == nil {
		t.Fatal("DataValue node is missing")
	}
	const hexValue = "89504e470d0a1a0a00ff1020"
	if value.Value != hexValue {
		t.Errorf("DataValue value = %v, want %q", value.Value, hexValue)
	}
	if value.Note != "binary, 12 byte(s)" {
		t.Errorf("DataValue note = %q, want %q", value.Note, "binary, 12 byte(s)")
	}
}

func TestDescribeDataValue(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		value       []byte
		wantDisplay string
		wantTextual bool
	}{
		{"json text", []byte(dataEntryText), dataEntryText, true},
		{"plain ascii", []byte("hello"), "hello", true},
		{"accented utf-8", []byte("caf\xc3\xa9"), "caf\xc3\xa9", true},
		{"empty", nil, "", false},
		{"newline is not printable on one line", []byte("a\nb"), "610a62", false},
		{"tab is not printable on one line", []byte("a\tb"), "610962", false},
		{"nul byte is binary", []byte{'a', 0x00, 'b'}, "610062", false},
		{"invalid utf-8 is binary", []byte{0xff, 0xfe}, "fffe", false},
		{"png signature is binary", []byte{0x89, 'P', 'N', 'G'}, "89504e47", false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			display, textual := lens.DescribeDataValue(tc.value)
			if display != tc.wantDisplay || textual != tc.wantTextual {
				t.Errorf("DescribeDataValue(%v) = (%q, %v), want (%q, %v)",
					tc.value, display, textual, tc.wantDisplay, tc.wantTextual)
			}
		})
	}
}

func TestDataEntryRejectsTruncatedInput(t *testing.T) {
	t.Parallel()

	if _, err := lens.DecodeAs(loadFixtureRaw("data_entry_text")[:32], "DataEntry"); err == nil {
		t.Error("DecodeAs(truncated) error = nil, want an error")
	}
}
