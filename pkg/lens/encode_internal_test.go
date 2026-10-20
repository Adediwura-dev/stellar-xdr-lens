package lens

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"math"
	"reflect"
	"strings"
	"testing"

	"github.com/stellar/go-stellar-sdk/strkey"
	"github.com/stellar/go-stellar-sdk/xdr"
)

const (
	encodeTestAccount  = "GBRPYHIL2CI3FNQ4BXLFMNDLFJUNPU2HY3ZMFSHONUCEOASW7QC7OX2H"
	encodeTestUSDCIssu = "GA5ZSEJYB37JRC5AVCIA5MOP4RHTM335X2KGX3IHOJAPP5RE34K4KZVN"
)

func encodeTestContract(t *testing.T) string {
	t.Helper()
	address, err := strkey.Encode(strkey.VersionByteContract, make([]byte, 32))
	if err != nil {
		t.Fatalf("strkey.Encode() error = %v", err)
	}
	return address
}

func TestEncoderInputScalars(t *testing.T) {
	t.Parallel()

	intCases := []struct {
		name  string
		input any
		want  int64
		ok    bool
	}{
		{name: "json number", input: json.Number("42"), want: 42, ok: true},
		{name: "json decimal", input: json.Number("4.2")},
		{name: "float integral", input: float64(7), want: 7, ok: true},
		{name: "float fractional", input: 7.5},
		{name: "int", input: 9, want: 9, ok: true},
		{name: "int64", input: int64(-9), want: -9, ok: true},
		{name: "uint64", input: uint64(11), want: 11, ok: true},
		{name: "uint64 overflow", input: uint64(math.MaxInt64) + 1},
		{name: "string", input: " 12 ", want: 12, ok: true},
		{name: "bad string", input: "twelve"},
		{name: "unsupported", input: true},
	}
	for _, tc := range intCases {
		t.Run("int64/"+tc.name, func(t *testing.T) {
			t.Parallel()
			got, ok := inputInt64(tc.input)
			if ok != tc.ok || (ok && got != tc.want) {
				t.Errorf("inputInt64(%v) = (%d, %v), want (%d, %v)", tc.input, got, ok, tc.want, tc.ok)
			}
		})
	}

	uintCases := []struct {
		name  string
		input any
		want  uint64
		ok    bool
	}{
		{name: "json number", input: json.Number("42"), want: 42, ok: true},
		{name: "json negative", input: json.Number("-1")},
		{name: "json large", input: json.Number("18446744073709551615"), want: math.MaxUint64, ok: true},
		{name: "json decimal", input: json.Number("4.2")},
		{name: "float integral", input: float64(7), want: 7, ok: true},
		{name: "float negative", input: -7.0},
		{name: "float fractional", input: 7.5},
		{name: "int", input: 9, want: 9, ok: true},
		{name: "int negative", input: -9},
		{name: "int64", input: int64(11), want: 11, ok: true},
		{name: "int64 negative", input: int64(-11)},
		{name: "uint64", input: uint64(13), want: 13, ok: true},
		{name: "string", input: " 14 ", want: 14, ok: true},
		{name: "bad string", input: "fourteen"},
		{name: "unsupported", input: true},
	}
	for _, tc := range uintCases {
		t.Run("uint64/"+tc.name, func(t *testing.T) {
			t.Parallel()
			got, ok := inputUint64(tc.input)
			if ok != tc.ok || (ok && got != tc.want) {
				t.Errorf("inputUint64(%v) = (%d, %v), want (%d, %v)", tc.input, got, ok, tc.want, tc.ok)
			}
		})
	}

	floatCases := []struct {
		name  string
		input any
		want  float64
		ok    bool
	}{
		{name: "json number", input: json.Number("1.25"), want: 1.25, ok: true},
		{name: "json bad", input: json.Number("nope")},
		{name: "float", input: 2.5, want: 2.5, ok: true},
		{name: "int64", input: int64(3), want: 3, ok: true},
		{name: "string", input: " 4.5 ", want: 4.5, ok: true},
		{name: "bad string", input: "four"},
		{name: "unsupported", input: []any{1}},
	}
	for _, tc := range floatCases {
		t.Run("float64/"+tc.name, func(t *testing.T) {
			t.Parallel()
			got, ok := inputFloat64(tc.input)
			if ok != tc.ok || (ok && got != tc.want) {
				t.Errorf("inputFloat64(%v) = (%v, %v), want (%v, %v)", tc.input, got, ok, tc.want, tc.ok)
			}
		})
	}
}

