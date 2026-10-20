#!/bin/sh
# Local source checks only. Does not start networking or use a node datadir.
set -eu
if [ "$#" -ne 1 ] || [ ! -x "$1" ]; then
  echo "Usage: sh build/qualify-rogue-source.sh /absolute/go1.16.15/bin/go" >&2
  exit 2
fi
task_go=$1
task_root=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
cd "$task_root"
export GO111MODULE=off
sh build/env.sh "$task_go" test -short -vet=off -p=2 -parallel=1 -timeout=15m ./core/... ./consensus/... ./eth/... ./miner/... ./params/... ./cmd/rogue-config/... ./cmd/rogue-replay/... ./cmd/rogue-emission-check/...
sh build/env.sh "$task_go" test -vet=off -count=1 -timeout=5m ./consensus/oggreward -run 'TestRogueHistoricalIssuancePrefix|TestRogueExactPreTailIssuancePrefix'
export FORGE_MULTI_MATRIX_PROOFS="$task_root/consensus/forge/testdata/rogue-lab/matrix-proofs.json"
export FORGE_MULTI_PROOF_FIXTURE="$task_root/consensus/forge/testdata/rogue-lab/proof.json"
sh build/env.sh "$task_go" test -vet=off -count=1 -timeout=5m ./consensus/forge -run 'TestRevenueAnchorConsensusMatrix|TestMultiV3NativePoolProofInteroperability|TestMultiV3AllSubsetConsensus|TestRogueNativeBindingsMatrix'
