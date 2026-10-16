package format

import (
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/stellar/go-stellar-sdk/xdr"

	"github.com/stellar-optics/stellar-xdr-lens/pkg/lens"
)

// ErrNotTransactionEnvelope is returned when TxRepFormatter is called with a non-envelope type.
var ErrNotTransactionEnvelope = errors.New("txrep requires a TransactionEnvelope")

// TxRepFormatter renders a TransactionEnvelope in standard SEP-0011 TxRep format.
type TxRepFormatter struct {
	palette  Palette
	network  string
	comments bool
}

// TxRepOption configures a TxRepFormatter.
type TxRepOption func(*TxRepFormatter)

// WithTxRepPalette sets the colour palette for TxRepFormatter.
func WithTxRepPalette(p Palette) TxRepOption {
	return func(t *TxRepFormatter) { t.palette = p }
}

// WithTxRepComments controls whether descriptive comments (e.g. human dates) are included.
func WithTxRepComments(comments bool) TxRepOption {
	return func(t *TxRepFormatter) { t.comments = comments }
}

// WithTxRepNetwork sets the network passphrase (affects native asset naming per SEP-0011).
func WithTxRepNetwork(network string) TxRepOption {
	return func(t *TxRepFormatter) { t.network = network }
}

// NewTxRepFormatter returns a new TxRepFormatter.
func NewTxRepFormatter(opts ...TxRepOption) *TxRepFormatter {
	t := &TxRepFormatter{
		palette:  NoColor,
		network:  "",
		comments: false,
	}
	for _, o := range opts {
		o(t)
	}
	return t
}

// Format writes the SEP-0011 TxRep rendering of v to w.
func (f *TxRepFormatter) Format(w io.Writer, v *lens.Value) error {
	if v == nil {
		return fmt.Errorf("format: nil value")
	}

	env, ok := v.Raw.(*xdr.TransactionEnvelope)
	if !ok || env == nil {
		return fmt.Errorf("%w; got %s", ErrNotTransactionEnvelope, v.Type)
	}

	var b strings.Builder
	if err := f.formatEnvelope(&b, env); err != nil {
		return fmt.Errorf("format txrep: %w", err)
	}

	if _, err := io.WriteString(w, b.String()); err != nil {
		return fmt.Errorf("format txrep write: %w", err)
	}
	return nil
}

func (f *TxRepFormatter) formatEnvelope(b *strings.Builder, env *xdr.TransactionEnvelope) error {
	switch env.Type {
	case xdr.EnvelopeTypeEnvelopeTypeTxV0:
		f.line(b, "type", "ENVELOPE_TYPE_TX_V0")
		v0 := env.V0
		if v0 == nil {
			return nil
		}
		f.formatTxV0(b, &v0.Tx)
		f.formatSignatures(b, "signatures", v0.Signatures)

	case xdr.EnvelopeTypeEnvelopeTypeTx:
		f.line(b, "type", "ENVELOPE_TYPE_TX")
		v1 := env.V1
		if v1 == nil {
			return nil
		}
		f.formatTx(b, &v1.Tx)
		f.formatSignatures(b, "signatures", v1.Signatures)

	case xdr.EnvelopeTypeEnvelopeTypeTxFeeBump:
		f.line(b, "type", "ENVELOPE_TYPE_TX_FEE_BUMP")
		fb := env.FeeBump
		if fb == nil {
			return nil
		}
		f.formatFeeBumpTx(b, &fb.Tx)
		f.formatSignatures(b, "signatures", fb.Signatures)

	default:
		f.line(b, "type", env.Type.String())
	}
	return nil
}

func (f *TxRepFormatter) formatTx(b *strings.Builder, tx *xdr.Transaction) {
	f.line(b, "tx.sourceAccount", f.formatMuxedAccount(&tx.SourceAccount))
	f.line(b, "tx.fee", fmt.Sprintf("%d", tx.Fee))
	f.line(b, "tx.seqNum", fmt.Sprintf("%d", tx.SeqNum))

	f.formatPreconditions(b, "tx", &tx.Cond)
	f.formatMemo(b, "tx.memo", &tx.Memo)
	f.formatOperations(b, "tx.operations", tx.Operations)
	f.formatExt(b, "tx.ext", &tx.Ext)
}

