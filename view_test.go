package ledger_test

import (
	"errors"
	"reflect"
	"testing"

	ledger "github.com/adonese/account-ledger"
)

func aed(minor int64) ledger.Money { return ledger.Money{Currency: ledger.AED, Minor: minor} }
func bhd(minor int64) ledger.Money { return ledger.Money{Currency: ledger.BHD, Minor: minor} }
func newTestLedger(t *testing.T) *ledger.Ledger {
	t.Helper()
	l, err := ledger.New(ledger.StreamAccounts())
	if err != nil {
		t.Fatal(err)
	}
	return l
}

func accountEvent(id string, day ledger.Day, kind ledger.EventKind, amount ledger.Money) ledger.Event {
	return ledger.Event{ID: id, BookedDay: day, ValueDay: day, Kind: kind, AccountID: "ACC-001", Amount: amount}
}
func applyEvents(t *testing.T, l *ledger.Ledger, events ...ledger.Event) {
	t.Helper()
	for _, event := range events {
		if err := l.Apply(event); err != nil {
			t.Fatalf("apply %s: %v", event.ID, err)
		}
	}
}

func TestNewLedgerOwnsItsAccounts(t *testing.T) {
	opening := ledger.Money{Currency: ledger.BHD, Minor: 10_000}
	accounts := []ledger.Account{{ID: "savings", Opening: opening}}

	l, err := ledger.New(accounts)
	if err != nil {
		t.Fatal(err)
	}

	// Change the original input.
	accounts[0].Opening.Minor = 1

	// Change the copy returned by the ledger.
	returned := l.Accounts()
	if len(returned) != 1 {
		t.Fatalf("got %d accounts, want 1", len(returned))
	}
	returned[0].Opening.Minor = 2

	// The ledger must still have the original opening balance.
	balance := l.Balance("savings", 1)
	if balance != opening {
		t.Fatalf("balance = %v, want %v", balance, opening)
	}
	if len(l.Journal()) != 0 {
		t.Fatal("new ledger contains event records")
	}
}

func TestNewLedgerRejectsInvalidAccounts(t *testing.T) {
	cases := [][]ledger.Account{
		{{ID: "", Opening: ledger.Money{Currency: ledger.AED}}},
		{{ID: "duplicate", Opening: ledger.Money{Currency: ledger.AED}}, {ID: "duplicate", Opening: ledger.Money{Currency: ledger.AED}}},
		{{ID: "unsupported", Opening: ledger.Money{Currency: "USD"}}},
	}
	for _, accounts := range cases {
		if _, err := ledger.New(accounts); err == nil {
			t.Errorf("accepted accounts: %+v", accounts)
		}
	}
}

func TestBeforeSelectsJournalPrefix(t *testing.T) {
	l := newTestLedger(t)
	applyEvents(t, l,
		accountEvent("fund", 1, ledger.Credit, aed(100_00)),
	)

	// Sequence 1 is the input; sequence 2 is its posting.
	for _, tc := range []struct {
		seq     int
		records int
		balance ledger.Money
	}{
		{1, 0, aed(0)},      // Before the input: empty journal.
		{2, 1, aed(0)},      // Input only: no money has moved.
		{3, 2, aed(100_00)}, // Input and posting: credit is visible.
	} {
		view := l.Before(tc.seq)

		if got := len(view.Journal()); got != tc.records {
			t.Fatalf("Before(%d): %d records, want %d",
				tc.seq, got, tc.records)
		}
		if got := view.Balance("ACC-001", 1); got != tc.balance {
			t.Fatalf("Before(%d): balance %v, want %v",
				tc.seq, got, tc.balance)
		}
	}
}

