# Design

What this ledger's design costs, and what it would need before it held real
money. [AMBIGUITIES.md](AMBIGUITIES.md) records the behavior itself.

## 1. Append-only at scale

The journal is the only state. Every balance, hold, fee and accrual is a sum
over it, and a past day is the same sum over a shorter journal.

### What broke first

Replay time. As first built (tag `as-built`), `reassess`
re-ran the end-of-day rules for every closed day of every account after every
event. Each run scanned the whole journal three times (`Balance`, `Fee`,
`Accrued`), so cost grew with accounts × events². At 100× the brief on both
axes, 1,000 events over 200 accounts, replay took 6.2 s, 97% of it in
`reassess`.

### The change made: the account is the unit of storage and work

- `append` records each line's position under its account, and the first
  input for each event ID. Accounts are kept in a map.
- `reassess` revisits only the event's account, from its value day onward. An
  authorization triggers no reassessment, because a hold is not a ledger entry.
- Reads walk that account's positions, not the whole journal.

The change is about 55 lines added and 25 removed. The index holds positions,
not money: it cannot disagree with the journal about a balance, and one pass
over the journal rebuilds it. Views are still journal prefixes. On every stream
both versions finish, the journal and report are byte-identical, and the tests
pass unchanged.

| Events × accounts | Before | After |
| ---: | ---: | ---: |
| 1,000 × 2 | 63 ms | 9.5 ms |
| 1,000 × 20 | 0.62 s | 1.6 ms |
| **1,000 × 200 (100×)** | **6.2 s** | **1.4 ms** |
| 10,000 × 2 | 12.8 s | 3.9 s |
| 10,000 × 2,000 | not run | 20 ms |
| 100,000 × 20,000 | not run | 0.29 s |
| 100,000 × 2 | not run | over 1 min |

The streams are seeded synthetic credits, debits, holds and settlements, with
one event in twenty back-valued. Figures are medians of three runs (one run
for the slowest rows) on an Apple M2 with go1.27.4. At the brief's own size
both versions replay in under 50 µs. "Before" did not finish 10,000 × 2,000 in
five minutes during an earlier run, so it is not run on the larger rows.
`make bench` reproduces the table and stops if the two versions ever produce
different output. [NUMBERS.md](NUMBERS.md) explains the constants.

### What breaks next

The change defers the problem without solving it: cost now grows with one
account's history instead of the whole bank's.

- **Busy accounts.** `Auths` rebuilds every authorization the account has ever
  had, with a linear search per line, on every authorization and settlement.
  At 10,000 events on two accounts it takes 79% of CPU time, against 2.5% for
  `reassess`. 100,000 events on two accounts do not finish in a minute.
- **The report.** It scans the journal once per event (`postingParts`) and
  once per account per day (`AtClose`). At 100× it takes 14 ms against a
  1.4 ms replay, so it is now the slowest part. At 10,000 × 2,000 it takes
  1.2 s against a 20 ms replay.

### Where state grows without bound

1. **The journal.** Nothing is ever dropped. Rejected and duplicate inputs keep
   their full payload. A line takes about 400 bytes including the index, which
   comes to 131 MB at 100,000 events.
2. **Open history.** Every closed day stays rewritable, so a back-valued entry
   rewrites fees and accruals for every later day. On a rolling calendar that
   reaches back to the day the account opened.
3. **Authorizations.** Only a settlement ends one (section 3), so their states
   pile up and `Auths` rebuilds them all.
4. **The index.** Every event ID ever seen stays a duplicate key, and the
   per-account positions grow with the journal.

### The next cheapest changes

- **Key `Auths` by authorization ID**, with a map instead of a search. That
  removes the quadratic term, and the output stays the same.
- **Make history final before a date.** Checkpoint each account's balance at
  that date, start reads and `reassess` from the checkpoint, and move older
  lines to cold storage. This bounds the cost per account, but it changes
  behavior, so it is the control in section 2.

## 2. Value-dated entries in production

Here a back-valued entry is one line, and `reassess` quietly rewrites
everything after its value day. In a bank licensed by the Central Bank of the
UAE (CBUAE), most of what it rewrites has already been seen outside the ledger.

