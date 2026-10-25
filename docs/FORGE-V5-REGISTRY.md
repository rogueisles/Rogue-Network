# FORGE V5: bounded merged-mining registry

**Production activation: block 1,121,800.** Version 4.1.0 supplies the approved
scheduled profile at `config/RGN-mainnet-v4.1.0-H1121800.json`. V4 continues below
that height; V5 is required from it. The initial authority is 3-of-4 separate
administrator public addresses, supplied by the operator as existing controlled
wallets. Initial possession was asserted by the operator, not independently signed
by this preparation workflow. No private keys are included.

V5 is a coordinated protocol upgrade. It adds SOQ as an established
native source, an approved registry for compatible extra coins, and delayed
native treasury/admin-key rotation. It preserves historical RGN blocks, genesis,
balances, transaction rules, staking and the existing per-block reward schedule.
The future V5 target spacing is **12,500 milliseconds (12.5 seconds)**; historical
blocks retain their original 12-second target and retarget calculation.

## Upgrade responsibilities

All validating RGN nodes, including exchange-operated nodes, must install the
compatible node and agreed configuration before activation height H = **1,121,800**.
Pools and mining-signing services also need the V5 integration. Miners continue
using ordinary Scrypt Stratum; V5 does not introduce a different mining algorithm.

Below H, the existing proof rules remain unchanged and V5 proofs are rejected.
From H onward, **V5 is required for every RGN block**. Allowing an old V4 proof
after H would allow its old fixed treasury destination to bypass a rotation.
This is why installing only new pool software cannot activate the initial upgrade.

Once V5 is installed, adding a coin within an installed compatible proof profile
or changing an approved treasury destination does not require another RGN
exchange/node upgrade. Participating pools and signers still need the native
integration and configuration. A new algorithm, unsupported header/proof format,
or a change to RGN verification/economic rules can require another coordinated
protocol upgrade. Registering a coin again cannot repair a consensus verifier bug.

## Established sources and optional extras

| Registry slot | Native identity | Installed profile | Role |
| --- | --- | --- | --- |
| 1 | DigiByte parent, ID 0 | Bitcoin-family 80-byte header | Core |
| 2 | Dogecoin AuxPoW, ID 98 | Bitcoin-family 80-byte header | Core |
| 3 | Soqucoin AuxPoW, ID 21329 (`0x5351`) | 80-byte header plus SOQ witness-v1 destination | Core |
| 4–16 | Approved compatible AuxPoW IDs | Installed Bitcoin-family 80-byte profile | Optional revenue |

At least one valid authorized core source must occur in every job. All seven
nonempty DGB/DOGE/SOQ combinations are supported. Core identities, profiles and
roles cannot be removed, disabled, expired or replaced by an administrator
operation. Their treasury destinations can rotate. Pools can omit unavailable
sources and optional extras; an optional extra alone cannot carry RGN issuance.

Each native chain has its own target. An RGN solution need not be a native winner.
A native reward is earned only when that native network accepts a winning block,
keeps it canonical and its coinbase matures. A successful RPC acknowledgement
alone is not a mature treasury payment. V5 does not automatically sell rewards,
perform a buyback or burn native balances.

## What every RGN node verifies

A distinct `OGGF` version-5 envelope contains the multi-chain mining proof and a
registry envelope. The registry envelope carries its resulting root, last
administrator-operation checkpoint, membership proofs for the included coins,
and at most one administrator operation.

The complete registry envelope is domain-separated and bound into the V5 work
signature. The exact work certificate is committed in a zero-value `RGNCERT5`
parent-coinbase output before hashing. The native AuxPoW tree still contains the
actual RGN work leaf. Hidden undeclared native leaves, substituted registries,
incorrect treasuries, duplicate signatures and mismatching native evidence fail
validation.

RGN nodes enforce the installed proof/profile rules, native coinbase Merkle
bindings, Scrypt work, mining quorum, registry authority and treasury script
commitments. **They do not download or run every native blockchain.** Independent
mining signers check the native genesis/network, synchronization, peers, current
tip, full block proposal and full reward claim. Native networks perform their own
complete consensus validation. A native genesis value in a registry record is
metadata the signers must check, not proof that RGN has independently validated
the entire native history.

## Registry commitments and peer synchronization

The registry is a fixed 32-leaf Merkle tree. Leaf 0 holds administrator metadata;
coin slots 1–16 hold entries and remaining leaves have canonical empty hashes.
Every used coin has exactly five sibling hashes in its membership proof. Coin
entries contain identity, profile, core role, activation/disable heights, current
treasury hash and at most one pending treasury change.

A normal block inherits its parent's registry root and checkpoint. An operation
block carries the old bounded snapshot, approved change and resulting root. A
node checks the old snapshot against the **exact parent header**, validates the
operation and deterministically computes the new state. The checkpoint becomes
the operation block's height. Future blocks inherit it.