func (f *TxRepFormatter) formatTxV0(b *strings.Builder, tx *xdr.TransactionV0) {
	key, err := xdr.NewSignerKey(xdr.SignerKeyTypeSignerKeyTypeEd25519, tx.SourceAccountEd25519)
	if err == nil {
		f.line(b, "tx.sourceAccount", key.Address())
	}
	f.line(b, "tx.fee", fmt.Sprintf("%d", tx.Fee))
	f.line(b, "tx.seqNum", fmt.Sprintf("%d", tx.SeqNum))

	if tx.TimeBounds != nil {
		f.line(b, "tx.timeBounds._present", "true")
		f.line(b, "tx.timeBounds.minTime", fmt.Sprintf("%d", tx.TimeBounds.MinTime))
		f.line(b, "tx.timeBounds.maxTime", fmt.Sprintf("%d", tx.TimeBounds.MaxTime))
	} else {
		f.line(b, "tx.timeBounds._present", "false")
	}

	f.formatMemo(b, "tx.memo", &tx.Memo)
	f.formatOperations(b, "tx.operations", tx.Operations)
	f.line(b, "tx.ext.v", fmt.Sprintf("%d", tx.Ext.V))
}

func (f *TxRepFormatter) formatFeeBumpTx(b *strings.Builder, fb *xdr.FeeBumpTransaction) {
	f.line(b, "feeBump.tx.feeSource", f.formatMuxedAccount(&fb.FeeSource))
	f.line(b, "feeBump.tx.fee", fmt.Sprintf("%d", fb.Fee))

	inner := &fb.InnerTx
	switch inner.Type {
	case xdr.EnvelopeTypeEnvelopeTypeTx:
		f.line(b, "feeBump.tx.innerTx.type", "ENVELOPE_TYPE_TX")
		if inner.V1 != nil {
			f.formatInnerTx(b, "feeBump.tx.innerTx.tx", &inner.V1.Tx)
			f.formatSignatures(b, "feeBump.tx.innerTx.signatures", inner.V1.Signatures)
		}
	default:
		f.line(b, "feeBump.tx.innerTx.type", inner.Type.String())
	}
	f.line(b, "feeBump.tx.ext.v", fmt.Sprintf("%d", fb.Ext.V))
}

func (f *TxRepFormatter) formatInnerTx(b *strings.Builder, prefix string, tx *xdr.Transaction) {
	f.line(b, prefix+".sourceAccount", f.formatMuxedAccount(&tx.SourceAccount))
	f.line(b, prefix+".fee", fmt.Sprintf("%d", tx.Fee))
	f.line(b, prefix+".seqNum", fmt.Sprintf("%d", tx.SeqNum))
	f.formatPreconditions(b, prefix, &tx.Cond)
	f.formatMemo(b, prefix+".memo", &tx.Memo)
	f.formatOperations(b, prefix+".operations", tx.Operations)
	f.formatExt(b, prefix+".ext", &tx.Ext)
}

