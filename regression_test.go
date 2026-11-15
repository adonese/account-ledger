package ledger_test

import (
	"errors"
	"reflect"
	"testing"

	ledger "github.com/adonese/account-ledger"
)

func TestLedgerHistoryCannotBeReplaced(t *testing.T) {
	l := newTestLedger(t)
	applyEvents(t, l, accountEvent("original", 1, ledger.Credit, aed(100_00)))
	snapshot := l.Before(3)
	want := snapshot.Journal()
	// Reflection keeps this reproducer compilable after the public field is removed.
	if field := reflect.ValueOf(l).Elem().FieldByName("View"); field.IsValid() && field.CanSet() {
		field.Set(reflect.ValueOf(l.Before(1)))
	}
	applyEvents(t, l, accountEvent("replacement", 1, ledger.Credit, aed(7_00)))
	if !reflect.DeepEqual(snapshot.Journal(), want) {
		t.Fatal("replacing View overwrote recorded history")
	}
}

func TestInvalidEventDoesNotCloseDays(t *testing.T) {
	l := newTestLedger(t)
	applyEvents(t, l, accountEvent("debit", 1, ledger.Debit, aed(1_00)))
	before := len(l.Journal())
	bad := accountEvent("invalid", 6, ledger.Credit, aed(1_00))
	bad.AccountID = "missing"
	if err := l.Apply(bad); !errors.Is(err, ledger.ErrUnknownAccount) {
		t.Fatalf("error = %v", err)
	}
	added := l.Journal()[before:]
	if len(added) != 1 || added[0].Kind != ledger.Input || added[0].Day != 1 || added[0].Rejected == "" {
		t.Fatal("invalid input must only append its rejection on the current day")
	}
}

func TestBeforeCannotExtendHistoricalView(t *testing.T) {
	l := newTestLedger(t)
	applyEvents(t, l, accountEvent("credit", 1, ledger.Credit, aed(1_00)))
	defer func() {
		if recover() == nil {
			t.Fatal("Before extended an empty view into later history")
		}
	}()
	l.Before(1).Before(2)
}
