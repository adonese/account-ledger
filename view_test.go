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

func TestAuthorizationReservesAvailabilityWithoutMovingMoney(t *testing.T) {
	l := newTestLedger(t)
	applyEvents(t, l, accountEvent("fund", 1, ledger.Credit, aed(250_00)))

	hold := accountEvent("reserve", 2, ledger.Authorization, aed(200_00))
	hold.AuthID = "purchase-a"
	applyEvents(t, l, hold)

	if got := l.Balance("ACC-001", 2); got != aed(250_00) {
		t.Fatalf("balance = %v", got)
	}
	if got := l.Holds("ACC-001"); got != aed(200_00) {
		t.Fatalf("holds = %v", got)
	}
	if got := l.Available("ACC-001", 2); got != aed(50_00) {
		t.Fatalf("available = %v", got)
	}

	// A value-day query alone does not rewind authorization knowledge.
	if got := l.Available("ACC-001", 1); got != aed(50_00) {
		t.Fatalf("current view of Day 1 = %v", got)
	}
	if got := l.AtClose(1).Available("ACC-001", 1); got != aed(250_00) {
		t.Fatalf("original Day 1 close = %v", got)
	}

	auths := l.Auths("ACC-001")
	if len(auths) != 1 || auths[0].Status != ledger.Active {
		t.Fatalf("authorizations = %+v", auths)
	}

	auths[0].Status = ledger.Settled
	auths[0].Hold.Minor = 1
	if l.Holds("ACC-001") != aed(200_00) {
		t.Fatal("editing query results changed the stored hold")
	}
}

func TestAuthorizationAllowsExactlyZeroAndKeepsDeclines(t *testing.T) {
	l := newTestLedger(t)
	applyEvents(t, l, accountEvent("fund", 1, ledger.Credit, aed(100_00)))

	exact := accountEvent("reserve-all", 1, ledger.Authorization, aed(100_00))
	exact.AuthID = "purchase-a"
	applyEvents(t, l, exact)

	if got := l.Available("ACC-001", 1); got != aed(0) {
		t.Fatalf("remaining available = %v", got)
	}

	// One minor unit beyond the available amount must be declined.
	over := accountEvent("reserve-more", 1, ledger.Authorization, aed(1))
	over.AuthID = "purchase-b"

	before := len(l.Journal())
	if err := l.Apply(over); !errors.Is(err, ledger.ErrDeclined) {
		t.Fatalf("decline error = %v", err)
	}

	added := l.Journal()[before:]
	if len(added) != 2 || added[0].Kind != ledger.Input ||
		added[0].Rejected == "" || added[1].Kind != ledger.AuthState ||
		added[1].Auth.Status != ledger.Declined {
		t.Fatalf("decline records = %+v", added)
	}

	// Later funds must not change the earlier decision.
	applyEvents(t, l, accountEvent("later-funds", 1, ledger.Credit, aed(10_00)))

	want := []ledger.Auth{
		{ID: "purchase-a", Status: ledger.Active, Hold: aed(100_00)},
		{ID: "purchase-b", Status: ledger.Declined, Hold: aed(1)},
	}
	if got := l.Auths("ACC-001"); !reflect.DeepEqual(got, want) {
		t.Fatalf("authorizations = %+v, want %+v", got, want)
	}

	if l.Balance("ACC-001", 1) != aed(110_00) ||
		l.Holds("ACC-001") != aed(100_00) ||
		l.Available("ACC-001", 1) != aed(10_00) {
		t.Fatal("decline changed money or reserved funds")
	}
}

func TestAuthorizationRequiresUniqueIDWithinAccount(t *testing.T) {
	l := newTestLedger(t)
	applyEvents(t, l, accountEvent("fund", 1, ledger.Credit, aed(100_00)))

	first := accountEvent("reserve-first", 1, ledger.Authorization, aed(10_00))
	first.AuthID = "purchase-a"
	applyEvents(t, l, first)
	before := l.Auths("ACC-001")

	for _, tc := range []struct {
		name   string
		authID string
		want   error
	}{
		{"missing", "", ledger.ErrInvalid},
		{"duplicate", "purchase-a", ledger.ErrDuplicate},
	} {
		t.Run(tc.name, func(t *testing.T) {
			// Fresh event IDs ensure we test AuthID validation.
			event := accountEvent(tc.name, 1, ledger.Authorization, aed(1))
			event.AuthID = tc.authID

			if err := l.Apply(event); !errors.Is(err, tc.want) {
				t.Fatalf("error = %v, want %v", err, tc.want)
			}
			if got := l.Auths("ACC-001"); !reflect.DeepEqual(got, before) {
				t.Fatalf("rejection changed authorizations: %+v", got)
			}
		})
	}
}

