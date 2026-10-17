package lens

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"regexp"
	"sort"
	"strings"

	"github.com/stellar/go-stellar-sdk/strkey"
	"github.com/stellar/go-stellar-sdk/xdr"
)

// ErrInvalidScVal indicates a failure constructing an ScVal from JSON.
var ErrInvalidScVal = errors.New("invalid scval specification")

var (
	symbolRegex = regexp.MustCompile(`^[a-zA-Z0-9_]{1,32}$`)
	u64Mask     = new(big.Int).SetUint64(^uint64(0))
)

// ParseScValJSON deserialises JSON bytes and constructs the corresponding xdr.ScVal.
func ParseScValJSON(jsonData []byte) (xdr.ScVal, error) {
	if len(jsonData) == 0 {
		return xdr.ScVal{}, fmt.Errorf("%w: empty input", ErrInvalidScVal)
	}
	var raw any
	dec := json.NewDecoder(strings.NewReader(string(jsonData)))
	dec.UseNumber()
	if err := dec.Decode(&raw); err != nil {
		return xdr.ScVal{}, fmt.Errorf("%w: invalid JSON syntax: %w", ErrInvalidScVal, err)
	}
	return ParseScVal(raw)
}

// MustParseScVal parses input and panics on failure.
func MustParseScVal(input any) xdr.ScVal {
	val, err := ParseScVal(input)
	if err != nil {
		panic(err)
	}
	return val
}

// EncodeScValBase64 converts an intuitive JSON literal or tagged object into base64 XDR.
func EncodeScValBase64(input any) (string, error) {
	scVal, err := ParseScVal(input)
	if err != nil {
		return "", err
	}
	bytes, err := scVal.MarshalBinary()
	if err != nil {
		return "", fmt.Errorf("scval encode binary: %w", err)
	}
	return base64.StdEncoding.EncodeToString(bytes), nil
}

// ParseScVal converts intuitive Go literals, maps, or tagged specs into an xdr.ScVal.
func ParseScVal(input any) (xdr.ScVal, error) {
	var zero xdr.ScVal

	if input == nil {
		return xdr.ScVal{Type: xdr.ScValTypeScvVoid}, nil
	}

	switch v := input.(type) {
	case xdr.ScVal:
		return v, nil
	case *xdr.ScVal:
		if v == nil {
			return zero, fmt.Errorf("%w: nil *xdr.ScVal", ErrInvalidScVal)
		}
		return *v, nil
	case bool:
		b := v
		return xdr.ScVal{Type: xdr.ScValTypeScvBool, B: &b}, nil
	case int, int8, int16, int32, int64:
		n, err := CoerceInt64(v)
		if err != nil {
			return zero, err
		}
		x := xdr.Int64(n)
		return xdr.ScVal{Type: xdr.ScValTypeScvI64, I64: &x}, nil
	case uint, uint8, uint16, uint32, uint64:
		n, err := CoerceUint64(v)
		if err != nil {
			return zero, err
		}
		x := xdr.Uint64(n)
		return xdr.ScVal{Type: xdr.ScValTypeScvU64, U64: &x}, nil
	case json.Number:
		if i, err := v.Int64(); err == nil {
			x := xdr.Int64(i)
			return xdr.ScVal{Type: xdr.ScValTypeScvI64, I64: &x}, nil
		}
		return parseStringScVal(string(v))
	case string:
		return parseStringScVal(v)
	case []any:
		return parseVecScVal(v)
	case map[string]any:
		return parseMapOrTaggedScVal(v)
	default:
		return zero, fmt.Errorf("%w: unsupported input type %T", ErrInvalidScVal, input)
	}
}

func parseStringScVal(s string) (xdr.ScVal, error) {
	trimmed := strings.TrimSpace(s)

	// StrKey Account ID (G...)
	if strings.HasPrefix(trimmed, "G") && len(trimmed) == 56 {
		acc, err := CoerceAccountID(trimmed)
		if err == nil {
			scAddr := xdr.ScAddress{
				Type:      xdr.ScAddressTypeScAddressTypeAccount,
				AccountId: &acc,
			}
			return xdr.ScVal{Type: xdr.ScValTypeScvAddress, Address: &scAddr}, nil
		}
	}

	// StrKey Contract ID (C...)
	if strings.HasPrefix(trimmed, "C") && len(trimmed) == 56 {
		raw, err := strkey.Decode(strkey.VersionByteContract, trimmed)
		if err == nil && len(raw) == 32 {
			var h xdr.Hash
			copy(h[:], raw)
			scAddr := xdr.ScAddress{
				Type:       xdr.ScAddressTypeScAddressTypeContract,
				ContractId: (*xdr.ContractId)(&h),
			}
			return xdr.ScVal{Type: xdr.ScValTypeScvAddress, Address: &scAddr}, nil
		}
	}

	// Alphanumeric symbol (<= 32 chars)
	if symbolRegex.MatchString(trimmed) {
		sym := xdr.ScSymbol(trimmed)
		return xdr.ScVal{Type: xdr.ScValTypeScvSymbol, Sym: &sym}, nil
	}

	// Fallback to ScvString
	str := xdr.ScString(s)
	return xdr.ScVal{Type: xdr.ScValTypeScvString, Str: &str}, nil
}

