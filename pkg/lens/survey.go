package lens

import (
	"encoding/hex"
	"fmt"
	"reflect"

	"github.com/stellar/go-stellar-sdk/xdr"
)

// Stellar Core's time-sliced survey lets an operator ask other nodes for a
// slice of their network topology: peer counts, latencies, sync history and
// the addresses of a bounded window of peers. A survey message travels over
// the same overlay as Hello and Auth and is always signed: the four wire
// shapes are the signed start, stop, request and response messages.
//
// Two renderings make the decoded messages readable. The command type is a
// bare enum whose generated name is long, so it gets the short
// documentation label; and the response body is an opaque encrypted blob,
// which is summarised by size instead of being shown as an unexplained byte
// array. Node ids, certificate keys and signatures already render as strkey
// addresses and hex through the SCP and overlay enrichments.
func init() {
	enricherFor[reflect.TypeOf(xdr.SurveyMessageCommandType(0))] = enrichSurveyCommandType
	enricherFor[reflect.TypeOf(xdr.EncryptedBody(nil))] = enrichEncryptedBody

	// Every survey message names the ledger it surveys; the field is a bare
	// Uint32, so the note is what turns it into a fact worth reading.
	fieldNotes["LedgerNum"] = surveyLedgerNote
}

// enrichSurveyCommandType renders the survey command enum with the
// lower-case label used by protocol documentation, matching how overlay
// message names are rendered.
func enrichSurveyCommandType(rv reflect.Value) (string, any, bool) {
	t, ok := rv.Interface().(xdr.SurveyMessageCommandType)
	if !ok {
		return "", nil, false
	}
	if t == xdr.SurveyMessageCommandTypeTimeSlicedSurveyTopology {
		return "", "time_sliced_survey_topology", true
	}
	return "", nil, false
}

// enrichEncryptedBody renders a survey response body as hex plus its size.
// The body is encrypted to the surveyor, so its content is out of reach for
// any decoder; the length and the fact that it is opaque are the honest
// rendering. An empty body is still called out rather than silently pruned.
func enrichEncryptedBody(rv reflect.Value) (string, any, bool) {
	body, ok := rv.Interface().(xdr.EncryptedBody)
	if !ok {
		return "", nil, false
	}
	return fmt.Sprintf("encrypted, %d byte(s)", len(body)), hex.EncodeToString(body), true
}

// surveyLedgerNote names the ledger a survey message refers to.
func surveyLedgerNote(rv reflect.Value) (string, bool) {
	v, ok := rv.Interface().(xdr.Uint32)
	if !ok {
		return "", false
	}
	return fmt.Sprintf("survey of ledger %d", uint64(v)), true
}
