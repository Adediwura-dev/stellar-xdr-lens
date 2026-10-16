package lens_test

import (
	"errors"
	"fmt"
	"testing"

	"github.com/stellar-optics/stellar-xdr-lens/pkg/lens"

	"github.com/stellar/go-stellar-sdk/xdr"
)

func TestErrorKindsSupportIsAndAs(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		err      error
		wantKind lens.ErrorKind
	}{
		{"empty input", lens.ErrEmptyInput, lens.ErrorKindEmptyInput},
		{"invalid base64", lens.ErrInvalidBase64, lens.ErrorKindInvalidBase64},
		{"invalid XDR", lens.ErrInvalidXDR, lens.ErrorKindInvalidXDR},
		{"no match", lens.ErrNoMatch, lens.ErrorKindNoMatch},
		{"unknown type", lens.ErrUnknownType, lens.ErrorKindUnknownType},
		{"invalid argument", lens.ErrInvalidArgument, lens.ErrorKindInvalidArgument},
		{"unsupported type", lens.ErrUnsupportedType, lens.ErrorKindUnsupportedType},
		{"not asset", lens.ErrNotAsset, lens.ErrorKindNotAsset},
		{"invalid network passphrase", lens.ErrInvalidNetworkPassphrase, lens.ErrorKindInvalidNetworkPassphrase},
		{"malformed asset", lens.ErrMalformedAsset, lens.ErrorKindMalformedAsset},
		{"not WASM", lens.ErrNotWASM, lens.ErrorKindNotWASM},
		{"malformed WASM", lens.ErrMalformedWASM, lens.ErrorKindMalformedWASM},
		{"no contract spec section", lens.ErrNoContractSpecSection, lens.ErrorKindNoContractSpecSection},
		{"invalid WASM header", lens.ErrInvalidWasm, lens.ErrorKindInvalidWasm},
		{"type not found", lens.ErrTypeNotFound, lens.ErrorKindTypeNotFound},
		{"function not found", lens.ErrFunctionNotFound, lens.ErrorKindFunctionNotFound},
		{"invalid contract spec", lens.ErrInvalidContractSpec, lens.ErrorKindInvalidContractSpec},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			wrapped := fmt.Errorf("operation context: %w", tc.err)
			if !errors.Is(wrapped, tc.err) {
				t.Errorf("errors.Is(%v, %v) = false", wrapped, tc.err)
			}
			var gotKind lens.ErrorKind
			if !errors.As(wrapped, &gotKind) {
				t.Fatalf("errors.As(%v, *ErrorKind) = false", wrapped)
			}
			if gotKind != tc.wantKind {
				t.Errorf("ErrorKind = %q, want %q", gotKind, tc.wantKind)
			}
			if gotKind.Error() != tc.wantKind.Error() {
				t.Errorf("ErrorKind.Error() = %q, want %q", gotKind.Error(), tc.wantKind.Error())
			}
		})
	}
}

func TestPublicErrorsAreClassifiable(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		err  error
		want error
	}{
		{"empty decode", func() error { _, err := lens.Detect(" \n"); return err }(), lens.ErrEmptyInput},
		{"invalid base64 decode", func() error { _, err := lens.Detect("%%%"); return err }(), lens.ErrInvalidBase64},
		{"unrecognized XDR", func() error { _, err := lens.Detect("aGVsbG8gd29ybGQ="); return err }(), lens.ErrNoMatch},
		{"invalid base64 DecodeAs", func() error { _, err := lens.DecodeAs("%%%", "Asset"); return err }(), lens.ErrInvalidBase64},
		{"invalid XDR DecodeAs", func() error { _, err := lens.DecodeAs("aGVsbG8gd29ybGQ=", "Asset"); return err }(), lens.ErrInvalidXDR},
		{"unknown DecodeAs type", func() error { _, err := lens.DecodeAs("AAAAAA==", "unknown"); return err }(), lens.ErrUnknownType},
		{"nil diff inputs", func() error { _, err := lens.Diff(nil, nil); return err }(), lens.ErrInvalidArgument},
		{"empty asset network", func() error { _, err := lens.SACContractID(xdr.Asset{}, ""); return err }(), lens.ErrInvalidNetworkPassphrase},
		{"empty contract spec", func() error { _, err := lens.ParseContractSpec(nil); return err }(), lens.ErrInvalidContractSpec},
		{"non-WASM contract code", func() error { _, err := lens.ParseContractCode(nil); return err }(), lens.ErrNotWASM},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if !errors.Is(tc.err, tc.want) {
				t.Errorf("error = %v, want errors.Is(_, %v)", tc.err, tc.want)
			}
		})
	}
}