func parseVecScVal(items []any) (xdr.ScVal, error) {
	var zero xdr.ScVal
	vec := make([]xdr.ScVal, len(items))
	for i, item := range items {
		elem, err := ParseScVal(item)
		if err != nil {
			return zero, fmt.Errorf("vector element [%d]: %w", i, err)
		}
		vec[i] = elem
	}
	scVec := xdr.ScVec(vec)
	scVecPtr := &scVec
	return xdr.ScVal{Type: xdr.ScValTypeScvVec, Vec: &scVecPtr}, nil
}

func parseMapOrTaggedScVal(m map[string]any) (xdr.ScVal, error) {
	var zero xdr.ScVal

	// 1. Tagged object with "type" / "Type"
	typVal, hasType := m["type"]
	if !hasType {
		typVal, hasType = m["Type"]
	}
	if hasType {
		typStr, ok := typVal.(string)
		if !ok {
			return zero, fmt.Errorf("%w: 'type' must be string, got %T", ErrInvalidScVal, typVal)
		}
		valPayload, hasVal := m["value"]
		if !hasVal {
			valPayload = m["val"]
		}
		return parseTaggedScVal(strings.ToLower(strings.TrimSpace(typStr)), valPayload, m)
	}

	// 2. Single-key tagged shortcut: e.g. {"u32": 123}, {"symbol": "abc"}, {"address": "C..."}
	if len(m) == 1 {
		for k, v := range m {
			kLower := strings.ToLower(k)
			switch kLower {
			case "u32", "i32", "u64", "i64", "u128", "i128", "u256", "i256",
				"symbol", "string", "bytes", "address", "vec", "timepoint", "duration", "bool", "void":
				return parseTaggedScVal(kLower, v, m)
			}
		}
	}

	// 3. Explicit map entries array: e.g. {"entries": [{"key": ..., "val": ...}]}
	if entriesVal, ok := m["entries"]; ok {
		if slice, ok := entriesVal.([]any); ok {
			return parseScMapEntries(slice)
		}
	}

	// 4. Untagged key-value map: convert keys to symbols and values to ScVal
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	entries := make([]xdr.ScMapEntry, len(keys))
	for i, k := range keys {
		sym := xdr.ScSymbol(k)
		keyVal := xdr.ScVal{Type: xdr.ScValTypeScvSymbol, Sym: &sym}
		elemVal, err := ParseScVal(m[k])
		if err != nil {
			return zero, fmt.Errorf("map entry %q: %w", k, err)
		}
		entries[i] = xdr.ScMapEntry{
			Key: keyVal,
			Val: elemVal,
		}
	}

	scMap := xdr.ScMap(entries)
	scMapPtr := &scMap
	return xdr.ScVal{Type: xdr.ScValTypeScvMap, Map: &scMapPtr}, nil
}

func parseScMapEntries(slice []any) (xdr.ScVal, error) {
	var zero xdr.ScVal
	entries := make([]xdr.ScMapEntry, len(slice))
	for i, item := range slice {
		entryMap, ok := item.(map[string]any)
		if !ok {
			return zero, fmt.Errorf("%w: map entry [%d] must be an object with 'key' and 'val'", ErrInvalidScVal, i)
		}
		kVal, hasK := entryMap["key"]
		vVal, hasV := entryMap["val"]
		if !hasV {
			vVal, hasV = entryMap["value"]
		}
		if !hasK || !hasV {
			return zero, fmt.Errorf("%w: map entry [%d] requires 'key' and 'val'", ErrInvalidScVal, i)
		}
		kScVal, err := ParseScVal(kVal)
		if err != nil {
			return zero, fmt.Errorf("entry [%d] key: %w", i, err)
		}
		vScVal, err := ParseScVal(vVal)
		if err != nil {
			return zero, fmt.Errorf("entry [%d] val: %w", i, err)
		}
		entries[i] = xdr.ScMapEntry{Key: kScVal, Val: vScVal}
	}
	scMap := xdr.ScMap(entries)
	scMapPtr := &scMap
	return xdr.ScVal{Type: xdr.ScValTypeScvMap, Map: &scMapPtr}, nil
}

