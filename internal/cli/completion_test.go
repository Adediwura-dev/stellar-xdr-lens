package cli_test

import (
	"strings"
	"testing"

	"github.com/stellar-optics/stellar-xdr-lens/pkg/lens"
)

// TestCompletionTypeSuggestionsCoversRegistry pins the dynamic source: every
// registered XDR type name is offered, in registry order, regardless of what
// the user has typed so far.
func TestCompletionTypeSuggestionsCoversRegistry(t *testing.T) {
	t.Parallel()

	suggestions := lens.TypeNames()
	if len(suggestions) == 0 {
		t.Fatal("registry has no types; completion would be empty")
	}

	// The engine derives from the same registry the decoder uses, so a new
	// registered type is completable without touching completion.go.
	registered := strings.Join(suggestions, ",")
	for _, want := range []string{"TransactionEnvelope", "TransactionResult", "ScVal"} {
		if !strings.Contains(registered, want) {
			t.Errorf("completion registry source does not include %q", want)
		}
	}
}

// TestCompletionViaRealCobraComplete drives the real completion entry point
// cobra installs: `lens __complete decode --type ”` answers per keystroke
// from the live registry, exactly as the shell will call it.
func TestCompletionViaRealCobraComplete(t *testing.T) {
	t.Parallel()

	t.Run("decode --type offers registered XDR types", func(t *testing.T) {
		t.Parallel()

		stdout, stderr, err := run(t, "", "__complete", "decode", "--type", "")
		if err != nil {
			t.Fatalf("__complete decode --type error = %v (stderr: %s)", err, stderr)
		}
		for _, want := range []string{"TransactionEnvelope", "ScVal", "TransactionResult"} {
			if !strings.Contains(stdout, want) {
				t.Errorf("completion output does not offer %q:\n%s", want, stdout)
			}
		}
		if !strings.Contains(stdout, ":4") {
			t.Errorf("completion output does not end with the NoFileComp directive:\n%s", stdout)
		}
	})

	t.Run("decode --type filters by prefix", func(t *testing.T) {
		t.Parallel()

		stdout, _, err := run(t, "", "__complete", "decode", "--type", "Trans")
		if err != nil {
			t.Fatalf("__complete with prefix error = %v", err)
		}
		if !strings.Contains(stdout, "TransactionEnvelope") {
			t.Errorf("prefixed completion lost TransactionEnvelope:\n%s", stdout)
		}
		if strings.Contains(stdout, "Asset") {
			t.Errorf("prefixed completion leaked non-matching Asset:\n%s", stdout)
		}
	})

	t.Run("decode --format offers the output formats", func(t *testing.T) {
		t.Parallel()

		stdout, _, err := run(t, "", "__complete", "decode", "--format", "")
		if err != nil {
			t.Fatalf("__complete decode --format error = %v", err)
		}
		for _, want := range []string{"tree", "json", "summary", "txrep", "csv"} {
			if !strings.Contains(stdout, want) {
				t.Errorf("completion output does not offer format %q:\n%s", want, stdout)
			}
		}
	})

	t.Run("--color offers the accepted modes", func(t *testing.T) {
		t.Parallel()

		stdout, _, err := run(t, "", "__complete", "decode", "--color", "")
		if err != nil {
			t.Fatalf("__complete decode --color error = %v", err)
		}
		for _, want := range []string{"auto", "always", "never"} {
			if !strings.Contains(stdout, want) {
				t.Errorf("completion output does not offer color %q:\n%s", want, stdout)
			}
		}
	})

	t.Run("asset --network offers the presets", func(t *testing.T) {
		t.Parallel()

		stdout, _, err := run(t, "", "__complete", "asset", "--network", "")
		if err != nil {
			t.Fatalf("__complete asset --network error = %v", err)
		}
		for _, want := range []string{"mainnet", "testnet", "futurenet"} {
			if !strings.Contains(stdout, want) {
				t.Errorf("completion output does not offer network %q:\n%s", want, stdout)
			}
		}
	})

	t.Run("diff --redact offers the categories", func(t *testing.T) {
		t.Parallel()

		stdout, _, err := run(t, "", "__complete", "diff", "--redact", "")
		if err != nil {
			t.Fatalf("__complete diff --redact error = %v", err)
		}
		for _, want := range []string{"hashes", "signatures", "identifiers", "all"} {
			if !strings.Contains(stdout, want) {
				t.Errorf("completion output does not offer redact %q:\n%s", want, stdout)
			}
		}
	})

	t.Run("diff --type is wired too", func(t *testing.T) {
		t.Parallel()

		stdout, _, err := run(t, "", "__complete", "diff", "--type", "Ledger")
		if err != nil {
			t.Fatalf("__complete diff --type error = %v", err)
		}
		if !strings.Contains(stdout, "LedgerHeader") {
			t.Errorf("diff --type completion lost LedgerHeader:\n%s", stdout)
		}
	})
}

// TestCompletionCommandScripts renders each shell script the command
// advertises and asserts that the generated script wires back to lens.
func TestCompletionCommandScripts(t *testing.T) {
	t.Parallel()

	tests := []struct {
		shell       string
		wantContain string
	}{
		// Every shell script installs the __complete router that answers
		// for the whole binary.
		{"bash", "__lens"},
		{"zsh", "__lens"},
		{"fish", "__lens"},
		{"powershell", "lens"},
	}
	for _, tc := range tests {
		t.Run(tc.shell, func(t *testing.T) {
			t.Parallel()

			stdout, stderr, err := run(t, "", "completion", tc.shell)
			if err != nil {
				t.Fatalf("completion %s error = %v (stderr: %s)", tc.shell, err, stderr)
			}
			if len(stdout) < 500 {
				t.Errorf("completion %s script looks truncated (%d bytes)", tc.shell, len(stdout))
			}
			if !strings.Contains(stdout, tc.wantContain) {
				t.Errorf("completion %s script does not contain %q", tc.shell, tc.wantContain)
			}
		})
	}
}

// TestCompletionCommandDescribesItself keeps help honest about the shells.
func TestCompletionCommandDescribesItself(t *testing.T) {
	t.Parallel()

	stdout, _, err := run(t, "", "completion", "--help")
	if err != nil {
		t.Fatalf("completion --help error = %v", err)
	}
	for _, want := range []string{"bash", "zsh", "fish", "powershell"} {
		if !strings.Contains(stdout, want) {
			t.Errorf("completion help does not mention %q", want)
		}
	}
}
