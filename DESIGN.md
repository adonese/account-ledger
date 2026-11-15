# Design

What this ledger's design costs, and what it would need before it held real
money. [AMBIGUITIES.md](AMBIGUITIES.md) records the behavior itself.

## 1. Append-only at scale

The journal is the only state. Every balance, hold, fee and accrual is a sum
over it, and a past day is the same sum over a shorter journal.

**What broke first: replay time.** As first built (tag `as-built`), `reassess`
re-ran the end-of-day rules for every closed day of every account after every
event, each run scanning the whole journal three times (`Balance`, `Fee`,
`Accrued`), so cost grew with accounts × events². At 100× the brief on both
axes, 1,000 events over 200 accounts, replay took 6.2 s, 97% of it in
`reassess`.

**The change: the account is the unit of storage and work.** `append` records
each line's position under its account, and the first input for each event ID.
`reassess` revisits only the event's account, from its value day onward (an
authorization triggers none: a hold is not a ledger entry), and reads walk that
account's positions. The index holds positions, not money, so it cannot
disagree with the journal about a balance, and one pass rebuilds it. The change
is about 55 lines added and 25 removed; wherever both versions finish, the
journal and report are byte-identical and the tests pass unchanged.

| Events × accounts | Before | After |
| ---: | ---: | ---: |
| 1,000 × 2 | 63 ms | 9.5 ms |
| 1,000 × 20 | 0.62 s | 1.6 ms |
| **1,000 × 200 (100×)** | **6.2 s** | **1.4 ms** |
| 10,000 × 2 | 12.8 s | 3.9 s |
| 10,000 × 2,000 | not run | 20 ms |
| 100,000 × 20,000 | not run | 0.29 s |
| 100,000 × 2 | not run | over 1 min |

Seeded synthetic streams, one event in twenty back-valued; medians of three
runs (one for the slowest rows) on an Apple M2, go1.27.4. "Before" did not
finish 10,000 × 2,000 in five minutes, so larger rows skip it. At the brief's
size both replay in under 50 µs. `make bench` reproduces the table and stops if
the outputs differ; [NUMBERS.md](NUMBERS.md) explains the constants.

**What breaks next.** Cost now grows with one account's history instead of the
bank's. `Auths` rebuilds every authorization the account has ever had, with a
linear search per line, on every authorization and settlement: 79% of CPU at
10,000 events on two accounts (2.5% for `reassess`), and 100,000 events on two
accounts do not finish in a minute. The report scans the journal once per event
(`postingParts`) and once per account per day (`AtClose`), so it is now slower
than replay: 14 ms against 1.4 ms at 100×, 1.2 s against 20 ms at
10,000 × 2,000.

**Where state grows without bound.** *The journal:* nothing is dropped, and
rejected and duplicate inputs keep their full payload; at about 400 bytes a
line with its index, that is 131 MB at 100,000 events. *Open history:* every
closed day stays rewritable, so on a rolling calendar a back-valued entry
rewrites fees and accruals back to account opening. *Authorizations:* only a
settlement ends one (section 3), so their states pile up and `Auths` rebuilds
them all. *The index:* every event ID ever seen stays a duplicate key, and the
per-account positions grow with the journal.

**The next cheapest changes.** Key `Auths` by authorization ID: a map instead
of a search removes the quadratic term, with identical output. Then make
history final before a date: checkpoint each account's balance there, start
reads and `reassess` from the checkpoint, and move older lines to cold storage.
That bounds per-account cost but changes behavior, so it is the control in
section 2.

## 2. Value-dated entries in production

Here a back-valued entry is one line, and `reassess` quietly rewrites
everything after its value day. In a bank licensed by the Central Bank of the
UAE (CBUAE), most of what it rewrites has already been seen outside the ledger.

