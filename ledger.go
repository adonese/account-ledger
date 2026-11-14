package ledger

import (
	"errors"
	"fmt"
)

var (
	ErrClosed         = errors.New("window closed")
	ErrInvalid        = errors.New("invalid event")
	ErrDuplicate      = errors.New("duplicate")
	ErrUnknownAccount = errors.New("unknown account")
	ErrCurrency       = errors.New("currency does not match the account")
	ErrDeclined       = errors.New("declined")
	ErrReversal       = errors.New("cannot reverse")
)

type Ledger struct {
	View
	today Day // opening day
}

// New opens ledger on day 1
func New(accounts []Account) (*Ledger, error) {
	l := &Ledger{today: FirstDay}
	for _, a := range accounts {
		if _, known := decimals[a.Opening.Currency]; !known {
			return nil, fmt.Errorf("Ledger.New(): account %q: unupported currency %q", a.ID, a.Opening.Currency)
		}
		if _, repeated := l.account(a.ID); repeated || a.ID == "" {
			return nil, fmt.Errorf("Ledger.New(): account ID %q is empty or repeated", a.ID)
		}
		l.accounts = append(l.accounts, a)
	}
	return l, nil

}
