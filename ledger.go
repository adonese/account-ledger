package ledger

import (
	"errors"
	"fmt"
	"slices"
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

func (l *Ledger) append(r Record) {
	r.Seq = len(l.journal) + 1
	r.Day = l.today
	l.journal = append(l.journal, r)
}

func (l *Ledger) Apply(event Event) error {
	var effects []Record
	var err error
	switch {
	case l.today > LastDay:
		err = ErrClosed
	case !event.BookedDay.inWindow():
		err = fmt.Errorf("Ledger.Apply: %w booked day %d is outside the window", ErrInvalid, event.BookedDay)
	default:
		// close proceeding days before processing the event.
		for l.today < event.BookedDay {
			l.closeDay()
		}
		effects, err = l.plan(event)
	}
	input := Record{
		Kind: Input, Event: event, EventID: event.ID, AccountID: event.AccountID,
	}
	if err != nil {
		input.Rejected = err.Error()
	}
	l.append(input)

	for _, record := range effects {
		record.EventID = event.ID
		record.AccountID = event.AccountID
		l.append(record)
	}
	return err
}

//nolint:cyclop // i will try to refactor it later. Notice its gocognit is low
func (l *Ledger) plan(event Event) ([]Record, error) {
	account, known := l.account(event.AccountID)
	_, seen := l.input(event.ID)
	switch {
	case event.ID == "":
		return nil, fmt.Errorf("Ledger.plan: %w: missing event ID", ErrInvalid)
	case seen:
		return nil, fmt.Errorf("Ledger.plan: %w: event ID %q", ErrDuplicate, event.ID)
	case !known:
		return nil, fmt.Errorf("Ledger.plan: %w %q", ErrUnknownAccount, event.AccountID)
	case !event.ValueDay.inWindow() || event.ValueDay > event.BookedDay:
		return nil, fmt.Errorf(
			"Ledger.plan: %w: value day %d must be within the window and not after booked day %d",
			ErrInvalid, event.ValueDay, event.BookedDay,
		)
	case event.Kind == Reversal:
		// original event suppliest amount and currency
	case event.Amount.Currency != account.Opening.Currency:
		return nil, fmt.Errorf("Ledger.plan: %w: recieved %s for a %s account", ErrCurrency, event.Amount.Currency, account.Opening.Currency)
	case event.Amount.Minor <= 0:
		return nil, fmt.Errorf("Ledger.plan: %w: amount must be positive", ErrInvalid)
	}
	switch event.Kind {
	case Reversal:
		return l.reverse(event)
	case Settlement:
		return l.settle(event)
	case Authorization:
		return l.authorize(event)
	case Credit:
		if event.Instalments < 0 {
			return nil, fmt.Errorf(
				"%w: negative instalment count %d",
				ErrInvalid, event.Instalments,
			)
		}

		var postings []Record
		for _, part := range event.Amount.split(max(event.Instalments, 1)) {
			postings = append(postings, posting(event, part))
		}
		return postings, nil
	case Debit:
		return []Record{posting(event, event.Amount.Neg())}, nil
	default:
		return nil, fmt.Errorf("Ledger.plan: %w unsupported event kind %q", ErrInvalid, event.Kind)
	}

}

func (l *Ledger) closeDay() {
	l.append(Record{Kind: DayClosed, ValueDay: l.today})
	l.today++
}

func (l *Ledger) Close() {
	for l.today <= LastDay {
		l.closeDay()
	}
}

func (l *Ledger) authorize(event Event) ([]Record, error) {
	if event.AuthID == "" {
		return nil, fmt.Errorf("Ledger.authorize %w: missing authorization ID", ErrInvalid)
	}
	if _, exists := l.auth(event.AccountID, event.AuthID); exists {
		return nil, fmt.Errorf("Ledger.authorize: %w: authorization ID %q", ErrDuplicate, event.AuthID)
	}

	auth := Auth{ID: event.AuthID, Status: Active, Hold: event.Amount}
	available := l.Available(event.AccountID, l.today)
	if available.Minor < event.Amount.Minor {
		auth.Status = Declined
		return []Record{{
			Kind: AuthState, Auth: auth,
		}}, fmt.Errorf("Ledger.authorize: %w: hold %s exceeds available %s", ErrDeclined, event.Amount, available)
	}
	return []Record{{Kind: AuthState, Auth: auth}}, nil
}

func (l *Ledger) settle(event Event) ([]Record, error) {
	if event.AuthID == "" {
		return nil, fmt.Errorf("Ledger.settle: %w: missing authorization ID", ErrInvalid)
	}
	auth, _ := l.auth(event.AccountID, event.AuthID)
	if auth.Status == Settled {
		return nil, fmt.Errorf("Ledger.settle: %w: settlement of %q", ErrDuplicate, event.AuthID)
	}

	settled := Auth{
		ID: event.AuthID, Status: Settled, Hold: auth.Hold, Paid: event.Amount, Unmatched: auth.Status != Active,
	}
	return []Record{
		posting(event, event.Amount.Neg()),
		{Kind: AuthState, Auth: settled},
	}, nil
}

//nolint:cyclop // refactor for cyclop later
func (l *Ledger) reverse(event Event) ([]Record, error) {
	target, found := l.input(event.Reverses)

	reversed := slices.ContainsFunc(l.journal, func(record Record) bool {
		return record.Kind == Input && record.Rejected == "" &&
			record.Event.Kind == Reversal &&
			record.Event.Reverses == event.Reverses
	})

	switch {
	case !found || target.Rejected != "" || target.AccountID != event.AccountID:
		return nil, fmt.Errorf(
			"%w: %q is not an accepted event on this account",
			ErrReversal, event.Reverses,
		)
	case reversed:
		return nil, fmt.Errorf(
			"%w: %q is already reversed", ErrReversal, event.Reverses,
		)
	case event.ValueDay != target.Event.ValueDay || event.Amount.Minor != 0:
		return nil, fmt.Errorf(
			"%w: reverse the whole amount on the original value day",
			ErrReversal,
		)
	}

	switch target.Event.Kind {
	case Credit:
		return []Record{posting(event, target.Event.Amount.Neg())}, nil
	case Debit, Settlement:
		return []Record{posting(event, target.Event.Amount)}, nil
	default:
		return nil, fmt.Errorf(
			"%w: %q is not a credit, debit, or settlement",
			ErrReversal, event.Reverses,
		)
	}
}