func TestParseDataValueInput(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		input   any
		want    string
		wantErr bool
	}{
		{name: "empty", input: ""},
		{name: "hex", input: "776f726c64", want: "world"},
		{name: "odd length text", input: "abc", want: "abc"},
		{name: "plain text", input: "hello", want: "hello"},
		{name: "byte array", input: []any{json.Number("1"), json.Number("2")}, want: "\x01\x02"},
		{name: "byte array overflow", input: []any{json.Number("300")}, wantErr: true},
		{name: "wrong type", input: 42, wantErr: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, err := parseDataValueInput(tc.input)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("parseDataValueInput(%v) error = nil, want an error", tc.input)
				}
				return
			}
			if err != nil {
				t.Fatalf("parseDataValueInput(%v) error = %v", tc.input, err)
			}
			if string(got) != tc.want {
				t.Errorf("parseDataValueInput(%v) = %q, want %q", tc.input, got, tc.want)
			}
		})
	}
}

func TestParsePeerAddressInput(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		input   any
		want    xdr.PeerAddressIp
		port    uint32
		wantErr bool
	}{
		{
			name:  "ipv4",
			input: "127.0.0.1:11625",
			want:  xdr.PeerAddressIp{Type: xdr.IpAddrTypeIPv4, Ipv4: &[4]byte{127, 0, 0, 1}},
			port:  11625,
		},
		{
			name:  "ipv6",
			input: "[::1]:11625",
			want:  xdr.PeerAddressIp{Type: xdr.IpAddrTypeIPv6, Ipv6: &[16]byte{15: 1}},
			port:  11625,
		},
		{name: "wrong type", input: 42, wantErr: true},
		{name: "missing port", input: "127.0.0.1", wantErr: true},
		{name: "bad port", input: "127.0.0.1:http", wantErr: true},
		{name: "hostname", input: "localhost:11625", wantErr: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, err := parsePeerAddressInput(tc.input)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("parsePeerAddressInput(%v) error = nil, want an error", tc.input)
				}
				return
			}
			if err != nil {
				t.Fatalf("parsePeerAddressInput(%v) error = %v", tc.input, err)
			}
			if got.Ip.Type != tc.want.Type || got.Port != xdr.Uint32(tc.port) {
				t.Errorf("parsePeerAddressInput(%v) = %+v, want type %v port %d", tc.input, got, tc.want.Type, tc.port)
			}
			switch tc.want.Type {
			case xdr.IpAddrTypeIPv4:
				if got.Ip.Ipv4 == nil || *got.Ip.Ipv4 != *tc.want.Ipv4 {
					t.Errorf("IPv4 = %v, want %v", got.Ip.Ipv4, tc.want.Ipv4)
				}
			case xdr.IpAddrTypeIPv6:
				if got.Ip.Ipv6 == nil || *got.Ip.Ipv6 != *tc.want.Ipv6 {
					t.Errorf("IPv6 = %v, want %v", got.Ip.Ipv6, tc.want.Ipv6)
				}
			}
		})
	}
}

func TestParsePoolParametersInput(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		input   any
		fee     int32
		wantErr bool
	}{
		{
			name:  "valid",
			input: "USDC:" + encodeTestUSDCIssu + " / native @ 0.3%",
			fee:   30,
		},
		{name: "wrong type", input: 42, wantErr: true},
		{name: "no fee", input: "native / native", wantErr: true},
		{name: "no pair", input: "native @ 0.3%", wantErr: true},
		{name: "bad asset", input: "nope / native @ 0.3%", wantErr: true},
		{name: "bad fee", input: "native / native @ nope%", wantErr: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, err := parsePoolParametersInput(tc.input)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("parsePoolParametersInput(%v) error = nil, want an error", tc.input)
				}
				return
			}
			if err != nil {
				t.Fatalf("parsePoolParametersInput(%v) error = %v", tc.input, err)
			}
			if int32(got.Fee) != tc.fee {
				t.Errorf("Fee = %d, want %d", got.Fee, tc.fee)
			}
		})
	}
}

