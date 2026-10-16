package lens

import (
	"fmt"
	"strings"

	"github.com/stellar/go-stellar-sdk/xdr"
)

// Threshold levels for Stellar operations. Low covers AllowTrust,
// SetTrustLineFlags, BumpSequence and ClaimClaimableBalance; high covers
// AccountMerge and SetOptions; everything else is medium.
const (
	// ThresholdLevelLow marks low-threshold operations.
	ThresholdLevelLow = "low"
	// ThresholdLevelMedium marks medium-threshold operations.
	ThresholdLevelMedium = "medium"
	// ThresholdLevelHigh marks high-threshold operations.
	ThresholdLevelHigh = "high"
)

// SignerInfo describes one signer on an account and whether it signed.
type SignerInfo struct {
	// Address is the strkey form of the signer key.
	Address string `json:"address"`
	// Weight is the signer's configured weight.
	Weight uint32 `json:"weight"`
	// Signed reports whether a matching signature was supplied.
	Signed bool `json:"signed"`
	// IsMaster reports whether this is the account's master key.
	IsMaster bool `json:"isMaster,omitempty"`
}

// SignatureDiagnosis explains whether the supplied signatures meet an
// account's signing threshold.
type SignatureDiagnosis struct {
	// RequiredThreshold is the weight threshold the signatures must reach.
	RequiredThreshold uint8 `json:"requiredThreshold"`
	// ThresholdLevel names the level the threshold was taken from.
	ThresholdLevel string `json:"thresholdLevel,omitempty"`
	// AccumulatedWeight is the summed weight of the signers that signed.
	AccumulatedWeight uint32 `json:"accumulatedWeight"`
	// MissingWeight is how much more weight is needed, or 0 when sufficient.
	MissingWeight uint32 `json:"missingWeight"`
	// Sufficient reports whether the accumulated weight meets the threshold.
	Sufficient bool `json:"sufficient"`
	// Signers lists every known signer and whether it signed.
	Signers []SignerInfo `json:"signers,omitempty"`
	// UnknownSignatures counts supplied signatures that match no known signer.
	UnknownSignatures int `json:"unknownSignatures,omitempty"`
	// Summary is a plain-English account of the shortfall or success.
	Summary string `json:"summary"`
	// Hint suggests which signatures to collect. Empty when sufficient.
	Hint string `json:"hint,omitempty"`
}

// OperationThresholdLevel returns the threshold level ("low", "medium" or
// "high") for an operation type, following the Stellar protocol mapping: low
// for AllowTrust, SetTrustLineFlags, BumpSequence and ClaimClaimableBalance;
// high for AccountMerge and SetOptions; medium for everything else.
func OperationThresholdLevel(opType xdr.OperationType) string {
	switch opType {
	case xdr.OperationTypeAllowTrust,
		xdr.OperationTypeSetTrustLineFlags,
		xdr.OperationTypeBumpSequence,
		xdr.OperationTypeClaimClaimableBalance:
		return ThresholdLevelLow
	case xdr.OperationTypeAccountMerge,
		xdr.OperationTypeSetOptions:
		return ThresholdLevelHigh
	default:
		return ThresholdLevelMedium
	}
}

// AccountThresholds extracts the (master weight, low, medium, high) values
// from an account entry.
func AccountThresholds(entry *xdr.AccountEntry) (master, low, med, high uint8) {
	if entry == nil {
		return 0, 0, 0, 0
	}
	return entry.Thresholds[0], entry.Thresholds[1], entry.Thresholds[2], entry.Thresholds[3]
}

// RequiredThresholdForLevel returns the configured threshold for a level name
// ("low", "medium" or "high"). Unknown levels fall back to medium.
func RequiredThresholdForLevel(entry *xdr.AccountEntry, level string) uint8 {
	_, low, med, high := AccountThresholds(entry)
	switch strings.ToLower(strings.TrimSpace(level)) {
	case ThresholdLevelLow:
		return low
	case ThresholdLevelHigh:
		return high
	default:
		return med
	}
}

// SignerHint derives the 4-byte signature hint for a signer key: the last
// four bytes of the underlying 32-byte key. For signed-payload signers the
// hint comes from the embedded ed25519 key, matching stellar-core behaviour.
func SignerHint(key xdr.SignerKey) (xdr.SignatureHint, error) {
	var raw [32]byte
	switch key.Type {
	case xdr.SignerKeyTypeSignerKeyTypeEd25519:
		v, ok := key.GetEd25519()
		if !ok {
			return xdr.SignatureHint{}, fmt.Errorf("%w: malformed ed25519 signer key", ErrInvalidXDR)
		}
		raw = v
	case xdr.SignerKeyTypeSignerKeyTypeHashX:
		v, ok := key.GetHashX()
		if !ok {
			return xdr.SignatureHint{}, fmt.Errorf("%w: malformed hashX signer key", ErrInvalidXDR)
		}
		raw = v
	case xdr.SignerKeyTypeSignerKeyTypePreAuthTx:
		v, ok := key.GetPreAuthTx()
		if !ok {
			return xdr.SignatureHint{}, fmt.Errorf("%w: malformed preAuthTx signer key", ErrInvalidXDR)
		}
		raw = v
	case xdr.SignerKeyTypeSignerKeyTypeEd25519SignedPayload:
		v, ok := key.GetEd25519SignedPayload()
		if !ok {
			return xdr.SignatureHint{}, fmt.Errorf("%w: malformed signed-payload signer key", ErrInvalidXDR)
		}
		raw = v.Ed25519
	default:
		return xdr.SignatureHint{}, fmt.Errorf("%w: unknown signer key type %s", ErrInvalidXDR, key.Type.String())
	}
	return xdr.SignatureHint{raw[28], raw[29], raw[30], raw[31]}, nil
}

