package lens

import (
	"strings"
	"testing"

	"github.com/stellar/go-stellar-sdk/xdr"
)

// TestSignerKeyLabelDefensivePaths drives signerKeyLabel with malformed signer
// keys — the shapes a corrupt or adversarially crafted envelope can carry.
// The label must degrade, never panic.
func TestSignerKeyLabelDefensivePaths(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		key  xdr.SignerKey
		want string
	}{
		{
			name: "ed25519 without the union arm",
			key:  xdr.SignerKey{Type: xdr.SignerKeyTypeSignerKeyTypeEd25519},
			want: "signer",
		},
		{
			name: "pre-auth without the union arm",
			key:  xdr.SignerKey{Type: xdr.SignerKeyTypeSignerKeyTypePreAuthTx},
			want: "pre-auth signer",
		},
		{
			name: "hash-x without the union arm",
			key:  xdr.SignerKey{Type: xdr.SignerKeyTypeSignerKeyTypeHashX},
			want: "hash-x signer",
		},
		{
			name: "signed payload without the union arm",
			key:  xdr.SignerKey{Type: xdr.SignerKeyTypeSignerKeyTypeEd25519SignedPayload},
			want: "signed-payload signer",
		},
		{
			name: "unknown signer key type",
			key:  xdr.SignerKey{Type: xdr.SignerKeyType(99)},
			want: "signer",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			if got := signerKeyLabel(tc.key); got != tc.want {
				t.Errorf("signerKeyLabel() = %q, want %q", got, tc.want)
			}
		})
	}
}

// TestSignerKeyLabelRendersKnownKeys pins the readable forms of well-formed
// keys, including the abbreviated hex for non-addressable kinds.
func TestSignerKeyLabelRendersKnownKeys(t *testing.T) {
	t.Parallel()

	preAuth := xdr.SignerKey{Type: xdr.SignerKeyTypeSignerKeyTypePreAuthTx, PreAuthTx: uint256Of(0x3C)}
	if got := signerKeyLabel(preAuth); !strings.HasPrefix(got, "pre-auth signer 3c3c3c3c…") {
		t.Errorf("signerKeyLabel(pre-auth) = %q, want abbreviated hex after the kind", got)
	}
	hashX := xdr.SignerKey{Type: xdr.SignerKeyTypeSignerKeyTypeHashX, HashX: uint256Of(0x4D)}
	if got := signerKeyLabel(hashX); !strings.HasPrefix(got, "hash-x signer 4d4d4d4d…") {
		t.Errorf("signerKeyLabel(hash-x) = %q, want abbreviated hex after the kind", got)
	}
}

func uint256Of(seed byte) *xdr.Uint256 {
	var k xdr.Uint256
	for i := range k {
		k[i] = seed
	}
	return &k
}
