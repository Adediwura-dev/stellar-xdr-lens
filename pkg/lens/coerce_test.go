package lens

import (
	"encoding/json"
	"errors"
	"math"
	"math/big"
	"testing"

	"github.com/stellar/go-stellar-sdk/strkey"
	"github.com/stellar/go-stellar-sdk/xdr"
)

func TestCoerceInt64(t *testing.T) {
	tests := []struct {
		name    string
		input   any
		want    int64
		wantErr bool
	}{
		{"int", 42, 42, false},
		{"int8", int8(42), 42, false},
		{"int16", int16(42), 42, false},
		{"int32", int32(42), 42, false},
		{"int64", int64(42), 42, false},
		{"uint", uint(42), 42, false},
		{"uint8", uint8(42), 42, false},
		{"uint16", uint16(42), 42, false},
		{"uint32", uint32(42), 42, false},
		{"uint64_ok", uint64(42), 42, false},
		{"uint64_overflow", uint64(math.MaxUint64), 0, true},
		{"float32", float32(100), 100, false},
		{"float64", float64(100), 100, false},
		{"float64_nan", math.NaN(), 0, true},
		{"float64_inf", math.Inf(1), 0, true},
		{"json_number", json.Number("10000000"), 10000000, false},
		{"string_int", "10000000", 10000000, false},
		{"string_with_underscores", "1_000_000", 1000000, false},
		{"string_hex", "0x2a", 42, false},
		{"string_hex_upper", "0X2A", 42, false},
		{"string_invalid_hex", "0xZZ", 0, true},
		{"string_float_exact", "100.0", 100, false},
		{"string_invalid", "notanumber", 0, true},
		{"empty_string", "", 0, true},
		{"nil", nil, 0, true},
		{"unsupported_type", []string{"a"}, 0, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := CoerceInt64(tt.input)
			if (err != nil) != tt.wantErr {
				t.Fatalf("CoerceInt64() error = %v, wantErr %v", err, tt.wantErr)
			}
			if !tt.wantErr && got != tt.want {
				t.Errorf("CoerceInt64() = %v, want %v", got, tt.want)
			}
			if tt.wantErr && !errors.Is(err, ErrCoercionFailed) {
				t.Errorf("expected ErrCoercionFailed, got %v", err)
			}
		})
	}
}