func TestAuthorizationIDsAreScopedToAccount(t *testing.T) {
	l := newTestLedger(t)
	applyEvents(t, l, accountEvent("fund-aed", 1, ledger.Credit, aed(100_00)))

	fund := accountEvent("fund-bhd", 1, ledger.Credit, bhd(20_000))
	fund.AccountID = "ACC-002"
	applyEvents(t, l, fund)

	first := accountEvent("hold-aed", 1, ledger.Authorization, aed(10_00))
	first.AuthID = "shared-id"

	second := accountEvent("hold-bhd", 1, ledger.Authorization, bhd(5_000))
	second.AccountID, second.AuthID = "ACC-002", "shared-id"
	applyEvents(t, l, first, second)

	if l.Holds("ACC-001") != aed(10_00) ||
		l.Holds("ACC-002") != bhd(5_000) {
		t.Fatal("holds leaked across accounts")
	}
	if l.Available("ACC-001", 1) != aed(90_00) ||
		l.Available("ACC-002", 1) != bhd(15_000) {
		t.Fatal("wrong available balance or currency")
	}
}

func TestSettlementReleasesWholeHoldAndRejectsDuplicates(t *testing.T) {
	for _, tc := range []struct {
		name    string
		paid    ledger.Money
		balance ledger.Money
	}{
		{"less than hold", aed(185_00), aed(315_00)},
		{"equal to hold", aed(200_00), aed(300_00)},
		{"exceeds hold and funds", aed(600_00), aed(-100_00)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			l := newTestLedger(t)
			applyEvents(t, l,
				accountEvent("fund", 1, ledger.Credit, aed(500_00)),
			)

			hold := accountEvent("reserve", 1, ledger.Authorization, aed(200_00))
			hold.AuthID = "purchase-a"
			applyEvents(t, l, hold)
			before := l.Journal()

			event := accountEvent("capture", 1, ledger.Settlement, tc.paid)
			event.AuthID = hold.AuthID
			applyEvents(t, l, event)

			if got := l.Balance("ACC-001", 1); got != tc.balance {
				t.Fatalf("balance = %v, want %v", got, tc.balance)
			}
			if l.Holds("ACC-001") != aed(0) ||
				l.Available("ACC-001", 1) != tc.balance {
				t.Fatal("settlement did not release the whole hold")
			}

			want := []ledger.Auth{{
				ID:     hold.AuthID,
				Status: ledger.Settled,
				Hold:   aed(200_00),
				Paid:   tc.paid,
			}}
			if got := l.Auths("ACC-001"); !reflect.DeepEqual(got, want) {
				t.Fatalf("authorization = %+v, want %+v", got, want)
			}
			if !reflect.DeepEqual(l.Journal()[:len(before)], before) {
				t.Fatal("settlement rewrote earlier journal records")
			}

			// A fresh event ID cannot settle the same authorization twice.
			event.ID = "duplicate-capture"
			size := len(l.Journal())

			if err := l.Apply(event); !errors.Is(err, ledger.ErrDuplicate) {
				t.Fatalf("duplicate error = %v", err)
			}
			added := l.Journal()[size:]
			if len(added) != 1 || added[0].Kind != ledger.Input ||
				added[0].Rejected == "" {
				t.Fatalf("duplicate records = %+v", added)
			}
			if l.Balance("ACC-001", 1) != tc.balance {
				t.Fatal("duplicate settlement moved money")
			}
		})
	}
}

func TestSettlementWithoutActiveAuthorizationPostsAsUnmatched(t *testing.T) {
	for _, prior := range []string{"missing", "declined"} {
		t.Run(prior, func(t *testing.T) {
			l := newTestLedger(t)
			applyEvents(t, l,
				accountEvent("fund", 1, ledger.Credit, aed(500_00)),
			)

			if prior == "declined" {
				hold := accountEvent("reserve", 1, ledger.Authorization, aed(600_00))
				hold.AuthID = "purchase-a"

				if err := l.Apply(hold); !errors.Is(err, ledger.ErrDeclined) {
					t.Fatalf("authorization error = %v", err)
				}
			}

			event := accountEvent("capture", 1, ledger.Settlement, aed(180_00))
			event.AuthID = "purchase-a"
			applyEvents(t, l, event)

			if l.Balance("ACC-001", 1) != aed(320_00) ||
				l.Holds("ACC-001") != aed(0) ||
				l.Available("ACC-001", 1) != aed(320_00) {
				t.Fatal("unmatched settlement has wrong financial effect")
			}

			auths := l.Auths("ACC-001")
			if len(auths) != 1 || auths[0].Status != ledger.Settled ||
				!auths[0].Unmatched || auths[0].Paid != aed(180_00) {
				t.Fatalf("authorization = %+v", auths)
			}
		})
	}
}

