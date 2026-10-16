package lens_test

import (
	"strings"
	"testing"

	"github.com/stellar/go-stellar-sdk/xdr"

	"github.com/stellar-optics/stellar-xdr-lens/pkg/lens"
)

const (
	fixtureSeller = "GAISEM2EKVTHPCEZVK54ZXPO74ABCIRTIRKWM54ITGVLXTG5537QAQZ5"
	fixtureIssuer = "GAQSEIZEEUTCOKBJFIVSYLJOF4YDCMRTGQ2TMNZYHE5DWPB5HY7UAIOK"
	testOfferID   = int64(42)
)

func TestDecodeOfferEntryFixture(t *testing.T) {
	t.Parallel()

	v, err := lens.DecodeAs(loadFixture(t, "offer_entry"), "OfferEntry")
	if err != nil {
		t.Fatalf("DecodeAs() error = %v", err)
	}
	if v.Type != "OfferEntry" {
		t.Errorf("type = %q, want OfferEntry", v.Type)
	}
	if v.Node.Kind != lens.KindStruct {
		t.Errorf("node kind = %v, want struct", v.Node.Kind)
	}
	if _, ok := v.Raw.(*xdr.OfferEntry); !ok {
		t.Fatalf("Raw is %T, want *xdr.OfferEntry", v.Raw)
	}

	raw := v.Raw.(*xdr.OfferEntry)
	if int64(raw.OfferId) != testOfferID {
		t.Errorf("OfferId = %d, want %d", raw.OfferId, testOfferID)
	}
	if raw.Amount != 70000000 {
		t.Errorf("Amount = %d, want 70000000 (7.0000000)", raw.Amount)
	}
	if raw.Price.N != 7 || raw.Price.D != 3 {
		t.Errorf("Price = %d/%d, want 7/3", raw.Price.N, raw.Price.D)
	}
	if raw.Flags != 1 {
		t.Errorf("Flags = %d, want 1 (passive)", raw.Flags)
	}

	// The offer id renders as a plain integer.
	offerID := v.Node.Find("OfferId")
	if offerID == nil {
		t.Fatal("could not find OfferId in the decoded tree")
	}
	if got := offerID.Value.(int64); got != testOfferID {
		t.Errorf("OfferId = %d, want %d", got, testOfferID)
	}

	// The seller renders as a strkey address.
	seller := v.Node.Find("SellerId")
	if seller == nil {
		t.Fatal("could not find SellerId in the decoded tree")
	}
	sellerAddr, ok := seller.Value.(string)
	if !ok {
		t.Fatalf("SellerId value is %T, want a strkey string", seller.Value)
	}
	if sellerAddr != fixtureSeller {
		t.Errorf("SellerId = %q, want %q", sellerAddr, fixtureSeller)
	}

	// Assets render in CODE:ISSUER form, with XLM for the native asset.
	selling := v.Node.Find("Selling")
	if selling == nil {
		t.Fatal("could not find Selling in the decoded tree")
	}
	if got, ok := selling.Value.(string); !ok || !strings.HasPrefix(got, "USDC:"+fixtureIssuer) {
		t.Errorf("Selling = %v, want USDC:%s", selling.Value, fixtureIssuer)
	}
	buying := v.Node.Find("Buying")
	if buying == nil {
		t.Fatal("could not find Buying in the decoded tree")
	}
	if got, ok := buying.Value.(string); !ok || got != "native" {
		t.Errorf("Buying = %v, want native", buying.Value)
	}

	// Amounts render in stroops with a human-friendly note.
	amount := v.Node.Find("Amount")
	if amount == nil {
		t.Fatal("could not find Amount in the decoded tree")
	}
	if got := amount.Value.(int64); got != 70000000 {
		t.Errorf("Amount = %d, want 70000000", got)
	}
	if !strings.Contains(amount.Note, "7.0000000") {
		t.Errorf("Amount note = %q, want it to render 7.0000000", amount.Note)
	}

	// Prices render as an "n/d" fraction with a decimal value.
	price := v.Node.Find("Price")
	if price == nil {
		t.Fatal("could not find Price in the decoded tree")
	}
	if price.Kind != lens.KindScalar {
		t.Errorf("Price kind = %v, want scalar", price.Kind)
	}
	if got := price.Value.(string); got != "7/3 (= 2.3333333)" {
		t.Errorf("Price = %q, want %q", got, "7/3 (= 2.3333333)")
	}

	flags := v.Node.Find("Flags")
	if flags == nil {
		t.Fatal("could not find Flags in the decoded tree")
	}
	if got := flags.Value.(uint64); got != 1 {
		t.Errorf("Flags = %d, want 1", got)
	}
}

