package lens

import (
	"reflect"
	"testing"

	"github.com/stellar/go-stellar-sdk/xdr"
)

// TestTypeDocsCoversRegistry keeps the curated descriptions exactly in step
// with the decode registry: a type without docs fails, and docs for a type
// that no longer exists fail too.
func TestTypeDocsCoversRegistry(t *testing.T) {
	t.Parallel()

	registered := map[string]bool{}
	for _, entry := range registry {
		registered[entry.name] = true
		if _, ok := typeDocs[entry.name]; !ok {
			t.Errorf("type %s is registered but has no typeDocs entry", entry.name)
		}
	}
	for name := range typeDocs {
		if !registered[name] {
			t.Errorf("typeDocs documents %s, which is not registered", name)
		}
	}
}

// TestRegistryIndexMatchesRegistryPriorities pins the bridge between the two
// data structures: the index must publish the detection priorities the
// registry actually uses, not a copy that can drift.
func TestRegistryIndexMatchesRegistryPriorities(t *testing.T) {
	t.Parallel()

	priority := map[string]int{}
	for _, entry := range registry {
		priority[entry.name] = entry.priority
	}
	for _, info := range RegistryIndex() {
		if priority[info.Name] != info.Priority {
			t.Errorf("%s: index priority %d, registry priority %d", info.Name, info.Priority, priority[info.Name])
		}
	}
}

func TestMinWireSizeSpotChecks(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		typ  reflect.Type
		want int
	}{
		{"Memo", reflect.TypeOf(xdr.Memo{}), 4},                   // void NONE arm
		{"Asset", reflect.TypeOf(xdr.Asset{}), 4},                 // void native arm
		{"ScVal", reflect.TypeOf(xdr.ScVal{}), 4},                 // void arm
		{"Auth", reflect.TypeOf(xdr.Auth{}), 4},                   // one int32
		{"Price", reflect.TypeOf(xdr.Price{}), 8},                 // two int32
		{"PeerAddressIp", reflect.TypeOf(xdr.PeerAddressIp{}), 8}, // discriminant + ipv4
		{"PeerAddress", reflect.TypeOf(xdr.PeerAddress{}), 16},
		{"AuthCert", reflect.TypeOf(xdr.AuthCert{}), 44}, // 32 key + 8 expiry + 4 empty sig
		{"DontHave", reflect.TypeOf(xdr.DontHave{}), 36}, // 4 type + 32 hash
		{"Hello", reflect.TypeOf(xdr.Hello{}), 164},
		{"StellarMessage", reflect.TypeOf(xdr.StellarMessage{}), 8},
		{"TransactionSet", reflect.TypeOf(xdr.TransactionSet{}), 36},     // 32 hash + empty list
		{"TransactionSetV1", reflect.TypeOf(xdr.TransactionSetV1{}), 36}, // 32 hash + empty phases
		{"GeneralizedTransactionSet", reflect.TypeOf(xdr.GeneralizedTransactionSet{}), 40},
		{"TransactionHistoryEntry", reflect.TypeOf(xdr.TransactionHistoryEntry{}), 44},
		{"TransactionHistoryResultEntry", reflect.TypeOf(xdr.TransactionHistoryResultEntry{}), 12},
		{"LedgerCloseMetaBatch", reflect.TypeOf(xdr.LedgerCloseMetaBatch{}), 12}, // two uint32 + empty list
		{"EvictionIterator", reflect.TypeOf(xdr.EvictionIterator{}), 16},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			if got := minWireSize(tc.typ); got != tc.want {
				t.Errorf("minWireSize(%s) = %d, want %d", tc.name, got, tc.want)
			}
		})
	}
}

func TestMinWireSizeBasicKinds(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		typ  reflect.Type
		want int
	}{
		{"int32", reflect.TypeOf(int32(0)), 4},
		{"uint64", reflect.TypeOf(uint64(0)), 8},
		{"bool", reflect.TypeOf(false), 4},
		{"string", reflect.TypeOf(""), 4},
		{"byte slice", reflect.TypeOf([]byte{}), 4},
		{"byte array", reflect.TypeOf([3]byte{}), 3},
		{"word array", reflect.TypeOf([2]uint32{}), 8},
		{"optional uint32", reflect.TypeOf((*uint32)(nil)), 8},
		{"enum", reflect.TypeOf(xdr.EnvelopeType(0)), 4},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			if got := minWireSize(tc.typ); got != tc.want {
				t.Errorf("minWireSize(%s) = %d, want %d", tc.name, got, tc.want)
			}
		})
	}
}

func TestMinUnionArmSizeVoidArm(t *testing.T) {
	t.Parallel()

	// Asset's native arm carries no payload, so the whole union is just its
	// discriminant.
	if got := minUnionArmSize(reflect.TypeOf(xdr.Asset{}), 0); got != 0 {
		t.Errorf("minUnionArmSize(Asset) = %d, want 0 for the void native arm", got)
	}
	if got := minUnionArmSize(reflect.TypeOf(xdr.PeerAddressIp{}), 0); got != 4 {
		t.Errorf("minUnionArmSize(PeerAddressIp) = %d, want 4 for opaque ipv4", got)
	}
}

func TestHasUnionMethods(t *testing.T) {
	t.Parallel()

	if !hasUnionMethods(reflect.TypeOf(xdr.TransactionEnvelope{})) {
		t.Error("hasUnionMethods(TransactionEnvelope) = false, want true")
	}
	if hasUnionMethods(reflect.TypeOf(xdr.Hello{})) {
		t.Error("hasUnionMethods(Hello) = true, want false")
	}
}

// TestMinWireSizeDepthCap uses a self-referential type to prove the walk is
// bounded: without the depth cap this test would blow the stack.
func TestMinWireSizeDepthCap(t *testing.T) {
	t.Parallel()

	type recursive struct {
		Next *recursive
		Data uint32
	}
	if got := minWireSize(reflect.TypeOf(recursive{})); got < 4 {
		t.Errorf("minWireSize(recursive) = %d, want at least the uint32 field", got)
	}
}
