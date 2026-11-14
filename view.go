package ledger

import "slices"

// View is a read-only state of the account ledger.
type View struct {
	accounts []Account // there's a small trick to use obscure identifiers e.g. 𝛂
	journal  []Record
}

func (v View) Accounts() []Account {
	return slices.Clone(v.accounts)
}
func (v View) Journal() []Record {
	return slices.Clone(v.journal)
}
func (v View) Balance(accountID string, day Day) Money {
	account, _ := v.account(accountID)
	balance := account.Opening
	for _, record := range v.journal {
		if record.Kind == Posting && record.AccountID == accountID && record.ValueDay <= day {
			balance = balance.Add(record.Amount)
		}
	}
	return balance
}

func (v View) account(id string) (Account, bool) {
	for _, acc := range v.accounts {
		if acc.ID == id {
			return acc, true
		}
	}
	return Account{}, false
}

func (v View) input(eventID string) (Record, bool) {
	for _, record := range v.journal {
		if record.Kind == Input && record.EventID == eventID {
			return record, true
		}
	}
	return Record{}, false
}

func (v View) AtClose(day Day) View {
	for _, record := range v.journal {
		if record.Kind == DayClosed && record.ValueDay == day {
			return View{
				accounts: v.accounts, journal: v.journal[:record.Seq],
			}
		}
	}
	return v
}

func (v View) Before(seq int) View {
	return View{
		accounts: v.accounts, journal: v.journal[:seq-1],
	}
}
