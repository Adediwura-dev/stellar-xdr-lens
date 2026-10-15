package lens

import (
	"testing"

	"github.com/stellar/go-stellar-sdk/strkey"
	"github.com/stellar/go-stellar-sdk/xdr"
)

func TestParseScValPrimitives(t *testing.T) {
	// nil / void
	v, err := ParseScVal(nil)
	if err != nil || v.Type != xdr.ScValTypeScvVoid {
		t.Fatalf("failed void: %v", err)
	}

	// bool
	v, err = ParseScVal(true)
	if err != nil || v.Type != xdr.ScValTypeScvBool || !*v.B {
		t.Fatalf("failed bool: %v", err)
	}

	// int
	v, err = ParseScVal(int64(-42))
	if err != nil || v.Type != xdr.ScValTypeScvI64 || *v.I64 != -42 {
		t.Fatalf("failed i64: %v", err)
	}

	// uint
	v, err = ParseScVal(uint64(42))
	if err != nil || v.Type != xdr.ScValTypeScvU64 || *v.U64 != 42 {
		t.Fatalf("failed u64: %v", err)
	}

	// JSON
	v, err = ParseScValJSON([]byte(`12345`))
	if err != nil || v.Type != xdr.ScValTypeScvI64 || *v.I64 != 12345 {
		t.Fatalf("failed JSON int: %v", err)
	}

	// Empty JSON error
	_, err = ParseScValJSON([]byte(``))
	if err == nil {
		t.Fatalf("expected error on empty JSON")
	}

	// MustParseScVal
	mustV := MustParseScVal(true)
	if mustV.Type != xdr.ScValTypeScvBool {
		t.Fatalf("failed MustParseScVal")
	}

	// Base64 encode
	b64, err := EncodeScValBase64(true)
	if err != nil || len(b64) == 0 {
		t.Fatalf("failed EncodeScValBase64: %v", err)
	}
}

func TestParseScValStringsAndAddresses(t *testing.T) {
	validAccount := "GA2JRQOF6EA3HQWDCEDBPPMLYPJCFLDDGYZLEQGMS5SOBQIB3BAFHVAW"
	validContract, _ := strkey.Encode(strkey.VersionByteContract, make([]byte, 32))

	// Account address
	v, err := ParseScVal(validAccount)
	if err != nil || v.Type != xdr.ScValTypeScvAddress || v.Address.Type != xdr.ScAddressTypeScAddressTypeAccount {
		t.Fatalf("failed account address: %v", err)
	}

	// Contract address
	v, err = ParseScVal(validContract)
	if err != nil || v.Type != xdr.ScValTypeScvAddress || v.Address.Type != xdr.ScAddressTypeScAddressTypeContract {
		t.Fatalf("failed contract address: %v", err)
	}

	// Symbol
	v, err = ParseScVal("my_symbol_1")
	if err != nil || v.Type != xdr.ScValTypeScvSymbol || string(*v.Sym) != "my_symbol_1" {
		t.Fatalf("failed symbol: %v", err)
	}

	// General string
	v, err = ParseScVal("hello world with spaces")
	if err != nil || v.Type != xdr.ScValTypeScvString || string(*v.Str) != "hello world with spaces" {
		t.Fatalf("failed string: %v", err)
	}
}

