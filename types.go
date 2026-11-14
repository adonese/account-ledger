package ledger

// Day is a day of the six day window.
type Day int

const (
	FirstDay Day = 1
	LastDay  Day = 6
)

type EventKind string

const (
	Credit        EventKind = "CREDIT"
	Debit         EventKind = "DEBIT"
	Authorization EventKind = "AUTHORIZATION"
	Settlement    EventKind = "SETTLEMENT"
	Reversal      EventKind = "REVERSAL"
)

type Event struct {
	ID          string
	BookedDay   Day
	ValueDay    Day
	Kind        EventKind
	AccountID   string
	Amount      Money  // positive. Reversal will be zero as it uses the original
	AuthID      string // authorization / settlement
	Reverses    string // id of the event to undo
	Instalments int    // credit to be splitted into n postings. Zero value means 1.

}

type Account struct {
	ID      string
	Opening Money
}

type AuthStatus string

const (
	Active   AuthStatus = "ACTIVE"
	Declined AuthStatus = "DECLINED"
	Settled  AuthStatus = "SETTLED"
)

type Auth struct {
	ID        string
	Status    AuthStatus
	Hold      Money // amount requested
	Paid      Money // settled amount
	Unmatched bool  // settled without hold
}

type RecordKind string

const (
	Input     RecordKind = "INPUT"   // an event as received
	Posting   RecordKind = "POSTING" // ledger postings: credit, debit, settlement, reversal
	Fee       RecordKind = "FEE"     // an overdraft fee
	Interest  RecordKind = "INTEREST"
	Accrual   RecordKind = "ACCRUAL"    // interest earned or correction to it
	AuthState RecordKind = "AUTH_STATE" // authorization request
	DayClosed RecordKind = "DAY_CLOSED"
)

type Record struct {
	Seq       int        // position in journal from 1
	Day       Day        // processing day the line was written on
	Kind      RecordKind // journey type
	EventID   string     // event that led to this line. empty for eod work
	AccountID string
	Event     Event  // Input: the event exactly as received
	Rejected  string // why it was rejected. empty if accepted
	ValueDay  Day    // identifies the accounting day
	Amount    Money  // signed change represented by this record
	Auth      Auth
}

func (d Day) inWindow() bool {
	return d >= FirstDay && d <= LastDay
}

func posting(e Event, amount Money) Record {
	return Record{Kind: Posting, ValueDay: e.ValueDay, Amount: amount}
}
