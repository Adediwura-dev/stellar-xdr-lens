package cli

import (
	"encoding/json"
	"fmt"
	"io"

	"github.com/spf13/cobra"
	"github.com/stellar/go-stellar-sdk/amount"
	"github.com/stellar/go-stellar-sdk/xdr"

	"github.com/stellar-optics/stellar-xdr-lens/pkg/lens"
)

func newMetaDiffCmd(g *globalFlags, stdout io.Writer, stdin io.Reader) *cobra.Command {
	var asJSON bool
	cmd := &cobra.Command{
		Use:   "meta-diff <transaction-meta>",
		Short: "Show balance changes recorded in transaction metadata",
		Long: `Derive offline account, trustline and claimable-balance deltas from
TransactionMeta XDR. Baselines absent from metadata are marked unknown; no
ledger or network lookups are performed.`,
		Example: `  lens meta-diff transaction-meta.xdr
  lens meta-diff --json "$META_XDR"
  cat transaction-meta.xdr | lens meta-diff -`,
		Args: func(cmd *cobra.Command, args []string) error {
			if g.file != "" {
				return cobra.NoArgs(cmd, args)
			}
			return cobra.ExactArgs(1)(cmd, args)
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			operand := g.file
			if operand == "" {
				operand = args[0]
			}
			payload, err := readOperand(operand, stdin)
			if err != nil {
				return fmt.Errorf("reading transaction metadata: %w", err)
			}
			value, err := lens.DecodeAs(payload, "TransactionMeta")
			if err != nil {
				return fmt.Errorf("decoding transaction metadata: %w", err)
			}
			meta, ok := value.Raw.(*xdr.TransactionMeta)
			if !ok || meta == nil {
				return fmt.Errorf("decoded TransactionMeta has unexpected value %T", value.Raw)
			}
			result, err := lens.DiffTransactionMetaBalances(meta)
			if err != nil {
				return err
			}
			if asJSON {
				encoder := json.NewEncoder(stdout)
				encoder.SetIndent("", "  ")
				if err := encoder.Encode(result); err != nil {
					return fmt.Errorf("encoding balance diff: %w", err)
				}
				return nil
			}
			return writeMetaBalanceDiff(stdout, result)
		},
	}
	cmd.Flags().BoolVar(&asJSON, "json", false, "emit balance changes as JSON")
	return cmd
}

func writeMetaBalanceDiff(w io.Writer, result *lens.MetaBalanceDiffResult) error {
	if result == nil {
		return fmt.Errorf("format: nil metadata balance diff")
	}
	if len(result.Changes) == 0 {
		if _, err := fmt.Fprintln(w, "no balance changes"); err != nil {
			return fmt.Errorf("writing balance diff: %w", err)
		}
		return nil
	}
	for _, change := range result.Changes {
		before := "?"
		if change.BeforeKnown {
			before = formatMetaBalance(change.Asset, change.Before)
		}
		after := "?"
		if change.AfterKnown {
			after = formatMetaBalance(change.Asset, change.After)
		}
		delta := "?"
		if change.BeforeKnown && change.AfterKnown {
			delta = signedMetaBalance(change.Asset, change.Delta)
		}
		if _, err := fmt.Fprintf(w, "%s %s: %s -> %s (%s)\n",
			change.Holder, change.Asset, before, after, delta); err != nil {
			return fmt.Errorf("writing balance diff: %w", err)
		}
	}
	return nil
}

func formatMetaBalance(asset string, value int64) string {
	if asset == "XLM" {
		return amount.String(xdr.Int64(value)) + " XLM"
	}
	return fmt.Sprintf("%d %s", value, asset)
}

func signedMetaBalance(asset string, value int64) string {
	prefix := ""
	if value >= 0 {
		prefix = "+"
	}
	return prefix + formatMetaBalance(asset, value)
}