func TestParseScValTaggedTypes(t *testing.T) {
	// u32
	v, err := ParseScVal(map[string]any{"type": "u32", "value": 123})
	if err != nil || v.Type != xdr.ScValTypeScvU32 || *v.U32 != 123 {
		t.Fatalf("failed tagged u32: %v", err)
	}

	// i32
	v, err = ParseScVal(map[string]any{"type": "i32", "value": -123})
	if err != nil || v.Type != xdr.ScValTypeScvI32 || *v.I32 != -123 {
		t.Fatalf("failed tagged i32: %v", err)
	}

	// u64
	v, err = ParseScVal(map[string]any{"type": "u64", "value": "1000000000"})
	if err != nil || v.Type != xdr.ScValTypeScvU64 || *v.U64 != 1000000000 {
		t.Fatalf("failed tagged u64: %v", err)
	}

	// i64
	v, err = ParseScVal(map[string]any{"type": "i64", "value": "-1000000000"})
	if err != nil || v.Type != xdr.ScValTypeScvI64 || *v.I64 != -1000000000 {
		t.Fatalf("failed tagged i64: %v", err)
	}

	// timepoint
	v, err = ParseScVal(map[string]any{"type": "timepoint", "value": 1700000000})
	if err != nil || v.Type != xdr.ScValTypeScvTimepoint || *v.Timepoint != 1700000000 {
		t.Fatalf("failed tagged timepoint: %v", err)
	}

	// duration
	v, err = ParseScVal(map[string]any{"type": "duration", "value": 3600})
	if err != nil || v.Type != xdr.ScValTypeScvDuration || *v.Duration != 3600 {
		t.Fatalf("failed tagged duration: %v", err)
	}

	// bytes
	v, err = ParseScVal(map[string]any{"type": "bytes", "value": "0x010203"})
	if err != nil || v.Type != xdr.ScValTypeScvBytes || len(*v.Bytes) != 3 {
		t.Fatalf("failed tagged bytes: %v", err)
	}

	// symbol
	v, err = ParseScVal(map[string]any{"type": "symbol", "value": "custom_sym"})
	if err != nil || v.Type != xdr.ScValTypeScvSymbol || string(*v.Sym) != "custom_sym" {
		t.Fatalf("failed tagged symbol: %v", err)
	}

	// symbol length error
	_, err = ParseScVal(map[string]any{"type": "symbol", "value": "123456789012345678901234567890123"})
	if err == nil {
		t.Fatalf("expected error on symbol > 32 chars")
	}

	// contract instance
	v, err = ParseScVal(map[string]any{"type": "contract_instance"})
	if err != nil || v.Type != xdr.ScValTypeScvLedgerKeyContractInstance {
		t.Fatalf("failed tagged contract_instance: %v", err)
	}

	// error
	v, err = ParseScVal(map[string]any{"type": "error", "code": 5})
	if err != nil || v.Type != xdr.ScValTypeScvError || *v.Error.ContractCode != 5 {
		t.Fatalf("failed tagged error: %v", err)
	}

	// single-key shortcut: {"u32": 999}
	v, err = ParseScVal(map[string]any{"u32": 999})
	if err != nil || v.Type != xdr.ScValTypeScvU32 || *v.U32 != 999 {
		t.Fatalf("failed single-key shortcut: %v", err)
	}
}

func TestParseScValBigInts(t *testing.T) {
	// u128 from big.Int string
	v, err := ParseScVal(map[string]any{"type": "u128", "value": "340282366920938463463374607431768211455"})
	if err != nil || v.Type != xdr.ScValTypeScvU128 || v.U128.Hi != ^xdr.Uint64(0) {
		t.Fatalf("failed u128: %v", err)
	}

	// u128 with hi and lo
	v, err = ParseScVal(map[string]any{"type": "u128", "hi": 1, "lo": 2})
	if err != nil || v.Type != xdr.ScValTypeScvU128 || v.U128.Hi != 1 || v.U128.Lo != 2 {
		t.Fatalf("failed u128 with hi/lo: %v", err)
	}

	// i128 with hi and lo
	v, err = ParseScVal(map[string]any{"type": "i128", "hi": -1, "lo": 5})
	if err != nil || v.Type != xdr.ScValTypeScvI128 || v.I128.Hi != -1 || v.I128.Lo != 5 {
		t.Fatalf("failed i128 with hi/lo: %v", err)
	}

	// i128 from string
	v, err = ParseScVal(map[string]any{"type": "i128", "value": "-1000"})
	if err != nil || v.Type != xdr.ScValTypeScvI128 {
		t.Fatalf("failed i128 from string: %v", err)
	}

	// u256
	var hex32 = "000102030405060708090a0b0c0d0e0f101112131415161718191a1b1c1d1e1f"
	v, err = ParseScVal(map[string]any{"type": "u256", "value": "0x" + hex32})
	if err != nil || v.Type != xdr.ScValTypeScvU256 {
		t.Fatalf("failed u256: %v", err)
	}

	// i256
	v, err = ParseScVal(map[string]any{"type": "i256", "value": "0x" + hex32})
	if err != nil || v.Type != xdr.ScValTypeScvI256 {
		t.Fatalf("failed i256: %v", err)
	}
}