// DiagnoseSignatures compares signatures against an account's signer set and
// reports whether they reach requiredThreshold.
//
// Matching is by signature hint, the same 4-byte index stellar-core uses, so
// no transaction hash or network passphrase is needed and the check stays
// offline. Each supplied signature can satisfy at most one signer; signatures
// matching no known signer are counted as unknown rather than failing the
// whole diagnosis.
func DiagnoseSignatures(entry *xdr.AccountEntry, signatures []xdr.DecoratedSignature, requiredThreshold uint8) (*SignatureDiagnosis, error) {
	if entry == nil {
		return nil, fmt.Errorf("%w: cannot diagnose signatures of nil AccountEntry", ErrInvalidArgument)
	}
	master, _, _, _ := AccountThresholds(entry)

	// Build the known signer list: master key first (when it has weight),
	// then every configured signer.
	var signers []SignerInfo
	masterAddr := ""
	if aid, err := entry.AccountId.GetAddress(); err == nil && aid != "" {
		masterAddr = aid
	}
	if master > 0 {
		signers = append(signers, SignerInfo{Address: masterAddr, Weight: uint32(master), IsMaster: true})
	}
	for _, s := range entry.Signers {
		addr := ""
		if a, err := s.Key.GetAddress(); err == nil {
			addr = a
		}
		signers = append(signers, SignerInfo{Address: addr, Weight: uint32(s.Weight)})
	}

	// Index signers by hint. Collisions on 4-byte hints are possible in
	// theory, so keep every index that shares a hint.
	byHint := map[xdr.SignatureHint][]int{}
	for i := range signers {
		var key xdr.SignerKey
		var err error
		if i == 0 && master > 0 {
			key, err = signerKeyForAccount(entry.AccountId)
			if err != nil {
				continue
			}
		} else {
			offset := i
			if master > 0 {
				offset = i - 1
			}
			if offset < 0 || offset >= len(entry.Signers) {
				continue
			}
			key = entry.Signers[offset].Key
		}
		h, err := SignerHint(key)
		if err != nil {
			continue
		}
		byHint[h] = append(byHint[h], i)
	}

	var accumulated uint32
	unknown := 0
	signed := make([]bool, len(signers))
	for _, sig := range signatures {
		matched := false
		for _, idx := range byHint[sig.Hint] {
			if !signed[idx] {
				signed[idx] = true
				accumulated += signers[idx].Weight
				matched = true
				break
			}
		}
		if !matched {
			unknown++
		}
	}
	for i := range signers {
		signers[i].Signed = signed[i]
	}

	d := &SignatureDiagnosis{
		RequiredThreshold: requiredThreshold,
		AccumulatedWeight: accumulated,
		Signers:           signers,
		UnknownSignatures: unknown,
		Sufficient:        accumulated >= uint32(requiredThreshold),
	}
	if !d.Sufficient {
		if accumulated < uint32(requiredThreshold) {
			d.MissingWeight = uint32(requiredThreshold) - accumulated
		}
	}
	d.Summary = renderSignatureSummary(d)
	if !d.Sufficient {
		d.Hint = renderSignatureHint(d)
	}
	return d, nil
}

// DiagnoseSignaturesForLevel is DiagnoseSignatures with the threshold taken
// from the account's configured level ("low", "medium" or "high").
func DiagnoseSignaturesForLevel(entry *xdr.AccountEntry, signatures []xdr.DecoratedSignature, level string) (*SignatureDiagnosis, error) {
	if entry == nil {
		return nil, fmt.Errorf("%w: cannot diagnose signatures of nil AccountEntry", ErrInvalidArgument)
	}
	threshold := RequiredThresholdForLevel(entry, level)
	d, err := DiagnoseSignatures(entry, signatures, threshold)
	if err != nil {
		return nil, err
	}
	if level == "" {
		level = ThresholdLevelMedium
	}
	d.ThresholdLevel = strings.ToLower(strings.TrimSpace(level))
	if d.ThresholdLevel == "" {
		d.ThresholdLevel = ThresholdLevelMedium
	}
	d.Summary = renderSignatureSummary(d)
	if !d.Sufficient {
		d.Hint = renderSignatureHint(d)
	}
	return d, nil
}

