//go:build failing

package ledger_test

import (
	"testing"

	ledger "github.com/adonese/account-ledger"
)

// Deliberate limitation: immediate reassessment observes the temporary deficit
// between offsetting corrections. Retaining those fees makes their total depend
// on arrival order even though the final principal facts are identical.
// This is a policy trade-off, not a requirement of append-only storage: a
// separate compensating fee credit or deferred assessment could change it.
func TestFeesDoNotDependOnArrivalOrder(t *testing.T) {
	debit := accountEvent("correction-debit", 5, ledger.Debit, aed(620_00))
	credit := accountEvent("correction-credit", 5, ledger.Credit, aed(620_00))
	debit.ValueDay, credit.ValueDay = 2, 2
	fees := func(events ...ledger.Event) ledger.Money {
		l := newTestLedger(t)
		applyEvents(t, l, accountEvent("fund", 1, ledger.Credit, aed(250_00)))
		applyEvents(t, l, events...)
		l.Close()
		total := aed(0)
		for day := ledger.FirstDay; day <= ledger.LastDay; day++ {
			total = total.Add(l.Fee("ACC-001", day))
		}
		return total
	}
	debitFirst, creditFirst := fees(debit, credit), fees(credit, debit)
	if debitFirst != creditFirst {
		t.Errorf("LIMITATION: retained fees depend on arrival order: debit first %s; credit first %s", debitFirst, creditFirst)
	}
}