func TestDecodeOfferEntryRejectsTruncatedInput(t *testing.T) {
	t.Parallel()

	// A truncated offer entry must report an error rather than panic or
	// silently decode a partial offer.
	if _, err := lens.DecodeAs(loadFixtureRaw("offer_entry")[:24], "OfferEntry"); err == nil {
		t.Error("DecodeAs() error = nil, want an error for a truncated offer entry")
	}
}

func TestDecodePriceFraction(t *testing.T) {
	t.Parallel()

	v, err := lens.DecodeAs(loadFixture(t, "price"), "Price")
	if err != nil {
		t.Fatalf("DecodeAs() error = %v", err)
	}
	if v.Type != "Price" {
		t.Errorf("type = %q, want Price", v.Type)
	}
	if v.Node.Kind != lens.KindScalar {
		t.Errorf("node kind = %v, want scalar", v.Node.Kind)
	}
	if got := v.Node.Value.(string); got != "7/3 (= 2.3333333)" {
		t.Errorf("value = %q, want %q", got, "7/3 (= 2.3333333)")
	}
	if _, ok := v.Raw.(*xdr.Price); !ok {
		t.Fatalf("Raw is %T, want *xdr.Price", v.Raw)
	}
}

func TestDecodePriceZeroDenominatorDegrades(t *testing.T) {
	t.Parallel()

	// A zero denominator is malformed: the enricher declines and the generic
	// tree walker renders the raw numerator and denominator instead.
	v, err := lens.DecodeAs(loadFixture(t, "price_zero"), "Price")
	if err != nil {
		t.Fatalf("DecodeAs() error = %v", err)
	}
	if v.Node.Kind != lens.KindStruct {
		t.Errorf("node kind = %v, want struct for a malformed price", v.Node.Kind)
	}
	n := v.Node.Find("N")
	if n == nil || n.Value.(int64) != 1 {
		t.Errorf("N = %v, want 1", n)
	}
	d := v.Node.Find("D")
	if d == nil || d.Value.(int64) != 0 {
		t.Errorf("D = %v, want 0", d)
	}
}

func TestDecodeOfferInsideLedgerEntry(t *testing.T) {
	t.Parallel()

	v, err := lens.DecodeAs(loadFixture(t, "ledger_entry_offer"), "LedgerEntry")
	if err != nil {
		t.Fatalf("DecodeAs() error = %v", err)
	}
	if v.Type != "LedgerEntry" {
		t.Errorf("type = %q, want LedgerEntry", v.Type)
	}

	dataType := v.Node.Find("Data.Type")
	if dataType == nil {
		t.Fatal("could not find Data.Type in the decoded ledger entry")
	}
	if got := dataType.Value.(string); got != "LedgerEntryTypeOffer" {
		t.Errorf("Data.Type = %q, want LedgerEntryTypeOffer", got)
	}

	offer := v.Node.Find("Data.Offer")
	if offer == nil {
		t.Fatal("could not find Data.Offer in the decoded ledger entry")
	}
	if offer.Kind != lens.KindStruct {
		t.Errorf("Data.Offer kind = %v, want struct", offer.Kind)
	}
	offerID := offer.Find("OfferId")
	if offerID == nil || offerID.Value.(int64) != testOfferID {
		t.Errorf("Data.Offer.OfferId = %v, want %d", offerID, testOfferID)
	}
	price := offer.Find("Price")
	if price == nil || price.Value.(string) != "7/3 (= 2.3333333)" {
		t.Errorf("Data.Offer.Price = %v, want 7/3 (= 2.3333333)", price)
	}
}

func TestDetectReportsOfferEntry(t *testing.T) {
	t.Parallel()

	cands, err := lens.Detect(loadFixture(t, "offer_entry"))
	if err != nil {
		t.Fatalf("Detect() error = %v", err)
	}
	names := make(map[string]bool, len(cands))
	for _, c := range cands {
		names[c.Type] = true
	}
	if !names["OfferEntry"] {
		t.Errorf("Detect() candidates = %v, want OfferEntry among them", cands)
	}
}