func TestSettlementRequiresAuthorizationID(t *testing.T) {
	l := newTestLedger(t)
	applyEvents(t, l,
		accountEvent("fund", 1, ledger.Credit, aed(500_00)),
	)
	event := accountEvent("capture", 1, ledger.Settlement, aed(180_00))

	if err := l.Apply(event); !errors.Is(err, ledger.ErrInvalid) {
		t.Fatalf("error = %v", err)
	}
	if l.Balance("ACC-001", 1) != aed(500_00) ||
		len(l.Auths("ACC-001")) != 0 {
		t.Fatal("invalid settlement changed financial state")
	}

	records := l.Journal()
	last := records[len(records)-1]
	if last.Kind != ledger.Input || last.Event != event || last.Rejected == "" {
		t.Fatalf("rejected input = %+v", last)
	}
}

func TestSettlementCannotReleaseAnotherAccountsHold(t *testing.T) {
	l := newTestLedger(t)
	applyEvents(t, l,
		accountEvent("fund-aed", 1, ledger.Credit, aed(100_00)),
	)

	hold := accountEvent("reserve", 1, ledger.Authorization, aed(50_00))
	hold.AuthID = "shared-id"
	applyEvents(t, l, hold)

	fund := accountEvent("fund-bhd", 1, ledger.Credit, bhd(20_000))
	fund.AccountID = "ACC-002"

	event := accountEvent("capture-bhd", 1, ledger.Settlement, bhd(5_000))
	event.AccountID, event.AuthID = "ACC-002", hold.AuthID
	applyEvents(t, l, fund, event)

	if l.Balance("ACC-001", 1) != aed(100_00) ||
		l.Holds("ACC-001") != aed(50_00) {
		t.Fatal("BHD settlement changed the AED account")
	}
	if l.Balance("ACC-002", 1) != bhd(15_000) ||
		l.Holds("ACC-002") != bhd(0) {
		t.Fatal("wrong BHD settlement effect")
	}

	auths := l.Auths("ACC-002")
	if len(auths) != 1 || auths[0].Status != ledger.Settled ||
		!auths[0].Unmatched {
		t.Fatalf("settlement matched another account's authorization: %+v", auths)
	}
}

func TestReversalAppendsCompensationOnOriginalValueDay(t *testing.T) {
	for _, tc := range []struct {
		name         string
		kind         ledger.EventKind
		original     ledger.Money
		compensation ledger.Money
	}{
		{"credit", ledger.Credit, aed(120_00), aed(-20_00)},
		{"debit", ledger.Debit, aed(80_00), aed(20_00)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			l := newTestLedger(t)
			applyEvents(t, l,
				accountEvent("fund", 1, ledger.Credit, aed(100_00)),
				accountEvent("target", 1, tc.kind, aed(20_00)),
				accountEvent("later-credit", 3, ledger.Credit, aed(1_00)),
			)
			before := l.Journal()

			reversal := ledger.Event{
				ID: "refund", BookedDay: 3, ValueDay: 1,
				Kind: ledger.Reversal, AccountID: "ACC-001",
				Reverses: "target",
			}
			applyEvents(t, l, reversal)

			if l.Balance("ACC-001", 1) != aed(100_00) ||
				l.Balance("ACC-001", 3) != aed(101_00) {
				t.Fatal("principal was not restored on its original value day")
			}
			if got := l.AtClose(1).Balance("ACC-001", 1); got != tc.original {
				t.Fatalf("original close = %v, want %v", got, tc.original)
			}

			after := l.Journal()
			if !reflect.DeepEqual(after[:len(before)], before) {
				t.Fatal("reversal rewrote earlier records")
			}
			added := after[len(before):]
			if len(added) < 2 || added[0].Kind != ledger.Input ||
				added[0].Event != reversal {
				t.Fatalf("reversal records = %+v", added)
			}
			for _, record := range added[2:] {
				if record.Kind != ledger.Accrual {
					t.Fatalf("unexpected reversal effect: %+v", record)
				}
			}

			posting := added[1]
			if posting.Kind != ledger.Posting || posting.Amount != tc.compensation ||
				posting.ValueDay != 1 || posting.Day != 3 ||
				posting.EventID != reversal.ID || posting.AccountID != "ACC-001" {
				t.Fatalf("compensation = %+v", posting)
			}

			reversal.ID = "second-refund"
			if err := l.Apply(reversal); !errors.Is(err, ledger.ErrReversal) {
				t.Fatalf("duplicate reversal error = %v", err)
			}
			if l.Balance("ACC-001", 3) != aed(101_00) {
				t.Fatal("duplicate reversal moved money")
			}
		})
	}
}

