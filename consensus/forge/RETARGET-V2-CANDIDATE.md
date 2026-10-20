# FORGE retarget v2 — experimental candidate, not a release

This patch fixes the canonical-difficulty floor trap found in D10. It also
replaces the single-block rational retarget with a relative exponential
candidate. It is a consensus change requiring an explicit `retargetV2: true`
FORGE configuration at a new deployment or a not-yet-activated FORGE fork.
The default is false. No mainnet activation height or deployed genesis is
changed. Do not turn the flag on in an already active FORGE chain: config
compatibility rejects this. Do not bypass that guard or reset an existing DB.

## Why the basic rounding fix was rejected

Legacy canonical difficulty 1 stays at 1 because floor(1*12/11)=1. Its
intermediate integer damping also ignores spacings from 5 through 19 seconds.
Retaining fractions and forcing a minimum one-unit change fixes those bugs,
but our Poisson simulation measured roughly 12.7 seconds at steady hashpower.
Its per-block downward clamp also leaves long stalls after rented power exits.
The initial rational implementation and results are retained in the evidence
package for comparison, not as the selected implementation.

## Selected candidate

For a candidate timestamp with spacing s, use:

    nextDifficulty ~= parentDifficulty * 2^((12 - s) / 120)

The half-life is fixed at 120 seconds in this candidate. Consensus uses only
integer arithmetic. The integer-only exponential approximation is derived
from the CC0 BCH ASERT specification by freetrader, Jonathan Toomim, Calin
Culianu and Mark Lundeberg (version 2020-08-12):
https://upgradespecs.bitcoincashnode.org/2020-11-15-asert/

This is NOT BCH's anchored ASERT algorithm and does not inherit its security
validation. The relative reference, candidate timestamp, 12-second spacing,
120-second half-life, integer difficulty and directional rounding differ.
Independent consensus review is still needed.

Round the scaled difficulty to nearest integer, ties up. When quantization
would suppress a directional change, move one integer unit. Clamp to the
configured minimum and the maximum difficulty with a nonzero canonical target.
At very low integer difficulty this causes substantial quantization effects;
12-second performance cannot be inferred there from high-hashrate tests.

For the first FORGE block, InitialDifficulty is a reference at 12 seconds
after its parent's timestamp. It undergoes the same adjustment so a poorly
chosen initial reference does not make the first block a fixed barrier.
Pre-FORGE difficulty units are never used for this reference.

Validating v2 nodes reject timestamps outside uint64 or more than 15 seconds
ahead of local wall clock. The future timestamp check is conditional on v2 to
preserve existing historical rules. Clock synchronization is operationally
necessary. Timestamp manipulation within accepted bounds, withholding,
hash-hopping incentives, deep reorgs, and network propagation remain review
items; a future timestamp bound is not proof against those attacks.

## Targets and practical limits

Canonical proof target and cumulative difficulty units are unchanged.
Difficulty 1 requires about 4,295,032,833 hashes on average. At 30 kH/s the
minimum therefore takes about 39.77 hours per block. No retarget algorithm
can restore 12-second blocks at that hashrate with minimum difficulty 1.

At 50 GH/s, a rough initial reference for 12 seconds is 140; at 5 TH/s it is
about 13,970. These are estimates, not promises of delivered hashpower or
guaranteed block times. Share difficulty is separate from native block
difficulty. All-miners-off means no blocks. A stable baseline of Scrypt
hashpower is needed after a rental ends.

## Validation scope

Tests exercise the floor escape, one-second directional changes, exact
half-life cases, large integers, overflow, minimum/maximum clamps, legacy
defaults, first-block behavior and immutable configuration. A separate
floating-point oracle checks the integer exponential only in tests.

Primary Prepare and independent peer VerifyHeader/VerifyHeaders agree on a
300-header sequence. Those tests explicitly disable seals; they are not live
mining or two-VPS proof-of-work evidence. Existing real Scrypt proof and
private-CPU tests remain in the regression suite unchanged.

Poisson simulations call the production CalcDifficulty for candidate jobs,
with refresh intervals of 1 and 5 seconds, three fixed seeds, and 25,000
blocks per scenario. The last 20,000 measure steady-state means. Scenarios
cover floor startup, stable ~50 GH/s and ~5 TH/s, 100x changes, a 1000x
overestimated reference, five alternating hashpower phases and an underpowered
floor case. Switching-phase means exclude the first 1,000 blocks per phase.
Startup mismatches
model the post-change condition; they are not adversarial hash-hopping tests.
The simulator includes candidate timestamp progression and worker future-time
waiting. It excludes network latency, stale work, external-node delays and
malicious timestamps. Its 2% steady-mean gate is a development criterion,
not a production guarantee.

## Next release gates

1. Independent review of the new rule, parameter choice and timestamp incentives.
2. Test repeated hashpower arrival/departure and propagation delays, including
   candidate refresh and stale-job behavior in the actual node/pool.
3. A fresh coordinated pilot configuration on both nodes; retain D10 state.
   Existing networks cannot be silently migrated to v2.
4. Live Scrypt mining, matching peer state, wallet confirmations and payout
   reconciliation on that pilot. Mainnet LTC/DOGE connectivity and actual
   treasury reward maturity/spends are separate tests.
5. A separately reviewed OGG mainnet activation and recovery plan.

Production readiness remains false. This package installs no services,
changes no VPN/firewall settings, and provides no mainnet deployment script.
