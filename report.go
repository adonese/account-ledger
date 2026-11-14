package ledger

import (
	"fmt"
	"io"
	"strings"
)

// Report prints original daily closes, final restatements, and interest
// reconciliation using this view. The compatible API has no writer-error return.
func (v View) Report(output io.Writer) {
	for day := FirstDay; day <= LastDay; day++ {
		v.reportDay(output, day)
	}
	v.reportBalances(output)
	v.reportInterest(output)
}

func (v View) reportDay(output io.Writer, day Day) {
	events, fees, errs := v.dayActivity(day)
	then := v.AtClose(day)
	var closing, auths []string
	for _, account := range v.accounts {
		closing = append(closing, fmt.Sprintf("%s  %s, available %s",
			account.ID, then.Balance(account.ID, day), then.Available(account.ID, day)))
		for _, auth := range then.Auths(account.ID) {
			auths = append(auths, account.ID+"  "+describeAuth(auth))
		}
	}
	_, _ = fmt.Fprintf(output, "Day %d\n", day)
	reportSection(output, "events", events)
	reportSection(output, "closing ledger balance", closing)
	reportSection(output, "fee assessments", fees)
	reportSection(output, "authorization states", auths)
	reportSection(output, "errors", errs)
	_, _ = fmt.Fprintln(output)
}

// Activity belongs to its processing day, even when its value day is earlier.
func (v View) dayActivity(day Day) (events, fees, errs []string) {
	for _, record := range v.journal {
		if record.Day != day {
			continue
		}
		switch record.Kind {
		case Input:
			events = append(events, v.describeInput(record))
			if record.Rejected != "" {
				errs = append(errs, record.EventID+" rejected: "+record.Rejected)
			}
		case Fee:
			fees = append(fees, fmt.Sprintf("%s  %s for Day %d, balance before fee %s",
				record.AccountID, record.Amount.Neg(), record.ValueDay,
				v.Before(record.Seq).Balance(record.AccountID, record.ValueDay)))
		case AuthState:
			if record.Auth.Unmatched {
				errs = append(errs, fmt.Sprintf("%s warning: %s has no active hold; settlement posted",
					record.EventID, record.Auth.ID))
			}
		}
	}
	return events, fees, errs
}

func (v View) reportBalances(output io.Writer) {
	_, _ = fmt.Fprintln(output, "Closing ledger balances: original closes and final restated values")
	for _, account := range v.accounts {
		for day := FirstDay; day <= LastDay; day++ {
			_, _ = fmt.Fprintf(output, "  %s Day %d: at close %s; restated %s\n",
				account.ID, day, v.AtClose(day).Balance(account.ID, day), v.Balance(account.ID, day))
		}
	}
}

func (v View) reportInterest(output io.Writer) {
	_, _ = fmt.Fprintln(output, "\nInterest reconciliation: rounded daily accruals and Day 6 capitalization")
	for _, account := range v.accounts {
		var days []string
		total := Money{Currency: account.Opening.Currency}
		for day := FirstDay; day <= LastDay; day++ {
			daily := v.Accrued(account.ID, day)
			days = append(days, daily.Decimal())
			total = total.Add(daily)
		}
		credited := Money{Currency: account.Opening.Currency}
		for _, record := range v.journal {
			if record.Kind == Interest && record.AccountID == account.ID {
				credited = credited.Add(record.Amount)
			}
		}
		_, _ = fmt.Fprintf(output, "  %s  %s = %s accrued, %s credited\n",
			account.ID, strings.Join(days, " + "), total, credited)
	}
}

func reportSection(output io.Writer, title string, lines []string) {
	if len(lines) == 0 {
		_, _ = fmt.Fprintf(output, "  %s: none\n", title)
		return
	}
	_, _ = fmt.Fprintf(output, "  %s:\n    %s\n", title, strings.Join(lines, "\n    "))
}

func (v View) describeInput(input Record) string {
	event := input.Event
	line := fmt.Sprintf("%s  %s  %s  ", event.ID, event.Kind, event.AccountID)
	switch event.Kind {
	case Reversal:
		line += "of " + event.Reverses
	case Authorization, Settlement:
		line += event.Amount.String() + " " + event.AuthID
	default:
		line += event.Amount.String()
	}
	line += fmt.Sprintf(", value Day %d", event.ValueDay)
	if event.BookedDay != input.Day {
		line += fmt.Sprintf(", booked Day %d", event.BookedDay)
	}
	if input.Rejected != "" {
		return line + ", REJECTED"
	}
	if parts := v.postingParts(input); len(parts) > 1 {
		line += ", posted as " + strings.Join(parts, " + ")
	}
	return line
}

func (v View) postingParts(input Record) []string {
	var parts []string
	for _, record := range v.journal {
		if record.Kind == Posting && record.EventID == input.EventID && record.Seq > input.Seq {
			parts = append(parts, record.Amount.Decimal())
		}
	}
	return parts
}

func describeAuth(auth Auth) string {
	switch {
	case auth.Status == Active:
		return fmt.Sprintf("%s  ACTIVE, holding %s", auth.ID, auth.Hold)
	case auth.Status == Declined:
		return fmt.Sprintf("%s  DECLINED, wanted %s", auth.ID, auth.Hold)
	case auth.Unmatched:
		return fmt.Sprintf("%s  SETTLED for %s, with no active hold to release", auth.ID, auth.Paid)
	default:
		return fmt.Sprintf("%s  SETTLED for %s, releasing the %s hold", auth.ID, auth.Paid, auth.Hold)
	}
}
