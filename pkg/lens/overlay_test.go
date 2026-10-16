package lens_test

import (
	"strings"
	"testing"

	"github.com/stellar/go-stellar-sdk/xdr"

	"github.com/stellar-optics/stellar-xdr-lens/pkg/lens"
)

const (
	helloPeerID    = "GAAQCAIBAEAQCAIBAEAQCAIBAEAQCAIBAEAQCAIBAEAQCAIBAEAQDZ7H"
	helloNetworkID = "7ac33997544e3175d266bd022439b22cdb16508c01163f26e5cb2a3e1045a979"
	certKeyHex     = "4040404040404040404040404040404040404040404040404040404040404040"
	certSigHex     = "90909090909090909090909090909090909090909090909090909090909090909090909090909090909090909090909090909090909090909090909090909090"
	helloNonceHex  = "8080808080808080808080808080808080808080808080808080808080808080"
)

func TestDecodeHelloFixture(t *testing.T) {
	t.Parallel()

	v, err := lens.DecodeAs(loadFixture(t, "overlay_hello"), "Hello")
	if err != nil {
		t.Fatalf("DecodeAs() error = %v", err)
	}
	if v.Type != "Hello" {
		t.Errorf("type = %q, want Hello", v.Type)
	}
	raw, ok := v.Raw.(*xdr.Hello)
	if !ok {
		t.Fatalf("Raw is %T, want *xdr.Hello", v.Raw)
	}
	if raw.LedgerVersion != 22 || raw.OverlayVersion != 34 || raw.OverlayMinVersion != 32 {
		t.Errorf("versions = %d/%d/%d, want 22/34/32",
			raw.LedgerVersion, raw.OverlayVersion, raw.OverlayMinVersion)
	}
	if raw.ListeningPort != 11625 {
		t.Errorf("ListeningPort = %d, want 11625", raw.ListeningPort)
	}

	// The peer id is an ed25519 key: it must render as the same strkey
	// address the SCP enrichers produce for validator identities.
	peerID := v.Node.Find("PeerId")
	if peerID == nil || peerID.Value != helloPeerID {
		t.Errorf("PeerId = %v, want %q", peerID, helloPeerID)
	}
	if got := v.Node.Find("NetworkId"); got == nil || got.Note != helloNetworkID {
		t.Errorf("NetworkId = %v, want note %q", got, helloNetworkID)
	}
	if got := v.Node.Find("Cert.Pubkey"); got == nil || got.Note != certKeyHex {
		t.Errorf("Cert.Pubkey = %v, want note %q", got, certKeyHex)
	}
	if got := v.Node.Find("Cert.Sig"); got == nil || got.Note != certSigHex {
		t.Errorf("Cert.Sig = %v, want hex %q", got, certSigHex)
	}
	if got := v.Node.Find("Nonce"); got == nil || got.Value != helloNonceHex {
		t.Errorf("Nonce = %v, want hex %q", got, helloNonceHex)
	}
}

func TestDecodeStellarMessageFixture(t *testing.T) {
	t.Parallel()

	v, err := lens.DecodeAs(loadFixture(t, "overlay_stellar_message"), "StellarMessage")
	if err != nil {
		t.Fatalf("DecodeAs() error = %v", err)
	}
	raw, ok := v.Raw.(*xdr.StellarMessage)
	if !ok {
		t.Fatalf("Raw is %T, want *xdr.StellarMessage", v.Raw)
	}
	if raw.Type != xdr.MessageTypeHello {
		t.Errorf("Type = %v, want MessageTypeHello", raw.Type)
	}
	// The documentation label, not the generated constant name.
	if got := v.Node.Find("Type"); got == nil || got.Value != "hello" {
		t.Errorf("Type = %v, want the label hello", got)
	}
	if got := v.Node.Find("Hello.OverlayVersion"); got == nil || got.Value != uint64(34) {
		t.Errorf("Hello.OverlayVersion = %v, want 34", got)
	}
}

// TestDecodePeersMessageFixture covers the nested case that matters in
// practice: PeerAddress values arrive as a list inside a Peers message, where
// each one should render as host:port.
func TestDecodePeersMessageFixture(t *testing.T) {
	t.Parallel()

	v, err := lens.DecodeAs(loadFixture(t, "overlay_peers"), "StellarMessage")
	if err != nil {
		t.Fatalf("DecodeAs() error = %v", err)
	}
	if got := v.Node.Find("Type"); got == nil || got.Value != "peers" {
		t.Errorf("Type = %v, want the label peers", got)
	}
	peers := v.Node.Find("Peers")
	if peers == nil || len(peers.Children) != 2 {
		t.Fatalf("Peers = %v, want a list of 2 addresses", peers)
	}

	ipv4 := peers.Children[0]
	if ipv4.Value != "192.0.2.1:11625" || ipv4.Note != "" {
		t.Errorf("Peers[0] = (%v, %q), want (192.0.2.1:11625, \"\")", ipv4.Value, ipv4.Note)
	}
	ipv6 := peers.Children[1]
	if ipv6.Value != "[2001:db8::1]:11625" {
		t.Errorf("Peers[1].Value = %v, want [2001:db8::1]:11625", ipv6.Value)
	}
	if ipv6.Note != "2 failed connection(s)" {
		t.Errorf("Peers[1].Note = %q, want the failure count", ipv6.Note)
	}
}