func TestParseContractExecutableInput(t *testing.T) {
	t.Parallel()

	wasmHash := strings.Repeat("ab", 32)
	contract := encodeTestContract(t)

	tests := []struct {
		name    string
		input   any
		want    xdr.ContractExecutableType
		wantErr bool
		check   func(*testing.T, xdr.ContractExecutable)
	}{
		{
			name:  "stellar asset",
			input: "stellar_asset",
			want:  xdr.ContractExecutableTypeContractExecutableStellarAsset,
		},
		{
			name:  "stellar asset description",
			input: "stellar asset contract: USDC",
			want:  xdr.ContractExecutableTypeContractExecutableStellarAsset,
		},
		{
			name:  "wasm prefix",
			input: "wasm:" + wasmHash,
			want:  xdr.ContractExecutableTypeContractExecutableWasm,
			check: func(t *testing.T, executable xdr.ContractExecutable) {
				t.Helper()
				if executable.WasmHash == nil || executable.WasmHash[0] != 0xab {
					t.Errorf("WasmHash = %v, want the prefixed hash", executable.WasmHash)
				}
			},
		},
		{
			name:  "bare hash",
			input: wasmHash,
			want:  xdr.ContractExecutableTypeContractExecutableWasm,
			check: func(t *testing.T, executable xdr.ContractExecutable) {
				t.Helper()
				if executable.WasmHash == nil || executable.WasmHash[0] != 0xab {
					t.Errorf("WasmHash = %v, want the bare hash", executable.WasmHash)
				}
			},
		},
		{
			name:  "external account owner",
			input: encodeTestAccount,
			want:  xdr.ContractExecutableTypeContractExecutableExternalRef,
			check: func(t *testing.T, executable xdr.ContractExecutable) {
				t.Helper()
				if executable.ExternalRef == nil || executable.ExternalRef.Tag != "" {
					t.Errorf("ExternalRef = %+v, want an untagged owner", executable.ExternalRef)
				}
			},
		},
		{
			name:  "external contract owner with tag",
			input: contract + " tag: vault",
			want:  xdr.ContractExecutableTypeContractExecutableExternalRef,
			check: func(t *testing.T, executable xdr.ContractExecutable) {
				t.Helper()
				if executable.ExternalRef == nil || executable.ExternalRef.Tag != "vault" {
					t.Errorf("ExternalRef = %+v, want tag vault", executable.ExternalRef)
				}
			},
		},
		{name: "wasm bad hash", input: "wasm:zz", wantErr: true},
		{name: "unparseable reference", input: "not an address", wantErr: true},
		{name: "wrong type", input: 42, wantErr: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, err := parseContractExecutableInput(tc.input)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("parseContractExecutableInput(%v) error = nil, want an error", tc.input)
				}
				return
			}
			if err != nil {
				t.Fatalf("parseContractExecutableInput(%v) error = %v", tc.input, err)
			}
			if got.Type != tc.want {
				t.Errorf("Type = %v, want %v", got.Type, tc.want)
			}
			if tc.check != nil {
				tc.check(t, got)
			}
		})
	}
}

func TestParseStorageKeyScVal(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		input   any
		want    xdr.ScValType
		nonce   int64
		wantErr bool
	}{
		{name: "instance", input: "instance storage key", want: xdr.ScValTypeScvLedgerKeyContractInstance},
		{name: "nonce", input: "nonce key: 7", want: xdr.ScValTypeScvLedgerKeyNonce, nonce: 7},
		{name: "bad nonce", input: "nonce key: seven", wantErr: true},
		{name: "unknown named key", input: "balance key", wantErr: true},
		{name: "wrong type", input: 42, wantErr: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, err := parseStorageKeyScVal(tc.input)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("parseStorageKeyScVal(%v) error = nil, want an error", tc.input)
				}
				return
			}
			if err != nil {
				t.Fatalf("parseStorageKeyScVal(%v) error = %v", tc.input, err)
			}
			if got.Type != tc.want {
				t.Errorf("Type = %v, want %v", got.Type, tc.want)
			}
			if tc.want == xdr.ScValTypeScvLedgerKeyNonce {
				if got.NonceKey == nil || int64(got.NonceKey.Nonce) != tc.nonce {
					t.Errorf("NonceKey = %+v, want nonce %d", got.NonceKey, tc.nonce)
				}
			}
		})
	}
}

