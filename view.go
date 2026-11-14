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
		if (record.Kind == Posting || record.Kind == Fee || record.Kind == Interest) &&
			record.AccountID == accountID && record.ValueDay <= day {
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

func (v View) Auths(accountID string) []Auth {
	var auths []Auth
	for _, record := range v.journal {
		if record.Kind != AuthState || record.AccountID != accountID {
			continue // check a valid event
		}
		i := slices.IndexFunc(auths, func(a Auth) bool {
			return a.ID == record.Auth.ID
		})
		if i >= 0 {
			auths[i] = record.Auth
		} else {
			auths = append(auths, record.Auth)
		}
	}
	return auths
}

func (v View) Holds(accountID string) Money {
	account, _ := v.account(accountID)
	holds := Money{Currency: account.Opening.Currency}
	for _, auth := range v.Auths(accountID) {
		if auth.Status == Active {
			holds = holds.Add(auth.Hold)
		}
	}
	return holds
}

func (v View) Available(accountID string, day Day) Money {
	return v.Balance(accountID, day).Add(v.Holds(accountID).Neg())
}

// Fee returns the amount charged for this day as a positive value.
func (v View) Fee(accountID string, day Day) Money {
	account, _ := v.account(accountID)
	total := Money{Currency: account.Opening.Currency}

	for _, record := range v.journal {
		if record.Kind == Fee &&
			record.AccountID == accountID && record.ValueDay == day {
			total = total.Add(record.Amount)
		}
	}

	return total.Neg()
}

// Accrued returns the current entitlement for one day, including adjustments.
func (v View) Accrued(accountID string, day Day) Money {
	account, _ := v.account(accountID)
	total := Money{Currency: account.Opening.Currency}

	for _, record := range v.journal {
		if record.Kind == Accrual &&
			record.AccountID == accountID && record.ValueDay == day {
			total = total.Add(record.Amount)
		}
	}

	return total
}

func (v View) auth(accountID, authID string) (Auth, bool) {
	for _, auth := range v.Auths(accountID) {
		if auth.ID == authID {
			return auth, true
		}
	}
	return Auth{}, false
}
