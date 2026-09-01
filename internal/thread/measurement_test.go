package thread

import (
	"strings"
	"testing"

	"github.com/pbleisch/kept/internal/ledger"
)

const anAddress = "sha256:" + "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"

// TestAMeasurementNamesASideAnInstrumentAndAnAddress breaks each of the three
// on purpose.
//
// A receipt is append-only and its entries are the only record that these two
// measurements were the sides of this session, so a record missing one of them
// is permanent and unrepairable. The write is the one moment it can be
// prevented.
func TestAMeasurementNamesASideAnInstrumentAndAnAddress(t *testing.T) {
	whole := Measurement{Side: Before, Name: "tests", Address: anAddress}
	if why := measurementEntryWhy(whole); why != "" {
		t.Fatalf("a whole measurement is refused: %s", why)
	}

	for _, broken := range []struct {
		what string
		it   Measurement
		says string
	}{
		{"no side", Measurement{Name: "tests", Address: anAddress}, "side"},
		{"a side nothing knows", Measurement{Side: "during", Name: "tests", Address: anAddress}, "side"},
		{"no instrument", Measurement{Side: After, Address: anAddress}, "instrument"},
		{"no address", Measurement{Side: After, Name: "tests"}, "measurement"},
		{"an address that is not one", Measurement{Side: After, Name: "tests", Address: "tests"}, "measurement"},
	} {
		why := measurementEntryWhy(broken.it)
		if why == "" {
			t.Errorf("a measurement with %s is accepted", broken.what)
			continue
		}
		if !strings.Contains(why, broken.says) {
			t.Errorf("a measurement with %s is refused as %q", broken.what, why)
		}
	}
}

// TestAMeasurementIsRefusedOnTheWayBackInToo. The guard is asked at the write
// and again at the read, from one function, so a line that reached the stream
// some other way cannot be folded as a measurement it is not.
func TestAMeasurementIsRefusedOnTheWayBackInToo(t *testing.T) {
	entry := Entry{Thread: "t", Measurement: &Measurement{Side: "during", Name: "tests", Address: anAddress}}
	if why := entryWhy(ledger.ThreadMeasurement, entry); why == "" {
		t.Fatal("a measurement naming no side of a session is read back as one")
	}
	entry.Measurement.Side = After
	if why := entryWhy(ledger.ThreadMeasurement, entry); why != "" {
		t.Fatalf("a whole measurement is quarantined: %s", why)
	}
}

// TestMeasurementsReturnsEveryOneAndNotTheLast.
//
// The opposite rule from [LatestAnchor] and [LatestModel], and deliberately: a
// promise with two instruments produces two entries per side, and folding them
// to one would drop the half that says what the repair must not have broken.
func TestMeasurementsReturnsEveryOneAndNotTheLast(t *testing.T) {
	entries := []Entry{
		{Envelope: ledger.Envelope{Kind: ledger.ThreadMeasurement}, Measurement: &Measurement{Side: Before, Name: "graph", Address: anAddress}},
		{Envelope: ledger.Envelope{Kind: ledger.ThreadMeasurement}, Measurement: &Measurement{Side: Before, Name: "names", Address: anAddress}},
		{Envelope: ledger.Envelope{Kind: ledger.ThreadLifecycle}, Lifecycle: &Lifecycle{Event: Started}},
		{Envelope: ledger.Envelope{Kind: ledger.ThreadMeasurement}, Measurement: &Measurement{Side: After, Name: "graph", Address: anAddress}},
	}
	got := Measurements(entries)
	if len(got) != 3 {
		t.Fatalf("a receipt with three measurements yields %d", len(got))
	}
	if got[0].Name != "graph" || got[1].Name != "names" || got[2].Side != After {
		t.Errorf("the measurements come back as %v", got)
	}
}

// TestAMeasurementIsNotARuntimesToWrite. An adapter reports what it did; what
// the promise measures is the parent's, and a receipt that let a runtime file a
// measurement would be the one place a session could reach the verdict.
func TestAMeasurementIsNotARuntimesToWrite(t *testing.T) {
	var adapter AdapterReceipt = adapterReceipt{}
	if _, isParent := adapter.(interface {
		RecordMeasurement(Measurement) error
	}); isParent {
		t.Fatal("an adapter can file a measurement, and policy:the-agents-report-is-never-an-input-to-the-verdict says it may not")
	}
}