- **Calendars.** A value date is a business day for a given payment rail and
  currency, not an integer. The UAE weekend is Saturday–Sunday and Bahrain's
  is Friday–Saturday, so a BHD item needs a day open in both. Islamic holidays
  are confirmed at short notice. Aani instant payments settle around the
  clock, while cheque and card clearing arrive a day or more late, and
  interest accrues every calendar day. The ledger must derive the value date
  from the rail, currency, booking time and calendar, not accept it as input.
- **Customers.** A customer has already seen the original close. A fee charged
  later for an earlier day, or one that depends on the order corrections
  arrived in ([failing_test.go](failing_test.go)), must still meet the CBUAE's
  [Consumer Protection Standards](https://centralbank.ae/media/5crd24gm/cp-standards-pdf.pdf)
  on disclosed fees and accurate communication (sections 2.1.1.18, 2.1.1.22
  and 2.1.1.23). Complaints the bank doesn't resolve go to Sanadak, the
  CBUAE's ombudsman unit.
- **VAT.** Explicit fees are standard-rated at 5%; interest is exempt. A
  back-valued fee that crosses a VAT period may belong to a return already
  filed. Refunding a fee needs a tax credit note as well as a credit to the
  account.
- **Regulatory reporting.** CBUAE returns and the general ledger are struck as
  at a date. A back-valued entry changes a number that has already been filed,
  and nothing flags that the return must be filed again. Interest already paid
  is restated, and on an Islamic account so is a profit distribution.
- **Financial crime and audit.** Back-valuing makes money look as if it arrived
  earlier: a back-valued credit earns interest and cancels fees for days on
  which the customer was overdrawn. Transaction monitoring must therefore key
  on booking time. UAE AML law requires records to be kept for five years. The
  journal keeps everything, but a line has no time, user, source or reason, so
  nobody can say who back-valued an entry or why.

### Control: a back-value horizon

Add a locked-through date that only an authorized period close moves forward,
once statements are cut and regulatory and VAT returns are filed.

- **An entry valued after the lock** posts as now, and appears on a daily
  back-value report showing its source, how far back it reaches, its amount,
  and any fees it caused.
- **An entry valued on or before the lock** needs a second approver and posts
  on the first open day. A separately journaled interest adjustment covers the
  missed days and references the original value date. The customer is made
  whole, and no reported day changes.

In code, this is one check in `plan`, next to the existing refusal of future
value dates, plus `reassess` starting from the later of the value day and the
day after the lock. It is also the bound that section 1 needs.

## 3. Authorization lifecycle

An authorization is ACTIVE, DECLINED or SETTLED. SETTLED is flagged
`Unmatched` when no active hold stood behind it ([types.go](types.go)). Only a
settlement changes the state. These are the ways an authorization ends other
than a settlement that matches an active hold for its amount.

### Declined

- **Model:** the funds check fails, and DECLINED is final. The ID cannot be
  reused, and later restatement does not reopen the decision.
- **Scenario:** insufficient funds. The merchant may retry under a new
  authorization.
- **Mandate:** keep the decline final. Record the available balance it was
  decided against on the state line, so the decision can still be explained
  after restatement.

### Settled after a decline

- **Model:** the settlement posts, and the state becomes SETTLED and `Unmatched`.
- **Scenario:** a forced sale, or an offline approval the issuer never saw. The
  scheme settles it either way.
- **Mandate:** post it, because the bank has already paid. Open a dispute case
  automatically within the scheme's deadline (Visa 11.2, Mastercard 4808), and
  charge no overdraft fee it causes while the case is open.

### Settled with no authorization

- **Model:** the ID is first seen at settlement, and is created SETTLED and
  `Unmatched`. An authorization that arrives later with the same ID is rejected
  as a duplicate.
- **Scenario:** any of these:
  - stand-in approval while the bank was unreachable;
  - an offline chip approval below the floor limit;
  - the authorization message arriving after clearing;
  - no authorization at all (Visa 11.3, Mastercard 4808).
- **Mandate:** post and flag it, as now. Then match it against stand-in advice
  and late authorization messages within a time window, and record a late
  authorization as the match. Open a dispute case only if nothing matches.

### Settled for a different amount

- **Model:** the whole hold is released and the settled amount is debited,
  without checking it against the hold or the balance. A second settlement is
  rejected.
- **Scenario:** for less, a fuel or hotel pre-authorization finalized at the
  real amount. For more, a tip or an exchange-rate movement. In several parts,
  split shipments.
- **Mandate:** a final capture releases the rest of the hold, as now. A larger
  capture is accepted within a tolerance set per merchant category, and flagged
  beyond it. A capture that is not final keeps the remainder on hold, which
  means tracking held and captured amounts separately.

### Never ends

- **Model:** ACTIVE has no exit except settlement. There is no expiry and no
  cancellation, and an authorization cannot be reversed. After the window
  closes every event is refused, so the hold stays forever.
- **Scenario:** any of these:
  - a cancellation at the till;
  - an acquirer reversal after a terminal timeout;
  - a pre-authorization reduced to the final amount;
  - a lapse under scheme rules, after about a week for an ordinary purchase,
    or about 30 days for a hotel or car rental.
- **Mandate:**
  - add an authorization-reversal event that releases all or part of the hold,
    idempotent on the acquirer's reference;
  - add an end-of-day sweep that journals EXPIRED once the scheme's validity
    period passes;
  - post a late settlement anyway, linked to the authorization as a late match.

  Every release is a new journal line.

## 4. What I cut and why

| Cut | Why | Risk deferred |
| --- | --- | --- |
| Memory only, with a single writer and no log | The brief rules out persistence, and replay is sequential | A crash loses everything. Concurrency needs per-account serialization (section 1). |
| An event's lines are appended one at a time, not atomically | Only an `Add` overflow panic can stop an event partway, and that needs amounts no account will hold | Once storage is durable, a half-applied event stays half-applied. |
| Per-account scans with no running balances, and a quadratic report | Keeps "a balance is a sum over the journal" literally true | Section 1 |
| One wide record type for every kind of line | One append path, and one type to read | About 400 bytes a line, mostly empty fields |
| Integer days in a fixed window, with no calendar, time zone or cut-off | The brief's days are ordinals | Section 2 |
| A day closes when a later-booked event arrives, before the event is validated | There is no clock | A known defect: a rejected event can still charge fees (AMBIGUITIES.md). |
| Nothing posts after Day 6, interest is capitalized once, and future value dates are refused | The window is the whole exercise | No period roll-over, no corrections after capitalization, and no forward-dated items |
| A line carries only a processing day and a sequence number | The brief supplies no time, user, source or reason | No audit trail (section 2) |
| Single entry: customer accounts only, with no general ledger | Only customer balances are asked for | No trial balance, and no fee-income, interest, VAT or suspense accounts |
| Fees are charged as soon as a day turns negative, and kept after a reversal | No refund rule is given | Fees depend on arrival order ([failing_test.go](failing_test.go)). |
| AED fee only, with no VAT and no FX, and one flat half-up rate | None of these is given | Negative BHD accounts go uncharged, VAT is misstated, and there are no tiers, rate changes or Islamic profit |
| No overdraft limit on plain debits | Only authorizations get a funds check | Unbounded credit exposure through `Apply` |
| `Add` panics on overflow; `Neg` and `Decimal` mishandle `MinInt64` | Only amounts no account will hold reach these | A crash or a misprinted amount at the boundary |
| Reversals are whole and once only; fees, interest and authorizations can't be reversed | Avoids partial-reversal accounting | Refunds and fee waivers become manual credits with no link to what they correct |
| Single capture: no expiry, release or increment, and settlement amounts are unchecked | One capture per authorization is enough in scope | Section 3 |
| Inputs are trusted, and a settlement is taken as a confirmed obligation | An in-memory core has no callers to authenticate | Anything that can call `Apply` can debit any account. |
| An event ID belongs to its first use, even a rejected one | The simplest form of idempotency | An honest retry looks like a conflicting reuse. |
| No caps on instalments or journal size; read methods don't validate arguments; the report ignores write errors | An internal exercise API | A single event can exhaust memory, an unknown account reads as zero, and a truncated report still exits 0. |