func TestReversalRejectsInvalidTargetsAndAllowsRetry(t *testing.T) {
	l := newTestLedger(t)
	applyEvents(t, l,
		accountEvent("fund", 1, ledger.Credit, aed(100_00)),
		accountEvent("purchase", 1, ledger.Debit, aed(20_00)),
	)

	hold := accountEvent("reserve", 1, ledger.Authorization, aed(10_00))
	hold.AuthID = "active"
	applyEvents(t, l, hold)

	declined := accountEvent("declined", 1, ledger.Authorization, aed(200_00))
	declined.AuthID = "declined"
	if err := l.Apply(declined); !errors.Is(err, ledger.ErrDeclined) {
		t.Fatalf("authorization error = %v", err)
	}

	applyEvents(t, l, ledger.Event{
		ID: "first-refund", BookedDay: 2, ValueDay: 1,
		Kind: ledger.Reversal, AccountID: "ACC-001",
		Reverses: "purchase",
	})
	authsBefore := l.Auths("ACC-001")

	for _, tc := range []struct {
		name    string
		target  string
		account string
		day     ledger.Day
		amount  ledger.Money
	}{
		{"missing target", "missing", "ACC-001", 1, ledger.Money{}},
		{"rejected target", "declined", "ACC-001", 1, ledger.Money{}},
		{"active hold", "reserve", "ACC-001", 1, ledger.Money{}},
		{"other account", "fund", "ACC-002", 1, ledger.Money{}},
		{"wrong value day", "fund", "ACC-001", 2, ledger.Money{}},
		{"nonzero amount", "fund", "ACC-001", 1, aed(1)},
		{"negative amount", "fund", "ACC-001", 1, aed(-1)},
		{"already reversed", "purchase", "ACC-001", 1, ledger.Money{}},
		{"reversal target", "first-refund", "ACC-001", 1, ledger.Money{}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			event := ledger.Event{
				ID: tc.name, BookedDay: 2, ValueDay: tc.day,
				Kind: ledger.Reversal, AccountID: tc.account,
				Reverses: tc.target, Amount: tc.amount,
			}
			before := len(l.Journal())

			if err := l.Apply(event); !errors.Is(err, ledger.ErrReversal) {
				t.Fatalf("error = %v", err)
			}
			added := l.Journal()[before:]
			if len(added) != 1 || added[0].Kind != ledger.Input ||
				added[0].Event != event || added[0].Rejected == "" {
				t.Fatalf("rejection records = %+v", added)
			}

			if l.Balance("ACC-001", 2) != aed(100_00) ||
				l.Balance("ACC-002", 2) != bhd(0) ||
				!reflect.DeepEqual(l.Auths("ACC-001"), authsBefore) {
				t.Fatal("invalid reversal changed financial state")
			}
		})
	}

	// Failed attempts must not mark the fund event as already reversed.
	applyEvents(t, l, ledger.Event{
		ID: "valid-retry", BookedDay: 2, ValueDay: 1,
		Kind: ledger.Reversal, AccountID: "ACC-001",
		Reverses: "fund",
	})
	if l.Balance("ACC-001", 2) != aed(0) {
		t.Fatal("valid retry did not reverse the credit")
	}
}

func TestSettlementReversalDoesNotReopenTheHold(t *testing.T) {
	l := newTestLedger(t)
	applyEvents(t, l,
		accountEvent("fund", 1, ledger.Credit, aed(100_00)),
	)

	hold := accountEvent("reserve", 1, ledger.Authorization, aed(20_00))
	hold.AuthID = "purchase"
	settlement := accountEvent("capture", 1, ledger.Settlement, aed(15_00))
	settlement.AuthID = hold.AuthID
	applyEvents(t, l, hold, settlement)
	before := l.Auths("ACC-001")

	applyEvents(t, l, ledger.Event{
		ID: "refund", BookedDay: 1, ValueDay: 1,
		Kind: ledger.Reversal, AccountID: "ACC-001",
		Reverses: settlement.ID,
	})

	if l.Balance("ACC-001", 1) != aed(100_00) ||
		l.Holds("ACC-001") != aed(0) ||
		l.Available("ACC-001", 1) != aed(100_00) {
		t.Fatal("settlement reversal has wrong financial effect")
	}
	if got := l.Auths("ACC-001"); !reflect.DeepEqual(got, before) {
		t.Fatalf("settlement reversal changed authorization history: %+v", got)
	}
}

