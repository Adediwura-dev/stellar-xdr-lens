package lens

import (
	"reflect"
	"testing"

	"github.com/stellar/go-stellar-sdk/xdr"
)

func TestEnrichSurveyCommandType(t *testing.T) {
	t.Parallel()

	note, raw, ok := enrichSurveyCommandType(reflect.ValueOf(xdr.SurveyMessageCommandTypeTimeSlicedSurveyTopology))
	if !ok || note != "" || raw != "time_sliced_survey_topology" {
		t.Errorf("enrichSurveyCommandType(topology) = (%q, %v, %v), want the topology label", note, raw, ok)
	}

	// An enum value outside the generated map is not a valid survey command
	// and must be left to the generic rendering.
	if _, _, ok := enrichSurveyCommandType(reflect.ValueOf(xdr.SurveyMessageCommandType(0))); ok {
		t.Error("enrichSurveyCommandType(0) ok = true, want false")
	}
	if _, _, ok := enrichSurveyCommandType(reflect.ValueOf(xdr.Uint32(1))); ok {
		t.Error("enrichSurveyCommandType(Uint32) ok = true, want false")
	}
}

func TestEnrichEncryptedBody(t *testing.T) {
	t.Parallel()

	body := xdr.EncryptedBody{0x00, 0x01, 0xff}
	note, raw, ok := enrichEncryptedBody(reflect.ValueOf(body))
	if !ok || note != "encrypted, 3 byte(s)" || raw != "0001ff" {
		t.Errorf("enrichEncryptedBody(3 bytes) = (%q, %v, %v), want hex and a 3-byte note", note, raw, ok)
	}

	note, raw, ok = enrichEncryptedBody(reflect.ValueOf(xdr.EncryptedBody{}))
	if !ok || note != "encrypted, 0 byte(s)" || raw != "" {
		t.Errorf("enrichEncryptedBody(empty) = (%q, %v, %v), want an empty-body note", note, raw, ok)
	}

	if _, _, ok := enrichEncryptedBody(reflect.ValueOf(xdr.Signature{})); ok {
		t.Error("enrichEncryptedBody(Signature) ok = true, want false")
	}
}

func TestSurveyLedgerNote(t *testing.T) {
	t.Parallel()

	note, ok := surveyLedgerNote(reflect.ValueOf(xdr.Uint32(55278432)))
	if !ok || note != "survey of ledger 55278432" {
		t.Errorf("surveyLedgerNote(55278432) = (%q, %v), want a ledger note", note, ok)
	}
	if _, ok := surveyLedgerNote(reflect.ValueOf(xdr.Int64(1))); ok {
		t.Error("surveyLedgerNote(Int64) ok = true, want false")
	}
}

// TestSurveyLedgerNoteReachesTheWalker pins the registration under the field
// name the survey structs actually use.
func TestSurveyLedgerNoteReachesTheWalker(t *testing.T) {
	t.Parallel()

	start := xdr.TimeSlicedSurveyStartCollectingMessage{LedgerNum: 42}
	node := buildNode("StartCollecting", reflect.ValueOf(start), 0)
	if node == nil {
		t.Fatal("buildNode(TimeSlicedSurveyStartCollectingMessage) returned nil")
	}
	ledger := node.Child("LedgerNum")
	if ledger == nil || ledger.Note != "survey of ledger 42" {
		t.Errorf("LedgerNum = %v, want a survey ledger note", ledger)
	}
}

// TestEnrichSurveyCommandTypeReachesTheWalker pins the type-based
// registration: the enum value inside a request must render as the topology
// label.
func TestEnrichSurveyCommandTypeReachesTheWalker(t *testing.T) {
	t.Parallel()

	request := xdr.SurveyRequestMessage{
		CommandType: xdr.SurveyMessageCommandTypeTimeSlicedSurveyTopology,
	}
	node := buildNode("Request", reflect.ValueOf(request), 0)
	if node == nil {
		t.Fatal("buildNode(SurveyRequestMessage) returned nil")
	}
	command := node.Child("CommandType")
	if command == nil || command.Value != "time_sliced_survey_topology" {
		t.Errorf("CommandType = %v, want the topology label", command)
	}
}
