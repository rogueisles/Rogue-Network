# Optional SOQ merged mining

> **Archived earlier V4 proposal; superseded by FORGE V5.** No
> mainnet activation was scheduled by this document. Its pool-local SOQ wallet
> model and unchanged-target description do not apply to V5. V5 uses approved
> global native treasury commitments, a separate administrative registry and a
> future 12.5-second target. Read [FORGE V5](docs/FORGE-V5-REGISTRY.md) for the
> current release. The remainder is preserved as design history.

**Preparation only. No mainnet activation has been scheduled or installed.**

This candidate extends the existing signed FORGE V4 mining job to support
Soqucoin (SOQ) alongside RGN, DigiByte and Dogecoin. It is an additional native
AuxPoW leaf in the same Scrypt job, not hash switching or a separate SOQ job.

## Proposed rules

Before the separately configured `config.forge.optionalSoqBlock`, the existing
mainnet rules continue to apply. A SOQ leaf is rejected. A missing value disables
the new feature indefinitely.

At and after that height, the accepted external sources are DGB, DOGE and SOQ.
At least one must participate with independently validated native work. The
supported sets are DGB; DOGE; SOQ; DGB+DOGE; DGB+SOQ; DOGE+SOQ; DGB+DOGE+SOQ.
RGN-only work is forbidden. Native targets remain independent: a valid RGN block
does not imply a winning native-chain block.

SOQ uses AuxPoW chain ID **21329 (0x5351)** and participation bit **32**. The
existing mainnet bits are DGB=1 and DOGE=2; the new permitted set is 35. The V4
RLP envelope, certificate domain and signature quorum are preserved. Undeclared
tree leaves, unknown chains and malformed native evidence remain invalid.

The historical JSON field `requireDgbOrDogeWork` remains true to preserve stored
configuration compatibility. After this explicitly configured future fork its
native-source requirement also permits SOQ. It must not be unset during upgrade.

## Rewards and destinations

RGN issuance, emissions, the first fork at 1,100,100, allocation schedule,
staking rewards, difficulty rules, chain ID, genesis and transaction addresses
are unchanged. Existing DGB/DOGE treasury commitments remain mandatory whenever
those chains participate.

SOQ is optional pool revenue. Its receiving address is a pool/signer setting,
not a globally pinned RGN consensus wallet. Positive SOQ coinbase outputs must
use native witness-v1 destinations; signers validate the configured address and
script through the SOQ daemon and verify the full native reward claim.

SOQ-only operation produces no DGB or DOGE reward. It does not automatically
convert SOQ to another currency, transfer it to a different treasury, or trigger
buybacks or burns. An accepted native submission must become canonical and
mature under its native chain's rules before it is spendable.

## Compatibility and rollout

The proposed change is consensus-affecting. Unchanged mainnet nodes reject SOQ
work, so operators and exchanges need the reviewed node update before any final
activation height. Pool and signer updates alone cannot enable it.

Adding a future `optionalSoqBlock` preserves genesis and imported history.
Changing or removing it after activation is rejected; startup refuses to
automatically rewind a conflicting active RGN configuration. Existing FORGE
parameters remain immutable from the original fork.

No final activation height has been selected. Heights in test fixtures are
synthetic activation boundaries, not a published schedule. Block timing varies;
an agreed height provides an estimate rather than a wall-clock guarantee.

Before deployment, agree on one final height and apply that same value to the
reviewed node configuration, pool and independent signers. Keep the original
genesis, database and signer identities. Do not initialize a different network
or delete chain data. Confirm distribution readiness before enabling SOQ jobs.

## Verification

The review bundle includes test logs and the exact source differences. Tests
cover existing consensus and issuance, before/at/after activation, all seven
native combinations, quorum authorization, payout pins, canonical native
blocks and reward UTXOs, outages, lost replies, separate SOQ accounting and
database configuration compatibility without a head rewind.

Native regression tests use disposable local DGB/DOGE/SOQ daemons. Their solved
blocks are not mainnet blocks. The separate SOQ mainnet preflight validates an
unsolved real template and overclaim rejection, with zero block submissions or
transactions. These checks do not establish a post-fork mainnet block or mature
SOQ payment; those require a separately approved activation and live observation.

Candidate binaries and source are for review. Docker runtime qualification is recorded in the release bundle. Final publication,
coordinated node/signer rollout and activation remain separate steps. Do not replace the current public `latest` image with a candidate.