func TestInstalmentsConserveMoneyAndPreserveInput(t *testing.T) {
	for _, tc := range []struct {
		name    string
		account string
		total   ledger.Money
		count   int
		want    []ledger.Money
	}{
		{"BHD remainder", "ACC-002", bhd(10_000), 3,
			[]ledger.Money{bhd(3_334), bhd(3_333), bhd(3_333)}},
		{"AED remainder", "ACC-001", aed(101), 4,
			[]ledger.Money{aed(26), aed(25), aed(25), aed(25)}},
		{"exact division", "ACC-001", aed(100), 4,
			[]ledger.Money{aed(25), aed(25), aed(25), aed(25)}},
		{"more parts than units", "ACC-001", aed(2), 5,
			[]ledger.Money{aed(1), aed(1), aed(0), aed(0), aed(0)}},
		{"single part", "ACC-002", bhd(1), 1,
			[]ledger.Money{bhd(1)}},
		{"zero means one", "ACC-001", aed(10_00), 0,
			[]ledger.Money{aed(10_00)}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			l := newTestLedger(t)
			event := accountEvent("split-credit", 2, ledger.Credit, tc.total)
			event.AccountID = tc.account
			event.ValueDay = 1
			event.Instalments = tc.count
			applyEvents(t, l, event)

			var parts []ledger.Money
			inputs := 0
			total := ledger.Money{Currency: tc.total.Currency}

			for _, record := range l.Journal() {
				if record.EventID != event.ID {
					continue
				}

				switch record.Kind {
				case ledger.Input:
					inputs++
					if record.Event != event || record.Rejected != "" {
						t.Fatalf("original input changed: %+v", record)
					}
				case ledger.Posting:
					if record.AccountID != tc.account || record.ValueDay != 1 ||
						record.Day != 2 || record.Amount.Currency != tc.total.Currency ||
						record.Amount.Minor < 0 {
						t.Fatalf("invalid split posting: %+v", record)
					}
					parts = append(parts, record.Amount)
					total = total.Add(record.Amount)
				}
			}

			if inputs != 1 || !reflect.DeepEqual(parts, tc.want) {
				t.Fatalf("inputs = %d, parts = %v, want %v",
					inputs, parts, tc.want)
			}
			if total != tc.total || l.Balance(tc.account, 1) != tc.total {
				t.Fatalf("split did not conserve money: total = %v", total)
			}

			smallest, largest := parts[0].Minor, parts[0].Minor
			for _, part := range parts {
				smallest = min(smallest, part.Minor)
				largest = max(largest, part.Minor)
			}
			if largest-smallest > 1 {
				t.Fatalf("parts differ by more than one minor unit: %v", parts)
			}
		})
	}
}

func TestNegativeInstalmentCountIsRejected(t *testing.T) {
	l := newTestLedger(t)
	event := accountEvent("invalid-split", 1, ledger.Credit, aed(10_00))
	event.Instalments = -1

	if err := l.Apply(event); !errors.Is(err, ledger.ErrInvalid) {
		t.Fatalf("error = %v", err)
	}

	records := l.Journal()
	if len(records) != 1 || records[0].Kind != ledger.Input ||
		records[0].Event != event || records[0].Rejected == "" {
		t.Fatalf("rejection records = %+v", records)
	}
	if l.Balance("ACC-001", 1) != aed(0) {
		t.Fatal("invalid split moved money")
	}
}

func TestSplitCreditPostsOnceAndReversesTheWholeAmount(t *testing.T) {
	l := newTestLedger(t)
	event := accountEvent("split-credit", 1, ledger.Credit, bhd(10_000))
	event.AccountID, event.Instalments = "ACC-002", 3
	applyEvents(t, l, event)

	size := len(l.Journal())
	if err := l.Apply(event); !errors.Is(err, ledger.ErrDuplicate) {
		t.Fatalf("duplicate error = %v", err)
	}
	added := l.Journal()[size:]
	if len(added) != 1 || added[0].Kind != ledger.Input ||
		added[0].Rejected == "" {
		t.Fatalf("duplicate records = %+v", added)
	}
	if l.Balance("ACC-002", 1) != bhd(10_000) {
		t.Fatal("duplicate split moved money")
	}

	before := l.Journal()
	applyEvents(t, l, ledger.Event{
		ID: "undo-split", BookedDay: 1, ValueDay: 1,
		Kind: ledger.Reversal, AccountID: "ACC-002",
		Reverses: event.ID,
	})

	after := l.Journal()
	if !reflect.DeepEqual(after[:len(before)], before) {
		t.Fatal("reversal rewrote the original split")
	}
	added = after[len(before):]
	if len(added) != 2 || added[1].Kind != ledger.Posting ||
		added[1].Amount != bhd(-10_000) {
		t.Fatalf("reversal records = %+v", added)
	}
	if l.Balance("ACC-002", 1) != bhd(0) {
		t.Fatal("split credit was not fully reversed")
	}
}

