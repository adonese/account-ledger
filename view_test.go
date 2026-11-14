package ledger_test

import (
	"testing"

	ledger "github.com/adonese/account-ledger"
)

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
