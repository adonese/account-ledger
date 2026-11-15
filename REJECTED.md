

# Decisions or options that have been rejected
- choice of money package. There are popular open source packages for golang such as shopspring/decimal and go-money
they could have been used but i didn't want to add more complexity to the project that it needed to be.
So that left us with an int64 as a choice for our money minor units scale. It can overflow and we have tests that covered that.
- we have rejected implicit currency conversion
- we rejected floating point in monetery values as well
- in the journal, we don't allow callers to supply their own seq

## Refused criteria

Each criterion has a test of the same number in
[criteria_test.go](criteria_test.go).

| # | Criterion | Why it is refused |
| --- | --- | --- |
| 2 | E7 causes exactly one fee, on Day 2 | E7 leaves Days 2, 4 and 5 negative (−370.00, −360.00 and −385.00 before their fees), so it causes three fees, AED 75.00 in total. Day 3 stays at +5.00. |
| 4 | Unmatched settlements are rejected and the funds stay | A settlement reports money already paid. Rejecting E6 would leave the balance wrong by AED 180.00, so E6 posts and is flagged `Unmatched` for follow-up. |
| 6 | After E9, balances and fees return to their pre-E7 values | E9 restores the principal, but the fees were correctly assessed. Undoing them would mean deleting entries, which append-only forbids, or a refund policy the brief doesn't give. Days 2–5 end 25.00, 25.00, 50.00 and 75.00 below their pre-E7 balances. |
| 7 | Each instalment is BHD 3.334 | 3 × 3.334 = 10.002, which creates BHD 0.002. The credit is posted as 3.334 + 3.333 + 3.333. |
| 8 | An interest remainder is discarded | The rounded daily accruals must sum exactly to the credit, and the credit is that sum, so there is never a remainder. |

Accepted: 1 (Day 2 is −370.00 before fees), 3 (Auth-A settles and its whole
hold is released) and 5 (a hold reduces available balance, not ledger balance;
in the stream, Auth-B is in fact declined).

## Approaches replaced during the build

| Replaced | By | Why |
| --- | --- | --- |
| Jumping straight to an event's booked day (`54ed162`) | Closing each day in turn, with a close marker (`a0494fc`) | The jump left no record of the closes in between, so past days could not be read as they stood. |
| Reassessing every account from Day 1 after every event, with full-journal scans (`e51366a`) | Reassessing only the event's account, from its value day, using a position index and an account map | Replay at 1,000 × 200 fell from 6.2 s to 1.4 ms, with byte-identical output. The map alone made 100,000 × 20,000 go from 12 s to 0.23 s. |

## What the failing test reveals

[failing_test.go](failing_test.go) starts with AED 250.00 and applies two
offsetting AED 620.00 corrections, both valued Day 2. Applying the debit first
temporarily exposes negative balances and charges AED 75.00 in fees. Applying
the credit first charges nothing. The final principal entries are the same,
but the fees depend on arrival order.

The test expects equal fees and fails, exposing the cost of immediate
assessment followed by fee retention. Waiting until a correction batch is
complete, or issuing fee refunds, could resolve it; neither is implemented.