func TestApplyClosesDaysBeforeLaterInput(t *testing.T) {
	l := newTestLedger(t)
	applyEvents(t, l,
		accountEvent("day-three", 3, ledger.Credit, aed(10_00)),
	)

	records := l.Journal()
	if len(records) != 4 {
		t.Fatalf("journal length = %d, want 4", len(records))
	}

	// Days 1 and 2 close before the Day 3 input and posting.
	for i, day := range []ledger.Day{1, 2} {
		r := records[i]
		if r.Kind != ledger.DayClosed || r.Day != day || r.ValueDay != day {
			t.Fatalf("close record = %+v, want Day %d", r, day)
		}
	}

	if records[2].Kind != ledger.Input || records[2].Day != 3 ||
		records[3].Kind != ledger.Posting || records[3].Day != 3 {
		t.Fatalf("Day 3 records = %+v", records[2:])
	}

	if got := l.AtClose(1).Journal(); !reflect.DeepEqual(got, records[:1]) {
		t.Fatalf("Day 1 close = %+v", got)
	}
	if got := l.AtClose(2).Journal(); !reflect.DeepEqual(got, records[:2]) {
		t.Fatalf("Day 2 close = %+v", got)
	}

	// Day 3 is still open, so AtClose returns the current view.
	if got := l.AtClose(3).Journal(); !reflect.DeepEqual(got, records) {
		t.Fatalf("open Day 3 view = %+v", got)
	}
}

func TestValueDateAndKnowledgeCutoffAreDifferent(t *testing.T) {
	l := newTestLedger(t)
	applyEvents(t, l, accountEvent("fund", 1, ledger.Credit, aed(100_00)), accountEvent("later-credit", 3, ledger.Credit, aed(50_00)))
	original := l.AtClose(1)
	correction := accountEvent("late-debit", 3, ledger.Debit, aed(20_00))
	correction.ValueDay = 1
	applyEvents(t, l, correction)
	if original.Balance("ACC-001", 1) != aed(100_00) || l.AtClose(1).Balance("ACC-001", 1) != aed(100_00) {
		t.Fatal("original close changed")
	}
	if got := l.Balance("ACC-001", 1); got != aed(80_00) {
		t.Fatalf("restated Day 1 = %v", got)
	}
	for _, record := range l.Journal() {
		if record.Kind == ledger.Posting && record.EventID == correction.ID {
			if got := l.Before(record.Seq).Balance("ACC-001", 1); got != aed(100_00) {
				t.Fatalf("balance before posting = %v", got)
			}
		}
	}
}

func TestLateBookedDayDoesNotRewindProcessingTime(t *testing.T) {
	l := newTestLedger(t)
	applyEvents(t, l, accountEvent("fund", 1, ledger.Credit, aed(100_00)), accountEvent("day-six-credit", 6, ledger.Credit, aed(1_00)))
	late := accountEvent("late-credit", 5, ledger.Credit, aed(10_00))
	applyEvents(t, l, late)
	found := false
	for _, record := range l.Journal() {
		if record.Kind == ledger.Input && record.EventID == late.ID {
			found = true
			if record.Day != 6 || record.Event.BookedDay != 5 {
				t.Fatalf("late arrival = %+v", record)
			}
		}
	}
	if !found {
		t.Fatal("late input is missing")
	}
}

func TestCloseIsIdempotentAndRejectsLaterEvents(t *testing.T) {
	l := newTestLedger(t)
	l.Close()
	first := l.Journal()
	l.Close()
	if !reflect.DeepEqual(first, l.Journal()) {
		t.Fatal("repeated close appended records")
	}
	closed := map[ledger.Day]int{}
	for _, record := range first {
		if record.Kind == ledger.DayClosed {
			closed[record.ValueDay]++
		}
	}
	for day := ledger.FirstDay; day <= ledger.LastDay; day++ {
		if closed[day] != 1 {
			t.Fatalf("Day %d closed %d times", day, closed[day])
		}
	}
	if err := l.Apply(accountEvent("too-late", 6, ledger.Credit, aed(1))); !errors.Is(err, ledger.ErrClosed) {
		t.Fatalf("closed error = %v", err)
	}
	if l.Balance("ACC-001", 6) != aed(0) {
		t.Fatal("closed ledger accepted money")
	}
}
