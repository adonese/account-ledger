package ledger_test

import (
	"reflect"
	"testing"

	ledger "github.com/adonese/account-ledger"
)

func TestHistoricalViewsAndReturnedValuesAreIsolated(t *testing.T) {
	l := newTestLedger(t)
	applyEvents(t, l, accountEvent("fund", 1, ledger.Credit, aed(100_00)))
	hold := accountEvent("reserve", 1, ledger.Authorization, aed(20_00))
	hold.AuthID = "purchase"
	applyEvents(t, l, hold)
	original := l.View()
	history, accounts, auths := original.Journal(), original.Accounts(), original.Auths("ACC-001")
	for _, prefix := range []ledger.View{original.Before(1), original.Before(3), original.Before(len(history) + 1)} {
		before := prefix.Journal()
		exposed := prefix.Journal()
		if len(exposed) > 0 {
			exposed[0].Event.Amount = aed(1)
		}
		if !reflect.DeepEqual(prefix.Journal(), before) {
			t.Fatal("a returned journal rewrote a historical prefix")
		}
	}
	exposedAccounts := original.Accounts()
	exposedAccounts[0].Opening = aed(1)
	exposedAuths := original.Auths("ACC-001")
	exposedAuths[0].Status = ledger.Settled
	exposedAuths[0].Hold = aed(1)
	applyEvents(t, l, accountEvent("purchase", 2, ledger.Debit, aed(10_00)))
	if original.Balance("ACC-001", 2) != aed(100_00) || l.Balance("ACC-001", 2) != aed(90_00) ||
		original.Holds("ACC-001") != aed(20_00) ||
		!reflect.DeepEqual(original.Journal(), history) ||
		!reflect.DeepEqual(original.Accounts(), accounts) ||
		!reflect.DeepEqual(original.Auths("ACC-001"), auths) {
		t.Fatal("appends or public inspection changed a historical view")
	}
}

func TestEventKindNamesMatchReference(t *testing.T) {
	if string(ledger.Settlement) != "SETTLEMENT" || string(ledger.Reversal) != "REVERSAL" {
		t.Fatal("event kind names differ from the reference input vocabulary")
	}
}
