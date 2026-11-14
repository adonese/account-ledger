package ledger

import (
	"fmt"
	"math"
)

type Currency string

const (
	AED Currency = "AED"
	BHD Currency = "BHD"
)

// decimals for currency scales (aed is 2, bhd is 3).
var decimals = map[Currency]int{AED: 2, BHD: 3}

// Money is a whole number of currencys minor units.
// Money{AED, 3000} is AED 30.00
type Money struct {
	Currency Currency
	Minor    int64 // hmmm
}

// Decimal formats the amount without its currency (30.00)
func (m Money) Decimal() string {
	places := decimals[m.Currency]
	sign, minor := "", m.Minor
	if minor < 0 {
		sign, minor = "-", -minor
	}
	digits := fmt.Sprintf("%0*d", places+1, minor)
	if places == 0 {
		return sign + digits
	}
	whole := len(digits) - places
	return sign + digits[:whole] + "." + digits[whole:]
}

func (m Money) String() string {
	return string(m.Currency) + " " + m.Decimal()
}

func (m Money) Add(o Money) Money {
	if m.Currency != o.Currency {
		// panic at this stage, panic is safer than incorrect
		panic(fmt.Sprintf("Money().Add: ledger different currencies %s to %s", o.Currency, m.Currency))
	}
	if (o.Minor > 0 && m.Minor > math.MaxInt64-o.Minor) || (o.Minor < 0 && m.Minor < math.MinInt64-o.Minor) {
		panic("Money().Add: ledger amount overflows int64")
	}
	return Money{m.Currency, m.Minor + o.Minor}
}

func (m Money) Neg() Money { return Money{m.Currency, -m.Minor} }