This works during header-only synchronization, batch imports and competing
branches. Registry transitions and authorization checks run even when a caller
samples expensive PoW checks. Validation never uses a mutable website, current
native RPC response or a canonical-only registry cache. A branch reorganization
selects the registry committed by that branch; it does not rewrite older blocks.

`eth_forgeRegistry` returns the state for a supplied exact parent hash. The
checkpoint permits direct snapshot reconstruction instead of scanning history.
If an RPC lookup races a reorganization and cannot reconstruct the same root, it
fails and the caller retries; it must not substitute another branch's state.

## Separate administrator authority

Administrator keys are separate from mining-work signing keys. Current and
pending administrator sets must remain disjoint from the configured work signers.
Ordinary RGN nodes store public approval data, not administrator private keys.
Pools and online mining signers must never hold those private keys.

An approval binds a domain, RGN chain ID/genesis, parent registry root, operation
sequence and full requested change including its effective height. It is not
bound to one exact inclusion block, so an offline approval can remain usable
while its parent registry state is current and its notice deadline has not passed.
A different administrator operation invalidates stale prior-root approvals.

Only four operation kinds are installed:

1. Register a compatible extra in an available slot, with future activation.
2. Rotate a registered coin's treasury script commitment after notice.
3. Disable an optional extra at a future height. A disabled extra slot can be
   reused after its disable height; core slots cannot be reused.
4. Rotate the administrator set/quorum after notice. The current quorum approves
   and the proposed new quorum signs a separate acceptance domain to prove key
   possession.

An operation requires exactly the configured number of distinct sorted signatures.
It cannot overwrite a still-pending treasury or administrator rotation. Lost
administrator quorum prevents future changes; it does not expire established
mining rules or stop existing mining by itself. Plan independent custody/backups.

Treasury hashes are **SHA-256 of raw native output-script bytes**. They are not
RGN wallet addresses or hashes of address text. SOQ additionally requires its
native witness-v1 destination shape. Before a rotation becomes effective, pool
and signer services need the reviewed public script/address mapping. A local
setting cannot override the registered commitment.

Rotation affects future mining outputs only. Existing balances remain controlled
by their old native wallet keys until separately transferred; no operation moves
those coins. Rotating receiving wallets and rotating registry approval keys are
different operations.

## Resource bounds

| Resource | Consensus limit |
| --- | --- |
| Registered coin entries | 16, including the three core slots |
| Native sources included per job | 6 total |
| Administrator keys | 5 |
| Administrator operations | 1 per block, with a further configured rate limit |
| Registry tree | 32 leaves; membership depth exactly 5 |
| Canonical native AuxPoW tree | At most 32 leaves |
| Complete RGN proof envelope | 16 KiB, including registry data |
| Pending changes | One treasury change per coin; one administrator rotation |

The existing canonical native tree cannot separate chain IDs with the same
residue modulo 32. Registrations reject a native ID whose residue collides with
RGN ID 20295, DOGE, SOQ or another registered auxiliary. DGB is the parent and
does not occupy such an auxiliary slot. This constraint is part of the installed
compatible profile; being Scrypt alone is insufficient for registration.

Ordinary blocks carry only used-entry witnesses. Full snapshots appear only in
operation blocks. Registry JSON RPC replies and public operation files are capped
at 64 KiB. Mining signer transaction-evidence requests have a separate 16 MiB
transport limit; this does not enlarge the 16 KiB consensus proof limit.

The approved mainnet configuration requires **720 blocks of change notice** and at
least **64 blocks between administrator operations**. At an ideal 12.5-second
spacing these are about 2.5 hours and 13 minutes 20 seconds; actual time varies.
Isolated test networks can use smaller explicit values. Both configured values
must be between 1 and 1,000,000 blocks, subject to the mainnet minimums.

## Activation configuration

New settings are in the node's `config.forge` object:

| Field | Meaning |
| --- | --- |
| `registryBlock` | Agreed future V5 activation height; omitted/nil disables V5 |
| `registryBootstrap` | Hex RLP of reviewed public administrator/core-coin snapshot |
| `registryMinDelay` | Minimum future notice in blocks |
| `registryMinOperationInterval` | Minimum blocks between operations |
| `registryTargetBlockTimeMillis` | Exactly `12500` when V5 is scheduled |

An unscheduled profile must omit bootstrap/delay/interval/target data too. The
legacy `targetBlockTime` remains `12`, preserving historical validation. At H the
new deterministic integer retarget uses the 12,500 ms target and retains its
120-second relative exponential half-life. There is no floating-point consensus
calculation. Changing/removing active fork settings is incompatible; startup must
not silently rewind the chain to make conflicting settings fit.

