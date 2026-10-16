package lens_test

import (
	"errors"
	"testing"

	"github.com/stellar/go-stellar-sdk/xdr"

	"github.com/stellar-optics/stellar-xdr-lens/pkg/lens"
)

const (
	contractSpecName    = "contractspecv0"
	contractEnvMetaName = "contractenvmetav0"
	contractMetaName    = "contractmetav0"
)

// leb128 encodes a WASM u32. The test fixtures need to build modules by hand
// to exercise malformed layouts that the official generator cannot produce.
func leb128(v uint32) []byte {
	var out []byte
	for {
		b := byte(v & 0x7f)
		v >>= 7
		if v != 0 {
			b |= 0x80
		}
		out = append(out, b)
		if v == 0 {
			return out
		}
	}
}

func wasmSection(id byte, payload []byte) []byte {
	out := []byte{id}
	out = append(out, leb128(uint32(len(payload)))...)
	return append(out, payload...)
}

func wasmModule(sections ...[]byte) []byte {
	code := []byte("\x00asm\x01\x00\x00\x00")
	for _, section := range sections {
		code = append(code, section...)
	}
	return code
}

func customSection(name string, payload []byte) []byte {
	body := append(leb128(uint32(len(name))), []byte(name)...)
	return wasmSection(0, append(body, payload...))
}

func TestDecodeContractCode(t *testing.T) {
	t.Parallel()

	v, err := lens.DecodeAs(loadFixture(t, "contract_code"), "ContractCodeEntry")
	if err != nil {
		t.Fatalf("DecodeAs() error = %v", err)
	}
	raw, ok := v.Raw.(*xdr.ContractCodeEntry)
	if !ok {
		t.Fatalf("Raw is %T, want *xdr.ContractCodeEntry", v.Raw)
	}
	if raw.Ext.V != 0 {
		t.Errorf("Ext.V = %d, want 0", raw.Ext.V)
	}

	// The bytecode leaf states the module size so it is not just a wall of
	// hex.
	code := v.Node.Find("Code")
	if code == nil || code.Note != "WASM module, 699 byte(s)" {
		t.Errorf("Code = %v, want a WASM module note", code)
	}

	// Every declaration kind gets its own human name.
	spec := v.Node.Find("Spec")
	if spec == nil {
		t.Fatal("Spec node is missing")
	}
	wantNames := []string{"function", "struct", "union", "enum", "error enum", "event"}
	if len(spec.Children) != len(wantNames) {
		t.Fatalf("Spec has %d entries, want %d", len(spec.Children), len(wantNames))
	}
	for i, want := range wantNames {
		if spec.Children[i].Name != want {
			t.Errorf("Spec[%d].Name = %q, want %q", i, spec.Children[i].Name, want)
		}
	}
	if name := spec.Children[0].Find("FunctionV0.Name"); name == nil || name.Value != "increment" {
		t.Errorf("function name = %v, want increment", name)
	}
	if name := spec.Children[1].Find("UdtStructV0.Name"); name == nil || name.Value != "Point" {
		t.Errorf("struct name = %v, want Point", name)
	}
	if name := spec.Children[4].Find("UdtErrorEnumV0.Name"); name == nil || name.Value != "Error" {
		t.Errorf("error enum name = %v, want Error", name)
	}

	// The host interface version says which protocol the contract was built
	// against; it is the single most useful field in the env metadata.
	protocol := v.Node.Find("EnvMeta.interface version.InterfaceVersion.Protocol")
	if protocol == nil || protocol.Value != uint64(22) {
		t.Errorf("EnvMeta protocol = %v, want 22", protocol)
	}

	// Contract metadata is free-form key/value pairs; both must survive.
	meta := v.Node.Find("Meta")
	if meta == nil || len(meta.Children) != 2 {
		t.Fatalf("Meta = %v, want two entries", meta)
	}
	if key := meta.Children[0].Find("V0.Key"); key == nil || key.Value != "rsver" {
		t.Errorf("Meta[0].Key = %v, want rsver", key)
	}
	if val := meta.Children[0].Find("V0.Val"); val == nil || val.Value != "1.84.0" {
		t.Errorf("Meta[0].Val = %v, want 1.84.0", val)
	}
	if key := meta.Children[1].Find("V0.Key"); key == nil || key.Value != "rssdkver" {
		t.Errorf("Meta[1].Key = %v, want rssdkver", key)
	}
}

func TestDecodeContractCodeV1(t *testing.T) {
	t.Parallel()

	v, err := lens.DecodeAs(loadFixture(t, "contract_code_v1"), "ContractCodeEntry")
	if err != nil {
		t.Fatalf("DecodeAs() error = %v", err)
	}
	raw, ok := v.Raw.(*xdr.ContractCodeEntry)
	if !ok {
		t.Fatalf("Raw is %T, want *xdr.ContractCodeEntry", v.Raw)
	}
	if raw.Ext.V != 1 || raw.Ext.V1 == nil {
		t.Fatalf("Ext = %+v, want the v1 extension", raw.Ext)
	}
	if raw.Ext.V1.CostInputs.NInstructions != 1497 {
		t.Errorf("NInstructions = %d, want 1497", raw.Ext.V1.CostInputs.NInstructions)
	}

	// Cost inputs live under the v1 extension, and the module itself has no
	// metadata sections: all three derived lists must be absent, not empty.
	if got := v.Node.Find("Ext.V1.CostInputs.NInstructions"); got == nil || got.Value != uint64(1497) {
		t.Errorf("CostInputs.NInstructions = %v, want 1497", got)
	}
	if got := v.Node.Find("Code"); got == nil || got.Note != "WASM module, 24 byte(s)" {
		t.Errorf("Code = %v, want a WASM module note", got)
	}
	for _, path := range []string{"Spec", "EnvMeta", "Meta"} {
		if got := v.Node.Find(path); got != nil {
			t.Errorf("%s = %v, want it to be absent", path, got)
		}
	}
}