- **Calendars.** A value date is a business day per rail and currency, not an
  integer. The UAE weekend is Saturday–Sunday and Bahrain's Friday–Saturday, so
  a BHD item needs a day open in both; Islamic holidays are confirmed at short
  notice. Aani settles around the clock, cheque and card clearing a day or more
  late, and interest accrues every calendar day. The ledger must derive the
  value date from rail, currency, booking time and calendar, not take it as
  input.
- **Customers.** A customer has already seen the original close. A later fee
  for an earlier day, or one that depends on correction order
  ([failing_test.go](failing_test.go)), must still meet the CBUAE's
  [Consumer Protection Standards](https://centralbank.ae/media/5crd24gm/cp-standards-pdf.pdf)
  on disclosed fees and accurate communication (2.1.1.18, 2.1.1.22, 2.1.1.23);
  unresolved complaints go to Sanadak, the CBUAE's ombudsman unit.
- **VAT.** Fees are standard-rated at 5%; interest is exempt. A back-valued fee
  crossing a VAT period may belong to a filed return, and a refund needs a tax
  credit note as well as an account credit.
- **Regulatory reporting.** CBUAE returns and the general ledger are struck as
  at a date. A back-valued entry changes a filed number, and nothing flags the
  resubmission. Interest already paid is restated, and on an Islamic account
  so is a profit distribution.
- **Financial crime and audit.** A back-valued credit makes money look earlier:
  it earns interest and cancels fees for days the customer was overdrawn, so
  transaction monitoring must key on booking time. UAE AML law requires five
  years of records. The journal keeps everything, but a line has no time, user,
  source or reason, so nobody can say who back-valued an entry or why.

**Control: a back-value horizon.** A locked-through date that only an
authorized period close moves forward, once statements are cut and regulatory
and VAT returns filed. An entry valued after the lock posts as now and appears
on a daily back-value report with its source, reach, amount and any fees it
caused. One valued on or before the lock needs a second approver and posts on
the first open day, with a separately journaled interest adjustment for the
missed days that references the original value date: the customer is made
whole, and no reported day changes. In code this is one check in `plan`, beside
the refusal of future value dates, plus `reassess` starting from the later of
the value day and the day after the lock. It is also the bound section 1
needs.

## 3. Authorization lifecycle

An authorization is ACTIVE, DECLINED or SETTLED, and SETTLED is flagged
`Unmatched` when no active hold stood behind it ([types.go](types.go)). Only a
settlement changes the state. These are the ways one ends other than a
settlement matching an active hold for its amount.

**Declined.** *Model:* the funds check fails, and DECLINED is final: the ID
cannot be reused, and restatement does not reopen it. *Scenario:* insufficient
funds; the merchant may retry under a new authorization. *Mandate:* keep it
final, and record the available balance it was decided against on the state
line, so it stays explainable after restatement.

**Settled after a decline.** *Model:* the settlement posts, and the state
becomes SETTLED and `Unmatched`. *Scenario:* a forced sale, or an offline
approval the issuer never saw; the scheme settles it either way. *Mandate:*
post it, since the bank has already paid; open a dispute case automatically
within the scheme's deadline (Visa 11.2, Mastercard 4808), and charge no
overdraft fee it causes while the case is open.

**Settled with no authorization.** *Model:* the ID is first seen at settlement
and created SETTLED and `Unmatched`; a later authorization with that ID is
rejected as a duplicate. *Scenario:* stand-in approval while the bank was
unreachable, an offline chip approval below the floor limit, the authorization
message arriving after clearing, or no authorization at all (Visa 11.3,
Mastercard 4808). *Mandate:* post and flag it, as now, then match it against
stand-in advice and late authorization messages within a time window; open a
dispute only if nothing matches.

**Settled for a different amount.** *Model:* the whole hold is released and
the settled amount debited, unchecked against the hold or the balance; a second
settlement is rejected. *Scenario:* less, a fuel or hotel pre-authorization
finalized at the real amount; more, a tip or an exchange-rate movement; in
parts, split shipments. *Mandate:* a final capture releases the rest of the
hold, as now; a larger one is accepted within a per-merchant-category tolerance
and flagged beyond it; a non-final capture keeps the remainder on hold, which
means tracking held and captured amounts separately.

**Never ends.** *Model:* ACTIVE exits only by settlement, with no expiry,
cancellation or reversal, and after the window every event is refused, so the
hold stays forever. *Scenario:* a cancellation at the till, an acquirer
reversal after a terminal timeout, a pre-authorization reduced to the final
amount, or a lapse under scheme rules (about a week for an ordinary purchase,
about 30 days for a hotel or car rental). *Mandate:* an authorization-reversal
event that releases all or part of the hold, idempotent on the acquirer's
reference; an end-of-day sweep that journals EXPIRED once the scheme's validity
period passes; and a late settlement posted anyway, linked as a late match.
Every release is a new journal line.

## 4. What I cut and why

- **Memory only, single writer, no log.** The brief rules out persistence, and
  replay is sequential. *Risk:* a crash loses everything, and concurrency needs
  per-account serialization (section 1).
- **An event's lines append one at a time, not atomically.** Only an `Add`
  overflow panic can stop an event partway. *Risk:* once storage is durable, a
  half-applied event stays half-applied.
- **Per-account scans, no running balances, a quadratic report.** This keeps
  "a balance is a sum over the journal" literally true. *Risk:* section 1.
- **One wide record type for every kind of line.** One append path, one type
  to read. *Risk:* about 400 bytes a line, mostly empty.
- **Integer days; no calendar, time zone or cut-off.** The brief's days are
  ordinals. *Risk:* section 2.
- **A day closes when a later-booked event arrives, before the event is
  validated.** There is no clock. *Risk:* a known defect, where a rejected event
  can still charge fees (AMBIGUITIES.md).
- **Nothing posts after Day 6, interest capitalizes once, and future value dates
  are refused.** The window is the whole exercise. *Risk:* no period roll-over,
  no corrections after capitalization, no forward-dated items.
- **A line carries only a processing day and a sequence number.** The brief
  gives no time, user, source or reason. *Risk:* no audit trail (section 2).
- **Single entry: customer accounts only, no general ledger.** Only customer
  balances are asked for. *Risk:* no trial balance, and no fee-income,
  interest, VAT or suspense accounts.
- **Fees are charged as soon as a day turns negative, and kept after a
  reversal.** No refund rule is given. *Risk:* fees depend on arrival order
  ([failing_test.go](failing_test.go)).
- **AED fee only, no VAT or FX, one flat half-up rate.** None of these is given.
  *Risk:* negative BHD accounts go uncharged, VAT is misstated, and there are no
  tiers, rate changes or Islamic profit.
- **No overdraft limit on plain debits.** Only authorizations get a funds check.
  *Risk:* unbounded credit exposure through `Apply`.
- **`Add` panics on overflow; `Neg` and `Decimal` mishandle `MinInt64`.** Only
  amounts no account will hold reach them. *Risk:* a crash or a misprinted
  amount at the boundary.
- **Reversals are whole and once only; fees, interest and authorizations can't
  be reversed.** This avoids partial-reversal accounting. *Risk:* refunds and
  fee waivers become manual credits with no link to what they correct.
- **Single capture: no expiry, release or increment, and settlement amounts are
  unchecked.** One capture per authorization is enough in scope. *Risk:*
  section 3.
- **Inputs are trusted, and a settlement is taken as a confirmed obligation.**
  An in-memory core has no callers to authenticate. *Risk:* anything that can
  call `Apply` can debit any account.
- **An event ID belongs to its first use, even if rejected.** The simplest
  idempotency. *Risk:* an honest retry looks like a conflicting reuse.
- **No caps on instalments or journal size; reads don't validate arguments; the
  report ignores write errors.** An internal exercise API. *Risk:* one event can
  exhaust memory, an unknown account reads as zero, and a truncated report still
  exits 0.
