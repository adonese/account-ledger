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
	return account.Opening
}

func (v View) account(id string) (Account, bool) {
	for _, acc := range v.accounts {
		if acc.ID == id {
			return acc, true
		}
	}
	return Account{}, false
}
