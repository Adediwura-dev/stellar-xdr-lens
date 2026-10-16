package lens

import (
	"math"
	"reflect"
	"testing"

	"github.com/stellar/go-stellar-sdk/xdr"
	xdr3 "github.com/stellar/go-xdr/xdr3"
)

func TestSpecEntryLabel(t *testing.T) {
	t.Parallel()

	tests := []struct {
		kind xdr.ScSpecEntryKind
		want string
	}{
		{xdr.ScSpecEntryKindScSpecEntryFunctionV0, "function"},
		{xdr.ScSpecEntryKindScSpecEntryUdtStructV0, "struct"},
		{xdr.ScSpecEntryKindScSpecEntryUdtUnionV0, "union"},
		{xdr.ScSpecEntryKindScSpecEntryUdtEnumV0, "enum"},
		{xdr.ScSpecEntryKindScSpecEntryUdtErrorEnumV0, "error enum"},
		{xdr.ScSpecEntryKindScSpecEntryEventV0, "event"},
		{xdr.ScSpecEntryKind(99), "entry"},
	}

	for _, tc := range tests {
		t.Run(tc.want, func(t *testing.T) {
			t.Parallel()

			if got := specEntryLabel(tc.kind); got != tc.want {
				t.Errorf("specEntryLabel(%d) = %q, want %q", tc.kind, got, tc.want)
			}
		})
	}
}

func TestReadU32(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		input   []byte
		want    uint32
		wantN   int
		wantErr bool
	}{
		{"single byte", []byte{0x05}, 5, 1, false},
		{"zero", []byte{0x00}, 0, 1, false},
		{"two bytes", []byte{0x80, 0x01}, 128, 2, false},
		{"max u32", []byte{0xff, 0xff, 0xff, 0xff, 0x0f}, math.MaxUint32, 5, false},
		{"truncated continuation", []byte{0x80}, 0, 0, true},
		{"empty", nil, 0, 0, true},
		{"overflow beyond u32", []byte{0xff, 0xff, 0xff, 0xff, 0x10}, 0, 0, true},
		{"overlong encoding", []byte{0x80, 0x80, 0x80, 0x80, 0x80, 0x00}, 0, 0, true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got, n, err := readU32(tc.input)
			if (err != nil) != tc.wantErr {
				t.Fatalf("readU32(%v) error = %v, wantErr %v", tc.input, err, tc.wantErr)
			}
			if tc.wantErr {
				return
			}
			if got != tc.want || n != tc.wantN {
				t.Errorf("readU32(%v) = (%d, %d), want (%d, %d)", tc.input, got, n, tc.want, tc.wantN)
			}
		})
	}
}

func TestIsWASM(t *testing.T) {
	t.Parallel()

	header := []byte("\x00asm\x01\x00\x00\x00")
	tests := []struct {
		name string
		code []byte
		want bool
	}{
		{"valid header", header, true},
		{"header with trailing section", append(append([]byte{}, header...), 0x00, 0x00), true},
		{"empty", nil, false},
		{"truncated header", header[:7], false},
		{"wrong magic", append([]byte("asm\x00"), header[4:]...), false},
		{"version two", []byte("\x00asm\x02\x00\x00\x00"), false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			if got := isWASM(tc.code); got != tc.want {
				t.Errorf("isWASM(%v) = %v, want %v", tc.code, got, tc.want)
			}
		})
	}
}

func TestContractCodeNote(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		code []byte
		err  error
		want string
	}{
		{"empty", nil, nil, "empty"},
		{"not wasm", []byte("nope"), ErrNotWASM, "not a WASM module, 4 byte(s)"},
		{"malformed", []byte("\x00asm\x01\x00\x00\x00"), ErrMalformedWASM, "malformed WASM module, 8 byte(s)"},
		{"valid", []byte("\x00asm\x01\x00\x00\x00"), nil, "WASM module, 8 byte(s)"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			if got := contractCodeNote(tc.code, tc.err); got != tc.want {
				t.Errorf("contractCodeNote() = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestCustomSectionTruncatedName(t *testing.T) {
	t.Parallel()

	if _, _, err := customSection([]byte{0x05, 'a'}); err == nil {
		t.Error("customSection() error = nil, want a truncation error")
	}
}

func TestDecodeSectionStreamNoProgress(t *testing.T) {
	t.Parallel()

	err := decodeSectionStream([]byte{0x00, 0x00, 0x00, 0x00}, func(*xdr3.Decoder) (int, error) {
		return 0, nil
	})
	if err == nil {
		t.Error("decodeSectionStream() error = nil, want a no-progress error")
	}
}

func TestDecorateContractCodeIgnoresOtherTypes(t *testing.T) {
	t.Parallel()

	node := &Node{Name: "AccountEntry", Kind: KindStruct}
	decorateContractCode(node, xdr.AccountEntry{})
	if len(node.Children) != 0 {
		t.Errorf("decorateContractCode(AccountEntry) added %d children, want none", len(node.Children))
	}
}

// TestContractCodeHookIsRegistered pins the registry entry: without it the
// Spec/EnvMeta/Meta children never appear, because decoration happens only
// through the node hook seam.
func TestContractCodeHookIsRegistered(t *testing.T) {
	t.Parallel()

	if _, ok := nodeHooks[reflect.TypeOf(xdr.ContractCodeEntry{})]; !ok {
		t.Fatal("no node hook registered for xdr.ContractCodeEntry")
	}
}
