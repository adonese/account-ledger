# Account ledger

An in-memory, append-only account ledger in Go for the six-day brief in
[specs.md](specs.md). Standard library only: no server, storage or UI.

## Run

Go 1.27 or later, from this directory:

```sh
go test -count=1 ./...          # the suite; passes
go run ./cmd/replay             # replay the brief's stream and print the report
go test -count=1 -tags failing . # the suite plus the one test meant to fail
make lint                       # golangci-lint v2 on PATH; source files only
make bench                      # scaling benchmark, about 2 minutes
```

The failing test exits 1 with:

```
--- FAIL: TestFeesDoNotDependOnArrivalOrder (0.00s)
    failing_test.go:33: LIMITATION: retained fees depend on arrival order: debit first AED 75.00; credit first AED 0.00
```

It's a test against this design, not against the brief. Two corrections that
cancel exactly cost AED 75.00 in fees or nothing, depending on which one arrives
first. The comment in [failing_test.go](failing_test.go) explains why, and
REJECTED.md says what would fix it. The build tag keeps `go test ./...` green.

## Reading the replay output

The report has one block per day, then two tables. Day 5:

```
Day 5
  events:
    E7  DEBIT  ACC-001  AED 620.00, value Day 2
    E8  AUTHORIZATION  ACC-001  AED 90.00 Auth-B, value Day 5, REJECTED
  closing ledger balance:
    ACC-001  AED -410.00, available AED -410.00
    ACC-002  BHD 0.000, available BHD 0.000
  fee assessments:
    ACC-001  AED 25.00 for Day 2, balance before fee AED -370.00
    ACC-001  AED 25.00 for Day 4, balance before fee AED -360.00
    ACC-001  AED 25.00 for Day 5, balance before fee AED -385.00
  authorization states:
    ACC-001  Auth-A  SETTLED for AED 185.00, releasing the AED 200.00 hold
    ACC-001  Auth-Z  SETTLED for AED 180.00, with no active hold to release
    ACC-001  Auth-B  DECLINED, wanted AED 90.00
  errors:
    E8 rejected: Ledger.authorize: declined: hold AED 90.00 exceeds available AED -385.00
```

- **A day's block is that day as it stood when it closed.** Later events never
  rewrite it. Day 2's block shows AED 250.00, even though E7 later makes Day 2
  negative; the restated value is in the table at the end.
- **events** are the inputs processed that day, in arrival order. `value Day N`
  is the day whose balance the entry counts toward. `booked Day N` appears only
  when that differs from the day the event was processed: E10 is booked Day 5
  but arrives on Day 6. `REJECTED` marks a refused input, and `posted as …`
  shows instalments.
- **closing ledger balance** is every entry value-dated on or before that day,
  as known at its close. `available` is that minus active holds.
- **fee assessments** are listed on the day they were charged. `for Day N` is
  the day assessed, which is also the fee's value date. `balance before fee` is
  that day's close just before the fee. E7 arrived on Day 5 and made Days 2 and
  4 negative, so their fees appear here, dated back. Day 3 stayed at +5.00.
- **authorization states** are every authorization known at that close.
- **errors** are rejected events with the reason, plus warnings, such as Auth-Z
  settling with no hold on Day 4.

Because later events restate earlier days, the first table puts each close
beside its final value. For example, ACC-001 Day 2 closed at 250.00 and ends at
225.00: E7 and its reversal E9 cancel, and the 25.00 fee stays.

```
  ACC-001 Day 2: at close AED 250.00; restated AED 225.00
  ACC-001 Day 5: at close AED -410.00; restated AED 210.00
  ACC-002 Day 5: at close BHD 0.000; restated BHD 10.000
```

The second table checks interest. The rounded daily accruals must add up
exactly to the single credit at the end of Day 6:

```
  ACC-001  0.10 + 0.09 + 0.25 + 0.09 + 0.08 + 0.08 = AED 0.69 accrued, AED 0.69 credited
  ACC-002  0.000 + 0.000 + 0.000 + 0.000 + 0.004 + 0.004 = BHD 0.008 accrued, BHD 0.008 credited
```

## Reading the benchmark output

`make bench` runs [cmd/bench/run.sh](cmd/bench/run.sh). It builds the ledger
twice: "before" from commit `e51366a`, the ledger as first built, and "after"
from this checkout. Both replay the same seeded synthetic streams. Each row
prints one line per build:

```
  before events=1000 accounts=200 lines=3078 replay=6.204121292s report=101.537209ms heap=1.15MB bytes/line=372 hash=ebfa237533b09fb0
  after  events=1000 accounts=200 lines=3078 replay=1.389792ms report=13.686834ms heap=1.25MB bytes/line=406 hash=ebfa237533b09fb0
```

- `lines` is the journal length. `replay` and `report` are median times.
  `heap` is the memory the ledger holds after replay.
- When both builds show the same `hash`, their journal and report are
  byte-identical. The script stops if the two ever differ.
- `report=skipped` rows hash the journal only. `unfinished-after-1m0s` means
  the row hit the deadline (`DEADLINE=5m make bench` raises it).
- Times depend on the machine; line counts and hashes don't. The constants are
  explained in NUMBERS.md.

## The documents

- [REJECTED.md](REJECTED.md): the eight acceptance criteria, the ones refused
  and why, and approaches abandoned along the way.
- [AMBIGUITIES.md](AMBIGUITIES.md): where the brief allows more than one
  reading, the reading taken, and known defects.
- [NUMBERS.md](NUMBERS.md): every constant, and why not half of it.
- [DESIGN.md](DESIGN.md): scale, value dating in a UAE bank, how
  authorizations end, and what was cut.
- [WORKLOG.md](WORKLOG.md): what was done, and when.

## The code

| File | Contents |
| --- | --- |
| [money.go](money.go) | `Money` as `int64` minor units, instalment split, daily interest |
| [types.go](types.go) | Events, authorizations, and the journal's record type |
| [ledger.go](ledger.go) | `Apply`: validation, postings, holds, day close, fees, interest |
| [view.go](view.go) | Balances, holds, fees and accruals as of any journal prefix, plus the per-account index |
| [report.go](report.go), [cmd/replay](cmd/replay/main.go) | The printed report |
| [stream.go](stream.go) | The brief's accounts and events |
| [cmd/bench](cmd/bench/main.go) | Synthetic stream generator and timing harness |
| `*_test.go` | One test per acceptance criterion (`criteria_test.go`), plus rules, replay and isolation tests |