func parseTaggedScVal(typ string, val any, fullMap map[string]any) (xdr.ScVal, error) {
	var zero xdr.ScVal

	switch typ {
	case "void":
		return xdr.ScVal{Type: xdr.ScValTypeScvVoid}, nil
	case "bool":
		b, err := CoerceBool(val)
		if err != nil {
			return zero, err
		}
		return xdr.ScVal{Type: xdr.ScValTypeScvBool, B: &b}, nil
	case "u32":
		n, err := CoerceUint32(val)
		if err != nil {
			return zero, err
		}
		u := xdr.Uint32(n)
		return xdr.ScVal{Type: xdr.ScValTypeScvU32, U32: &u}, nil
	case "i32":
		n, err := CoerceInt32(val)
		if err != nil {
			return zero, err
		}
		i := xdr.Int32(n)
		return xdr.ScVal{Type: xdr.ScValTypeScvI32, I32: &i}, nil
	case "u64":
		n, err := CoerceUint64(val)
		if err != nil {
			return zero, err
		}
		u := xdr.Uint64(n)
		return xdr.ScVal{Type: xdr.ScValTypeScvU64, U64: &u}, nil
	case "i64":
		n, err := CoerceInt64(val)
		if err != nil {
			return zero, err
		}
		i := xdr.Int64(n)
		return xdr.ScVal{Type: xdr.ScValTypeScvI64, I64: &i}, nil
	case "timepoint":
		n, err := CoerceUint64(val)
		if err != nil {
			return zero, err
		}
		tp := xdr.TimePoint(n)
		return xdr.ScVal{Type: xdr.ScValTypeScvTimepoint, Timepoint: &tp}, nil
	case "duration":
		n, err := CoerceUint64(val)
		if err != nil {
			return zero, err
		}
		dur := xdr.Duration(n)
		return xdr.ScVal{Type: xdr.ScValTypeScvDuration, Duration: &dur}, nil
	case "u128":
		return parseU128(val, fullMap)
	case "i128":
		return parseI128(val, fullMap)
	case "u256":
		return parseU256(val)
	case "i256":
		return parseI256(val)
	case "bytes":
		b, err := CoerceBytes(val)
		if err != nil {
			return zero, err
		}
		scBytes := xdr.ScBytes(b)
		return xdr.ScVal{Type: xdr.ScValTypeScvBytes, Bytes: &scBytes}, nil
	case "string":
		s, err := CoerceString(val)
		if err != nil {
			return zero, err
		}
		str := xdr.ScString(s)
		return xdr.ScVal{Type: xdr.ScValTypeScvString, Str: &str}, nil
	case "symbol":
		s, err := CoerceString(val)
		if err != nil {
			return zero, err
		}
		if len(s) > 32 {
			return zero, fmt.Errorf("%w: symbol length %d exceeds 32 characters", ErrInvalidScVal, len(s))
		}
		sym := xdr.ScSymbol(s)
		return xdr.ScVal{Type: xdr.ScValTypeScvSymbol, Sym: &sym}, nil
	case "vec":
		slice, ok := val.([]any)
		if !ok {
			return zero, fmt.Errorf("%w: vec payload must be array, got %T", ErrInvalidScVal, val)
		}
		return parseVecScVal(slice)
	case "map":
		if slice, ok := val.([]any); ok {
			return parseScMapEntries(slice)
		}
		if m, ok := val.(map[string]any); ok {
			return parseMapOrTaggedScVal(m)
		}
		return zero, fmt.Errorf("%w: map payload must be object or array of entries", ErrInvalidScVal)
	case "address":
		s, err := CoerceString(val)
		if err != nil {
			return zero, err
		}
		return parseStringScVal(s)
	case "contract_instance":
		return xdr.ScVal{Type: xdr.ScValTypeScvLedgerKeyContractInstance}, nil
	case "error":
		code, err := CoerceUint32(fullMap["code"])
		if err != nil {
			return zero, err
		}
		u := xdr.Uint32(code)
		errVal := xdr.ScError{
			Type:         xdr.ScErrorTypeSceContract,
			ContractCode: &u,
		}
		return xdr.ScVal{Type: xdr.ScValTypeScvError, Error: &errVal}, nil
	default:
		return zero, fmt.Errorf("%w: unsupported ScVal type discriminant %q", ErrInvalidScVal, typ)
	}
}

