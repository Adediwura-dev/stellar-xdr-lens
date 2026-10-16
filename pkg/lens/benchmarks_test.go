package lens_test

import (
	"errors"
	"fmt"
	"os"
	"testing"

	"github.com/stellar/go-stellar-sdk/xdr"

	"github.com/stellar-optics/stellar-xdr-lens/pkg/lens"
)

type decodeFixtureSpec struct {
	name     string
	fixture  string
	typeName string
}

type decodeFixture struct {
	decodeFixtureSpec
	payload string
}

var benchmarkDecodedValue *lens.Value

var decodeFixtureSpecs = []decodeFixtureSpec{
	{name: "TransactionEnvelope", fixture: "tx_success.env", typeName: "TransactionEnvelope"},
	{name: "SorobanTransactionEnvelope", fixture: "tx_soroban.env", typeName: "TransactionEnvelope"},
	{name: "TransactionResult", fixture: "tx_success.res", typeName: "TransactionResult"},
	{name: "LedgerHeader", fixture: "ledger_header", typeName: "LedgerHeader"},
	{name: "BucketEntryProtocol22", fixture: "bucket_entry_meta", typeName: "BucketEntry"},
	{name: "HotArchiveBucketEntry", fixture: "hot_archive_bucket_entry", typeName: "HotArchiveBucketEntry"},
	{name: "StellarMessage", fixture: "overlay_stellar_message", typeName: "StellarMessage"},
}

var detectFixtureSpecs = []decodeFixtureSpec{
	{name: "TransactionEnvelope", fixture: "tx_success.env", typeName: "TransactionEnvelope"},
	{name: "SorobanTransactionEnvelope", fixture: "tx_soroban.env", typeName: "TransactionEnvelope"},
	{name: "TransactionResult", fixture: "tx_success.res", typeName: "TransactionResult"},
	{name: "LedgerHeader", fixture: "ledger_header", typeName: "LedgerHeader"},
	{name: "StellarMessage", fixture: "overlay_stellar_message", typeName: "StellarMessage"},
}

func prepareDecodeFixtures(specs []decodeFixtureSpec) ([]decodeFixture, error) {
	fixtures := make([]decodeFixture, 0, len(specs))
	for _, spec := range specs {
		payload, err := readFixture(spec.fixture)
		if err != nil {
			return nil, fmt.Errorf("loading benchmark fixture %q: %w", spec.fixture, err)
		}
		if _, err := lens.DecodeAs(payload, spec.typeName); err != nil {
			return nil, fmt.Errorf("validating benchmark fixture %q as %s: %w", spec.fixture, spec.typeName, err)
		}
		fixtures = append(fixtures, decodeFixture{
			decodeFixtureSpec: spec,
			payload:           payload,
		})
	}
	return fixtures, nil
}

func ledgerHeaderPayloadForVersion(payload string, version xdr.Uint32) (string, error) {
	var header xdr.LedgerHeader
	if err := xdr.SafeUnmarshalBase64(payload, &header); err != nil {
		return "", fmt.Errorf("decoding canonical LedgerHeader fixture: %w", err)
	}
	header.LedgerVersion = version
	encoded, err := xdr.MarshalBase64(header)
	if err != nil {
		return "", fmt.Errorf("encoding LedgerHeader for protocol %d: %w", version, err)
	}
	return encoded, nil
}

func TestDecodeBenchmarkFixtures(t *testing.T) {
	fixtures, err := prepareDecodeFixtures(decodeFixtureSpecs)
	if err != nil {
		t.Fatalf("prepareDecodeFixtures() error = %v", err)
	}
	if len(fixtures) != len(decodeFixtureSpecs) {
		t.Fatalf("prepared %d fixtures, want %d", len(fixtures), len(decodeFixtureSpecs))
	}

	for _, fixture := range fixtures {
		t.Run(fixture.name, func(t *testing.T) {
			t.Parallel()
			value, err := lens.DecodeAs(fixture.payload, fixture.typeName)
			if err != nil {
				t.Fatalf("DecodeAs() error = %v", err)
			}
			if value.Type != fixture.typeName {
				t.Errorf("DecodeAs() type = %q, want %q", value.Type, fixture.typeName)
			}
		})
	}
}

