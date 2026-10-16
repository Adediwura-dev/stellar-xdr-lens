package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/spf13/cobra"
	"github.com/stellar/go-stellar-sdk/network"

	"github.com/stellar-optics/stellar-xdr-lens/pkg/lens"
)

// knownNetworks maps the network names accepted by --network to their
// passphrases. Anything else is treated as a raw passphrase, which is what
// custom networks need.
var knownNetworks = map[string]string{
	"mainnet":   network.PublicNetworkPassphrase,
	"testnet":   network.TestNetworkPassphrase,
	"futurenet": network.FutureNetworkPassphrase,
}

func newAssetCmd(g *globalFlags, stdout io.Writer, stdin io.Reader) *cobra.Command {
	var (
		networkName string
		asJSON      bool
	)

	cmd := &cobra.Command{
		Use:   "asset [base64]",
		Short: "Decode a classic asset and derive its Stellar Asset Contract id",
		Long: `Decode a base64-encoded classic Asset.

The Stellar Asset Contract id is deterministic: it is derived from the network
passphrase and the asset, so the same asset has a different contract address on
each network. Use --network to pick mainnet, testnet or futurenet, or pass a
custom network passphrase.`,
		Example: `  # The native asset
  lens asset "AAAAAA=="

  # A credit asset on testnet
  lens asset --network testnet "$ASSET"

  # Machine-readable
  lens asset --json "$ASSET" | jq -r .contractId`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			payload, err := resolveInput(firstArg(args), g.file, stdin)
			if err != nil {
				return err
			}

			passphrase, label := resolveNetwork(networkName)
			details, err := lens.DescribeAsset(payload, passphrase)
			if err != nil {
				return err
			}

			if asJSON {
				enc := json.NewEncoder(stdout)
				enc.SetIndent("", "  ")
				if err := enc.Encode(details); err != nil {
					return fmt.Errorf("encoding asset details: %w", err)
				}
				return nil
			}

			for _, line := range []struct{ label, value string }{
				{"Network", label},
				{"Type", details.Type},
				{"Code", details.Code},
				{"Issuer", details.Issuer},
				{"Canonical", details.Canonical},
				{"Contract ID", details.ContractID},
			} {
				if line.value == "" {
					continue
				}
				if _, err := fmt.Fprintf(stdout, "%-12s %s\n", line.label, line.value); err != nil {
					return fmt.Errorf("writing asset details: %w", err)
				}
			}
			return nil
		},
	}

	cmd.Flags().StringVar(&networkName, "network", "mainnet",
		`which network to derive the contract id for: "mainnet", "testnet", "futurenet", or a passphrase`)
	cmd.Flags().BoolVar(&asJSON, "json", false, "emit the asset details as JSON")

	return cmd
}

// resolveNetwork maps a known network name to its passphrase and returns the
// name back for display. A value that is not a known name is passed through
// unchanged, on the assumption it is a custom passphrase.
func resolveNetwork(name string) (passphrase, label string) {
	trimmed := strings.TrimSpace(name)
	if pass, ok := knownNetworks[strings.ToLower(trimmed)]; ok {
		return pass, strings.ToLower(trimmed)
	}
	return name, name
}
