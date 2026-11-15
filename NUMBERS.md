# Numbers

## Given inputs

These values are fixed by the specification. `stream.go` preserves the supplied
event amounts, order, and dates.

| Value | Role |
| --- | --- |
| Days 1 through 6 | `FirstDay`, `LastDay`, and end-of-window capitalization. |
| AED 2 places; BHD 3 places | 100 and 1,000 minor units per whole currency unit. |
| AED 25.00 (`25_00`) | Daily overdraft tariff; no BHD tariff was supplied. |
| 0.04% daily | Interest rate for positive closing ledger balances. |
| Two zero-opening accounts | ACC-001 AED and ACC-002 BHD. |
| E1–E10, Auth-A/B/Z | Event and authorization identifiers from the specification. |
| AED 1,200; 950; 200; 400; 185; 180; 620; 90 | Stream amounts in order; E9 derives its amount from E7. |
| BHD 10.000 in 3 installments | E10 total and part count; booked/value Day 5, arriving after E9. |

## Representation and policy

| Value | Rationale |
| --- | --- |
| Signed `int64` minor units | Stores amounts exactly as whole minor units, including negative balances. A 32-bit integer would allow a much smaller range of amounts. |
| `2500` interest divisor | Derived from the required rate: 0.04% = 1/2500. |
| `1250` rounding threshold | After dividing by 2500, a remainder of 1250 or more rounds up. This rounds to the nearest minor unit, with exact ties rounded up. |
| Remainder units go to the earliest installments | Gives a repeatable split that preserves the total. E10 becomes 3.334 + 3.333 + 3.333. |
| Installment count 0 defaults to 1 | An omitted count produces one posting; negative counts are invalid. |
| Sequences start at 1 | `Before(1)` is the empty prefix; slice boundary is `seq-1`. Zero-based sequences need another API contract. |
| `LastDay+1` (7) | Marks the window as closed, beyond the valid accounting days. |
| Zero | Starting balance, authorization approval boundary, and omitted reversal amount. Also represents no hold, fee, or interest. |
| `places+1` formatting width | Ensures at least one digit before the decimal point. |
| One fee per account/day; one final interest credit per account | Implements the required fee frequency and end-of-window capitalization. Zero interest emits no credit. |

Installments use integer division, then distribute the leftover minor units.
Addition checks for overflow against `math.MinInt64` and `math.MaxInt64`.
Negation and formatting at `MinInt64` remain known limitations (AMBIGUITIES.md).

## Benchmark (`cmd/bench`, `make bench`)

These settings define a repeatable workload. The event mix and amount ranges
were chosen for coverage; alternatives have not been compared.

| Value | Rationale |
| --- | --- |
| Seed `1`; PCG second word `seed ^ 0x9e3779b97f4a7c15` | Fixed seeds make the generated stream repeatable. Other fixed seeds would be equally valid workloads. |
| Every 20th event is a debit with a randomly chosen value day up to its booked day | Exercises reassessment when the value day is earlier. Every 40th event would provide half as many opportunities; streams shorter than 20 events have none. |
| Event choices: 35% credits, 25% debits, 25% holds, 15% settlements | Applies to events other than the every-20th debit. A settlement becomes a credit if the generator has no authorization ID left to settle for that account, so the actual mix varies. |
| Whole-unit amounts: credits 50–1,000; debits 10–400; back-valued debits 50–600; holds and settlements 10–300 | Keeps amounts on a similar scale to the brief's AED events (90 to 1,200). Smaller amounts would change balances relative to the fixed AED 25 fee. |
| Even accounts AED, odd accounts BHD | Exercises both precisions; only AED accounts can be charged fees. |
| 10 × 2 up to 100,000 × 20,000 (events × accounts) | Starts at the brief's size, interprets 100× volume as 1,000 × 200, and includes larger workloads. Rows with only two accounts isolate the cost of longer account histories. |
| Median of 3 runs; selected slow rows run once | Three runs allow a median that is less affected by an unusually fast or slow run. Slow rows run once to keep the benchmark practical, at the cost of less stable timings. |
| `run.sh` replay deadline 1 minute (`DEADLINE`); harness default 5 minutes | Limits the wait for slow replays. Completed replays in the recorded runs took under 13 s. The timeout covers replay only; reporting can take longer. |
| `-report=false` on 100,000-event rows | The report scans the journal per event and per account-day; at 100,000 × 20,000 it did not finish within minutes. Those rows compare journals only. |
| Hash = first 8 bytes of SHA-256 | A compact fingerprint for comparing runs. Four bytes would increase the chance of a collision. |
