package ledger

// StreamAccounts and Stream return independent copies of the brief's inputs.
func StreamAccounts() []Account {
	return []Account{
		{ID: "ACC-001", Opening: Money{AED, 0}},
		{ID: "ACC-002", Opening: Money{BHD, 0}},
	}
}

func Stream() []Event {
	return []Event{
		{ID: "E1", BookedDay: 1, ValueDay: 1, Kind: Credit, AccountID: "ACC-001", Amount: Money{AED, 1200_00}},
		{ID: "E2", BookedDay: 1, ValueDay: 1, Kind: Debit, AccountID: "ACC-001", Amount: Money{AED, 950_00}},
		{ID: "E3", BookedDay: 2, ValueDay: 2, Kind: Authorization, AccountID: "ACC-001", Amount: Money{AED, 200_00}, AuthID: "Auth-A"},
		{ID: "E4", BookedDay: 3, ValueDay: 3, Kind: Credit, AccountID: "ACC-001", Amount: Money{AED, 400_00}},
		{ID: "E5", BookedDay: 4, ValueDay: 4, Kind: Settlement, AccountID: "ACC-001", Amount: Money{AED, 185_00}, AuthID: "Auth-A"},
		{ID: "E6", BookedDay: 4, ValueDay: 4, Kind: Settlement, AccountID: "ACC-001", Amount: Money{AED, 180_00}, AuthID: "Auth-Z"},
		{ID: "E7", BookedDay: 5, ValueDay: 2, Kind: Debit, AccountID: "ACC-001", Amount: Money{AED, 620_00}},
		{ID: "E8", BookedDay: 5, ValueDay: 5, Kind: Authorization, AccountID: "ACC-001", Amount: Money{AED, 90_00}, AuthID: "Auth-B"},
		{ID: "E9", BookedDay: 6, ValueDay: 2, Kind: Reversal, AccountID: "ACC-001", Reverses: "E7"},
		{ID: "E10", BookedDay: 5, ValueDay: 5, Kind: Credit, AccountID: "ACC-002", Amount: Money{BHD, 10_000}, Instalments: 3},
	}
}
