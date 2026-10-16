package lens

import (
	"encoding/hex"
	"fmt"
	"reflect"
	"unicode"
	"unicode/utf8"

	"github.com/stellar/go-stellar-sdk/xdr"
)

// A DataEntry is arbitrary key/value storage attached to an account: the name
// is a short UTF-8 label (DataName) and the payload is up to 64 opaque bytes
// (DataValue). Nothing on the wire says whether those bytes are text, so a
// decoder that always prints hex buries the common case — configuration
// strings, small JSON documents, flags — behind an encoding step. The value is
// therefore inspected and rendered as text when it is valid printable UTF-8,
// and as hex otherwise. Both forms are deterministic and require no network
// access.
func init() {
	enricherFor[reflect.TypeOf(xdr.DataValue{})] = enrichDataValue
}

// DescribeDataValue returns the display form of an account data value and
// whether that form is UTF-8 text.
//
// A value is considered text when it is non-empty, valid UTF-8 and consists
// solely of printable runes. Control characters — including newlines and tabs —
// disqualify it, because rendering them inside a one-line tree node would
// corrupt the surrounding output. Anything else, including invalid UTF-8 and
// embedded NUL bytes, is rendered as lower-case hex.
func DescribeDataValue(b []byte) (display string, textual bool) {
	if len(b) > 0 && utf8.Valid(b) {
		text := string(b)
		textual = true
		for _, r := range text {
			if !unicode.IsPrint(r) {
				textual = false
				break
			}
		}
		if textual {
			return text, true
		}
	}
	return hex.EncodeToString(b), false
}

// enrichDataValue renders the payload of a DataEntry. The raw value is the
// text itself when the bytes decode as text and the hex form otherwise, so
// machine-readable output carries whichever representation the detector chose;
// the note states which that was and how many bytes it came from.
func enrichDataValue(rv reflect.Value) (string, any, bool) {
	v, ok := rv.Interface().(xdr.DataValue)
	if !ok {
		return "", nil, false
	}
	switch display, textual := DescribeDataValue(v); {
	case len(v) == 0:
		return "empty", "", true
	case textual:
		return fmt.Sprintf("UTF-8 text, %d byte(s)", len(v)), display, true
	default:
		return fmt.Sprintf("binary, %d byte(s)", len(v)), display, true
	}
}