func TestPrepareDecodeFixturesReportsFailures(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		spec decodeFixtureSpec
		want func(error) bool
	}{
		{
			name: "missing fixture",
			spec: decodeFixtureSpec{name: "missing", fixture: "not_a_fixture", typeName: "LedgerHeader"},
			want: func(err error) bool {
				var pathErr *os.PathError
				return errors.As(err, &pathErr)
			},
		},
		{
			name: "unknown XDR type",
			spec: decodeFixtureSpec{name: "unknown type", fixture: "ledger_header", typeName: "NotAnXdrType"},
			want: func(err error) bool {
				return errors.Is(err, lens.ErrUnknownType)
			},
		},
		{
			name: "fixture does not match type",
			spec: decodeFixtureSpec{name: "wrong type", fixture: "ledger_header", typeName: "TransactionEnvelope"},
			want: func(err error) bool {
				return errors.Is(err, lens.ErrInvalidXDR)
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			_, err := prepareDecodeFixtures([]decodeFixtureSpec{tc.spec})
			if err == nil {
				t.Fatal("prepareDecodeFixtures() error = nil, want an error")
			}
			if !tc.want(err) {
				t.Errorf("prepareDecodeFixtures() error = %v, did not match the expected failure", err)
			}
		})
	}
}

func TestProtocolVersionBenchmarkFixtures(t *testing.T) {
	t.Parallel()

	canonical, err := readFixture("ledger_header")
	if err != nil {
		t.Fatalf("reading canonical LedgerHeader fixture: %v", err)
	}
	versions := []xdr.Uint32{20, 21, 22, 23}
	for _, version := range versions {
		t.Run(fmt.Sprintf("Protocol%d", version), func(t *testing.T) {
			t.Parallel()
			payload, err := ledgerHeaderPayloadForVersion(canonical, version)
			if err != nil {
				t.Fatalf("ledgerHeaderPayloadForVersion() error = %v", err)
			}
			value, err := lens.DecodeAs(payload, "LedgerHeader")
			if err != nil {
				t.Fatalf("DecodeAs() error = %v", err)
			}
			header, ok := value.Raw.(*xdr.LedgerHeader)
			if !ok {
				t.Fatalf("decoded Raw is %T, want *xdr.LedgerHeader", value.Raw)
			}
			if header.LedgerVersion != version {
				t.Errorf("LedgerVersion = %d, want %d", header.LedgerVersion, version)
			}
		})
	}
}

func TestProtocolVersionBenchmarkRejectsMalformedFixture(t *testing.T) {
	t.Parallel()

	_, err := ledgerHeaderPayloadForVersion("not-base64", 22)
	if err == nil {
		t.Fatal("ledgerHeaderPayloadForVersion() error = nil, want an error")
	}
}

func BenchmarkDecodeCanonicalFixtures(b *testing.B) {
	fixtures, err := prepareDecodeFixtures(decodeFixtureSpecs)
	if err != nil {
		b.Fatal(err)
	}

	for _, fixture := range fixtures {
		b.Run(fixture.name, func(b *testing.B) {
			b.ReportAllocs()
			b.SetBytes(int64(len(fixture.payload)))
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				value, err := lens.DecodeAs(fixture.payload, fixture.typeName)
				if err != nil {
					b.Fatal(err)
				}
				benchmarkDecodedValue = value
			}
		})
	}
}

func BenchmarkDetectCanonicalFixtures(b *testing.B) {
	fixtures, err := prepareDecodeFixtures(detectFixtureSpecs)
	if err != nil {
		b.Fatal(err)
	}
	for _, fixture := range fixtures {
		b.Run(fixture.name, func(b *testing.B) {
			b.ReportAllocs()
			b.SetBytes(int64(len(fixture.payload)))
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				value, err := lens.Decode(fixture.payload)
				if err != nil {
					b.Fatal(err)
				}
				if value.Type != fixture.typeName {
					b.Fatalf("Decode() type = %q, want %q", value.Type, fixture.typeName)
				}
				benchmarkDecodedValue = value
			}
		})
	}
}

func BenchmarkDecodeByProtocolVersion(b *testing.B) {
	canonical, err := readFixture("ledger_header")
	if err != nil {
		b.Fatal(err)
	}
	for _, version := range []xdr.Uint32{20, 21, 22, 23} {
		payload, err := ledgerHeaderPayloadForVersion(canonical, version)
		if err != nil {
			b.Fatal(err)
		}
		b.Run(fmt.Sprintf("Protocol%d", version), func(b *testing.B) {
			b.ReportAllocs()
			b.SetBytes(int64(len(payload)))
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				value, err := lens.DecodeAs(payload, "LedgerHeader")
				if err != nil {
					b.Fatal(err)
				}
				header, ok := value.Raw.(*xdr.LedgerHeader)
				if !ok || header.LedgerVersion != version {
					b.Fatalf("decoded protocol version = %v, want %d", value.Raw, version)
				}
				benchmarkDecodedValue = value
			}
		})
	}
}