func TestParseContractCodeMatchesFixture(t *testing.T) {
	t.Parallel()

	v, err := lens.DecodeAs(loadFixture(t, "contract_code"), "ContractCodeEntry")
	if err != nil {
		t.Fatalf("DecodeAs() error = %v", err)
	}
	raw := v.Raw.(*xdr.ContractCodeEntry)

	parsed, err := lens.ParseContractCode(raw.Code)
	if err != nil {
		t.Fatalf("ParseContractCode() error = %v", err)
	}
	if parsed.Size != len(raw.Code) {
		t.Errorf("Size = %d, want %d", parsed.Size, len(raw.Code))
	}
	if len(parsed.Spec) != 6 {
		t.Errorf("Spec has %d entries, want 6", len(parsed.Spec))
	}
	if len(parsed.EnvMeta) != 1 || len(parsed.Meta) != 2 {
		t.Errorf("EnvMeta/Meta = %d/%d, want 1/2", len(parsed.EnvMeta), len(parsed.Meta))
	}
}

func TestParseContractCodeRejectsNonWASM(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		code []byte
	}{
		{"empty", nil},
		{"plain text", []byte("hello world, definitely not wasm")},
		{"magic only", []byte("\x00asm")},
		{"magic with version zero", []byte("\x00asm\x00\x00\x00\x00")},
		{"magic with version two", []byte("\x00asm\x02\x00\x00\x00")},
		{"wrong magic", []byte("asm\x00\x01\x00\x00\x00")},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			parsed, err := lens.ParseContractCode(tc.code)
			if !errors.Is(err, lens.ErrNotWASM) {
				t.Errorf("ParseContractCode() error = %v, want ErrNotWASM", err)
			}
			if parsed != nil {
				t.Errorf("ParseContractCode() = %+v, want nil on error", parsed)
			}
		})
	}
}

func TestParseContractCodeEmptyModule(t *testing.T) {
	t.Parallel()

	parsed, err := lens.ParseContractCode(wasmModule())
	if err != nil {
		t.Fatalf("ParseContractCode() error = %v", err)
	}
	if parsed.Size != 8 {
		t.Errorf("Size = %d, want 8", parsed.Size)
	}
	if len(parsed.Spec) != 0 || len(parsed.EnvMeta) != 0 || len(parsed.Meta) != 0 {
		t.Errorf("parsed = %+v, want no metadata sections", parsed)
	}
}

func TestParseContractCodeRejectsMalformedSections(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		code []byte
	}{
		{"truncated section size", wasmModule([]byte{0x00})},
		{"size beyond payload", wasmModule([]byte{0x01, 0x0a, 0x60})},
		{"custom name beyond payload", wasmModule([]byte{0x00, 0x02, 0x0e, 'x'})},
		{"malformed spec stream", wasmModule(customSection(contractSpecName, []byte{0x00, 0x00, 0x00}))},
		{"truncated spec entry", wasmModule(customSection(contractSpecName, []byte{0x00, 0x00, 0x00, 0x00}))},
		{"malformed env meta stream", wasmModule(customSection(contractEnvMetaName, []byte{0xff, 0xff}))},
		{"malformed contract meta stream", wasmModule(customSection(contractMetaName, []byte{0x00, 0x00, 0x00}))},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			parsed, err := lens.ParseContractCode(tc.code)
			if !errors.Is(err, lens.ErrMalformedWASM) {
				t.Errorf("ParseContractCode() error = %v, want ErrMalformedWASM", err)
			}
			if parsed != nil {
				t.Errorf("ParseContractCode() = %+v, want nil on error", parsed)
			}
		})
	}
}

func TestParseContractCodeIgnoresUnrelatedSections(t *testing.T) {
	t.Parallel()

	// A toolchain producers section and an unknown future section id must
	// both be skipped without complaint: only the metadata sections matter.
	code := wasmModule(
		wasmSection(1, []byte{0x01, 0x60, 0x00, 0x00}),
		customSection("producers", []byte{0x00, 0x01, 0x02}),
		wasmSection(13, []byte{0x00, 0x00}),
		customSection(contractSpecName, buildSpecStream(t)),
	)
	parsed, err := lens.ParseContractCode(code)
	if err != nil {
		t.Fatalf("ParseContractCode() error = %v", err)
	}
	if len(parsed.Spec) != 1 {
		t.Errorf("Spec has %d entries, want 1", len(parsed.Spec))
	}
}

// buildSpecStream marshals one minimal spec entry so the section test does
// not depend on the fixture format.
func buildSpecStream(t *testing.T) []byte {
	t.Helper()
	entry := xdr.ScSpecEntry{
		Kind: xdr.ScSpecEntryKindScSpecEntryFunctionV0,
		FunctionV0: &xdr.ScSpecFunctionV0{
			Name:    "hello",
			Inputs:  []xdr.ScSpecFunctionInputV0{},
			Outputs: []xdr.ScSpecTypeDef{},
		},
	}
	encoded, err := entry.MarshalBinary()
	if err != nil {
		t.Fatalf("marshalling spec entry: %v", err)
	}
	return encoded
}
