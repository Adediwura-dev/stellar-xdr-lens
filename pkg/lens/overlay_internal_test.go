package lens

import (
	"encoding/hex"
	"reflect"
	"strings"
	"testing"

	"github.com/stellar/go-stellar-sdk/xdr"
)

func TestEnrichPeerAddress(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		addr      xdr.PeerAddress
		wantNote  string
		wantValue string
		wantOK    bool
	}{
		{
			name:      "ipv4 without failures",
			addr:      xdr.PeerAddress{Ip: xdr.PeerAddressIp{Type: xdr.IpAddrTypeIPv4, Ipv4: &[4]byte{192, 0, 2, 1}}, Port: 11625},
			wantValue: "192.0.2.1:11625",
			wantOK:    true,
		},
		{
			name: "ipv6 with failures",
			addr: xdr.PeerAddress{
				Ip:          xdr.PeerAddressIp{Type: xdr.IpAddrTypeIPv6, Ipv6: &[16]byte{0x20, 0x01, 0x0d, 0xb8}},
				Port:        11625,
				NumFailures: 2,
			},
			wantNote:  "2 failed connection(s)",
			wantValue: "[2001:db8::]:11625",
			wantOK:    true,
		},
		{
			name:   "missing ipv4 arm",
			addr:   xdr.PeerAddress{Ip: xdr.PeerAddressIp{Type: xdr.IpAddrTypeIPv4}, Port: 11625},
			wantOK: false,
		},
		{
			name:   "missing ipv6 arm",
			addr:   xdr.PeerAddress{Ip: xdr.PeerAddressIp{Type: xdr.IpAddrTypeIPv6}, Port: 11625},
			wantOK: false,
		},
		{
			name:   "unknown ip type",
			addr:   xdr.PeerAddress{Ip: xdr.PeerAddressIp{Type: xdr.IpAddrType(9)}, Port: 11625},
			wantOK: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			note, raw, ok := enrichPeerAddress(reflect.ValueOf(tc.addr))
			if ok != tc.wantOK {
				t.Fatalf("enrichPeerAddress() ok = %v, want %v", ok, tc.wantOK)
			}
			if !ok {
				return
			}
			if raw != tc.wantValue {
				t.Errorf("enrichPeerAddress() value = %v, want %q", raw, tc.wantValue)
			}
			if note != tc.wantNote {
				t.Errorf("enrichPeerAddress() note = %q, want %q", note, tc.wantNote)
			}
		})
	}

	if _, _, ok := enrichPeerAddress(reflect.ValueOf(xdr.Auth{})); ok {
		t.Error("enrichPeerAddress(Auth) succeeded, want ok=false")
	}
}

func TestPeerHost(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		ip     xdr.PeerAddressIp
		want   string
		wantOK bool
	}{
		{"ipv4", xdr.PeerAddressIp{Type: xdr.IpAddrTypeIPv4, Ipv4: &[4]byte{10, 0, 0, 7}}, "10.0.0.7", true},
		{"ipv6", xdr.PeerAddressIp{Type: xdr.IpAddrTypeIPv6, Ipv6: &[16]byte{0x20, 0x01, 0x0d, 0xb8}}, "2001:db8::", true},
		{"ipv4 without arm", xdr.PeerAddressIp{Type: xdr.IpAddrTypeIPv4}, "", false},
		{"ipv6 without arm", xdr.PeerAddressIp{Type: xdr.IpAddrTypeIPv6}, "", false},
		{"unknown type", xdr.PeerAddressIp{Type: xdr.IpAddrType(9)}, "", false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got, ok := peerHost(tc.ip)
			if got != tc.want || ok != tc.wantOK {
				t.Errorf("peerHost(%v) = (%q, %v), want (%q, %v)", tc.ip.Type, got, ok, tc.want, tc.wantOK)
			}
		})
	}
}

func TestEnrichCurve25519Public(t *testing.T) {
	t.Parallel()

	var pk xdr.Curve25519Public
	for i := range pk.Key {
		pk.Key[i] = byte(i)
	}
	note, raw, ok := enrichCurve25519Public(reflect.ValueOf(pk))
	if !ok {
		t.Fatal("enrichCurve25519Public() declined a valid key")
	}
	if want := hex.EncodeToString(pk.Key[:]); note != want {
		t.Errorf("enrichCurve25519Public() note = %q, want %q", note, want)
	}
	if raw != pk {
		t.Errorf("enrichCurve25519Public() value = %v, want the key struct", raw)
	}

	if _, _, ok := enrichCurve25519Public(reflect.ValueOf(xdr.Auth{})); ok {
		t.Error("enrichCurve25519Public(Auth) succeeded, want ok=false")
	}
}

func TestEnrichMessageType(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		value xdr.MessageType
		want  string
	}{
		{xdr.MessageTypeHello, "hello"},
		{xdr.MessageTypeTxSet, "tx_set"},
		{xdr.MessageTypeGeneralizedTxSet, "generalized_tx_set"},
		{xdr.MessageTypeSendMoreExtended, "send_more_extended"},
	} {
		note, raw, ok := enrichMessageType(reflect.ValueOf(tc.value))
		if !ok || note != "" || raw != tc.want {
			t.Errorf("enrichMessageType(%d) = (%q, %v, %v), want (_, %q, true)", tc.value, note, raw, ok, tc.want)
		}
	}

	if _, _, ok := enrichMessageType(reflect.ValueOf(xdr.MessageType(999))); ok {
		t.Error("enrichMessageType(999) succeeded, want ok=false")
	}
	if _, _, ok := enrichMessageType(reflect.ValueOf(xdr.Auth{})); ok {
		t.Error("enrichMessageType(Auth) succeeded, want ok=false")
	}
}

// TestOverlayMessageLabelIsExhaustive pins the invariant that every message
// type Stellar Core can send has a documentation label; a new SDK constant
// should fail this test rather than render as a raw number.
func TestOverlayMessageLabelIsExhaustive(t *testing.T) {
	t.Parallel()

	for i := 0; i < 64; i++ {
		mt := xdr.MessageType(i)
		if !isEnumType(reflect.TypeOf(mt)) || !mt.ValidEnum(int32(i)) {
			continue
		}
		label, ok := overlayMessageLabel(mt)
		if !ok {
			t.Errorf("overlayMessageLabel(%d) declined a valid message type", i)
			continue
		}
		if label != strings.ToLower(label) {
			t.Errorf("overlayMessageLabel(%d) = %q, want a lower-case label", i, label)
		}
	}
}
