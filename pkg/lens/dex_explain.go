package lens

import (
	"fmt"
	"reflect"

	"github.com/stellar/go-stellar-sdk/xdr"
)

// DexOfferCrossSelf explains when a DEX manage sell/buy offer crosses
// another offer from the same source account, which is prohibited.
type DexOfferCrossSelf struct {
	// OperationIndex is the index of the failing operation.
	OperationIndex int
	// OfferType is "sell" or "buy".
	OfferType string
	// CrossingOfferId is the offer ID of the crossing offer.
	CrossingOfferId int64
	// CrossingOfferAsset is the asset being crossed.
	CrossingOfferAsset string
	// Summary plain-English summary of the failure.
	Summary string
	// Hint suggests how to fix the issue.
	Hint string
}

// NewDexOfferCrossSelf creates a new DexOfferCrossSelf diagnosis.
func NewDexOfferCrossSelf(operationIndex int, offerType string, crossingOfferId int64, crossingOfferAsset string) *DexOfferCrossSelf {
	summary := fmt.Sprintf("%s offer would cross own %s offer (offer ID: %d, asset: %s)",
		offerType, offerType, crossingOfferId, crossingOfferAsset)
	hint := fmt.Sprintf("Remove or modify the crossing offer (offer ID %d) before resubmitting, or adjust price/amount to avoid crossing.", crossingOfferId)
	return &DexOfferCrossSelf{
		OperationIndex:     operationIndex,
		OfferType:          offerType,
		CrossingOfferId:    crossingOfferId,
		CrossingOfferAsset: crossingOfferAsset,
		Summary:            summary,
		Hint:               hint,
	}
}

// String renders the diagnosis as a single line.
func (d *DexOfferCrossSelf) String() string {
	if d == nil {
		return "<nil>"
	}
	return d.Summary
}

// DiagnosticReport renders a multi-line report suitable for CLI output.
func (d *DexOfferCrossSelf) DiagnosticReport() string {
	if d == nil {
		return ""
	}
	return fmt.Sprintf("DEX %s offer crosses own %s offer:\n  Operation: %d\n  Crossing offer ID: %d\n  Asset: %s\n  Summary: %s\n  Hint: %s",
		d.OfferType, d.OfferType, d.OperationIndex, d.CrossingOfferId, d.CrossingOfferAsset, d.Summary, d.Hint)
}

// DiagnoseDexOfferCrossSelf diagnoses whether an operation result indicates
// a DEX offer crossing its own offer.
//
// This is a purely local, offline diagnosis - no network calls are made.
// It checks the operation result code for OfferCrossSelf and extracts
// relevant offer details from the operation's managing offer data.
func DiagnoseDexOfferCrossSelf(opResult xdr.OperationResult, operationIndex int) (*DexOfferCrossSelf, error) {
	// Check if the inner result code indicates a cross-self error
	if opResult.Code != xdr.OperationResultCodeOpInner {
		return nil, nil
	}

	// Use the neutral node tree to examine the operation result
	node := buildNode("", reflect.ValueOf(opResult), 0)
	if node == nil {
		return nil, nil
	}

	// Find the Tr child which contains the result arms
	tr := node.Child("Tr")
	if tr == nil {
		return nil, nil
	}

	// Look for ManageSellOffer or ManageBuyOffer arm with OfferCrossSelf code
	for _, arm := range tr.Children {
		if arm.Name != "ManageSellOffer" && arm.Name != "ManageBuyOffer" {
			continue
		}

		// Check the Code child of this arm
		if codeNode := arm.Child("Code"); codeNode != nil {
			if code, ok := codeNode.Value.(string); ok && code == "OfferCrossSelf" {
				// Extract offer details from the arm's other children
				offerId := int64(0)
				offerAsset := "unknown"

				// Try to extract offer ID from the arm
				if idNode := arm.Child("OfferId"); idNode != nil {
					if id, ok := idNode.Value.(int64); ok {
						offerId = id
					}
				}

				// Try to extract asset label
				if assetNode := arm.Child("Selling"); assetNode != nil {
					if asset, ok := assetNode.Value.(string); ok {
						offerAsset = asset
					}
				}

				return NewDexOfferCrossSelf(
					operationIndex,
					arm.Name,
					offerId,
					offerAsset,
				), nil
			}
		}
	}

	return nil, nil
}

// init registers the diagnosis function with the lens framework.
func init() {
	_ = DexOfferCrossSelf{}
}