func (f *TxRepFormatter) formatPreconditions(b *strings.Builder, prefix string, cond *xdr.Preconditions) {
	if cond == nil {
		f.line(b, prefix+".timeBounds._present", "false")
		return
	}

	switch cond.Type {
	case xdr.PreconditionTypePrecondNone:
		f.line(b, prefix+".timeBounds._present", "false")

	case xdr.PreconditionTypePrecondTime:
		if cond.TimeBounds != nil {
			f.line(b, prefix+".timeBounds._present", "true")
			f.line(b, prefix+".timeBounds.minTime", fmt.Sprintf("%d", cond.TimeBounds.MinTime))
			f.line(b, prefix+".timeBounds.maxTime", fmt.Sprintf("%d", cond.TimeBounds.MaxTime))
		} else {
			f.line(b, prefix+".timeBounds._present", "false")
		}

	case xdr.PreconditionTypePrecondV2:
		f.line(b, prefix+".cond.type", "PRECOND_V2")
		v2 := cond.V2
		if v2 == nil {
			return
		}
		if v2.TimeBounds != nil {
			f.line(b, prefix+".cond.v2.timeBounds._present", "true")
			f.line(b, prefix+".cond.v2.timeBounds.minTime", fmt.Sprintf("%d", v2.TimeBounds.MinTime))
			f.line(b, prefix+".cond.v2.timeBounds.maxTime", fmt.Sprintf("%d", v2.TimeBounds.MaxTime))
		} else {
			f.line(b, prefix+".cond.v2.timeBounds._present", "false")
		}
		if v2.LedgerBounds != nil {
			f.line(b, prefix+".cond.v2.ledgerBounds._present", "true")
			f.line(b, prefix+".cond.v2.ledgerBounds.minLedger", fmt.Sprintf("%d", v2.LedgerBounds.MinLedger))
			f.line(b, prefix+".cond.v2.ledgerBounds.maxLedger", fmt.Sprintf("%d", v2.LedgerBounds.MaxLedger))
		} else {
			f.line(b, prefix+".cond.v2.ledgerBounds._present", "false")
		}
		if v2.MinSeqNum != nil {
			f.line(b, prefix+".cond.v2.minSeqNum._present", "true")
			f.line(b, prefix+".cond.v2.minSeqNum", fmt.Sprintf("%d", *v2.MinSeqNum))
		} else {
			f.line(b, prefix+".cond.v2.minSeqNum._present", "false")
		}
		f.line(b, prefix+".cond.v2.minSeqAge", fmt.Sprintf("%d", v2.MinSeqAge))
		f.line(b, prefix+".cond.v2.minSeqLedgerGap", fmt.Sprintf("%d", v2.MinSeqLedgerGap))
		f.line(b, prefix+".cond.v2.extraSigners.len", fmt.Sprintf("%d", len(v2.ExtraSigners)))
		for i, s := range v2.ExtraSigners {
			f.line(b, fmt.Sprintf("%s.cond.v2.extraSigners[%d]", prefix, i), s.Address())
		}
	}
}

func (f *TxRepFormatter) formatMemo(b *strings.Builder, prefix string, memo *xdr.Memo) {
	if memo == nil {
		f.line(b, prefix+".type", "MEMO_NONE")
		return
	}
	switch memo.Type {
	case xdr.MemoTypeMemoNone:
		f.line(b, prefix+".type", "MEMO_NONE")
	case xdr.MemoTypeMemoText:
		f.line(b, prefix+".type", "MEMO_TEXT")
		text, _ := memo.GetText()
		f.line(b, prefix+".text", strconv.Quote(text))
	case xdr.MemoTypeMemoId:
		f.line(b, prefix+".type", "MEMO_ID")
		id, _ := memo.GetId()
		f.line(b, prefix+".id", fmt.Sprintf("%d", id))
	case xdr.MemoTypeMemoHash:
		f.line(b, prefix+".type", "MEMO_HASH")
		hash, _ := memo.GetHash()
		f.line(b, prefix+".hash", hex.EncodeToString(hash[:]))
	case xdr.MemoTypeMemoReturn:
		f.line(b, prefix+".type", "MEMO_RETURN")
		ret, _ := memo.GetRetHash()
		f.line(b, prefix+".retHash", hex.EncodeToString(ret[:]))
	}
}

func (f *TxRepFormatter) formatOperations(b *strings.Builder, prefix string, ops []xdr.Operation) {
	f.line(b, prefix+".len", fmt.Sprintf("%d", len(ops)))
	for i := range ops {
		op := &ops[i]
		opPrefix := fmt.Sprintf("%s[%d]", prefix, i)
		if op.SourceAccount != nil {
			f.line(b, opPrefix+".sourceAccount._present", "true")
			f.line(b, opPrefix+".sourceAccount", f.formatMuxedAccount(op.SourceAccount))
		} else {
			f.line(b, opPrefix+".sourceAccount._present", "false")
		}

		f.formatOperationBody(b, opPrefix+".body", &op.Body)
	}
}

