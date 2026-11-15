
# Worklog

Times are in git: `git log --date=iso`.

So so far it was just setting up the repo with golang (1.27) and adding golanci as well

- First actual work is money.go part. It has 4 methods (Decimal, String, Add, Neg). They are covered with unit tests.
- One decision is to use int64 for minor. The problem here is that this can overflow. We have updated in the REJECTED.md why other packages were rejected.

- instalments part: there will be leftovers because 1000 is not divisable by 3

We currently are at ledger and journal part. Nothing big, basically having views over the journal and added tests

- added golangci lint and created a an entry for it in Makefile. I had to fix reported bugs as well
- I prefer having method names in the error logs so i had to skip ST1005 

The rest of the notes are codex generated but directly with me reviewing back and forth. The problem i had was this:
- i wanted to do the exercise myself without letting any ai do it all
- the task is not necessarily trivial either, so it took me time (check hte metadata of the first commits from Saturday night). Understanding and visualizing the two dimensions nature of the exercise was fun too

## Ledger and replay

- Added tests that callers cannot mutate journal records or change an existing
  view by appending new events.
- Replay keeps arrival order, records rejected inputs, and continues. The daily
  report shows original and restated closes, fees, authorization states, and
  interest reconciliation.
- Cross-checked against an earlier implementation of the same brief: all 49
  journal records and 12 account/day snapshots matched. Final balances are
  AED 210.69 and BHD 10.008; accrued and credited interest match at 0.69 / 0.008.
- The failing test exposes arrival-order dependence: offsetting corrections
  produce AED 75.00 in fees when the debit comes first, and zero when the credit
  comes first. Retaining assessed fees causes this difference.
- `Report` still discards write errors to keep its existing API. A failed output
  write can therefore go unnoticed.

## Scaling

- At 1,000 events across 200 accounts, replay took 6.2 s. `reassess` used 97%
  of CPU time: after every event it revisited every closed day of every account,
  scanning the journal three times per assessment.
- Added indexes for account lookup, journal positions by account, and the first
  input for each event ID. Reassessment now visits only the affected account
  from the event's value day onward; authorizations skip it entirely.
- Tried the change on a scratch copy first. The tests passed, and journal and
  report hashes matched on every workload both versions completed. After
  applying it to the ledger, replay output and the failing test were unchanged.
- Replay fell to 1.4 ms at 1,000 events × 200 accounts, and from 12.8 s to
  3.9 s at 10,000 × 2. A separate comparison at 100,000 × 20,000 took 12 s
  with linear account lookup versus 0.23 s with the map.
- The report at 100,000 × 20,000 did not finish within minutes. Those large
  benchmark rows use `-report=false` and compare journals only. `make bench`
  compares the current checkout against `e51366a` with a fixed seed.
- Long account histories are still expensive: 100,000 events across two
  accounts did not finish within a minute. At 10,000 × 2, `View.Auths` used
  79% of CPU time because it searches linearly while rebuilding authorization
  states. Keying those states by authorization ID is the next change to try.

## Day-close defect

- Days close before the rest of an incoming event is validated. A scratch test
  with a Day 1 overdraft followed by a rejected Day 6 event charged AED 125.00
  for Days 1–5. Rejecting an event can therefore still trigger fees; validation
  needs to happen before advancing the day.

## Review fixes

- Reproduced public-view replacement, invalid-input day close, and historical
  prefix extension in three failing regression tests, then made all three pass.
- Made the stored view private, added `View()` for snapshots, validated inputs
  before advancing days, and checked `Before` against the selected view's length.
