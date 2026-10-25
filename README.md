# Rogue Network node — v4.1.0

RGN is an EVM-compatible blockchain secured by FORGE Scrypt merged mining.
This release schedules **FORGE V5 at block 1,121,800**. Existing V4 rules remain
in force below that height. The original genesis, chain ID **70088**, balances,
transactions, staking and per-block reward allocations remain unchanged.

**Every validating node, including exchange-operated nodes, must upgrade before
activation.** Participating pools and mining-signing services also need the
matching integration and approved configuration. Updating a pool alone does not
upgrade other validators. Wallet users keep their existing addresses and keys.

The estimated activation is around **27 October 2026, 13:35 UTC**. Consensus uses
the block height; the clock estimate changes with actual block production.

- [FORGE V5 protocol and administration](docs/FORGE-V5-REGISTRY.md)
- [Release notes](RELEASE-NOTES-4.1.0.md)
- [Approved public configuration and review](config/MAINNET-V5-RELEASE.md)
- [Node downloads](https://github.com/rogueisles/Rogue-Node/releases/tag/v4.1.0)
- [Docker image](https://hub.docker.com/r/roguenetwork/rogue-network)
- [Pool integration](https://github.com/rogueisles/Mining-Pool)
- [Atlas documentation](https://rogueisles.com/atlas/)

## What changes at activation

V5 permits any nonempty authorized subset of **DigiByte, Dogecoin and SOQ**.
SOQ is optional; pools can continue using DGB/DOGE. An unavailable optional source
can be omitted while another eligible core source and the mining work quorum
remain healthy. If all core sources, the work quorum or effective hashpower are
unavailable, mining can pause until those services recover.

A bounded registry supports later compatible Scrypt/AuxPoW coins and delayed
native treasury changes. A separate **3-of-4 administrator quorum** approves
registry operations; routine online mining signers do not hold this authority.
All validators derive the effective registry from the canonical RGN chain.
Local pool settings cannot redirect an approved treasury destination.

Compatible registrations do not require another RGN validator release when they
fit the installed proof profile and bounds. Participating pools still need the
native daemon and integration. New algorithms or verifier changes can require
another coordinated upgrade. Registering a coin again cannot repair verifier code.

The target becomes **12.5 seconds at H**, configured as `12500` milliseconds.
Historical headers retain the original 12-second calculation. Reward amounts,
allocation percentages and issuance heights per block do not change. Calendar
estimates for fixed block intervals become approximately 4.17% longer at ideal
spacing. Native rewards remain subject to their own targets and maturity; this
release does not sell rewards, buy back RGN, burn funds or transfer existing funds.

## Upgrading an existing node

Back up service configuration and wallet custody. Preserve the existing data
directory. Replace the binary and initialize the **approved scheduled profile**
using the normal service-controlled upgrade procedure:

```sh
./rgn-node-linux-amd64 --datadir /YOUR/EXISTING/RGN/DATA --nousb init \
  config/RGN-mainnet-v4.1.0-H1121800.json
```

Do not start a second process on the same database. Preserve peer/network and
protected RPC settings. Start the upgraded service, confirm the unchanged genesis
and a canonical head matching healthy peers, and compare its loaded configuration
against the approved review. The new public profile SHA256 is:

```text
87c66819311b112adce09855f9c2c38490fb32a193ea35ca7c466892319aefd8
```

The historical `config/RGN-mainnet-current-SOQ-unscheduled.json` and root
`genesis.json` are retained for reference and tests. They are **not** the profile
to initialize for this release. Starting an old unscheduled profile before H can
remove a future schedule; enforce the approved checksum throughout rollout.
At/after H, conflicting active settings fail without automatic rewinding.

Container users must use the matching scheduled-profile image and documented
persistent volume. Updating files under the previous image does not update its
payload checksum. Node binaries/images do not start a complete solo miner.

## Build from source

Use the pinned legacy toolchain **Go 1.16.15**, GCC and libc development headers.
Dependencies are vendored; inherited module/import names are intentional.
Build on Ubuntu 22.04 when targeting Ubuntu 22.04 and newer.

```sh
export GO111MODULE=off CGO_ENABLED=1 GOMAXPROCS=2
export CGO_CFLAGS='-O2 -g0' CGO_CXXFLAGS='-O2 -g0'
mkdir -p build/bin
bash build/env.sh /absolute/path/to/go1.16.15/bin/go build \
  -trimpath -ldflags='-buildid= -X main.gitCommit=rgn400v5' \
  -o build/bin/rgn-node-linux-amd64 ./cmd/egem
```

For byte-identical Linux artifact reproduction, use the source-build Dockerfile
and its pinned Ubuntu/Go inputs. It builds from the canonical **`/node`** source
path to **`/release/egem`**. Qualification reproduced the same binary in a native
Ubuntu 22.04 build using a fresh isolated Go cache. Go 1.16 CGO cache entries can
retain source-path metadata after moving a tree; do not reuse a cache populated
from a different checkout path when comparing binary checksums. The general
command above builds the client; exact release reproduction also requires the
recorded build paths, compiler environment and fresh-cache procedure.

Package version is **4.1.0** and the release identifier is `rgn400v5`. The inherited
engine's internal version/banner is retained for compatibility; release manifests
bind the exact source and binary checksums.

## Verification and resource bounds

The registry is limited to **16 coin slots, six native sources per job, five
administrator keys, one rate-limited operation per block and 16 KiB for the entire
proof**. Administrator changes require at least 720 blocks' notice and 64 blocks
between operations. Ordinary validators check proofs and registry commitments;
they do not run every native full node.

Qualification includes private Scrypt mining, all seven core-source subsets,
canonical native regtest rewards, transfers, cross-server peers, fresh sync,
restart and outage recovery, treasury/admin rotations, compatible extra-coin
registration, durable accounting, vectors and race tests. Native mainnet proposal
checks accepted valid DGB/DOGE/SOQ bodies and rejected overclaims; they do not
establish a mined mainnet SOQ reward.

A maximum-capacity 10,000-header validation test retained approximately 1.34–1.38 MB
of Go heap after GC with a fixed 256-header reader window. This measures validation
retention, not full chain/database size or every deployment. Full verification
benchmarks measured approximately 0.5 ms for ordinary V5 fixtures and 1.68 ms for a
maximum administrator rotation on one development machine. See the qualification
report for artifact identities, allocations, private-difficulty/regtest adapters
and networking limits. No software can guarantee external chain availability.

Historical consensus/reward vectors and the optimized SHA3 boundary repair are
covered by regression tests. The SHA3 repair preserves hash outputs and passes
upstream known vectors, portable equivalence and default race/checkptr stress.

Before H, a coordinated withdrawal can return the network to a coherent V4
configuration. Once V5 blocks exist, retain V5 validation and repair or omit an
optional source; reverting validators to V4 is unsafe.

## Source and license

See `RELEASE-SOURCE.json` and the release build manifest for source provenance.
EGEM/go-ethereum attribution and GPL/LGPL notices are preserved in `AUTHORS`,
`COPYING`, `COPYING.LESSER` and source headers. Historical documentation remains
under `docs/history/`; earlier optional-SOQ proposals do not configure this release.

The 4.1.0 packaging retains the tested node's internal build identifier `rgn400v5`. This identifier is opaque; the release version, scheduled profile and published checksums identify this package.