func (f *TxRepFormatter) formatOperationBody(b *strings.Builder, prefix string, body *xdr.OperationBody) {
	switch body.Type {
	case xdr.OperationTypeCreateAccount:
		f.line(b, prefix+".type", "CREATE_ACCOUNT")
		op, _ := body.GetCreateAccountOp()
		f.line(b, prefix+".createAccountOp.destination", op.Destination.Address())
		f.line(b, prefix+".createAccountOp.startingBalance", fmt.Sprintf("%d", op.StartingBalance))

	case xdr.OperationTypePayment:
		f.line(b, prefix+".type", "PAYMENT")
		op, _ := body.GetPaymentOp()
		f.line(b, prefix+".paymentOp.destination", f.formatMuxedAccount(&op.Destination))
		f.line(b, prefix+".paymentOp.asset", f.formatAsset(&op.Asset))
		f.line(b, prefix+".paymentOp.amount", fmt.Sprintf("%d", op.Amount))

	case xdr.OperationTypePathPaymentStrictReceive:
		f.line(b, prefix+".type", "PATH_PAYMENT_STRICT_RECEIVE")
		op, _ := body.GetPathPaymentStrictReceiveOp()
		f.line(b, prefix+".pathPaymentStrictReceiveOp.sendAsset", f.formatAsset(&op.SendAsset))
		f.line(b, prefix+".pathPaymentStrictReceiveOp.sendMax", fmt.Sprintf("%d", op.SendMax))
		f.line(b, prefix+".pathPaymentStrictReceiveOp.destination", f.formatMuxedAccount(&op.Destination))
		f.line(b, prefix+".pathPaymentStrictReceiveOp.destAsset", f.formatAsset(&op.DestAsset))
		f.line(b, prefix+".pathPaymentStrictReceiveOp.destAmount", fmt.Sprintf("%d", op.DestAmount))
		f.line(b, prefix+".pathPaymentStrictReceiveOp.path.len", fmt.Sprintf("%d", len(op.Path)))
		for j, p := range op.Path {
			f.line(b, fmt.Sprintf("%s.pathPaymentStrictReceiveOp.path[%d]", prefix, j), f.formatAsset(&p))
		}

	case xdr.OperationTypeManageSellOffer:
		f.line(b, prefix+".type", "MANAGE_SELL_OFFER")
		op, _ := body.GetManageSellOfferOp()
		f.line(b, prefix+".manageSellOfferOp.selling", f.formatAsset(&op.Selling))
		f.line(b, prefix+".manageSellOfferOp.buying", f.formatAsset(&op.Buying))
		f.line(b, prefix+".manageSellOfferOp.amount", fmt.Sprintf("%d", op.Amount))
		f.line(b, prefix+".manageSellOfferOp.price.n", fmt.Sprintf("%d", op.Price.N))
		f.line(b, prefix+".manageSellOfferOp.price.d", fmt.Sprintf("%d", op.Price.D))
		f.line(b, prefix+".manageSellOfferOp.offerID", fmt.Sprintf("%d", op.OfferId))

	case xdr.OperationTypeCreatePassiveSellOffer:
		f.line(b, prefix+".type", "CREATE_PASSIVE_SELL_OFFER")
		op, _ := body.GetCreatePassiveSellOfferOp()
		f.line(b, prefix+".createPassiveSellOfferOp.selling", f.formatAsset(&op.Selling))
		f.line(b, prefix+".createPassiveSellOfferOp.buying", f.formatAsset(&op.Buying))
		f.line(b, prefix+".createPassiveSellOfferOp.amount", fmt.Sprintf("%d", op.Amount))
		f.line(b, prefix+".createPassiveSellOfferOp.price.n", fmt.Sprintf("%d", op.Price.N))
		f.line(b, prefix+".createPassiveSellOfferOp.price.d", fmt.Sprintf("%d", op.Price.D))

	case xdr.OperationTypeSetOptions:
		f.line(b, prefix+".type", "SET_OPTIONS")
		op, _ := body.GetSetOptionsOp()
		if op.InflationDest != nil {
			f.line(b, prefix+".setOptionsOp.inflationDest._present", "true")
			f.line(b, prefix+".setOptionsOp.inflationDest", op.InflationDest.Address())
		} else {
			f.line(b, prefix+".setOptionsOp.inflationDest._present", "false")
		}
		if op.ClearFlags != nil {
			f.line(b, prefix+".setOptionsOp.clearFlags._present", "true")
			f.line(b, prefix+".setOptionsOp.clearFlags", fmt.Sprintf("%d", *op.ClearFlags))
		} else {
			f.line(b, prefix+".setOptionsOp.clearFlags._present", "false")
		}
		if op.SetFlags != nil {
			f.line(b, prefix+".setOptionsOp.setFlags._present", "true")
			f.line(b, prefix+".setOptionsOp.setFlags", fmt.Sprintf("%d", *op.SetFlags))
		} else {
			f.line(b, prefix+".setOptionsOp.setFlags._present", "false")
		}
		if op.MasterWeight != nil {
			f.line(b, prefix+".setOptionsOp.masterWeight._present", "true")
			f.line(b, prefix+".setOptionsOp.masterWeight", fmt.Sprintf("%d", *op.MasterWeight))
		} else {
			f.line(b, prefix+".setOptionsOp.masterWeight._present", "false")
		}
		if op.LowThreshold != nil {
			f.line(b, prefix+".setOptionsOp.lowThreshold._present", "true")
			f.line(b, prefix+".setOptionsOp.lowThreshold", fmt.Sprintf("%d", *op.LowThreshold))
		} else {
			f.line(b, prefix+".setOptionsOp.lowThreshold._present", "false")
		}
		if op.MedThreshold != nil {
			f.line(b, prefix+".setOptionsOp.medThreshold._present", "true")
			f.line(b, prefix+".setOptionsOp.medThreshold", fmt.Sprintf("%d", *op.MedThreshold))
		} else {
			f.line(b, prefix+".setOptionsOp.medThreshold._present", "false")
		}
		if op.HighThreshold != nil {
			f.line(b, prefix+".setOptionsOp.highThreshold._present", "true")
			f.line(b, prefix+".setOptionsOp.highThreshold", fmt.Sprintf("%d", *op.HighThreshold))
		} else {
			f.line(b, prefix+".setOptionsOp.highThreshold._present", "false")
		}
		if op.HomeDomain != nil {
			f.line(b, prefix+".setOptionsOp.homeDomain._present", "true")
			f.line(b, prefix+".setOptionsOp.homeDomain", strconv.Quote(string(*op.HomeDomain)))
		} else {
			f.line(b, prefix+".setOptionsOp.homeDomain._present", "false")
		}
		if op.Signer != nil {
			f.line(b, prefix+".setOptionsOp.signer._present", "true")
			f.line(b, prefix+".setOptionsOp.signer.key", op.Signer.Key.Address())
			f.line(b, prefix+".setOptionsOp.signer.weight", fmt.Sprintf("%d", op.Signer.Weight))
		} else {
			f.line(b, prefix+".setOptionsOp.signer._present", "false")
		}

	case xdr.OperationTypeChangeTrust:
		f.line(b, prefix+".type", "CHANGE_TRUST")
		op, _ := body.GetChangeTrustOp()
		f.line(b, prefix+".changeTrustOp.line", f.formatChangeTrustAsset(&op.Line))
		f.line(b, prefix+".changeTrustOp.limit", fmt.Sprintf("%d", op.Limit))

	case xdr.OperationTypeAllowTrust:
		f.line(b, prefix+".type", "ALLOW_TRUST")
		op, _ := body.GetAllowTrustOp()
		f.line(b, prefix+".allowTrustOp.trustor", op.Trustor.Address())
		f.line(b, prefix+".allowTrustOp.asset", f.formatAllowTrustAsset(&op.Asset))
		f.line(b, prefix+".allowTrustOp.authorize", fmt.Sprintf("%d", op.Authorize))

	case xdr.OperationTypeAccountMerge:
		f.line(b, prefix+".type", "ACCOUNT_MERGE")
		dest, _ := body.GetDestination()
		f.line(b, prefix+".destination", f.formatMuxedAccount(&dest))

	case xdr.OperationTypeInflation:
		f.line(b, prefix+".type", "INFLATION")

	case xdr.OperationTypeManageData:
		f.line(b, prefix+".type", "MANAGE_DATA")
		op, _ := body.GetManageDataOp()
		f.line(b, prefix+".manageDataOp.dataName", strconv.Quote(string(op.DataName)))
		if op.DataValue != nil {
			f.line(b, prefix+".manageDataOp.dataValue._present", "true")
			f.line(b, prefix+".manageDataOp.dataValue", hex.EncodeToString(*op.DataValue))
		} else {
			f.line(b, prefix+".manageDataOp.dataValue._present", "false")
		}

	case xdr.OperationTypeBumpSequence:
		f.line(b, prefix+".type", "BUMP_SEQUENCE")
		op, _ := body.GetBumpSequenceOp()
		f.line(b, prefix+".bumpSequenceOp.bumpTo", fmt.Sprintf("%d", op.BumpTo))

	case xdr.OperationTypeManageBuyOffer:
		f.line(b, prefix+".type", "MANAGE_BUY_OFFER")
		op, _ := body.GetManageBuyOfferOp()
		f.line(b, prefix+".manageBuyOfferOp.selling", f.formatAsset(&op.Selling))
		f.line(b, prefix+".manageBuyOfferOp.buying", f.formatAsset(&op.Buying))
		f.line(b, prefix+".manageBuyOfferOp.buyAmount", fmt.Sprintf("%d", op.BuyAmount))
		f.line(b, prefix+".manageBuyOfferOp.price.n", fmt.Sprintf("%d", op.Price.N))
		f.line(b, prefix+".manageBuyOfferOp.price.d", fmt.Sprintf("%d", op.Price.D))
		f.line(b, prefix+".manageBuyOfferOp.offerID", fmt.Sprintf("%d", op.OfferId))

	case xdr.OperationTypePathPaymentStrictSend:
		f.line(b, prefix+".type", "PATH_PAYMENT_STRICT_SEND")
		op, _ := body.GetPathPaymentStrictSendOp()
		f.line(b, prefix+".pathPaymentStrictSendOp.sendAsset", f.formatAsset(&op.SendAsset))
		f.line(b, prefix+".pathPaymentStrictSendOp.sendAmount", fmt.Sprintf("%d", op.SendAmount))
		f.line(b, prefix+".pathPaymentStrictSendOp.destination", f.formatMuxedAccount(&op.Destination))
		f.line(b, prefix+".pathPaymentStrictSendOp.destAsset", f.formatAsset(&op.DestAsset))
		f.line(b, prefix+".pathPaymentStrictSendOp.destMin", fmt.Sprintf("%d", op.DestMin))
		f.line(b, prefix+".pathPaymentStrictSendOp.path.len", fmt.Sprintf("%d", len(op.Path)))
		for j, p := range op.Path {
			f.line(b, fmt.Sprintf("%s.pathPaymentStrictSendOp.path[%d]", prefix, j), f.formatAsset(&p))
		}

	case xdr.OperationTypeCreateClaimableBalance:
		f.line(b, prefix+".type", "CREATE_CLAIMABLE_BALANCE")
		op, _ := body.GetCreateClaimableBalanceOp()
		f.line(b, prefix+".createClaimableBalanceOp.asset", f.formatAsset(&op.Asset))
		f.line(b, prefix+".createClaimableBalanceOp.amount", fmt.Sprintf("%d", op.Amount))
		f.line(b, prefix+".createClaimableBalanceOp.claimants.len", fmt.Sprintf("%d", len(op.Claimants)))

	case xdr.OperationTypeClaimClaimableBalance:
		f.line(b, prefix+".type", "CLAIM_CLAIMABLE_BALANCE")
		op, _ := body.GetClaimClaimableBalanceOp()
		if id, err := op.BalanceId.MarshalBinary(); err == nil {
			f.line(b, prefix+".claimClaimableBalanceOp.balanceID", hex.EncodeToString(id))
		}

	case xdr.OperationTypeBeginSponsoringFutureReserves:
		f.line(b, prefix+".type", "BEGIN_SPONSORING_FUTURE_RESERVES")
		op, _ := body.GetBeginSponsoringFutureReservesOp()
		f.line(b, prefix+".beginSponsoringFutureReservesOp.sponsoredID", op.SponsoredId.Address())

	case xdr.OperationTypeEndSponsoringFutureReserves:
		f.line(b, prefix+".type", "END_SPONSORING_FUTURE_RESERVES")

	case xdr.OperationTypeRevokeSponsorship:
		f.line(b, prefix+".type", "REVOKE_SPONSORSHIP")

	case xdr.OperationTypeClawback:
		f.line(b, prefix+".type", "CLAWBACK")
		op, _ := body.GetClawbackOp()
		f.line(b, prefix+".clawbackOp.asset", f.formatAsset(&op.Asset))
		f.line(b, prefix+".clawbackOp.from", f.formatMuxedAccount(&op.From))
		f.line(b, prefix+".clawbackOp.amount", fmt.Sprintf("%d", op.Amount))

	case xdr.OperationTypeClawbackClaimableBalance:
		f.line(b, prefix+".type", "CLAWBACK_CLAIMABLE_BALANCE")
		op, _ := body.GetClawbackClaimableBalanceOp()
		if id, err := op.BalanceId.MarshalBinary(); err == nil {
			f.line(b, prefix+".clawbackClaimableBalanceOp.balanceID", hex.EncodeToString(id))
		}

	case xdr.OperationTypeSetTrustLineFlags:
		f.line(b, prefix+".type", "SET_TRUST_LINE_FLAGS")
		op, _ := body.GetSetTrustLineFlagsOp()
		f.line(b, prefix+".setTrustLineFlagsOp.trustor", op.Trustor.Address())
		f.line(b, prefix+".setTrustLineFlagsOp.asset", f.formatAsset(&op.Asset))
		f.line(b, prefix+".setTrustLineFlagsOp.clearFlags", fmt.Sprintf("%d", op.ClearFlags))
		f.line(b, prefix+".setTrustLineFlagsOp.setFlags", fmt.Sprintf("%d", op.SetFlags))

	case xdr.OperationTypeLiquidityPoolDeposit:
		f.line(b, prefix+".type", "LIQUIDITY_POOL_DEPOSIT")
		op, _ := body.GetLiquidityPoolDepositOp()
		f.line(b, prefix+".liquidityPoolDepositOp.liquidityPoolID", hex.EncodeToString(op.LiquidityPoolId[:]))
		f.line(b, prefix+".liquidityPoolDepositOp.maxAmountA", fmt.Sprintf("%d", op.MaxAmountA))
		f.line(b, prefix+".liquidityPoolDepositOp.maxAmountB", fmt.Sprintf("%d", op.MaxAmountB))

	case xdr.OperationTypeLiquidityPoolWithdraw:
		f.line(b, prefix+".type", "LIQUIDITY_POOL_WITHDRAW")
		op, _ := body.GetLiquidityPoolWithdrawOp()
		f.line(b, prefix+".liquidityPoolWithdrawOp.liquidityPoolID", hex.EncodeToString(op.LiquidityPoolId[:]))
		f.line(b, prefix+".liquidityPoolWithdrawOp.amount", fmt.Sprintf("%d", op.Amount))

	case xdr.OperationTypeInvokeHostFunction:
		f.line(b, prefix+".type", "INVOKE_HOST_FUNCTION")
		op, _ := body.GetInvokeHostFunctionOp()
		f.line(b, prefix+".invokeHostFunctionOp.hostFunction.type", op.HostFunction.Type.String())
		f.line(b, prefix+".invokeHostFunctionOp.auth.len", fmt.Sprintf("%d", len(op.Auth)))

	case xdr.OperationTypeExtendFootprintTtl:
		f.line(b, prefix+".type", "EXTEND_FOOTPRINT_TTL")
		op, _ := body.GetExtendFootprintTtlOp()
		f.line(b, prefix+".extendFootprintTTLOp.ext.v", fmt.Sprintf("%d", op.Ext.V))
		f.line(b, prefix+".extendFootprintTTLOp.extendTo", fmt.Sprintf("%d", op.ExtendTo))

	case xdr.OperationTypeRestoreFootprint:
		f.line(b, prefix+".type", "RESTORE_FOOTPRINT")
		op, _ := body.GetRestoreFootprintOp()
		f.line(b, prefix+".restoreFootprintOp.ext.v", fmt.Sprintf("%d", op.Ext.V))

	default:
		f.line(b, prefix+".type", body.Type.String())
	}
}

