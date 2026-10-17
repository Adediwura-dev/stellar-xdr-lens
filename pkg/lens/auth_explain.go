package lens

import (
	"fmt"
	"reflect"
	"strings"

	"github.com/stellar/go-stellar-sdk/xdr"
)

// AuthFailure explains a Soroban contract authorization failure.
// This covers invalid signatures, expired nonce sequences, and missing
// authorization entries that prevent a transaction from executing.
type AuthFailure struct {
	// FailureKind is the category of authorization failure.
	FailureKind string
	// OperationIndex is the index of the failing operation.
	OperationIndex int
	// MissingAuthenticator describes what authenticator is missing, if known.
	MissingAuthenticator string
	// NonceExpiry explains if and when a nonce expired, if applicable.
	NonceExpiry string
	// Summary plain-English summary of the failure.
	Summary string
	// Hint suggests the next debugging step.
	Hint string
}

// NewAuthFailure creates a new AuthFailure diagnosis.
func NewAuthFailure(failureKind string, operationIndex int, missingAuthenticator, nonceExpiry, summary, hint string) *AuthFailure {
	return &AuthFailure{
		FailureKind:          failureKind,
		OperationIndex:       operationIndex,
		MissingAuthenticator: missingAuthenticator,
		NonceExpiry:          nonceExpiry,
		Summary:              summary,
		Hint:                 hint,
	}
}

// String renders the diagnosis as a single line.
func (a *AuthFailure) String() string {
	if a == nil {
		return "<nil>"
	}
	return a.Summary
}

// DiagnosticReport renders a multi-line report suitable for CLI output.
func (a *AuthFailure) DiagnosticReport() string {
	if a == nil {
		return ""
	}
	var b strings.Builder
	fmt.Fprintf(&b, "Soroban authorization failure:\n")
	fmt.Fprintf(&b, "  Kind:          %s\n", a.FailureKind)
	fmt.Fprintf(&b, "  Operation:     %d\n", a.OperationIndex)
	if a.MissingAuthenticator != "" {
		fmt.Fprintf(&b, "  Missing:       %s\n", a.MissingAuthenticator)
	}
	if a.NonceExpiry != "" {
		fmt.Fprintf(&b, "  Nonce expiry:  %s\n", a.NonceExpiry)
	}
	fmt.Fprintf(&b, "  Summary:       %s\n", a.Summary)
	fmt.Fprintf(&b, "  Hint:          %s\n", a.Hint)
	return b.String()
}

// DiagnoseAuthFailure diagnoses a Soroban authorization failure from an
// operation result. This is a purely local, offline diagnosis - no network
// calls are made.
//
// It identifies three categories of auth failures:
//  1. Missing or invalid signature authorization entries
//  2. Expired or invalid nonce sequences
//  3. Missing contract-level auth entries
func DiagnoseAuthFailure(opResult xdr.OperationResult, operationIndex int) *AuthFailure {
	// Check if the inner result code indicates an auth failure
	if opResult.Code != xdr.OperationResultCodeOpInner {
		return nil
	}

	node := buildNode("", reflect.ValueOf(opResult), 0)
	if node == nil {
		return nil
	}

	tr := node.Child("Tr")
	if tr == nil {
		return nil
	}

	// Look for the invoked contract's auth-related fields
	for _, arm := range tr.Children {
		// Check for auth entry related arms
		if arm.Name == "Auth" || strings.HasPrefix(arm.Name, "Auth") {
			// Extract auth details
			var missingAuth, nonceExpiry, summary, hint string

			// Look for specific auth code children
			if codeNode := arm.Child("Code"); codeNode != nil {
				if code, ok := codeNode.Value.(string); ok {
					switch {
					case strings.Contains(code, "Auth") && strings.Contains(code, "Missing"):
						missingAuth = code
						summary = "Missing or invalid Soroban authorization entry"
						hint = "Collect the missing authorization entry and sign the authorizeEntry payload with the required signers"
					case strings.Contains(code, "Nonce") && strings.Contains(code, "Expired"):
						nonceExpiry = code
						summary = "Soroban nonce sequence has expired"
						hint = "Rebuild the transaction with a fresh nonce sequence or extend the time bounds"
					default:
						summary = "Authorization failure: " + code
					}
				}
			}

			if summary != "" {
				return NewAuthFailure(
					getFailureKind(missingAuth, nonceExpiry),
					operationIndex,
					missingAuth,
					nonceExpiry,
					summary,
					hint,
				)
			}
		}

		// Also check for general result code patterns
		if codeNode := arm.Child("Code"); codeNode != nil {
			if code, ok := codeNode.Value.(string); ok {
				switch {
				case strings.Contains(code, "AuthMissing") || strings.Contains(code, "auth_missing"):
					return NewAuthFailure(
						"auth_missing",
						operationIndex,
						"missing authorization entry",
						"",
						"Missing or invalid Soroban authorization entry",
						"Collect the missing authorization entry and sign the authorizeEntry payload with the required signers",
					)
				case strings.Contains(code, "NonceExpired") || strings.Contains(code, "nonce_expired"):
					return NewAuthFailure(
						"nonce_expired",
						operationIndex,
						"",
						"nonce sequence expired",
						"Soroban nonce sequence has expired",
						"Rebuild the transaction with a fresh nonce sequence or extend the time bounds",
					)
				}
			}
		}
	}

	return nil
}

// getFailureKind determines the kind of auth failure from the code and hint text.
func getFailureKind(missingAuth, nonceExpiry string) string {
	if missingAuth != "" && strings.Contains(missingAuth, "Missing") {
		return "auth_missing"
	}
	if nonceExpiry != "" && strings.Contains(nonceExpiry, "Expired") {
		return "nonce_expired"
	}
	return "auth_failure"
}

// init registers the diagnosis function with the lens framework.
func init() {
	_ = AuthFailure{}
}