// DiagnoseTransactionSignatures checks the signatures on a transaction
// envelope against an account entry, using the threshold level of the first
// operation (medium when the envelope carries no operations).
func DiagnoseTransactionSignatures(env *xdr.TransactionEnvelope, entry *xdr.AccountEntry) (*SignatureDiagnosis, error) {
	if env == nil {
		return nil, fmt.Errorf("%w: cannot diagnose signatures of nil TransactionEnvelope", ErrInvalidArgument)
	}
	if entry == nil {
		return nil, fmt.Errorf("%w: cannot diagnose signatures of nil AccountEntry", ErrInvalidArgument)
	}
	level := ThresholdLevelMedium
	if ops := env.Operations(); len(ops) > 0 {
		level = OperationThresholdLevel(ops[0].Body.Type)
	}
	return DiagnoseSignaturesForLevel(entry, env.Signatures(), level)
}

// DiagnoseSignaturesFromValues is the Value-based wrapper around
// DiagnoseTransactionSignatures: envelopeVal must wrap
// *xdr.TransactionEnvelope and accountVal must wrap *xdr.AccountEntry.
func DiagnoseSignaturesFromValues(envelopeVal *Value, accountVal *Value) (*SignatureDiagnosis, error) {
	if envelopeVal == nil || accountVal == nil {
		return nil, fmt.Errorf("%w: envelope and account values are required", ErrInvalidArgument)
	}
	env, ok := envelopeVal.Raw.(*xdr.TransactionEnvelope)
	if !ok {
		return nil, fmt.Errorf("%w: envelope value is %T, want *xdr.TransactionEnvelope", ErrInvalidArgument, envelopeVal.Raw)
	}
	entry, ok := accountVal.Raw.(*xdr.AccountEntry)
	if !ok {
		return nil, fmt.Errorf("%w: account value is %T, want *xdr.AccountEntry", ErrInvalidArgument, accountVal.Raw)
	}
	return DiagnoseTransactionSignatures(env, entry)
}

func signerKeyForAccount(aid xdr.AccountId) (xdr.SignerKey, error) {
	addr, err := aid.GetAddress()
	if err != nil {
		return xdr.SignerKey{}, err
	}
	var key xdr.SignerKey
	if err := key.SetAddress(addr); err != nil {
		return xdr.SignerKey{}, err
	}
	return key, nil
}

func renderSignatureSummary(d *SignatureDiagnosis) string {
	var b strings.Builder
	if d.Sufficient {
		fmt.Fprintf(&b, "Signatures supply weight %d, meeting threshold %d.", d.AccumulatedWeight, d.RequiredThreshold)
		if d.ThresholdLevel != "" {
			fmt.Fprintf(&b, " (%s threshold)", d.ThresholdLevel)
		}
		signed := []string{}
		for _, s := range d.Signers {
			if s.Signed {
				label := shortAddr(s.Address)
				if label == "" {
					label = "unknown"
				}
				signed = append(signed, fmt.Sprintf("%s (weight %d)", label, s.Weight))
			}
		}
		if len(signed) > 0 {
			b.WriteString(" Signed: " + strings.Join(signed, ", ") + ".")
		}
		if d.UnknownSignatures > 0 {
			fmt.Fprintf(&b, " %d extra signature(s) match no known signer and add no weight.", d.UnknownSignatures)
		}
		return b.String()
	}
	fmt.Fprintf(&b, "Signatures supply weight %d but threshold %d requires %d more weight.",
		d.AccumulatedWeight, d.RequiredThreshold, d.MissingWeight)
	if d.ThresholdLevel != "" {
		fmt.Fprintf(&b, " (%s threshold)", d.ThresholdLevel)
	}
	missing := []string{}
	for _, s := range d.Signers {
		if !s.Signed {
			label := shortAddr(s.Address)
			if label == "" {
				label = "unknown"
			}
			missing = append(missing, fmt.Sprintf("%s (weight %d)", label, s.Weight))
		}
	}
	if len(missing) > 0 {
		b.WriteString(" Missing signatures from: " + strings.Join(missing, ", ") + ".")
	}
	if d.UnknownSignatures > 0 {
		fmt.Fprintf(&b, " %d supplied signature(s) match no known signer.", d.UnknownSignatures)
	}
	return b.String()
}

func renderSignatureHint(d *SignatureDiagnosis) string {
	missing := []string{}
	for _, s := range d.Signers {
		if !s.Signed {
			if s.Address != "" {
				missing = append(missing, s.Address)
			}
		}
	}
	if len(missing) == 0 {
		return "Collect additional signatures until the summed signer weight reaches the threshold, and sign with the network passphrase of the target network."
	}
	return "Collect signatures from: " + strings.Join(missing, ", ") + ". Sign with the target network passphrase; a signature made for another network never counts."
}
