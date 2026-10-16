package lens_test

import (
	"strings"
	"testing"

	"github.com/stellar/go-stellar-sdk/xdr"

	"github.com/stellar-optics/stellar-xdr-lens/pkg/lens"
)

func TestDecodeSurveyRequest(t *testing.T) {
	t.Parallel()

	v, err := lens.DecodeAs(loadFixture(t, "survey_request"), "SignedTimeSlicedSurveyRequestMessage")
	if err != nil {
		t.Fatalf("DecodeAs() error = %v", err)
	}
	raw, ok := v.Raw.(*xdr.SignedTimeSlicedSurveyRequestMessage)
	if !ok {
		t.Fatalf("Raw is %T, want *xdr.SignedTimeSlicedSurveyRequestMessage", v.Raw)
	}
	if raw.Request.Nonce != 9 || raw.Request.InboundPeersIndex != 5 || raw.Request.OutboundPeersIndex != 8 {
		t.Errorf("request = %+v, want nonce 9, inbound 5, outbound 8", raw.Request)
	}

	// The bare enum carries a name that is too long for a tree line; the
	// documentation label is the readable form.
	command := v.Node.Find("Request.Request.CommandType")
	if command == nil || command.Value != "time_sliced_survey_topology" {
		t.Errorf("CommandType = %v, want time_sliced_survey_topology", command)
	}
	ledger := v.Node.Find("Request.Request.LedgerNum")
	if ledger == nil || ledger.Note != "survey of ledger 55278432" {
		t.Errorf("LedgerNum = %v, want a ledger note", ledger)
	}
}

func TestDecodeSurveyResponse(t *testing.T) {
	t.Parallel()

	v, err := lens.DecodeAs(loadFixture(t, "survey_response"), "SignedTimeSlicedSurveyResponseMessage")
	if err != nil {
		t.Fatalf("DecodeAs() error = %v", err)
	}
	raw, ok := v.Raw.(*xdr.SignedTimeSlicedSurveyResponseMessage)
	if !ok {
		t.Fatalf("Raw is %T, want *xdr.SignedTimeSlicedSurveyResponseMessage", v.Raw)
	}
	if len(raw.Response.Response.EncryptedBody) != 24 {
		t.Errorf("EncryptedBody has %d bytes, want 24", len(raw.Response.Response.EncryptedBody))
	}

	// The body is encrypted to the surveyor: the decoder can show its bytes
	// but must not pretend to understand them.
	body := v.Node.Find("Response.Response.EncryptedBody")
	if body == nil {
		t.Fatal("EncryptedBody node is missing")
	}
	if body.Note != "encrypted, 24 byte(s)" {
		t.Errorf("EncryptedBody note = %q, want %q", body.Note, "encrypted, 24 byte(s)")
	}
	if !strings.HasPrefix(body.Value.(string), "0001020304050607") {
		t.Errorf("EncryptedBody value = %v, want hex starting 0001020304050607", body.Value)
	}
}

func TestDecodeSurveyStartCollecting(t *testing.T) {
	t.Parallel()

	v, err := lens.DecodeAs(loadFixture(t, "survey_start"), "SignedTimeSlicedSurveyStartCollectingMessage")
	if err != nil {
		t.Fatalf("DecodeAs() error = %v", err)
	}
	raw, ok := v.Raw.(*xdr.SignedTimeSlicedSurveyStartCollectingMessage)
	if !ok {
		t.Fatalf("Raw is %T, want *xdr.SignedTimeSlicedSurveyStartCollectingMessage", v.Raw)
	}
	if raw.StartCollecting.Nonce != 7 || raw.StartCollecting.LedgerNum != 55278432 {
		t.Errorf("StartCollecting = %+v, want nonce 7 for ledger 55278432", raw.StartCollecting)
	}
	ledger := v.Node.Find("StartCollecting.LedgerNum")
	if ledger == nil || ledger.Note != "survey of ledger 55278432" {
		t.Errorf("LedgerNum = %v, want a ledger note", ledger)
	}
}

// TestDecodeSurveyStopCollectingNeedsExplicitType pins the one documented
// ambiguity in the survey family: start and stop collect messages have
// identical field lists, so their encodings are identical and auto-detection
// can only ever name the higher-ranked start message.
func TestDecodeSurveyStopCollectingNeedsExplicitType(t *testing.T) {
	t.Parallel()

	v, err := lens.DecodeAs(loadFixture(t, "survey_stop"), "SignedTimeSlicedSurveyStopCollectingMessage")
	if err != nil {
		t.Fatalf("DecodeAs() error = %v", err)
	}
	raw, ok := v.Raw.(*xdr.SignedTimeSlicedSurveyStopCollectingMessage)
	if !ok {
		t.Fatalf("Raw is %T, want *xdr.SignedTimeSlicedSurveyStopCollectingMessage", v.Raw)
	}
	if raw.StopCollecting.Nonce != 7 || raw.StopCollecting.LedgerNum != 55278432 {
		t.Errorf("StopCollecting = %+v, want nonce 7 for ledger 55278432", raw.StopCollecting)
	}

	cands, err := lens.Detect(loadFixture(t, "survey_stop"))
	if err != nil {
		t.Fatalf("Detect() error = %v", err)
	}
	// The start encoding is byte-identical, so it must outrank stop.
	if len(cands) < 2 || cands[0].Type != "SignedTimeSlicedSurveyStartCollectingMessage" {
		t.Fatalf("Detect() best = %v, want the start message to outrank stop", cands)
	}
	stopRanked := false
	for _, c := range cands {
		if c.Type == "SignedTimeSlicedSurveyStopCollectingMessage" {
			stopRanked = true
		}
	}
	if !stopRanked {
		t.Error("Detect() did not offer the stop message as a candidate")
	}
}

func TestDecodeSurveyRequestDetection(t *testing.T) {
	t.Parallel()

	cands, err := lens.Detect(loadFixture(t, "survey_request"))
	if err != nil {
		t.Fatalf("Detect() error = %v", err)
	}
	if len(cands) == 0 || cands[0].Type != "SignedTimeSlicedSurveyRequestMessage" {
		t.Errorf("Detect() best = %v, want the signed request", cands)
	}
}

func TestSurveyMessagesRejectTruncatedInput(t *testing.T) {
	t.Parallel()

	tests := []struct {
		fixture  string
		typeName string
	}{
		{"survey_request", "SignedTimeSlicedSurveyRequestMessage"},
		{"survey_response", "SignedTimeSlicedSurveyResponseMessage"},
		{"survey_start", "SignedTimeSlicedSurveyStartCollectingMessage"},
		{"survey_stop", "SignedTimeSlicedSurveyStopCollectingMessage"},
	}
	for _, tc := range tests {
		t.Run(tc.fixture, func(t *testing.T) {
			t.Parallel()

			if _, err := lens.DecodeAs(loadFixtureRaw(tc.fixture)[:32], tc.typeName); err == nil {
				t.Errorf("DecodeAs(%s truncated) error = nil, want an error", tc.fixture)
			}
		})
	}
}