func TestParseScValCollections(t *testing.T) {
	// Vector
	v, err := ParseScVal([]any{1, 2, "three"})
	if err != nil || v.Type != xdr.ScValTypeScvVec || len(**v.Vec) != 3 {
		t.Fatalf("failed vec: %v", err)
	}

	// Tagged Vector
	v, err = ParseScVal(map[string]any{"type": "vec", "value": []any{10, 20}})
	if err != nil || v.Type != xdr.ScValTypeScvVec || len(**v.Vec) != 2 {
		t.Fatalf("failed tagged vec: %v", err)
	}

	// Untagged Map
	v, err = ParseScVal(map[string]any{"k1": "v1", "k2": 100})
	if err != nil || v.Type != xdr.ScValTypeScvMap || len(**v.Map) != 2 {
		t.Fatalf("failed untagged map: %v", err)
	}

	// Explicit Map with entries
	v, err = ParseScVal(map[string]any{
		"entries": []any{
			map[string]any{"key": "a", "val": 1},
			map[string]any{"key": "b", "val": 2},
		},
	})
	if err != nil || v.Type != xdr.ScValTypeScvMap || len(**v.Map) != 2 {
		t.Fatalf("failed explicit entries map: %v", err)
	}

	// Map tagged as {"type": "map", "value": [...]}
	v, err = ParseScVal(map[string]any{
		"type": "map",
		"value": []any{
			map[string]any{"key": "foo", "value": "bar"},
		},
	})
	if err != nil || v.Type != xdr.ScValTypeScvMap {
		t.Fatalf("failed map tagged value: %v", err)
	}
}

func TestParseScValEdgeCases(t *testing.T) {
	// *xdr.ScVal and xdr.ScVal direct
	orig := xdr.ScVal{Type: xdr.ScValTypeScvVoid}
	v, err := ParseScVal(orig)
	if err != nil || v.Type != xdr.ScValTypeScvVoid {
		t.Fatalf("failed xdr.ScVal direct")
	}
	v, err = ParseScVal(&orig)
	if err != nil || v.Type != xdr.ScValTypeScvVoid {
		t.Fatalf("failed *xdr.ScVal")
	}
	var nilPtr *xdr.ScVal
	_, err = ParseScVal(nilPtr)
	if err == nil {
		t.Fatalf("expected error on nil *xdr.ScVal")
	}

	// Unsupported type
	_, err = ParseScVal(struct{}{})
	if err == nil {
		t.Fatalf("expected error on struct")
	}

	// MustParseScVal panic
	defer func() {
		if r := recover(); r == nil {
			t.Fatalf("expected panic in MustParseScVal")
		}
	}()
	_ = MustParseScVal(struct{}{})
}

func TestParseScValTaggedMore(t *testing.T) {
	// void
	v, err := ParseScVal(map[string]any{"type": "void"})
	if err != nil || v.Type != xdr.ScValTypeScvVoid {
		t.Fatalf("failed tagged void")
	}

	// bool
	v, err = ParseScVal(map[string]any{"type": "bool", "value": false})
	if err != nil || v.Type != xdr.ScValTypeScvBool || *v.B {
		t.Fatalf("failed tagged bool false")
	}

	// string
	v, err = ParseScVal(map[string]any{"type": "string", "value": "hello"})
	if err != nil || v.Type != xdr.ScValTypeScvString || string(*v.Str) != "hello" {
		t.Fatalf("failed tagged string")
	}

	// address
	validAccount := "GA2JRQOF6EA3HQWDCEDBPPMLYPJCFLDDGYZLEQGMS5SOBQIB3BAFHVAW"
	v, err = ParseScVal(map[string]any{"type": "address", "value": validAccount})
	if err != nil || v.Type != xdr.ScValTypeScvAddress {
		t.Fatalf("failed tagged address")
	}

	// u256 from big number string
	v, err = ParseScVal(map[string]any{"type": "u256", "value": "123456789"})
	if err != nil || v.Type != xdr.ScValTypeScvU256 {
		t.Fatalf("failed u256 from big int string: %v", err)
	}

	// i256 from big number string
	v, err = ParseScVal(map[string]any{"type": "i256", "value": "987654321"})
	if err != nil || v.Type != xdr.ScValTypeScvI256 {
		t.Fatalf("failed i256 from big int string: %v", err)
	}

	// unsupported discriminant
	_, err = ParseScVal(map[string]any{"type": "invalid_discriminant"})
	if err == nil {
		t.Fatalf("expected error on invalid discriminant")
	}
}
