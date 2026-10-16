package lens

import (
	"bytes"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"

	"github.com/stellar/go-stellar-sdk/xdr"
)

// Sentinel errors for canonical XDR validation.
var (
	ErrNonCanonical        = errors.New("non-canonical XDR serialization")
	ErrMisalignedXDR       = fmt.Errorf("%w: byte length is not a multiple of 4", ErrNonCanonical)
	ErrNonCanonicalPadding = fmt.Errorf("%w: trailing padding bytes are non-zero", ErrNonCanonical)
	ErrTrailingGarbage     = fmt.Errorf("%w: trailing unconsumed bytes detected", ErrNonCanonical)
	ErrInvalidXDRLength    = fmt.Errorf("%w: payload exceeds available buffer", ErrNonCanonical)
	ErrNonCanonicalBool    = fmt.Errorf("%w: boolean integer must be strictly 0 or 1", ErrNonCanonical)
)

// CanonicalValidationError gives precise offset and diagnostic context for non-canonical bytes.
type CanonicalValidationError struct {
	Offset   int
	Expected byte
	Actual   byte
	Reason   string
}

func (e *CanonicalValidationError) Error() string {
	return fmt.Sprintf("%s at byte offset %d (expected 0x%02x, got 0x%02x)", e.Reason, e.Offset, e.Expected, e.Actual)
}

func (e *CanonicalValidationError) Unwrap() error {
	return ErrNonCanonicalPadding
}

func (e *CanonicalValidationError) Is(target error) bool {
	return errors.Is(target, ErrNonCanonical) || errors.Is(target, ErrNonCanonicalPadding)
}

// ValidateCanonicalXDR checks whether the provided raw XDR byte sequence adheres to RFC 4506 canonical standards:
// 1. Length must be a multiple of 4 bytes.
// 2. All padding bytes in opaque and string slices must strictly be 0x00.
// 3. Trailing byte boundaries must be consistent with declared record lengths.
func ValidateCanonicalXDR(data []byte) error {
	if len(data) == 0 {
		return nil
	}

	if len(data)%4 != 0 {
		return fmt.Errorf("%w: got %d bytes", ErrMisalignedXDR, len(data))
	}

	b64 := base64.StdEncoding.EncodeToString(data)
	cands, err := Detect(b64)
	if err == nil && len(cands) > 0 {
		return CheckCanonicalEncoding(data, cands[0].Value.Raw)
	}

	// If type is not recognized in registry, check standalone opaque/string pattern
	if len(data) >= 4 {
		valLen := int(uint32(data[0])<<24 | uint32(data[1])<<16 | uint32(data[2])<<8 | uint32(data[3]))
		rem := valLen % 4
		padLen := 0
		if rem != 0 {
			padLen = 4 - rem
		}
		if 4+valLen+padLen == len(data) && padLen > 0 {
			padStart := 4 + valLen
			for p := 0; p < padLen; p++ {
				if data[padStart+p] != 0x00 {
					return &CanonicalValidationError{
						Offset:   padStart + p,
						Expected: 0x00,
						Actual:   data[padStart+p],
						Reason:   "non-canonical padding leak",
					}
				}
			}
		}
	}

	return nil
}

// IsCanonicalXDR reports whether data passes canonical XDR validation.
func IsCanonicalXDR(data []byte) bool {
	return ValidateCanonicalXDR(data) == nil
}

// CanonicalizeXDR takes an XDR byte stream and cleans any non-canonical padding bytes,
// replacing non-zero padding with 0x00 without altering actual data payloads.
func CanonicalizeXDR(data []byte) ([]byte, error) {
	if len(data) == 0 {
		return []byte{}, nil
	}
	if len(data)%4 != 0 {
		return nil, fmt.Errorf("%w: got %d bytes", ErrMisalignedXDR, len(data))
	}

	b64 := base64.StdEncoding.EncodeToString(data)
	cands, err := Detect(b64)
	if err == nil && len(cands) > 0 {
		return CanonicalEncode(cands[0].Value.Raw)
	}

	clean := make([]byte, len(data))
	copy(clean, data)

	if len(clean) >= 4 {
		valLen := int(uint32(clean[0])<<24 | uint32(clean[1])<<16 | uint32(clean[2])<<8 | uint32(clean[3]))
		rem := valLen % 4
		padLen := 0
		if rem != 0 {
			padLen = 4 - rem
		}
		if 4+valLen+padLen == len(clean) && padLen > 0 {
			padStart := 4 + valLen
			for p := 0; p < padLen; p++ {
				clean[padStart+p] = 0x00
			}
		}
	}

	return clean, nil
}

// CanonicalEncode marshals any XDR data structure into canonical binary XDR.
// It verifies that the resulting encoding is 4-byte aligned and contains zero padding leaks.
func CanonicalEncode(val any) ([]byte, error) {
	if val == nil {
		return nil, fmt.Errorf("%w: cannot encode nil value", ErrNonCanonical)
	}

	var buf bytes.Buffer
	if _, err := xdr.Marshal(&buf, val); err != nil {
		return nil, fmt.Errorf("canonical encoding failed: %w", err)
	}

	b := buf.Bytes()
	if len(b)%4 != 0 {
		return nil, fmt.Errorf("%w: encoded length is %d", ErrMisalignedXDR, len(b))
	}

	return b, nil
}

// CanonicalEncodeBase64 marshals an XDR value to standard base64 using canonical serialization.
func CanonicalEncodeBase64(val any) (string, error) {
	b, err := CanonicalEncode(val)
	if err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(b), nil
}

// CheckCanonicalEncoding verifies that an existing XDR byte slice matches the canonical
// re-encoding of its decoded structure, detecting any hidden or non-canonical variations.
func CheckCanonicalEncoding(raw []byte, val any) error {
	if len(raw)%4 != 0 {
		return fmt.Errorf("%w: got %d bytes", ErrMisalignedXDR, len(raw))
	}

	reencoded, err := CanonicalEncode(val)
	if err != nil {
		return err
	}

	if len(raw) != len(reencoded) {
		if len(raw) > len(reencoded) {
			return fmt.Errorf("%w: input has %d bytes, canonical has %d bytes", ErrTrailingGarbage, len(raw), len(reencoded))
		}
		return fmt.Errorf("%w: byte length mismatch (expected %d, got %d)", ErrNonCanonical, len(reencoded), len(raw))
	}

	for i := 0; i < len(raw); i++ {
		if raw[i] != reencoded[i] {
			if reencoded[i] == 0x00 {
				return &CanonicalValidationError{
					Offset:   i,
					Expected: 0x00,
					Actual:   raw[i],
					Reason:   "non-canonical padding leak",
				}
			}
			return fmt.Errorf("%w: mismatch at byte offset %d (expected 0x%02x, got 0x%02x)", ErrNonCanonical, i, reencoded[i], raw[i])
		}
	}

	return nil
}

// ValidateTypeCanonicalXDR decodes raw XDR as the specified registered type and verifies its canonical encoding.
func ValidateTypeCanonicalXDR(raw []byte, typeName string) error {
	if len(raw)%4 != 0 {
		return fmt.Errorf("%w: got %d bytes", ErrMisalignedXDR, len(raw))
	}

	b64 := base64.StdEncoding.EncodeToString(raw)
	val, err := DecodeAs(b64, typeName)
	if err != nil {
		if strings.Contains(err.Error(), "input not fully consumed") {
			return fmt.Errorf("%w: %w", ErrTrailingGarbage, err)
		}
		return err
	}

	return CheckCanonicalEncoding(raw, val.Raw)
}