func TestParseAddressAndIDInputs(t *testing.T) {
	t.Parallel()

	contract := encodeTestContract(t)
	balanceID := xdr.ClaimableBalanceId{
		Type: xdr.ClaimableBalanceIdTypeClaimableBalanceIdTypeV0,
		V0:   &xdr.Hash{1, 2, 3},
	}
	balanceStrkey, err := balanceID.EncodeToStrkey()
	if err != nil {
		t.Fatalf("EncodeToStrkey() error = %v", err)
	}

	if _, err := parseScAddressInput(42); err == nil {
		t.Error("parseScAddressInput(42) error = nil, want an error")
	}
	if _, err := parseScAddressInput("Xnotastrkey"); err == nil {
		t.Error("parseScAddressInput(X...) error = nil, want an error")
	}
	if _, err := parseScAddressInput("Gbad"); err == nil {
		t.Error("parseScAddressInput(Gbad) error = nil, want an error")
	}
	accountAddr, err := parseScAddressInput(encodeTestAccount)
	if err != nil || accountAddr.Type != xdr.ScAddressTypeScAddressTypeAccount {
		t.Errorf("account address = (%+v, %v)", accountAddr, err)
	}
	contractAddr, err := parseScAddressInput(contract)
	if err != nil || contractAddr.Type != xdr.ScAddressTypeScAddressTypeContract {
		t.Errorf("contract address = (%+v, %v)", contractAddr, err)
	}

	if _, err := parseContractIDInput(42); err == nil {
		t.Error("parseContractIDInput(42) error = nil, want an error")
	}
	if _, err := parseContractIDInput(encodeTestAccount); err == nil {
		t.Error("parseContractIDInput(account) error = nil, want an error")
	}
	if _, err := parseContractIDInput("Cbad"); err == nil {
		t.Error("parseContractIDInput(Cbad) error = nil, want an error")
	}
	if _, err := parseContractIDInput(contract); err != nil {
		t.Errorf("parseContractIDInput(contract) error = %v", err)
	}

	if _, err := parseClaimableBalanceIDInput(42); err == nil {
		t.Error("parseClaimableBalanceIDInput(42) error = nil, want an error")
	}
	if _, err := parseClaimableBalanceIDInput("Bbad"); err == nil {
		t.Error("parseClaimableBalanceIDInput(Bbad) error = nil, want an error")
	}
	if got, err := parseClaimableBalanceIDInput(balanceStrkey); err != nil || got.Type != balanceID.Type {
		t.Errorf("parseClaimableBalanceIDInput() = (%+v, %v)", got, err)
	}

	if _, err := parsePublicKeyInput(42); err == nil {
		t.Error("parsePublicKeyInput(42) error = nil, want an error")
	}
	if _, err := parsePublicKeyInput(encodeTestAccount); err != nil {
		t.Errorf("parsePublicKeyInput(account) error = %v", err)
	}
	if _, err := parseNodeIDInput(42); err == nil {
		t.Error("parseNodeIDInput(42) error = nil, want an error")
	}
	if got, err := parseNodeIDInput(encodeTestAccount); err != nil || got.Ed25519 == nil {
		t.Errorf("parseNodeIDInput(account) = (%+v, %v)", got, err)
	}
}

func TestParsePriceInput(t *testing.T) {
	t.Parallel()

	if got, err := parsePriceInput("7/3 (= 2.3333333)"); err != nil || got.N != 7 || got.D != 3 {
		t.Errorf("parsePriceInput(fraction) = (%+v, %v)", got, err)
	}
	if _, err := parsePriceInput("not a price"); err == nil {
		t.Error("parsePriceInput(text) error = nil, want an error")
	}
	if _, err := parsePriceInput("x/3"); err == nil {
		t.Error("parsePriceInput(bad numerator) error = nil, want an error")
	}
	if _, err := parsePriceInput("7/y"); err == nil {
		t.Error("parsePriceInput(bad denominator) error = nil, want an error")
	}
	if got, err := parsePriceInput(map[string]any{"n": json.Number("1"), "d": json.Number("2")}); err != nil || got.N != 1 {
		t.Errorf("parsePriceInput(object) = (%+v, %v)", got, err)
	}
}