Per-block emissions, allocation percentages and block-indexed era boundaries do
not change. At ideal spacing, moving from 12 to 12.5 seconds means about 4% fewer
blocks per day and 4.17% longer calendar estimates for a fixed number of blocks.
Actual network intervals and native mining income vary.

The old `optionalSoqBlock`-only V4 proposal is historical preparation material,
not the V5 rollout. V5 uses globally approved SOQ treasury commitments; it does
not preserve that earlier proposal's pool-local SOQ destination model.

## Qualification and release gates

Release qualification checks have passed registry and V5 consensus/race tests and the
offline administrator-tool tests. Isolated private-CPU lab reports also show
actual mined blocks, native regtest acceptance/maturity, transfers, local peer
connections, restart/catch-up, fresh synchronization, core treasury rotations
and registration/mining of a compatible extra without replacing RGN nodes.

Subsequent staging qualification also passed real RGN peer synchronization,
mining/transfer agreement and restart/catch-up between staging servers 177 and
63. That transport used actual RGN P2P through controller-managed SSH forwards;
it did not expose public RPC or change production firewall rules. Compiled pool
and mining-signer executables then mined canonical private RGN blocks through
actual Stratum, with canonical DGB, DOGE, SOQ and a compatible extra on real
native regtest daemons. Pool restart, SOQ outage/recovery, all-native outage and
mining-quorum loss/expiry/recovery checks passed.

These tests use private RGN difficulty and native regtests. The isolated pool
identity bridge changes only the native chain-name health label and DOGE's
out-of-range historical checkpoint; native genesis, peers, templates, full-block
proposal validation, mining and submissions remain real. They do not demonstrate
canonical mainnet difficulty, a mature mainnet SOQ reward, arbitrary public
network conditions or production activation. The final qualification report is
the authority for exact artifact identities, test scope and remaining gates.

Representative local microbenchmarks measured registry envelopes of 877 bytes
for three sources and 1,717 bytes for six. A same-run comparison on an i9-13900H
measured approximately 412 microseconds for V4/two sources, 501 for V5/two sources
and 1.68 milliseconds for a maximum 16-slot, six-source operation with five old
administrator approvals and five new-key acceptances. These fixtures include a
deliberately invalid nonce after all authorization/binding checks and Scrypt;
they measure computation, not successful mining. Fixtures differ in other ways,
so source counts must not be treated as a monotonic performance comparison.

An opt-in validation/retention stress passed 10,000 unique sequential V5 headers
with 16 registered coins, six included sources, five administrators and 156
treasury operations. With a fixed 256-header reader window, post-GC Go heap rose
from 1,339,024 bytes after 1,000 warmup headers to 1,381,520 bytes at completion;
peak sampled heap was 4,393,040 bytes. The 12.1-second run includes fixture
construction and GC sampling. It tests bounded validation retention, not full
database synchronization, transaction execution, network propagation or long-term
disk growth. Transient allocations remain measurable and are reported separately
in the benchmark logs. At ideal spacing, an extra 877 bytes/block is approximately
2.2 GB per year of raw data before database/index overhead; actual witnesses vary.

The stress run exposed an inherited optimized SHA3 pointer cast that described a
larger array than short backing allocations contained. The release narrows each
cast to the bytes actually read, preserving the same unrolled XOR operations.
Portable/optimized equality, exact-allocation boundaries, upstream known-answer
vectors and legacy Keccak vectors pass. The full 10,000-header stress also passes
default race/checkptr instrumentation after this fix; no check suppression is
required. This memory-boundary repair does not change hash outputs or consensus.

The approved production profile fixes H = 1,121,800, the public 3-of-4 authority,
native commitments and configuration checksum. No production private keys were
generated or included; the operator asserted control of the supplied wallets.
Final binary/container checks and coordinated node/exchange/pool/signer readiness
remain rollout gates. An approval file or published release alone does not update
running validators. Keep readiness/work gates and hourly supervisors aligned with
the exact approved scheduled configuration.

Before activation an unqualified rollout can be withdrawn. After V5 blocks
exist, reverting validators to old V4 consensus is not a safe rollback. Pools can
stop including an optional source while nodes retain the ability to validate the
accepted chain. If every core source, mining quorum or effective hashpower is
unavailable, block production can pause; recovery must restore valid services,
not bypass consensus.

See the pool's [V5 operator guide](https://github.com/rogueisles/Mining-Pool/blob/main/docs/FORGE-V5-OPERATORS.md)
for deployment responsibilities and the offline approval workflow. Source files
`registry.go`, `flexible.go`, `flexible_verify.go` and `registry_target_test.go`
define the installed rules; historical V4 documentation remains reference only.