func (f *TxRepFormatter) formatExt(b *strings.Builder, prefix string, ext *xdr.TransactionExt) {
	if ext == nil {
		f.line(b, prefix+".v", "0")
		return
	}
	f.line(b, prefix+".v", fmt.Sprintf("%d", ext.V))
	if ext.V == 1 && ext.SorobanData != nil {
		f.line(b, prefix+".sorobanData._present", "true")
		f.line(b, prefix+".sorobanData.resources.instructions", fmt.Sprintf("%d", ext.SorobanData.Resources.Instructions))
		f.line(b, prefix+".sorobanData.resources.readBytes", fmt.Sprintf("%d", ext.SorobanData.Resources.DiskReadBytes))
		f.line(b, prefix+".sorobanData.resources.writeBytes", fmt.Sprintf("%d", ext.SorobanData.Resources.WriteBytes))
		f.line(b, prefix+".sorobanData.resourceFee", fmt.Sprintf("%d", ext.SorobanData.ResourceFee))
	}
}

func (f *TxRepFormatter) formatSignatures(b *strings.Builder, prefix string, sigs []xdr.DecoratedSignature) {
	f.line(b, prefix+".len", fmt.Sprintf("%d", len(sigs)))
	for i := range sigs {
		s := &sigs[i]
		sigPrefix := fmt.Sprintf("%s[%d]", prefix, i)
		hintHex := hex.EncodeToString(s.Hint[:])
		sigHex := hex.EncodeToString(s.Signature)
		f.line(b, sigPrefix+".hint", hintHex)
		f.line(b, sigPrefix+".signature", sigHex)
	}
}

