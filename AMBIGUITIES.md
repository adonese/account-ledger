# Ambiguities

Wherever the brief allows more than one reading, this file records the reading
taken and what the alternative would change. The refusals of criteria 4 and 6
are argued in [REJECTED.md](REJECTED.md). Known defects are listed last, so
that they are not mistaken for policy.

## Time and history

1. **In what order is the stream replayed?** In arrival order, as listed. The
   booked day advances processing but never rewinds it, and the value day
   decides which balance an entry counts toward. So E10, booked Day 5, is
   processed on Day 6 and restates Day 5.
   *Alternative:* sorting by booked day changes ACC-002's original Day 5 close,
   but not the final numbers, and it hides the fact that E10 arrived late.
2. **Which dates are accepted?** Both dates must fall in Days 1–6, and the
   value day cannot be later than the booked day. A booked day earlier than the
   current day is accepted while the window is open.
   *Alternative:* future value dates would need rules for funds that are still
   pending.
3. **When does a day close?** When an event booked on a later day arrives, or
   at `Close`, which finishes the remaining days through Day 6. Calling `Close`
   again changes nothing.
   *Alternative:* explicit close commands would separate the passage of time
   from event arrival, and would fix defect 1 below.
4. **Which closing balance is reported?** Both. The report shows each day as it
   stood at its own close (`AtClose(d)`), and as later corrections restated it
   (`Balance` on the current view). Criterion 1's −370.00 is Day 2 after E7 but
   before Day 2's fee.
   *Alternative:* reporting only the restated values would hide what was known,
   and shown, at the time.
5. **How are holds dated?** By what is known in the selected journal prefix,
   not by value day. Use `AtClose(d).Available(id, d)` for availability as it
   stood at a past close.
   *Alternative:* filtering holds by value day would mix knowledge time with
   accounting time.

## Fees and interest

6. **Which day is "the day assessed"?** The day whose closing balance is
   tested. The fee for Day 2 is value-dated Day 2 even though it was charged on
   Day 5, and the journal records the processing day separately.
   *Alternative:* dating each fee on the day it is charged would change the
   restated balances and the interest.
7. **When are late fees found?** After every accepted posting, its account's
   closed days are reassessed from its value day, oldest first, because a fee
   feeds later balances. The open day waits for its own close, and no day is
   charged twice.
   *Alternative:* assessing once per correction batch would avoid fees from
   temporary deficits (see the failing test).
8. **Which balance do fees and interest use?** The ledger balance. A fee is
   tested on the balance before that day's fee, and interest is computed after
   it. Holds affect neither, and accruals not yet capitalized are not part of
   the ledger balance.
   *Alternative:* using available balance would let a hold cause a fee without
   any posting.
9. **Does a reversal refund fees?** No. The principal is reversed and interest
   is recalculated, but the fees stay (REJECTED.md, criterion 6).
   *Alternative:* journaled refund credits. Append-only storage allows either
   policy.
10. **How is interest rounded?** To the nearest minor unit, with exact ties
    rounded up. The brief gives the precision but not a tie rule.
    *Alternative:* half-even rounding changes accruals at ties. Either way, the
    credit equals the sum of the rounded daily amounts.
11. **Does the final credit earn interest?** No. Day 6 is accrued first and the
    sum is credited afterwards, so the credit does not feed itself. Accruals
    don't change available funds until then.
    *Alternative:* capitalizing daily would compound.
12. **What does "equal instalments" mean?** Parts that differ by at most one
    minor unit, with the extra units going to the earliest parts (3.334 + 3.333
    + 3.333), all on the event's value day. A count of 0 means one posting, and
    a negative count is rejected. More parts than minor units gives zero-value
    parts.
    *Alternative:* giving the remainder to the last part also conserves the
    total. Spreading the parts across dates would need a schedule the brief
    doesn't give.
13. **What if a tariff is missing?** Only AED has an overdraft fee, so a
    negative BHD balance is not charged, and no exchange rate is assumed. An
    unsupported currency is rejected by `New`.
    *Alternative:* require a BHD tariff, or convert the AED fee under an
    explicit rate policy.

## Authorizations, settlements and reversals

14. **Which balance does an authorization test?** The ledger balance as known
    so far, minus active holds. The open day's fee isn't in it yet, so E8 was
    tested against −385.00. A result of exactly zero is approved.
    *Alternative:* including the pending fee would test E8 against −410.00. It
    would still be declined.
15. **May a debit overdraw the account?** Yes. Only authorizations have a funds
    check, so debits and settlements post whatever the balance.
    *Alternative:* checking every debit would reject E7 and remove the overdraft
    case entirely.
16. **Do later entries revisit an authorization decision?** No. An approval
    stays active, and a decline stays declined, even after restatement.
    *Alternative:* re-evaluating decisions would need explicit authorization
    transitions.
17. **What happens to a settlement with no active hold?** It posts and is
    flagged `Unmatched`, including a settlement after a decline (REJECTED.md,
    criterion 4).
    *Alternative:* reject or quarantine it, but the money has already moved.
18. **Does a settlement release the whole hold?** Yes. One settlement is final
    at any amount, and a second one is rejected.
    *Alternative:* partial capture would keep the rest on hold, but the event
    has no field to say so.
19. **How else can a hold end?** It can't. There is no cancellation or expiry,
    so an unsettled hold outlives the window (DESIGN.md, section 3).
20. **What can be reversed?** An accepted credit, debit or settlement on the
    same account, once, in full, on its original value day. The target supplies
    the amount. A split credit is reversed as one total, and reversing a
    settlement does not reopen its hold.
    *Alternative:* partial or current-dated reversals would need separate rules.

## IDs, retries and the journal

21. **Are retries idempotent?** No. Every `Apply` journals its input, including
    rejected ones, and the first use of an event ID owns it. A corrected retry
    therefore needs a new ID.
    *Alternative:* return the original result for an identical retry.
22. **Can authorization IDs be reused?** Across accounts, yes. Within one
    account, no: any recorded state blocks reuse, including a decline.
    *Alternative:* allowing reuse would need a way to tell which authorization a
    settlement refers to.
23. **What happens after the window closes?** Inputs are journaled as rejected
    (`ErrClosed`) and have no effect. Replay continues past rejected events and
    returns only setup errors.
    *Alternative:* stopping at the first error would drop later valid events.
24. **Can a query see half an event?** Yes. `Before(seq)` can stop between an
    input and its postings. That is how the report shows each fee's pre-fee
    balance.
    *Alternative:* queries limited to completed operations would need commit
    boundaries.
25. **What can callers change?** Nothing already recorded. `Accounts` and
    `Journal` return copies, and a copied `View` keeps its length while the
    ledger grows. The ledger is not safe for concurrent use.

## Known defects

1. **A rejected input can still charge fees.** Days close before the rest of
   the input is validated. A Day 1 overdraft of AED 1.00 followed by an invalid
   Day 6 event charged AED 125.00 for Days 1–5.
2. **Panics are not atomic.** `Add` panics on a currency mismatch or overflow,
   and a panic after records have been appended leaves partial effects. `Neg`
   and `Decimal` mishandle `MinInt64`.
3. **Some failures are silent.** An unknown account reads as zero with no
   currency, a bad `Before` bound panics, and `Report` drops write errors.
4. **No resource limits.** Journal size and instalment counts are unbounded.

These decisions are exercised in [view_test.go](view_test.go),
[replay_test.go](replay_test.go), [criteria_test.go](criteria_test.go) and
[isolation_test.go](isolation_test.go).
[failing_test.go](failing_test.go) demonstrates the fee-order limitation.
