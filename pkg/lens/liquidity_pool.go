package lens

import (
	"fmt"
	"reflect"

	"github.com/stellar/go-stellar-sdk/xdr"
)

// Liquidity pool ledger entries: LiquidityPoolEntry and the constant product
// parameters that describe what a pool trades and what it charges.
//
// The generic walker decodes every field already. What it cannot know is that
// Fee is a protocol fee in basis points and that AssetA/AssetB form a trading
// pair, so the enricher registered here renders both in one line.

func init() {
	registerEnricher(reflect.TypeOf(xdr.LiquidityPoolConstantProductParameters{}), enrichPoolParameters)
}

// describePoolParameters renders a constant product pool's configuration as
// one line: the asset pair and the trading fee, for example
// "native / USDC:GA... @ 0.3%".
func describePoolParameters(p xdr.LiquidityPoolConstantProductParameters) string {
	return fmt.Sprintf("%s / %s @ %s", p.AssetA.StringCanonical(), p.AssetB.StringCanonical(), poolFeePercent(p.Fee))
}

// poolFeePercent converts the protocol fee, which XDR stores in basis points
// (30 means 0.30%), into a percentage. %g keeps whole and one-decimal fees
// free of trailing zeros while still rendering the smallest 1 bp fee as
// "0.01%".
func poolFeePercent(fee xdr.Int32) string {
	return fmt.Sprintf("%g%%", float64(fee)/100)
}

// enrichPoolParameters collapses the parameters subtree into the pair and fee
// line. Everything else in the entry — reserves, share count — stays
// structured and amount-formatted by the generic walker.
func enrichPoolParameters(rv reflect.Value) (string, any, bool) {
	p, valid := rv.Interface().(xdr.LiquidityPoolConstantProductParameters)
	if !valid {
		return "", nil, false
	}
	return "", describePoolParameters(p), true
}
