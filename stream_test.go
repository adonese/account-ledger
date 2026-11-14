package ledger_test

import (
	"fmt"
	"reflect"
	"testing"

	ledger "github.com/adonese/account-ledger"
)

func TestStreamAccounts(t *testing.T) {
	tests := []struct {
		name string // description of this test case
		want []ledger.Account
	}{
		{"should return two accounts", []ledger.Account{{ID: "ACC-001", Opening: ledger.Money{ledger.AED, 0}}, {ID: "ACC-002", Opening: ledger.Money{ledger.BHD, 0}}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ledger.StreamAccounts()
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("StreamAccounts() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestStreamPreservesInputFacts(t *testing.T) {

	events := ledger.Stream()
	if len(events) != 10 {
		t.Fatalf("events = %d, want 10", len(events))
	}
	for i, event := range events {
		if event.ID != fmt.Sprintf("E%d", i+1) {
			t.Fatalf("arrival %d has ID %q", i, event.ID)
		}
		if event.ValueDay < ledger.FirstDay || event.ValueDay > event.BookedDay || event.BookedDay > ledger.LastDay {
			t.Fatalf("date metadata = %+v", event)
		}
	}
	if events[6].BookedDay != 5 || events[6].ValueDay != 2 || events[6].Amount != (ledger.Money{Currency: ledger.AED, Minor: 620_00}) {
		t.Fatalf("E7 = %+v", events[6])
	}
	if events[8].Reverses != "E7" || events[8].Amount != (ledger.Money{}) {
		t.Fatalf("E9 = %+v", events[8])
	}
	if events[9].BookedDay != 5 || events[9].ValueDay != 5 || events[9].Instalments != 3 || events[9].Amount != (ledger.Money{Currency: ledger.BHD, Minor: 10_000}) {
		t.Fatalf("E10 = %+v", events[9])
	}
}