func TestCoerceUint64(t *testing.T) {
	tests := []struct {
		name    string
		input   any
		want    uint64
		wantErr bool
	}{
		{"uint", uint(42), 42, false},
		{"uint8", uint8(42), 42, false},
		{"uint16", uint16(42), 42, false},
		{"uint32", uint32(42), 42, false},
		{"uint64", uint64(42), 42, false},
		{"int_pos", int(42), 42, false},
		{"int_neg", int(-1), 0, true},
		{"int8_neg", int8(-1), 0, true},
		{"int16_neg", int16(-1), 0, true},
		{"int32_neg", int32(-1), 0, true},
		{"int64_neg", int64(-1), 0, true},
		{"float32_pos", float32(100), 100, false},
		{"float32_neg", float32(-10), 0, true},
		{"float64_pos", float64(100), 100, false},
		{"float64_neg", float64(-10), 0, true},
		{"json_number", json.Number("500"), 500, false},
		{"string_hex", "0xff", 255, false},
		{"string_hex_upper", "0XFF", 255, false},
		{"string_invalid_hex", "0xZZ", 0, true},
		{"string_int", "500", 500, false},
		{"string_float_exact", "500.0", 500, false},
		{"string_invalid", "abc", 0, true},
		{"empty_string", "", 0, true},
		{"nil", nil, 0, true},
		{"unsupported", struct{}{}, 0, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := CoerceUint64(tt.input)
			if (err != nil) != tt.wantErr {
				t.Fatalf("CoerceUint64() error = %v, wantErr %v", err, tt.wantErr)
			}
			if !tt.wantErr && got != tt.want {
				t.Errorf("CoerceUint64() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestCoerceInt32AndUint32(t *testing.T) {
	// Int32
	i32, err := CoerceInt32("12345")
	if err != nil || i32 != 12345 {
		t.Errorf("CoerceInt32 failed: %v, %v", i32, err)
	}
	_, err = CoerceInt32(int64(math.MaxInt64))
	if err == nil {
		t.Errorf("expected overflow error for int32")
	}

	// Uint32
	u32, err := CoerceUint32("67890")
	if err != nil || u32 != 67890 {
		t.Errorf("CoerceUint32 failed: %v, %v", u32, err)
	}
	_, err = CoerceUint32(uint64(math.MaxUint64))
	if err == nil {
		t.Errorf("expected overflow error for uint32")
	}
}

func TestCoerceBigInt(t *testing.T) {
	// big.Int
	orig := big.NewInt(123456789)
	bi, err := CoerceBigInt(orig)
	if err != nil || bi.Cmp(orig) != 0 {
		t.Errorf("CoerceBigInt from *big.Int failed: %v", err)
	}
	// nil
	_, err = CoerceBigInt(nil)
	if err == nil {
		t.Errorf("expected error for nil")
	}
	// int
	bi, err = CoerceBigInt(int64(100))
	if err != nil || bi.Int64() != 100 {
		t.Errorf("CoerceBigInt from int failed: %v", err)
	}
	// uint
	bi, err = CoerceBigInt(uint64(200))
	if err != nil || bi.Uint64() != 200 {
		t.Errorf("CoerceBigInt from uint failed: %v", err)
	}
	// json.Number
	bi, err = CoerceBigInt(json.Number("300"))
	if err != nil || bi.Int64() != 300 {
		t.Errorf("CoerceBigInt from json.Number failed: %v", err)
	}
	// string hex
	bi, err = CoerceBigInt("0xff")
	if err != nil || bi.Int64() != 255 {
		t.Errorf("CoerceBigInt from hex string failed: %v", err)
	}
	// string neg hex
	bi, err = CoerceBigInt("-0x10")
	if err != nil || bi.Int64() != -16 {
		t.Errorf("CoerceBigInt from neg hex string failed: %v", err)
	}
	// empty string
	_, err = CoerceBigInt("")
	if err == nil {
		t.Errorf("expected error on empty string")
	}
	// invalid string
	_, err = CoerceBigInt("invalid")
	if err == nil {
		t.Errorf("expected error on invalid string")
	}
	// unsupported
	_, err = CoerceBigInt([]int{1})
	if err == nil {
		t.Errorf("expected error on unsupported type")
	}
}

func TestCoerceBool(t *testing.T) {
	// bool
	b, err := CoerceBool(true)
	if err != nil || !b {
		t.Errorf("failed bool: %v", err)
	}
	// int
	b, err = CoerceBool(1)
	if err != nil || !b {
		t.Errorf("failed int 1: %v", err)
	}
	b, err = CoerceBool(0)
	if err != nil || b {
		t.Errorf("failed int 0: %v", err)
	}
	_, err = CoerceBool(2)
	if err == nil {
		t.Errorf("expected error on int 2")
	}
	// uint
	b, err = CoerceBool(uint(1))
	if err != nil || !b {
		t.Errorf("failed uint 1: %v", err)
	}
	b, err = CoerceBool(uint(0))
	if err != nil || b {
		t.Errorf("failed uint 0: %v", err)
	}
	_, err = CoerceBool(uint(2))
	if err == nil {
		t.Errorf("expected error on uint 2")
	}
	// string
	b, err = CoerceBool("yes")
	if err != nil || !b {
		t.Errorf("failed string yes: %v", err)
	}
	b, err = CoerceBool("no")
	if err != nil || b {
		t.Errorf("failed string no: %v", err)
	}
	_, err = CoerceBool("maybe")
	if err == nil {
		t.Errorf("expected error on invalid string")
	}
	// nil & unsupported
	_, err = CoerceBool(nil)
	if err == nil {
		t.Errorf("expected error on nil")
	}
	_, err = CoerceBool(struct{}{})
	if err == nil {
		t.Errorf("expected error on struct")
	}
}

func TestCoerceString(t *testing.T) {
	s, err := CoerceString("hello")
	if err != nil || s != "hello" {
		t.Errorf("failed string")
	}
	s, err = CoerceString([]byte("bytes"))
	if err != nil || s != "bytes" {
		t.Errorf("failed bytes")
	}
	s, err = CoerceString(json.Number("42"))
	if err != nil || s != "42" {
		t.Errorf("failed json.Number")
	}
	s, err = CoerceString(123)
	if err != nil || s != "123" {
		t.Errorf("failed default int")
	}
	_, err = CoerceString(nil)
	if err == nil {
		t.Errorf("expected error for nil")
	}
}

func TestCoerceBytesAndHash(t *testing.T) {
	// nil
	b, err := CoerceBytes(nil)
	if err != nil || b != nil {
		t.Errorf("failed nil bytes")
	}
	// []byte
	orig := []byte{1, 2, 3}
	b, err = CoerceBytes(orig)
	if err != nil || len(b) != 3 {
		t.Errorf("failed []byte")
	}
	// empty string
	b, err = CoerceBytes("")
	if err != nil || len(b) != 0 {
		t.Errorf("failed empty string")
	}
	// hex string with 0x prefix
	b, err = CoerceBytes("0xabcd")
	if err != nil || len(b) != 2 || b[0] != 0xab || b[1] != 0xcd {
		t.Errorf("failed 0x hex string: %v", err)
	}
	// hex string odd length
	b, err = CoerceBytes("0xabc")
	if err != nil || len(b) != 2 {
		t.Errorf("failed odd hex string: %v", err)
	}
	// plain hex
	b, err = CoerceBytes("cafe")
	if err != nil || len(b) != 2 || b[0] != 0xca || b[1] != 0xfe {
		t.Errorf("failed plain hex")
	}
	// base64 string
	b, err = CoerceBytes("aGVsbG8=")
	if err != nil || string(b) != "hello" {
		t.Errorf("failed base64: %v", err)
	}
	// raw fallback
	b, err = CoerceBytes("not hex or b64?!")
	if err != nil || string(b) != "not hex or b64?!" {
		t.Errorf("failed raw string")
	}
	// []any
	b, err = CoerceBytes([]any{uint8(10), uint8(20)})
	if err != nil || len(b) != 2 || b[0] != 10 {
		t.Errorf("failed []any")
	}
	// []any out of range
	_, err = CoerceBytes([]any{300})
	if err == nil {
		t.Errorf("expected error on out-of-range byte")
	}
	// unsupported
	_, err = CoerceBytes(12345)
	if err == nil {
		t.Errorf("expected error on int")
	}

	// Hash
	var hex32 = "000102030405060708090a0b0c0d0e0f101112131415161718191a1b1c1d1e1f"
	h, err := CoerceHash("0x" + hex32)
	if err != nil || h[0] != 0x00 || h[31] != 0x1f {
		t.Errorf("failed CoerceHash: %v", err)
	}
	_, err = CoerceHash("short")
	if err == nil {
		t.Errorf("expected error on short hash")
	}
}

func TestCoerceAmount(t *testing.T) {
	// integer stroops
	a, err := CoerceAmount(10000000)
	if err != nil || a != 10000000 {
		t.Errorf("failed integer amount: %v", err)
	}
	// float decimal XLM
	a, err = CoerceAmount(1.5)
	if err != nil || a != 15000000 {
		t.Errorf("failed float amount: %v, got %d", err, a)
	}
	// string decimal
	a, err = CoerceAmount("1.5")
	if err != nil || a != 15000000 {
		t.Errorf("failed string decimal: %v, got %d", err, a)
	}
	// string with XLM suffix
	a, err = CoerceAmount("2.5 XLM")
	if err != nil || a != 25000000 {
		t.Errorf("failed string XLM: %v, got %d", err, a)
	}
	// string with stroops suffix
	a, err = CoerceAmount("100 stroops")
	if err != nil || a != 100 {
		t.Errorf("failed string stroops: %v, got %d", err, a)
	}
	// json.Number
	a, err = CoerceAmount(json.Number("5000000"))
	if err != nil || a != 5000000 {
		t.Errorf("failed json.Number amount: %v", err)
	}
	// errors
	_, err = CoerceAmount(nil)
	if err == nil {
		t.Errorf("expected error on nil")
	}
	_, err = CoerceAmount([]string{"bad"})
	if err == nil {
		t.Errorf("expected error on unsupported")
	}
}

func TestCoerceAccountIDAndMuxedAccount(t *testing.T) {
	validAccount := "GA2JRQOF6EA3HQWDCEDBPPMLYPJCFLDDGYZLEQGMS5SOBQIB3BAFHVAW"
	acc, err := CoerceAccountID(validAccount)
	if err != nil || acc.Ed25519 == nil {
		t.Fatalf("failed CoerceAccountID from strkey: %v", err)
	}

	// From pointer
	accPtr, err := CoerceAccountID(&acc)
	if err != nil || accPtr.Ed25519 == nil {
		t.Errorf("failed from *AccountId")
	}

	// From [32]byte
	var b32 [32]byte
	copy(b32[:], acc.Ed25519[:])
	acc2, err := CoerceAccountID(b32)
	if err != nil || *acc2.Ed25519 != *acc.Ed25519 {
		t.Errorf("failed from [32]byte")
	}

	// From []byte
	acc3, err := CoerceAccountID(b32[:])
	if err != nil || *acc3.Ed25519 != *acc.Ed25519 {
		t.Errorf("failed from []byte")
	}

	// Invalid
	_, err = CoerceAccountID(nil)
	if err == nil {
		t.Errorf("expected error on nil")
	}
	_, err = CoerceAccountID("invalid")
	if err == nil {
		t.Errorf("expected error on invalid strkey")
	}

	// MuxedAccount from G...
	mux, err := CoerceMuxedAccount(validAccount)
	if err != nil || mux.Type != xdr.CryptoKeyTypeKeyTypeEd25519 {
		t.Errorf("failed muxed from G...: %v", err)
	}

	// MuxedAccount from map
	muxMap, err := CoerceMuxedAccount(map[string]any{
		"account": validAccount,
		"id":      uint64(999),
	})
	if err != nil || muxMap.Type != xdr.CryptoKeyTypeKeyTypeMuxedEd25519 || muxMap.Med25519.Id != 999 {
		t.Errorf("failed muxed from map: %v", err)
	}

	// MuxedAccount from map without ID
	muxMap2, err := CoerceMuxedAccount(map[string]any{
		"account": validAccount,
	})
	if err != nil || muxMap2.Type != xdr.CryptoKeyTypeKeyTypeEd25519 {
		t.Errorf("failed muxed from map without id")
	}

	// MuxedAccount from M...
	muxExpected := xdr.MuxedAccount{
		Type: xdr.CryptoKeyTypeKeyTypeMuxedEd25519,
		Med25519: &xdr.MuxedAccountMed25519{
			Id:      12345,
			Ed25519: *acc.Ed25519,
		},
	}
	validMuxed := muxExpected.Address()
	muxM, err := CoerceMuxedAccount(validMuxed)
	if err != nil || muxM.Type != xdr.CryptoKeyTypeKeyTypeMuxedEd25519 {
		t.Errorf("failed muxed from M...: %v", err)
	}
}

func TestCoerceAssetAndChangeTrustAsset(t *testing.T) {
	// Native string
	asset, err := CoerceAsset("native")
	if err != nil || asset.Type != xdr.AssetTypeAssetTypeNative {
		t.Errorf("failed native asset: %v", err)
	}
	asset, err = CoerceAsset("XLM")
	if err != nil || asset.Type != xdr.AssetTypeAssetTypeNative {
		t.Errorf("failed XLM asset: %v", err)
	}

	validAccount := "GA2JRQOF6EA3HQWDCEDBPPMLYPJCFLDDGYZLEQGMS5SOBQIB3BAFHVAW"

	// AlphaNum4
	asset4, err := CoerceAsset("USD:" + validAccount)
	if err != nil || asset4.Type != xdr.AssetTypeAssetTypeCreditAlphanum4 {
		t.Errorf("failed alphanum4 asset: %v", err)
	}

	// AlphaNum12
	asset12, err := CoerceAsset("USDC123456:" + validAccount)
	if err != nil || asset12.Type != xdr.AssetTypeAssetTypeCreditAlphanum12 {
		t.Errorf("failed alphanum12 asset: %v", err)
	}

	// Map asset
	assetMap, err := CoerceAsset(map[string]any{
		"code":   "USDC",
		"issuer": validAccount,
	})
	if err != nil || assetMap.Type != xdr.AssetTypeAssetTypeCreditAlphanum4 {
		t.Errorf("failed map asset: %v", err)
	}

	// ChangeTrustAsset
	cta, err := CoerceChangeTrustAsset(asset4)
	if err != nil || cta.Type != xdr.AssetTypeAssetTypeCreditAlphanum4 {
		t.Errorf("failed ChangeTrustAsset: %v", err)
	}

	// Pool ChangeTrustAsset
	var poolHex = "000102030405060708090a0b0c0d0e0f101112131415161718191a1b1c1d1e1f"
	ctaPool, err := CoerceChangeTrustAsset(map[string]any{
		"type":    "pool",
		"pool_id": poolHex,
	})
	if err != nil || ctaPool.Type != xdr.AssetTypeAssetTypePoolShare {
		t.Errorf("failed pool ChangeTrustAsset: %v", err)
	}
}

func TestCoercePrice(t *testing.T) {
	// Fraction string
	p, err := CoercePrice("1/2")
	if err != nil || p.N != 1 || p.D != 2 {
		t.Errorf("failed price fraction: %v", err)
	}
	// Float string
	p, err = CoercePrice("0.5")
	if err != nil || float64(p.N)/float64(p.D) != 0.5 {
		t.Errorf("failed price float: %v", err)
	}
	// Int
	p, err = CoercePrice(10)
	if err != nil || p.N != 10 || p.D != 1 {
		t.Errorf("failed price int: %v", err)
	}
	// Map
	p, err = CoercePrice(map[string]any{"n": 3, "d": 4})
	if err != nil || p.N != 3 || p.D != 4 {
		t.Errorf("failed price map: %v", err)
	}
	// Division by zero
	_, err = CoercePrice("1/0")
	if err == nil {
		t.Errorf("expected error on division by zero")
	}

	// Additional price cases
	if _, err := CoercePrice(nil); err == nil {
		t.Errorf("expected error on nil price")
	}
	if _, err := CoercePrice("not-a-number"); err == nil {
		t.Errorf("expected error on invalid price string")
	}
	if _, err := CoercePrice(map[string]any{"n": "bad"}); err == nil {
		t.Errorf("expected error on invalid price numerator")
	}
	if _, err := CoercePrice(map[string]any{"n": 1, "d": 0}); err == nil {
		t.Errorf("expected error on zero denominator map")
	}
}

func TestCoerceChangeTrustAssetAdditional(t *testing.T) {
	validAccount := "GBRPYHIL2CI3FNQ4BXLFMNDLFJUNPU2HY3ZMFSHONUCEOASW7QC7OX2H"
	// Native
	ctaNative, err := CoerceChangeTrustAsset("native")
	if err != nil || ctaNative.Type != xdr.AssetTypeAssetTypeNative {
		t.Errorf("failed native change trust asset: %v", err)
	}

	// 12-char asset
	cta12, err := CoerceChangeTrustAsset("LONGASSETNAM:" + validAccount)
	if err != nil || cta12.Type != xdr.AssetTypeAssetTypeCreditAlphanum12 {
		t.Errorf("failed alphanum12 change trust asset: %v", err)
	}

	// Pool with asset_a and asset_b
	ctaPool, err := CoerceChangeTrustAsset(map[string]any{
		"type":    "pool",
		"asset_a": "native",
		"asset_b": "USDC:" + validAccount,
		"fee":     30,
	})
	if err != nil || ctaPool.Type != xdr.AssetTypeAssetTypePoolShare {
		t.Errorf("failed full pool change trust asset: %v", err)
	}

	// Error cases
	if _, err := CoerceChangeTrustAsset(nil); err == nil {
		t.Errorf("expected error on nil")
	}
	if _, err := CoerceChangeTrustAsset(12345); err == nil {
		t.Errorf("expected error on unsupported type")
	}
	if _, err := CoerceChangeTrustAsset(map[string]any{
		"type":    "pool",
		"asset_a": "native",
		"asset_b": "invalid-asset",
	}); err == nil {
		t.Errorf("expected error on pool with invalid asset_b")
	}
}

func TestCoerceMuxedAndAccountIDAdditional(t *testing.T) {
	validAccount := "GBRPYHIL2CI3FNQ4BXLFMNDLFJUNPU2HY3ZMFSHONUCEOASW7QC7OX2H"
	muxAcc := &strkey.MuxedAccount{}
	if err := muxAcc.SetAccountID(validAccount); err != nil {
		t.Fatal(err)
	}
	muxAcc.SetID(12345)
	muxedStr, err := muxAcc.Address()
	if err != nil {
		t.Fatal(err)
	}

	med, err := CoerceMuxedAccount(muxedStr)
	if err != nil || med.Type != xdr.CryptoKeyTypeKeyTypeMuxedEd25519 {
		t.Errorf("failed muxed address coerce: %v", err)
	}

	medMap, err := CoerceMuxedAccount(map[string]any{
		"address": validAccount,
		"id":      12345,
	})
	if err != nil || medMap.Type != xdr.CryptoKeyTypeKeyTypeMuxedEd25519 {
		t.Errorf("failed map muxed coerce: %v", err)
	}

	// Invalid cases
	if _, err := CoerceMuxedAccount(nil); err == nil {
		t.Errorf("expected error on nil muxed account")
	}
	if _, err := CoerceMuxedAccount(999); err == nil {
		t.Errorf("expected error on invalid muxed type")
	}
	if _, err := CoerceAccountID(nil); err == nil {
		t.Errorf("expected error on nil account ID")
	}
	if _, err := CoerceAccountID(true); err == nil {
		t.Errorf("expected error on bool account ID")
	}
}
