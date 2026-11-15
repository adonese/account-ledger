package ledger_test

import (
	"bytes"
	"errors"
	"reflect"
	"strings"
	"testing"

	ledger "github.com/adonese/account-ledger"
)

func TestReplayPreservesArrivalOrderAndClosesWindow(t *testing.T) {
	events := ledger.Stream()
	l, err := ledger.Replay(ledger.StreamAccounts(), events)
	if err != nil {
		t.Fatal(err)
	}
	var inputs []ledger.Event
	closes := map[ledger.Day]int{}
	var rejected []string
	for _, record := range l.Journal() {
		if record.Kind == ledger.Input {
			inputs = append(inputs, record.Event)
			if record.Rejected != "" {
				rejected = append(rejected, record.EventID)
			}
			if record.EventID == "E10" && record.Day != 6 {
				t.Fatalf("E10 processing day = %d", record.Day)
			}
		}
		if record.Kind == ledger.DayClosed {
			closes[record.ValueDay]++
		}
	}
	if !reflect.DeepEqual(inputs, events) || !reflect.DeepEqual(events, ledger.Stream()) {
		t.Fatal("replay changed or reordered input facts")
	}
	again, err := ledger.Replay(ledger.StreamAccounts(), events)
	if err != nil || !reflect.DeepEqual(l.Journal(), again.Journal()) {
		t.Fatalf("same inputs produced different history: %v", err)
	}
	if !reflect.DeepEqual(rejected, []string{"E8"}) {
		t.Fatalf("rejections = %v", rejected)
	}
	for day := ledger.FirstDay; day <= ledger.LastDay; day++ {
		if closes[day] != 1 {
			t.Fatalf("Day %d closed %d times", day, closes[day])
		}
	}
	if l.Balance("ACC-001", 6) != aed(210_69) || l.Balance("ACC-002", 6) != bhd(10_008) {
		t.Fatal("final balances do not match the supplied stream")
	}
	if err := l.Apply(accountEvent("after-replay", 6, ledger.Credit, aed(1))); !errors.Is(err, ledger.ErrClosed) {
		t.Fatalf("replay did not close the window: %v", err)
	}
}

func TestReplayAcceptsCustomAccountsAndContinuesAfterRejection(t *testing.T) {
	accounts := []ledger.Account{{ID: "savings", Opening: bhd(1_000)}}
	events := []ledger.Event{
		{ID: "credit", AccountID: "savings", Kind: ledger.Credit, BookedDay: 1, ValueDay: 1, Amount: bhd(2_000)},
		{ID: "wrong-currency", AccountID: "savings", Kind: ledger.Debit, BookedDay: 1, ValueDay: 1, Amount: aed(10_00)},
		{ID: "debit", AccountID: "savings", Kind: ledger.Debit, BookedDay: 2, ValueDay: 2, Amount: bhd(500)},
	}
	l, err := ledger.Replay(accounts, events)
	if err != nil {
		t.Fatal(err)
	}
	var inputs []ledger.Event
	for _, record := range l.Journal() {
		if record.Kind == ledger.Input {
			inputs = append(inputs, record.Event)
			if (record.Rejected != "") != (record.EventID == "wrong-currency") {
				t.Fatalf("unexpected input outcome: %+v", record)
			}
		}
	}
	if !reflect.DeepEqual(inputs, events) || l.Balance("savings", 6) != bhd(2_506) {
		t.Fatal("custom replay lost inputs or stopped at a business rejection")
	}
}

func TestReplayReturnsAccountSetupErrors(t *testing.T) {
	for _, accounts := range [][]ledger.Account{
		{{ID: "invalid", Opening: ledger.Money{Currency: "USD"}}},
		{{ID: "", Opening: aed(0)}},
		{{ID: "same", Opening: aed(0)}, {ID: "same", Opening: aed(0)}},
	} {
		if l, err := ledger.Replay(accounts, nil); err == nil || l != nil {
			t.Fatalf("invalid accounts returned ledger %v, error %v", l, err)
		}
	}
}

func TestReportShowsOriginalClosesRestatementsAndInterest(t *testing.T) {
	l, err := ledger.Replay(ledger.StreamAccounts(), ledger.Stream())
	if err != nil {
		t.Fatal(err)
	}
	before := l.Journal()
	var first, second bytes.Buffer
	l.Report(&first)
	l.Report(&second)
	if first.String() != second.String() || !reflect.DeepEqual(before, l.Journal()) {
		t.Fatal("report changed state or produced nondeterministic output")
	}
	report := first.String()
	dayBlock := func(day string) string {
		t.Helper()
		_, tail, found := strings.Cut(report, "Day "+day+"\n")
		if !found {
			t.Fatalf("missing Day %s", day)
		}
		block, _, _ := strings.Cut(tail, "\n\n")
		return block
	}
	for _, day := range []string{"1", "2", "3", "4", "5", "6"} {
		for _, label := range []string{"closing ledger balance", "fee assessments", "authorization states", "errors", "ACC-001", "ACC-002"} {
			if !strings.Contains(dayBlock(day), label) {
				t.Errorf("Day %s missing %q", day, label)
			}
		}
	}
	for _, tc := range []struct{ day, text string }{
		{"2", "ACC-001  AED 250.00, available AED 50.00"},
		{"2", "Auth-A  ACTIVE"},
		{"4", "E6 warning: Auth-Z"},
		{"4", "Auth-A  SETTLED for AED 185.00"},
		{"5", "ACC-001  AED -410.00"},
		{"5", "E8 rejected:"},
		{"6", "ACC-001  AED 210.69"},
		{"6", "ACC-002  BHD 10.008"},
		{"6", "booked Day 5, posted as 3.334 + 3.333 + 3.333"},
	} {
		if !strings.Contains(dayBlock(tc.day), tc.text) {
			t.Errorf("Day %s missing %q", tc.day, tc.text)
		}
	}
	if strings.Contains(dayBlock("5"), "E10") || strings.Index(dayBlock("6"), "E9") > strings.Index(dayBlock("6"), "E10") {
		t.Fatal("report moved the late E10 input ahead of E9")
	}
	for _, text := range []string{
		"ACC-001 Day 2: at close AED 250.00; restated AED 225.00",
		"ACC-002 Day 5: at close BHD 0.000; restated BHD 10.000",
		"AED 0.69 accrued, AED 0.69 credited",
		"BHD 0.008 accrued, BHD 0.008 credited",
	} {
		if !strings.Contains(report, text) {
			t.Errorf("report missing %q", text)
		}
	}
}

func TestReportUsesCustomAccountsAndDistinguishesRejectedDuplicate(t *testing.T) {
	event := ledger.Event{ID: "transfer", AccountID: "wallet", Kind: ledger.Credit,
		BookedDay: 1, ValueDay: 1, Amount: bhd(10_000), Instalments: 3}
	l, err := ledger.Replay([]ledger.Account{{ID: "wallet", Opening: bhd(0)}}, []ledger.Event{event, event})
	if err != nil {
		t.Fatal(err)
	}
	var output bytes.Buffer
	l.Report(&output)
	report := output.String()
	if !strings.Contains(report, "wallet") || strings.Contains(report, "ACC-001") ||
		strings.Count(report, "posted as 3.334 + 3.333 + 3.333") != 1 {
		t.Fatal("report used hard-coded accounts or attributed postings to a duplicate")
	}
	for _, line := range strings.Split(report, "\n") {
		if strings.Contains(line, "REJECTED") && strings.Contains(line, "posted as") {
			t.Fatalf("rejected input claims postings: %s", line)
		}
	}
}
