package ledger_test

import (
	"errors"
	"reflect"
	"testing"

	ledger "github.com/adonese/account-ledger"
)

func streamPrefix(t *testing.T, count int) *ledger.Ledger {
	t.Helper()
	l := newTestLedger(t)
	for _, event := range ledger.Stream()[:count] {
		err := l.Apply(event)
		if event.ID == "E8" && !errors.Is(err, ledger.ErrDeclined) {
			t.Fatalf("E8 should decline: %v", err)
		}
		if event.ID != "E8" && err != nil {
			t.Fatalf("apply %s: %v", event.ID, err)
		}
	}
	return l
}

// Accepted: hold reservations and later value dates do not enter Day 2 principal.
func TestCriterion1Day2BeforeFees(t *testing.T) {
	l := streamPrefix(t, 7)
	for _, record := range l.Journal() {
		if record.Kind == ledger.Fee {
			if got := l.Before(record.Seq).Balance("ACC-001", 2); got != aed(-370_00) {
				t.Fatalf("Day 2 principal = %v", got)
			}
			return
		}
	}
	t.Fatal("missing fee checkpoint")
}

// Refused: the backdated debit affects several days, not only its value day.
func TestCriterion2E7CausesFeesOnDaysTwoFourAndFive(t *testing.T) {
	without := streamPrefix(t, 6)
	without.Close()
	with := streamPrefix(t, 10)
	with.Close()
	var days []ledger.Day
	for day := ledger.FirstDay; day <= ledger.LastDay; day++ {
		if without.Fee("ACC-001", day) != aed(0) {
			t.Fatalf("unexpected fee without E7 on Day %d", day)
		}
		if with.Fee("ACC-001", day) == aed(25_00) {
			days = append(days, day)
		}
	}
	if !reflect.DeepEqual(days, []ledger.Day{2, 4, 5}) {
		t.Fatalf("fee days = %v", days)
	}
}

// Accepted: a final capture releases the entire reservation and posts 185.00.
func TestCriterion3AuthASettlementIsAccepted(t *testing.T) {
	l := streamPrefix(t, 5)
	want := []ledger.Auth{{ID: "Auth-A", Status: ledger.Settled, Hold: aed(200_00), Paid: aed(185_00)}}
	if !reflect.DeepEqual(l.Auths("ACC-001"), want) || l.Balance("ACC-001", 4) != aed(465_00) || l.Holds("ACC-001") != aed(0) {
		t.Fatal("Auth-A settlement was not accepted and fully released")
	}
}

// Conditional policy refusal: settlement inputs are trusted confirmed obligations.
func TestCriterion4UnmatchedSettlementPostsWithWarning(t *testing.T) {
	l := streamPrefix(t, 6)
	auths := l.Auths("ACC-001")
	if len(auths) != 2 || auths[1] != (ledger.Auth{ID: "Auth-Z", Status: ledger.Settled, Paid: aed(180_00), Unmatched: true}) ||
		l.Balance("ACC-001", 4) != aed(285_00) {
		t.Fatal("unmatched settlement was not posted and flagged")
	}
}

// Accepted: explicitly exercise approval although E8 is declined in the stream.
func TestCriterion5ApprovedAuthBOnlyReducesAvailability(t *testing.T) {
	l := newTestLedger(t)
	applyEvents(t, l, accountEvent("fund", 5, ledger.Credit, aed(90_00)), ledger.Stream()[7])
	if l.Balance("ACC-001", 5) != aed(90_00) || l.Available("ACC-001", 5) != aed(0) || l.Holds("ACC-001") != aed(90_00) {
		t.Fatal("approved Auth-B changed principal")
	}
	l.Close()
	if l.Holds("ACC-001") != aed(90_00) {
		t.Fatal("unsettled Auth-B expired without an expiry policy")
	}
}

// Conditional policy refusal: principal reversal leaves already assessed fees in place.
func TestCriterion6ReversalRetainsFees(t *testing.T) {
	before, after := streamPrefix(t, 6), streamPrefix(t, 9)
	for day, shortfall := range map[ledger.Day]ledger.Money{2: aed(25_00), 3: aed(25_00), 4: aed(50_00), 5: aed(75_00)} {
		got := before.Balance("ACC-001", day).Add(after.Balance("ACC-001", day).Neg())
		if got != shortfall {
			t.Fatalf("Day %d shortfall = %v, want %v", day, got, shortfall)
		}
	}
}

// Refused by conservation: 3 * 3.334 would create BHD 0.002.
func TestCriterion7BHDInstalmentsConserveTheCredit(t *testing.T) {
	l := streamPrefix(t, 10)
	var parts []ledger.Money
	for _, record := range l.Journal() {
		if record.Kind == ledger.Posting && record.EventID == "E10" {
			parts = append(parts, record.Amount)
		}
	}
	if !reflect.DeepEqual(parts, []ledger.Money{bhd(3_334), bhd(3_333), bhd(3_333)}) || l.Balance("ACC-002", 5) != bhd(10_000) {
		t.Fatalf("BHD instalments = %v", parts)
	}
}

// Refused by the reconciliation rule: capitalize the daily rounded sum exactly.
func TestCriterion8CapitalizationEqualsDailyAccruals(t *testing.T) {
	l := streamPrefix(t, 10)
	l.Close()
	for _, account := range l.Accounts() {
		total, credited := ledger.Money{Currency: account.Opening.Currency}, ledger.Money{Currency: account.Opening.Currency}
		for day := ledger.FirstDay; day <= ledger.LastDay; day++ {
			total = total.Add(l.Accrued(account.ID, day))
		}
		for _, record := range l.Journal() {
			if record.Kind == ledger.Interest && record.AccountID == account.ID {
				credited = credited.Add(record.Amount)
			}
		}
		if total != credited {
			t.Fatalf("%s accrued %v, credited %v", account.ID, total, credited)
		}
	}
}