func TestDecodeUpgradeTypeInput(t *testing.T) {
	t.Parallel()

	version := xdr.Uint32(21)
	upgrade := xdr.LedgerUpgrade{
		Type:             xdr.LedgerUpgradeTypeLedgerUpgradeVersion,
		NewLedgerVersion: &version,
	}
	bin, err := upgrade.MarshalBinary()
	if err != nil {
		t.Fatalf("MarshalBinary() error = %v", err)
	}

	got, err := decodeUpgradeTypeInput(base64.StdEncoding.EncodeToString(bin), "")
	if err != nil {
		t.Fatalf("decodeUpgradeTypeInput(base64) error = %v", err)
	}
	if len(got) != len(bin) {
		t.Errorf("decodeUpgradeTypeInput(base64) = %d bytes, want %d", len(got), len(bin))
	}

	got, err = decodeUpgradeTypeInput([]any{json.Number("1"), json.Number("2")}, "upgrade")
	if err != nil || !reflect.DeepEqual(got, []byte{1, 2}) {
		t.Errorf("decodeUpgradeTypeInput(array) = (%v, %v)", got, err)
	}

	if _, err := decodeUpgradeTypeInput("!!!", ""); err == nil {
		t.Error("decodeUpgradeTypeInput(!!!)) error = nil, want an error")
	}
	if _, err := decodeUpgradeTypeInput(42, ""); err == nil {
		t.Error("decodeUpgradeTypeInput(42) error = nil, want an error")
	}
}

func TestDecodeJSONBytesPaths(t *testing.T) {
	t.Parallel()

	got, err := decodeJSONBytes("AQID", "", true)
	if err != nil || !reflect.DeepEqual(got, []byte{1, 2, 3}) {
		t.Errorf("base64 string = (%v, %v)", got, err)
	}
	if _, err := decodeJSONBytes("!!!", "sig", true); err == nil {
		t.Error("invalid base64/hex string error = nil, want an error")
	}

	got, err = decodeJSONBytes("0x0102", "", false)
	if err != nil || !reflect.DeepEqual(got, []byte{1, 2}) {
		t.Errorf("hex string = (%v, %v)", got, err)
	}
	if _, err := decodeJSONBytes("zz", "", false); err == nil {
		t.Error("invalid hex string error = nil, want an error")
	}

	got, err = decodeJSONBytes([]any{json.Number("1"), json.Number("2")}, "", false)
	if err != nil || !reflect.DeepEqual(got, []byte{1, 2}) {
		t.Errorf("byte array = (%v, %v)", got, err)
	}
	if _, err := decodeJSONBytes([]any{json.Number("300")}, "", false); err == nil {
		t.Error("byte array overflow error = nil, want an error")
	}
	if _, err := decodeJSONBytes(42, "", false); err == nil {
		t.Error("wrong type error = nil, want an error")
	}
}