func TestOneOverdraftFeePerAccountAndDay(t *testing.T) {
	l, err := ledger.New([]ledger.Account{
		{ID: "ACC-001", Opening: aed(0)},
		{ID: "second", Opening: aed(0)},
	})
	if err != nil {
		t.Fatal(err)
	}

	for _, id := range []string{"ACC-001", "second"} {
		event := accountEvent("debit-"+id, 1, ledger.Debit, aed(1))
		event.AccountID = id
		applyEvents(t, l, event)

		if l.Fee(id, 1) != aed(0) {
			t.Fatal("open day was charged before close")
		}
	}

	l.Close()
	before := l.Journal()
	l.Close()
	if !reflect.DeepEqual(before, l.Journal()) {
		t.Fatal("repeated Close changed the journal")
	}

	for _, id := range []string{"ACC-001", "second"} {
		for day := ledger.FirstDay; day <= ledger.LastDay; day++ {
			if l.Fee(id, day) != aed(25_00) ||
				l.AtClose(day).Fee(id, day) != aed(25_00) {
				t.Fatalf("%s Day %d: missing fee at close", id, day)
			}

			count := 0
			for _, record := range l.Journal() {
				if record.Kind == ledger.Fee &&
					record.AccountID == id && record.ValueDay == day {
					count++
					if record.Amount != aed(-25_00) ||
						record.Day != day || record.EventID != "" {
						t.Fatalf("fee record = %+v", record)
					}
				}
			}
			if count != 1 {
				t.Fatalf("%s Day %d: %d fee records", id, day, count)
			}
		}

		if l.Balance(id, 6) != aed(-150_01) {
			t.Fatalf("%s: fees did not reduce the ledger balance", id)
		}
	}
}

func TestBackdatedFeesCascadeAndSurviveReversal(t *testing.T) {
	l := newTestLedger(t)
	applyEvents(t, l,
		accountEvent("fund", 1, ledger.Credit, aed(100_00)),
		accountEvent("day-two-credit", 2, ledger.Credit, aed(11_00)),
	)

	late := accountEvent("late-debit", 3, ledger.Debit, aed(101_00))
	late.ValueDay = 1
	applyEvents(t, l, late)

	// Day 1: 100 - 101 - 25 = -26.
	// Day 2: -26 + 11 - 25 = -40.
	// The Day 1 fee causes Day 2's overdraft.
	if l.Balance("ACC-001", 1) != aed(-26_00) ||
		l.Balance("ACC-001", 2) != aed(-40_00) {
		t.Fatal("earlier fees did not carry into later balances")
	}

	if l.AtClose(1).Balance("ACC-001", 1) != aed(100_00) ||
		l.AtClose(2).Balance("ACC-001", 2) != aed(111_00) ||
		l.AtClose(1).Fee("ACC-001", 1) != aed(0) ||
		l.AtClose(2).Fee("ACC-001", 2) != aed(0) {
		t.Fatal("original closes changed")
	}
	if l.Fee("ACC-001", 3) != aed(0) {
		t.Fatal("open Day 3 was charged early")
	}

	var feeDays []ledger.Day
	for _, record := range l.Journal() {
		if record.Kind == ledger.Fee {
			feeDays = append(feeDays, record.ValueDay)
			if record.Day != 3 || record.EventID != late.ID ||
				record.Amount != aed(-25_00) {
				t.Fatalf("retrospective fee = %+v", record)
			}
		}
	}
	if !reflect.DeepEqual(feeDays, []ledger.Day{1, 2}) {
		t.Fatalf("fee assessment order = %v", feeDays)
	}

	// Another accepted event reassesses still-negative closed days.
	applyEvents(t, l,
		accountEvent("today-debit", 3, ledger.Debit, aed(1_00)),
	)

	before := l.Journal()
	applyEvents(t, l, ledger.Event{
		ID: "refund", BookedDay: 3, ValueDay: 1,
		Kind: ledger.Reversal, AccountID: "ACC-001",
		Reverses: late.ID,
	})

	if !reflect.DeepEqual(l.Journal()[:len(before)], before) {
		t.Fatal("reversal rewrote existing fees")
	}
	if l.Balance("ACC-001", 2) != aed(61_00) {
		t.Fatal("reversal did not retain both assessed fees")
	}
	for _, day := range []ledger.Day{1, 2} {
		if l.Fee("ACC-001", day) != aed(25_00) {
			t.Fatalf("Day %d fee was duplicated or removed", day)
		}
	}

	count := 0
	for _, record := range l.Journal() {
		if record.Kind == ledger.Fee {
			count++
		}
	}
	if count != 2 {
		t.Fatalf("fee records = %d, want 2", count)
	}
}

