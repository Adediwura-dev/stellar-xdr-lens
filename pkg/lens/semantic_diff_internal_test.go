package lens

import (
	"testing"

	"github.com/stellar/go-stellar-sdk/xdr"
)

func TestSemanticScValIntegerMalformedAndUnsupported(t *testing.T) {
	t.Parallel()

	for _, valueType := range []xdr.ScValType{
		xdr.ScValTypeScvU128,
		xdr.ScValTypeScvI128,
		xdr.ScValTypeScvU256,
		xdr.ScValTypeScvI256,
	} {
		t.Run(valueType.String(), func(t *testing.T) {
			t.Parallel()
			if _, ok := semanticScValInteger(xdr.ScVal{Type: valueType}); ok {
				t.Errorf("semanticScValInteger(%s) accepted missing arm", valueType)
			}
		})
	}
	if _, ok := semanticScValInteger(xdr.ScVal{Type: xdr.ScValTypeScvVoid}); ok {
		t.Error("semanticScValInteger(void) reported an integer")
	}
	if got := semanticScValIntegerArm(xdr.ScValTypeScvVoid); got != "Value" {
		t.Errorf("semanticScValIntegerArm(void) = %q, want Value", got)
	}
}