func TestDecodePeerAddressFixtures(t *testing.T) {
	t.Parallel()

	tests := []struct {
		fixture string
		want    string
	}{
		{"overlay_peer_address", "192.0.2.1:11625"},
		{"overlay_peer_address_v6", "[2001:db8::1]:11625"},
	}

	for _, tc := range tests {
		t.Run(tc.fixture, func(t *testing.T) {
			t.Parallel()

			v, err := lens.DecodeAs(loadFixture(t, tc.fixture), "PeerAddress")
			if err != nil {
				t.Fatalf("DecodeAs() error = %v", err)
			}
			if _, ok := v.Raw.(*xdr.PeerAddress); !ok {
				t.Fatalf("Raw is %T, want *xdr.PeerAddress", v.Raw)
			}
			if v.Node.Value != tc.want {
				t.Errorf("PeerAddress = %v, want %q", v.Node.Value, tc.want)
			}
		})
	}
}

func TestDecodeAuthCertFixture(t *testing.T) {
	t.Parallel()

	v, err := lens.DecodeAs(loadFixture(t, "overlay_auth_cert"), "AuthCert")
	if err != nil {
		t.Fatalf("DecodeAs() error = %v", err)
	}
	raw, ok := v.Raw.(*xdr.AuthCert)
	if !ok {
		t.Fatalf("Raw is %T, want *xdr.AuthCert", v.Raw)
	}
	if raw.Expiration != 1893456000 {
		t.Errorf("Expiration = %d, want 1893456000", raw.Expiration)
	}
	if got := v.Node.Find("Pubkey"); got == nil || got.Note != certKeyHex {
		t.Errorf("Pubkey = %v, want note %q", got, certKeyHex)
	}
	if got := v.Node.Find("Sig"); got == nil || got.Note != certSigHex {
		t.Errorf("Sig = %v, want hex %q", got, certSigHex)
	}
}

func TestDecodeAuthFixture(t *testing.T) {
	t.Parallel()

	v, err := lens.DecodeAs(loadFixture(t, "overlay_auth"), "Auth")
	if err != nil {
		t.Fatalf("DecodeAs() error = %v", err)
	}
	raw, ok := v.Raw.(*xdr.Auth)
	if !ok {
		t.Fatalf("Raw is %T, want *xdr.Auth", v.Raw)
	}
	if raw.Flags != 1 {
		t.Errorf("Flags = %d, want 1", raw.Flags)
	}
}

func TestDecodeDontHaveFixture(t *testing.T) {
	t.Parallel()

	v, err := lens.DecodeAs(loadFixture(t, "overlay_dont_have"), "DontHave")
	if err != nil {
		t.Fatalf("DecodeAs() error = %v", err)
	}
	raw, ok := v.Raw.(*xdr.DontHave)
	if !ok {
		t.Fatalf("Raw is %T, want *xdr.DontHave", v.Raw)
	}
	if raw.Type != xdr.MessageTypeTxSet {
		t.Errorf("Type = %v, want MessageTypeTxSet", raw.Type)
	}
	if got := v.Node.Find("Type"); got == nil || got.Value != "tx_set" {
		t.Errorf("Type = %v, want the label tx_set", got)
	}
}

func TestDecodeOverlayErrorFixture(t *testing.T) {
	t.Parallel()

	v, err := lens.DecodeAs(loadFixture(t, "overlay_error"), "Error")
	if err != nil {
		t.Fatalf("DecodeAs() error = %v", err)
	}
	if got := v.Node.Find("Code"); got == nil || got.Value != "ErrorCodeErrConf" {
		t.Errorf("Code = %v, want ErrorCodeErrConf", got)
	}
	if got := v.Node.Find("Msg"); got == nil || got.Value != "ledger out of sync" {
		t.Errorf("Msg = %v, want the message", got)
	}
}

func TestDecodeOverlayRejectsTruncatedInput(t *testing.T) {
	t.Parallel()

	tests := []struct {
		fixture  string
		typeName string
	}{
		{"overlay_hello", "Hello"},
		{"overlay_auth", "Auth"},
		{"overlay_dont_have", "DontHave"},
		{"overlay_peer_address", "PeerAddress"},
		{"overlay_auth_cert", "AuthCert"},
		{"overlay_error", "Error"},
		{"overlay_stellar_message", "StellarMessage"},
	}

	for _, tc := range tests {
		t.Run(tc.fixture, func(t *testing.T) {
			t.Parallel()

			raw := loadFixtureRaw(tc.fixture)
			if _, err := lens.DecodeAs(raw[:len(raw)/2], tc.typeName); err == nil {
				t.Errorf("DecodeAs(%s truncated) error = nil, want an error", tc.fixture)
			}
		})
	}
}

func TestOverlayDetectionIsUnambiguous(t *testing.T) {
	t.Parallel()

	tests := []struct {
		fixture  string
		wantType string
	}{
		{"overlay_hello", "Hello"},
		{"overlay_peer_address", "PeerAddress"},
		{"overlay_peer_address_v6", "PeerAddress"},
		{"overlay_auth_cert", "AuthCert"},
		{"overlay_error", "Error"},
		{"overlay_stellar_message", "StellarMessage"},
		{"overlay_peers", "StellarMessage"},
	}

	for _, tc := range tests {
		t.Run(tc.fixture, func(t *testing.T) {
			t.Parallel()

			cands, err := lens.Detect(loadFixture(t, tc.fixture))
			if err != nil {
				t.Fatalf("Detect() error = %v", err)
			}
			if len(cands) != 1 || cands[0].Type != tc.wantType {
				names := make([]string, 0, len(cands))
				for _, c := range cands {
					names = append(names, c.Type)
				}
				t.Errorf("Detect() = [%s], want exactly [%s]", strings.Join(names, ", "), tc.wantType)
			}
		})
	}
}