func TestEncodeJSONIntoErrors(t *testing.T) {
	t.Parallel()

	enumType := reflect.TypeOf(xdr.TransactionResultCode(0))
	enumValue := func() reflect.Value { return reflect.New(enumType).Elem() }
	channelType := reflect.TypeOf(make(chan int))

	tests := []struct {
		name  string
		rv    func() reflect.Value
		input any
	}{
		{name: "invalid value", rv: func() reflect.Value { return reflect.Value{} }, input: 1},
		{
			name: "interface wrong type",
			rv: func() reflect.Value {
				return reflect.New(reflect.TypeOf((*interface{ String() string })(nil)).Elem()).Elem()
			},
			input: 1,
		},
		{name: "struct shape", rv: func() reflect.Value { return reflect.New(reflect.TypeOf(xdr.LedgerUpgrade{})).Elem() }, input: "1/2"},
		{name: "slice shape", rv: func() reflect.Value { return reflect.New(reflect.TypeOf([]xdr.Price{})).Elem() }, input: 1},
		{name: "array shape", rv: func() reflect.Value { return reflect.New(reflect.TypeOf([2]xdr.Price{})).Elem() }, input: 1},
		{name: "array length", rv: func() reflect.Value { return reflect.New(reflect.TypeOf([2]xdr.Price{})).Elem() }, input: []any{}},
		{name: "bool shape", rv: func() reflect.Value { return reflect.New(reflect.TypeOf(false)).Elem() }, input: "yes"},
		{name: "string shape", rv: func() reflect.Value { return reflect.New(reflect.TypeOf("")).Elem() }, input: 1},
		{name: "int shape", rv: func() reflect.Value { return reflect.New(reflect.TypeOf(int32(0))).Elem() }, input: 1.5},
		{name: "int overflow", rv: func() reflect.Value { return reflect.New(reflect.TypeOf(int32(0))).Elem() }, input: json.Number("9007199254740992")},
		{name: "uint negative", rv: func() reflect.Value { return reflect.New(reflect.TypeOf(uint32(0))).Elem() }, input: -1},
		{name: "uint overflow", rv: func() reflect.Value { return reflect.New(reflect.TypeOf(uint32(0))).Elem() }, input: json.Number("1099511627776")},
		{name: "float shape", rv: func() reflect.Value { return reflect.New(reflect.TypeOf(float32(0))).Elem() }, input: []any{}},
		{name: "float overflow", rv: func() reflect.Value { return reflect.New(reflect.TypeOf(float32(0))).Elem() }, input: 1e300},
		{name: "enum type", rv: func() reflect.Value { return enumValue() }, input: []any{}},
		{name: "enum name", rv: func() reflect.Value { return enumValue() }, input: "not a code"},
		{name: "enum overflow", rv: func() reflect.Value { return enumValue() }, input: json.Number("9007199254740992")},
		{name: "unsupported kind", rv: func() reflect.Value { return reflect.New(channelType).Elem() }, input: 1},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if err := encodeJSONInto(tc.rv(), tc.input, "field"); err == nil {
				t.Errorf("encodeJSONInto(%T) error = nil, want an error", tc.input)
			}
		})
	}

	// Interface assignment succeeds when the concrete type is assignable.
	iface := reflect.New(reflect.TypeOf((*any)(nil)).Elem()).Elem()
	if err := encodeJSONInto(iface, xdr.Price{}, ""); err != nil {
		t.Errorf("encodeJSONInto(interface) error = %v", err)
	}
	if iface.IsNil() {
		t.Error("encodeJSONInto(interface) left the interface nil")
	}

	// Optional pointers stay nil when the field is absent.
	pointer := reflect.New(reflect.TypeOf((*xdr.Price)(nil))).Elem()
	if err := encodeJSONInto(pointer, nil, ""); err != nil {
		t.Errorf("encodeJSONInto(nil pointer) error = %v", err)
	}
	if !pointer.IsNil() {
		t.Error("encodeJSONInto(nil pointer) allocated a value")
	}

	// A derived annotation is accepted and ignored.
	contractCodeType := reflect.TypeOf(xdr.ContractCodeEntry{})
	rv := reflect.New(contractCodeType).Elem()
	derived := derivedFieldName(contractCodeType)
	if derived != "" {
		if err := encodeStruct(rv, map[string]any{derived: true}, ""); err != nil {
			t.Errorf("encodeStruct(derived field %q) error = %v", derived, err)
		}
	}
	if err := encodeStruct(rv, map[string]any{"Nope": true}, ""); err == nil {
		t.Error("encodeStruct(unknown field) error = nil, want an error")
	}
}

// derivedFieldName returns one registered annotation for t, if any.
func derivedFieldName(t reflect.Type) string {
	for name := range derivedNodeFields[t] {
		return name
	}
	return ""
}

func TestEncodeByteArrayLength(t *testing.T) {
	t.Parallel()

	rv := reflect.New(reflect.TypeOf([2]byte{})).Elem()
	if err := encodeByteArray(rv, []any{json.Number("1")}, ""); err == nil {
		t.Error("encodeByteArray(short) error = nil, want an error")
	}
}

func TestEncodeSpecialJSONClaimPredicate(t *testing.T) {
	t.Parallel()

	rv := reflect.New(reflect.TypeOf(xdr.ClaimPredicate{})).Elem()
	handled, err := encodeSpecialJSON(rv, "and(...)", "")
	if !handled {
		t.Error("encodeSpecialJSON(ClaimPredicate) handled = false, want true")
	}
	if !errors.Is(err, ErrUnsupportedType) {
		t.Errorf("encodeSpecialJSON(ClaimPredicate) error = %v, want ErrUnsupportedType", err)
	}
}
