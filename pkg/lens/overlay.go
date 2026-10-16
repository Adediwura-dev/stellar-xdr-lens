package lens

import (
	"encoding/hex"
	"net"
	"reflect"
	"strconv"

	"github.com/stellar/go-stellar-sdk/xdr"
)

// Stellar Core peers speak a small message protocol of their own, separate
// from the transaction XDR. A peer announces itself with a Hello carrying its
// version, network and authentication certificate; replies with Auth; answers
// a request for a transaction set it does not hold with DontHave; and shares
// the addresses it knows about as PeerAddress records. Only three of those —
// Hello, Auth and DontHave — are messages in their own right; the rest are
// payloads nested inside them or inside StellarMessage.
//
// On the wire the types are unfriendly: a peer id is a bare ed25519 key, a
// certificate key and nonce are opaque bytes, and a peer address is an IP
// union plus a port. The renderings in this file turn those into the forms
// Stellar Core logs and operators recognise.
func init() {
	// NodeId is already covered by the SCP enricher registered in scp.go:
	// a peer id and a validator identity are the same ed25519 key rendered
	// as the same strkey address. Only the overlay-specific shapes need
	// their own handlers.
	enricherFor[reflect.TypeOf(xdr.PeerAddress{})] = enrichPeerAddress
	enricherFor[reflect.TypeOf(xdr.Curve25519Public{})] = enrichCurve25519Public
	enricherFor[reflect.TypeOf(xdr.MessageType(0))] = enrichMessageType
}

// enrichPeerAddress renders a peer address as host:port, the form used in
// logs and configuration. The port is carried separately from the IP union,
// so it is joined here rather than by the generic walker. A non-zero failure
// count is worth surfacing: Stellar Core only advertises peers it has not
// repeatedly failed to reach, so a count on an address explains why it is
// being dropped.
func enrichPeerAddress(rv reflect.Value) (string, any, bool) {
	pa, ok := rv.Interface().(xdr.PeerAddress)
	if !ok {
		return "", nil, false
	}
	host, ok := peerHost(pa.Ip)
	if !ok {
		return "", nil, false
	}
	addr := net.JoinHostPort(host, strconv.FormatUint(uint64(pa.Port), 10))
	if pa.NumFailures > 0 {
		return strconv.FormatUint(uint64(pa.NumFailures), 10) + " failed connection(s)", addr, true
	}
	return "", addr, true
}

// peerHost extracts the textual host from the address union. An absent arm
// declines rather than guessing, which keeps a malformed value on the generic
// rendering path instead of printing a misleading 0.0.0.0.
func peerHost(ip xdr.PeerAddressIp) (string, bool) {
	switch ip.Type {
	case xdr.IpAddrTypeIPv4:
		if ip.Ipv4 == nil {
			return "", false
		}
		return net.IPv4(ip.Ipv4[0], ip.Ipv4[1], ip.Ipv4[2], ip.Ipv4[3]).String(), true
	case xdr.IpAddrTypeIPv6:
		if ip.Ipv6 == nil {
			return "", false
		}
		return net.IP(ip.Ipv6[:]).String(), true
	default:
		return "", false
	}
}

// enrichCurve25519Public renders a peer certificate key as hex. The size is
// fixed and small, and hex is how keys appear in Core configuration and logs;
// the generic walker would instead print a struct wrapping a 32-element list.
func enrichCurve25519Public(rv reflect.Value) (string, any, bool) {
	pk, ok := rv.Interface().(xdr.Curve25519Public)
	if !ok {
		return "", nil, false
	}
	return hex.EncodeToString(pk.Key[:]), pk, true
}

// enrichMessageType names an overlay message in the lower-case form used by
// protocol documentation. The generated constant (MessageTypeGeneralizedTxSet)
// is accurate but long, and every one of these appears at the top of a decoded
// message where the short name reads much better.
func enrichMessageType(rv reflect.Value) (string, any, bool) {
	t, ok := rv.Interface().(xdr.MessageType)
	if !ok {
		return "", nil, false
	}
	name, ok := overlayMessageLabel(t)
	if !ok {
		return "", nil, false
	}
	return "", name, true
}

// overlayMessageLabel maps a MessageType to its documentation label. Unknown
// discriminants decline and keep the generic enum rendering.
func overlayMessageLabel(t xdr.MessageType) (string, bool) {
	switch t {
	case xdr.MessageTypeErrorMsg:
		return "error", true
	case xdr.MessageTypeHello:
		return "hello", true
	case xdr.MessageTypeAuth:
		return "auth", true
	case xdr.MessageTypeDontHave:
		return "dont_have", true
	case xdr.MessageTypePeers:
		return "peers", true
	case xdr.MessageTypeGetTxSet:
		return "get_tx_set", true
	case xdr.MessageTypeTxSet:
		return "tx_set", true
	case xdr.MessageTypeGeneralizedTxSet:
		return "generalized_tx_set", true
	case xdr.MessageTypeTransaction:
		return "transaction", true
	case xdr.MessageTypeGetScpQuorumset:
		return "get_scp_quorumset", true
	case xdr.MessageTypeScpQuorumset:
		return "scp_quorumset", true
	case xdr.MessageTypeScpMessage:
		return "scp_message", true
	case xdr.MessageTypeGetScpState:
		return "get_scp_state", true
	case xdr.MessageTypeSendMore:
		return "send_more", true
	case xdr.MessageTypeSendMoreExtended:
		return "send_more_extended", true
	case xdr.MessageTypeFloodAdvert:
		return "flood_advert", true
	case xdr.MessageTypeFloodDemand:
		return "flood_demand", true
	case xdr.MessageTypeTimeSlicedSurveyRequest:
		return "time_sliced_survey_request", true
	case xdr.MessageTypeTimeSlicedSurveyResponse:
		return "time_sliced_survey_response", true
	case xdr.MessageTypeTimeSlicedSurveyStartCollecting:
		return "time_sliced_survey_start_collecting", true
	case xdr.MessageTypeTimeSlicedSurveyStopCollecting:
		return "time_sliced_survey_stop_collecting", true
	default:
		return "", false
	}
}