func TestZeroClosingBalanceHasNoFee(t *testing.T) {
	l := newTestLedger(t)
	applyEvents(t, l,
		accountEvent("fund", 1, ledger.Credit, aed(1_00)),
		accountEvent("spend", 1, ledger.Debit, aed(1_00)),
	)
	l.Close()

	for day := ledger.FirstDay; day <= ledger.LastDay; day++ {
		if l.Balance("ACC-001", day) != aed(0) ||
			l.Fee("ACC-001", day) != aed(0) {
			t.Fatalf("zero balance incurred a fee on Day %d", day)
		}
	}
}

func TestNegativeAvailabilityDoesNotCauseFees(t *testing.T) {
	l := newTestLedger(t)
	applyEvents(t, l,
		accountEvent("fund", 1, ledger.Credit, aed(100_00)),
	)

	hold := accountEvent("reserve", 1, ledger.Authorization, aed(100_00))
	hold.AuthID = "purchase"
	applyEvents(t, l,
		hold,
		accountEvent("spend", 1, ledger.Debit, aed(1_00)),
	)
	l.Close()

	if l.Balance("ACC-001", 1) != aed(99_00) ||
		l.Available("ACC-001", 1) != aed(-1_00) {
		t.Fatal("unexpected ledger or available balance")
	}
	for day := ledger.FirstDay; day <= ledger.LastDay; day++ {
		if l.Fee("ACC-001", day) != aed(0) {
			t.Fatalf("hold caused a fee on Day %d", day)
		}
	}
}

func TestUnpricedBHDOverdraftHasNoFee(t *testing.T) {
	l := newTestLedger(t)
	event := accountEvent("bhd-debit", 1, ledger.Debit, bhd(1))
	event.AccountID = "ACC-002"
	applyEvents(t, l, event)
	l.Close()

	for day := ledger.FirstDay; day <= ledger.LastDay; day++ {
		if l.Fee("ACC-002", day) != bhd(0) ||
			l.Balance("ACC-002", day) != bhd(-1) {
			t.Fatalf("invented a BHD fee on Day %d", day)
		}
	}
}

func TestDailyInterestRoundsHalfUpInEachCurrency(t *testing.T) {
	for _, currency := range []ledger.Currency{ledger.AED, ledger.BHD} {
		for _, tc := range []struct {
			name  string
			minor int64
			want  int64
		}{
			{"negative", -2500, 0},
			{"zero", 0, 0},
			{"below tie", 1249, 0},
			{"at tie", 1250, 1},
			{"above tie", 1251, 1},
			{"below next tie", 3749, 1},
			{"at next tie", 3750, 2},
		} {
			t.Run(string(currency)+"/"+tc.name, func(t *testing.T) {
				l, err := ledger.New([]ledger.Account{{
					ID:      "savings",
					Opening: ledger.Money{Currency: currency, Minor: tc.minor},
				}})
				if err != nil {
					t.Fatal(err)
				}

				l.Close()
				want := ledger.Money{Currency: currency, Minor: tc.want}
				if got := l.Accrued("savings", 1); got != want {
					t.Fatalf("daily interest = %v, want %v", got, want)
				}
			})
		}
	}
}

