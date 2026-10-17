package lens

import (
	"encoding/base64"
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/stellar/go-stellar-sdk/xdr"
)

func TestValidateCanonicalXDR(t *testing.T) {
	// Empty slice is valid
	if err := ValidateCanonicalXDR(nil); err != nil {
		t.Errorf("expected nil for empty slice: %v", err)
	}

	// Misaligned length (not multiple of 4)
	misaligned := []byte{0x00, 0x01, 0x02}
	if err := ValidateCanonicalXDR(misaligned); !errors.Is(err, ErrMisalignedXDR) {
		t.Errorf("expected ErrMisalignedXDR, got: %v", err)
	}

	// Valid 16-byte ScVal string: ScvString(9), length 5 ("HELLO"), 3 zero padding bytes
	validPadded := []byte{
		0x00, 0x00, 0x00, 0x09, // ScvString = 9
		0x00, 0x00, 0x00, 0x05, // length 5
		'H', 'E', 'L', 'L',
		'O', 0x00, 0x00, 0x00, // 'O' + 3 zero padding bytes
	}
	if err := ValidateCanonicalXDR(validPadded); err != nil {
		t.Errorf("expected clean validation, got: %v", err)
	}
	if !IsCanonicalXDR(validPadded) {
		t.Errorf("expected IsCanonicalXDR to return true")
	}

	// Corrupted non-zero padding byte at offset 13
	corruptPadding := make([]byte, len(validPadded))
	copy(corruptPadding, validPadded)
	corruptPadding[13] = 0xFF // non-canonical padding leak!

	err := ValidateCanonicalXDR(corruptPadding)
	if err == nil {
		t.Fatal("expected canonical validation error on corrupt padding")
	}
	if !errors.Is(err, ErrNonCanonicalPadding) {
		t.Errorf("expected errors.Is to match ErrNonCanonicalPadding, got: %v", err)
	}
	var valErr *CanonicalValidationError
	if !errors.As(err, &valErr) || valErr.Offset != 13 || valErr.Actual != 0xFF {
		t.Errorf("expected error details at offset 13, actual 0xFF, got: %+v", valErr)
	}
	if !strings.Contains(valErr.Error(), "byte offset 13") {
		t.Errorf("unexpected error string: %s", valErr.Error())
	}
	if !valErr.Is(ErrNonCanonical) {
		t.Errorf("expected valErr.Is(ErrNonCanonical) to be true")
	}

	// Test CanonicalizeXDR cleans up the corrupt padding
	cleaned, err := CanonicalizeXDR(corruptPadding)
	if err != nil {
		t.Fatalf("failed CanonicalizeXDR: %v", err)
	}
	if err := ValidateCanonicalXDR(cleaned); err != nil {
		t.Errorf("cleaned bytes failed canonical validation: %v", err)
	}
	if cleaned[13] != 0x00 {
		t.Errorf("expected offset 13 to be zeroed, got 0x%02x", cleaned[13])
	}

	// CanonicalizeXDR on empty and misaligned
	cEmpty, err := CanonicalizeXDR(nil)
	if err != nil || len(cEmpty) != 0 {
		t.Errorf("failed CanonicalizeXDR on empty: %v", err)
	}
	_, err = CanonicalizeXDR(misaligned)
	if err == nil || !errors.Is(err, ErrMisalignedXDR) {
		t.Errorf("expected ErrMisalignedXDR from CanonicalizeXDR, got: %v", err)
	}
}

func TestCanonicalEncode(t *testing.T) {
	// Nil value error
	if _, err := CanonicalEncode(nil); !errors.Is(err, ErrNonCanonical) {
		t.Errorf("expected ErrNonCanonical for nil value, got: %v", err)
	}

	// Valid XDR structure
	asset := xdr.Asset{Type: xdr.AssetTypeAssetTypeNative}
	b, err := CanonicalEncode(asset)
	if err != nil {
		t.Fatalf("failed CanonicalEncode: %v", err)
	}
	if len(b)%4 != 0 {
		t.Errorf("expected multiple of 4, got length %d", len(b))
	}

	// CanonicalEncodeBase64
	b64, err := CanonicalEncodeBase64(asset)
	if err != nil || b64 == "" {
		t.Fatalf("failed CanonicalEncodeBase64: %v", err)
	}
	decoded, err := base64.StdEncoding.DecodeString(b64)
	if err != nil || len(decoded) != len(b) {
		t.Fatalf("base64 mismatch: %v", err)
	}

	// CheckCanonicalEncoding with match
	if err := CheckCanonicalEncoding(b, asset); err != nil {
		t.Errorf("expected match in CheckCanonicalEncoding, got: %v", err)
	}

	// CheckCanonicalEncoding with mismatch
	altered := append(b, 0x00, 0x00, 0x00, 0x00)
	if err := CheckCanonicalEncoding(altered, asset); err == nil {
		t.Errorf("expected error on mismatched byte slice")
	}
}

func TestValidateTypeCanonicalXDRWithFixtures(t *testing.T) {
	// Read real fixture: testdata/tx_success.env.txt
	rawB64, err := os.ReadFile("../../testdata/tx_success.env.txt")
	if err != nil {
		rawB64, err = os.ReadFile("testdata/tx_success.env.txt")
	}
	if err != nil {
		rawB64, err = os.ReadFile("../testdata/tx_success.env.txt")
	}
	if err != nil {
		t.Fatalf("failed reading fixture tx_success.env.txt: %v", err)
	}

	trimmed := strings.TrimSpace(string(rawB64))
	rawBytes, err := base64.StdEncoding.DecodeString(trimmed)
	if err != nil {
		t.Fatalf("failed decoding base64 fixture: %v", err)
	}

	// Validate fixture as TransactionEnvelope
	if err := ValidateTypeCanonicalXDR(rawBytes, "TransactionEnvelope"); err != nil {
		t.Errorf("expected canonical validation to pass on real fixture: %v", err)
	}

	// Unknown type error
	if err := ValidateTypeCanonicalXDR(rawBytes, "UnknownTypeXYZ"); err == nil {
		t.Errorf("expected error for unknown XDR type")
	}

	// Trailing garbage error
	trailing := append(rawBytes, 0x00, 0x00, 0x00, 0x00)
	if err := ValidateTypeCanonicalXDR(trailing, "TransactionEnvelope"); !errors.Is(err, ErrTrailingGarbage) {
		t.Errorf("expected ErrTrailingGarbage, got: %v", err)
	}

	// Misaligned input error
	misaligned := rawBytes[:len(rawBytes)-1]
	if err := ValidateTypeCanonicalXDR(misaligned, "TransactionEnvelope"); !errors.Is(err, ErrMisalignedXDR) {
		t.Errorf("expected ErrMisalignedXDR, got: %v", err)
	}
}