func parseU128(val any, fullMap map[string]any) (xdr.ScVal, error) {
	var zero xdr.ScVal
	if hiVal, ok := fullMap["hi"]; ok {
		hi, err1 := CoerceUint64(hiVal)
		lo, err2 := CoerceUint64(fullMap["lo"])
		if err1 == nil && err2 == nil {
			u128 := xdr.UInt128Parts{Hi: xdr.Uint64(hi), Lo: xdr.Uint64(lo)}
			return xdr.ScVal{Type: xdr.ScValTypeScvU128, U128: &u128}, nil
		}
	}
	bi, err := CoerceBigInt(val)
	if err != nil {
		return zero, err
	}
	if bi.Sign() < 0 {
		return zero, fmt.Errorf("%w: u128 cannot be negative", ErrInvalidScVal)
	}
	lo := new(big.Int).And(bi, u64Mask).Uint64()
	hi := new(big.Int).Rsh(bi, 64).Uint64()
	u128 := xdr.UInt128Parts{Hi: xdr.Uint64(hi), Lo: xdr.Uint64(lo)}
	return xdr.ScVal{Type: xdr.ScValTypeScvU128, U128: &u128}, nil
}

func parseI128(val any, fullMap map[string]any) (xdr.ScVal, error) {
	var zero xdr.ScVal
	if hiVal, ok := fullMap["hi"]; ok {
		hi, err1 := CoerceInt64(hiVal)
		lo, err2 := CoerceUint64(fullMap["lo"])
		if err1 == nil && err2 == nil {
			i128 := xdr.Int128Parts{Hi: xdr.Int64(hi), Lo: xdr.Uint64(lo)}
			return xdr.ScVal{Type: xdr.ScValTypeScvI128, I128: &i128}, nil
		}
	}
	bi, err := CoerceBigInt(val)
	if err != nil {
		return zero, err
	}
	lo := new(big.Int).And(bi, u64Mask).Uint64()
	hi := new(big.Int).Rsh(bi, 64).Int64()
	i128 := xdr.Int128Parts{Hi: xdr.Int64(hi), Lo: xdr.Uint64(lo)}
	return xdr.ScVal{Type: xdr.ScValTypeScvI128, I128: &i128}, nil
}

func parseU256(val any) (xdr.ScVal, error) {
	var zero xdr.ScVal
	hash, err := CoerceHash(val)
	if err != nil {
		bi, bErr := CoerceBigInt(val)
		if bErr != nil {
			return zero, err
		}
		b := bi.Bytes()
		if len(b) > 32 {
			return zero, fmt.Errorf("%w: u256 exceeds 32 bytes", ErrInvalidScVal)
		}
		copy(hash[32-len(b):], b)
	}
	u256 := xdr.UInt256Parts{
		HiHi: xdr.Uint64(new(big.Int).SetBytes(hash[0:8]).Uint64()),
		HiLo: xdr.Uint64(new(big.Int).SetBytes(hash[8:16]).Uint64()),
		LoHi: xdr.Uint64(new(big.Int).SetBytes(hash[16:24]).Uint64()),
		LoLo: xdr.Uint64(new(big.Int).SetBytes(hash[24:32]).Uint64()),
	}
	return xdr.ScVal{Type: xdr.ScValTypeScvU256, U256: &u256}, nil
}

func parseI256(val any) (xdr.ScVal, error) {
	var zero xdr.ScVal
	hash, err := CoerceHash(val)
	if err != nil {
		bi, bErr := CoerceBigInt(val)
		if bErr != nil {
			return zero, err
		}
		b := bi.Bytes()
		if len(b) > 32 {
			return zero, fmt.Errorf("%w: i256 exceeds 32 bytes", ErrInvalidScVal)
		}
		copy(hash[32-len(b):], b)
	}
	i256 := xdr.Int256Parts{
		HiHi: xdr.Int64(new(big.Int).SetBytes(hash[0:8]).Int64()),
		HiLo: xdr.Uint64(new(big.Int).SetBytes(hash[8:16]).Uint64()),
		LoHi: xdr.Uint64(new(big.Int).SetBytes(hash[16:24]).Uint64()),
		LoLo: xdr.Uint64(new(big.Int).SetBytes(hash[24:32]).Uint64()),
	}
	return xdr.ScVal{Type: xdr.ScValTypeScvI256, I256: &i256}, nil
}