func TestInterestCapitalizesOnlyTheSumOfRoundedAccruals(t *testing.T) {
	l := newTestLedger(t)
	bhdFunding := accountEvent("fund-bhd", 1, ledger.Credit, bhd(31_249))
	bhdFunding.AccountID = "ACC-002"

	applyEvents(t, l,
		accountEvent("fund-aed", 1, ledger.Credit, aed(12_50)),
		bhdFunding,
	)

	// Advance to Day 6 without closing it. The hold also checks that
	// interest uses ledger balance rather than available balance.
	hold := accountEvent("reserve", 6, ledger.Authorization, aed(1))
	hold.AuthID = "purchase"
	applyEvents(t, l, hold)

	if l.Accrued("ACC-001", 1) != aed(1) ||
		l.Balance("ACC-001", 6) != aed(12_50) ||
		l.Available("ACC-001", 6) != aed(12_49) {
		t.Fatal("accruals changed spendable money")
	}
	if l.Accrued("ACC-001", 6) != aed(0) {
		t.Fatal("open Day 6 accrued early")
	}
	for _, record := range l.Journal() {
		if record.Kind == ledger.Interest {
			t.Fatal("interest capitalized before Day 6 closed")
		}
	}

	l.Close()
	before := l.Journal()
	l.Close()
	if !reflect.DeepEqual(before, l.Journal()) {
		t.Fatal("repeated Close capitalized twice")
	}

	for _, tc := range []struct {
		account string
		basis   ledger.Money
		daily   ledger.Money
	}{
		{"ACC-001", aed(12_50), aed(1)},
		{"ACC-002", bhd(31_249), bhd(12)},
	} {
		total := ledger.Money{Currency: tc.basis.Currency}

		for day := ledger.FirstDay; day <= ledger.LastDay; day++ {
			if got := l.Accrued(tc.account, day); got != tc.daily {
				t.Fatalf("%s Day %d accrual = %v, want %v",
					tc.account, day, got, tc.daily)
			}
			total = total.Add(l.Accrued(tc.account, day))
		}

		count := 0
		for _, record := range l.Journal() {
			if record.Kind == ledger.Interest && record.AccountID == tc.account {
				count++
				if record.Day != 6 || record.ValueDay != 6 ||
					record.Amount != total {
					t.Fatalf("capitalization = %+v, daily sum = %v", record, total)
				}
				if l.Before(record.Seq).Balance(tc.account, 6) != tc.basis {
					t.Fatal("interest affected its own basis")
				}
			}
		}

		if count != 1 || l.Balance(tc.account, 6) != tc.basis.Add(total) ||
			l.AtClose(6).Balance(tc.account, 6) != tc.basis.Add(total) {
			t.Fatalf("%s: capitalization did not reconcile", tc.account)
		}
	}
}

func TestBackdatedInterestAdjustmentsAreAppendedAndNotRepeated(t *testing.T) {
	l := newTestLedger(t)
	applyEvents(t, l,
		accountEvent("fund", 1, ledger.Credit, aed(100_00)),
	)

	correction := accountEvent("late-debit", 3, ledger.Debit, aed(50_00))
	correction.ValueDay = 1
	applyEvents(t, l, correction)

	for _, day := range []ledger.Day{1, 2} {
		if l.AtClose(day).Accrued("ACC-001", day) != aed(4) ||
			l.Accrued("ACC-001", day) != aed(2) {
			t.Fatalf("Day %d original and restated accruals are wrong", day)
		}

		count := 0
		for _, record := range l.Journal() {
			if record.Kind == ledger.Accrual &&
				record.EventID == correction.ID && record.ValueDay == day {
				count++
				if record.Amount != aed(-2) || record.Day != 3 {
					t.Fatalf("adjustment = %+v", record)
				}
			}
		}
		if count != 1 {
			t.Fatalf("Day %d: %d adjustments, want 1", day, count)
		}
	}

	before := l.Journal()
	hold := accountEvent("reserve", 3, ledger.Authorization, aed(1))
	hold.AuthID = "purchase"
	applyEvents(t, l, hold)

	if !reflect.DeepEqual(l.Journal()[:len(before)], before) {
		t.Fatal("reassessment rewrote earlier records")
	}
	for _, record := range l.Journal()[len(before):] {
		if record.Kind == ledger.Accrual {
			t.Fatal("unchanged ledger balance accrued twice")
		}
	}
	if l.Balance("ACC-001", 3) != aed(50_00) {
		t.Fatal("accrual adjustment changed ledger money")
	}
}

func TestInterestCorrectionsUseBalancesAfterRetainedFees(t *testing.T) {
	l := newTestLedger(t)
	applyEvents(t, l,
		accountEvent("fund", 1, ledger.Credit, aed(100_00)),
	)

	late := accountEvent("late-debit", 3, ledger.Debit, aed(150_00))
	late.ValueDay = 1
	applyEvents(t, l, late)

	for _, day := range []ledger.Day{1, 2} {
		if l.AtClose(day).Accrued("ACC-001", day) != aed(4) ||
			l.Accrued("ACC-001", day) != aed(0) {
			t.Fatalf("negative Day %d kept positive interest", day)
		}
	}

	applyEvents(t, l, ledger.Event{
		ID: "refund", BookedDay: 3, ValueDay: 1,
		Kind: ledger.Reversal, AccountID: "ACC-001",
		Reverses: late.ID,
	})

	// Retained fees leave Day 1 at 75.00 and Day 2 at 50.00.
	if l.Accrued("ACC-001", 1) != aed(3) ||
		l.Accrued("ACC-001", 2) != aed(2) {
		t.Fatal("interest ignored retained fees")
	}
}

func TestZeroAccrualProducesNoCapitalization(t *testing.T) {
	l := newTestLedger(t)
	l.Close()

	for _, record := range l.Journal() {
		if record.Kind == ledger.Accrual || record.Kind == ledger.Interest {
			t.Fatalf("zero balance generated interest: %+v", record)
		}
	}
}