func (f *TxRepFormatter) formatMuxedAccount(m *xdr.MuxedAccount) string {
	if m == nil {
		return ""
	}
	return m.Address()
}

func (f *TxRepFormatter) formatAsset(a *xdr.Asset) string {
	if a == nil {
		return "native"
	}
	switch a.Type {
	case xdr.AssetTypeAssetTypeNative:
		if strings.Contains(strings.ToLower(f.network), "public") {
			return "XLM"
		}
		if strings.Contains(strings.ToLower(f.network), "test") {
			return "TestXLM"
		}
		return "native"
	case xdr.AssetTypeAssetTypeCreditAlphanum4:
		a4, _ := a.GetAlphaNum4()
		code := strings.TrimRight(string(a4.AssetCode[:]), "\x00")
		return fmt.Sprintf("%s:%s", code, a4.Issuer.Address())
	case xdr.AssetTypeAssetTypeCreditAlphanum12:
		a12, _ := a.GetAlphaNum12()
		code := strings.TrimRight(string(a12.AssetCode[:]), "\x00")
		return fmt.Sprintf("%s:%s", code, a12.Issuer.Address())
	default:
		return "native"
	}
}

func (f *TxRepFormatter) formatChangeTrustAsset(a *xdr.ChangeTrustAsset) string {
	if a == nil {
		return "native"
	}
	switch a.Type {
	case xdr.AssetTypeAssetTypeNative:
		return "native"
	case xdr.AssetTypeAssetTypeCreditAlphanum4:
		a4, _ := a.GetAlphaNum4()
		code := strings.TrimRight(string(a4.AssetCode[:]), "\x00")
		return fmt.Sprintf("%s:%s", code, a4.Issuer.Address())
	case xdr.AssetTypeAssetTypeCreditAlphanum12:
		a12, _ := a.GetAlphaNum12()
		code := strings.TrimRight(string(a12.AssetCode[:]), "\x00")
		return fmt.Sprintf("%s:%s", code, a12.Issuer.Address())
	case xdr.AssetTypeAssetTypePoolShare:
		p, _ := a.GetLiquidityPool()
		if id, err := p.MarshalBinary(); err == nil {
			return fmt.Sprintf("%s:lp", hex.EncodeToString(id))
		}
		return "native"
	default:
		return "native"
	}
}

func (f *TxRepFormatter) formatAllowTrustAsset(a *xdr.AssetCode) string {
	if a == nil {
		return ""
	}
	switch a.Type {
	case xdr.AssetTypeAssetTypeCreditAlphanum4:
		a4, _ := a.GetAssetCode4()
		return strings.TrimRight(string(a4[:]), "\x00")
	case xdr.AssetTypeAssetTypeCreditAlphanum12:
		a12, _ := a.GetAssetCode12()
		return strings.TrimRight(string(a12[:]), "\x00")
	default:
		return ""
	}
}

func (f *TxRepFormatter) line(b *strings.Builder, key, val string) {
	p := f.palette
	fmt.Fprintf(b, "%s%s%s: %s%s%s\n", p.Key, key, p.Reset, p.Str, val, p.Reset)
}
