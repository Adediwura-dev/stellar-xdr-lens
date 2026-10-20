package cli_test

import (
	"errors"
	"fmt"
	"testing"

	"github.com/stellar-optics/stellar-xdr-lens/internal/cli"
	"github.com/stellar-optics/stellar-xdr-lens/pkg/lens"
)

// TestExitCodePolicyPinsTheStrictCodes covers the classification of every
// error the CLI surface can produce. The four codes are a public contract:
// 0=clean, 1=generic error, 2=diff found, 3=invalid XDR.
func TestExitCodePolicyPinsTheStrictCodes(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		err       error
		wantCode  int
		wantPrint bool
	}{
		{
			name:      "nil is success and prints nothing",
			err:       nil,
			wantCode:  0,
			wantPrint: false,
		},
		{
			name:      "usage and argument errors are generic errors",
			err:       errors.New("unsupported value for --color: sometimes"),
			wantCode:  1,
			wantPrint: true,
		},
		{
			name:      "a missing file is a generic error",
			err:       fmt.Errorf("reading nope.xdr: %w", errors.New("no such file")),
			wantCode:  1,
			wantPrint: true,
		},
		{
			name:      "unknown type is a usage error, not payload corruption",
			err:       fmt.Errorf("decoding: %w", lens.ErrUnknownType),
			wantCode:  1,
			wantPrint: true,
		},
		{
			name:      "empty input is a generic error",
			err:       fmt.Errorf("decoding: %w", lens.ErrEmptyInput),
			wantCode:  1,
			wantPrint: true,
		},
		{
			name:      "invalid base64 is invalid XDR",
			err:       fmt.Errorf("decoding: %w", lens.ErrInvalidBase64),
			wantCode:  3,
			wantPrint: true,
		},
		{
			name:      "malformed XDR is invalid XDR",
			err:       fmt.Errorf("decode TransactionResult: %w", lens.ErrInvalidXDR),
			wantCode:  3,
			wantPrint: true,
		},
		{
			name:      "no matching type is invalid XDR",
			err:       fmt.Errorf("detecting type: %w", lens.ErrNoMatch),
			wantCode:  3,
			wantPrint: true,
		},
		{
			name:      "non-canonical padding is invalid XDR",
			err:       fmt.Errorf("canonical validation failed: %w", lens.ErrNonCanonicalPadding),
			wantCode:  3,
			wantPrint: true,
		},
		{
			name:      "corrupt stream records are invalid XDR",
			err:       fmt.Errorf("stream: %w", lens.ErrInvalidFraming),
			wantCode:  3,
			wantPrint: true,
		},
	}
	// Condition findings (exit 2) cannot be constructed from outside the
	// package; they are pinned through the real command surface in
	// TestExitCodePolicyOnRealCommands, which drives `diff --quiet` and
	// `explain --fail-on-error` end to end.

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			code, printable := cli.ExitCode(tc.err)
			if code != tc.wantCode {
				t.Errorf("ExitCode() = %d, want %d", code, tc.wantCode)
			}
			if printable != tc.wantPrint {
				t.Errorf("printable = %t, want %t", printable, tc.wantPrint)
			}
		})
	}
}

// TestExitCodePolicySentinelsAreStable freezes the exported constant values so
// renaming or renumbering is caught as a deliberate, reviewed change.
func TestExitCodePolicySentinelsAreStable(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name string
		got  int
		want int
	}{
		{"ExitSuccess", cli.ExitSuccess, 0},
		{"ExitError", cli.ExitError, 1},
		{"ExitDiffFound", cli.ExitDiffFound, 2},
		{"ExitInvalidXDR", cli.ExitInvalidXDR, 3},
	} {
		if tc.got != tc.want {
			t.Errorf("%s = %d, want %d", tc.name, tc.got, tc.want)
		}
	}
}

// TestExitCodePolicyOnRealCommands drives the real command surface: the same
// payload must exit 3 when unreadable, the tool must still exit 1 for its own
// mistakes, and the condition slots must exit 2, so a script can tell the
// three apart.
func TestExitCodePolicyOnRealCommands(t *testing.T) {
	t.Parallel()

	// Invalid base64 through decode.
	_, _, err := run(t, "", "decode", "!!!not base64!!!")
	code, printable := cli.ExitCode(err)
	if code != cli.ExitInvalidXDR || !printable {
		t.Errorf("decode bad payload = (%d, %t), want (%d, true)", code, printable, cli.ExitInvalidXDR)
	}

	// A misnamed file is the tool's environment, not XDR corruption.
	_, _, err = run(t, "", "decode", "--file", "/nonexistent/path.xdr")
	if code, printable := cli.ExitCode(err); code != cli.ExitError || !printable {
		t.Errorf("decode missing file = (%d, %t), want (%d, true)", code, printable, cli.ExitError)
	}

	// The success path.
	_, _, err = run(t, "", "decode", fixture(t, "tx_success.env"))
	if code, _ := cli.ExitCode(err); code != cli.ExitSuccess {
		t.Errorf("decode valid payload exit = %d, want 0", code)
	}

	// A detected difference is a condition, not an error.
	_, _, err = run(t, "", "diff", "--quiet",
		fixturePath("tx_soroban.res"), fixturePath("tx_failed.res"))
	if code, printable := cli.ExitCode(err); code != cli.ExitDiffFound || printable {
		t.Errorf("diff --quiet on differing values = (%d, %t), want (%d, false)",
			code, printable, cli.ExitDiffFound)
	}

	// A failed transaction reported by the CI gate shares the condition slot.
	_, _, err = run(t, "", "explain", fixture(t, "tx_failed.env"),
		"--result", fixture(t, "tx_failed.res"), "--fail-on-error")
	if code, printable := cli.ExitCode(err); code != cli.ExitDiffFound || printable {
		t.Errorf("explain --fail-on-error on a failed tx = (%d, %t), want (%d, false)",
			code, printable, cli.ExitDiffFound)
	}
}

// TestIsInvalidXDRErrorSmoke exercises the exported classifier directly.
func TestIsInvalidXDRErrorSmoke(t *testing.T) {
	t.Parallel()

	if !cli.IsInvalidXDRError(lens.ErrNoMatch) {
		t.Error("ErrNoMatch should classify as invalid XDR")
	}
	if cli.IsInvalidXDRError(lens.ErrUnknownType) {
		t.Error("ErrUnknownType is a usage error, not invalid XDR")
	}
	if cli.IsInvalidXDRError(nil) {
		t.Error("nil should not classify as invalid XDR")
	}
}
