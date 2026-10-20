package eth

import (
	"encoding/json"
	"math/big"
	"testing"

	"github.com/TeamEGEM/go-egem/common"
	"github.com/TeamEGEM/go-egem/consensus/ethash"
	"github.com/TeamEGEM/go-egem/consensus/forge"
	"github.com/TeamEGEM/go-egem/consensus/oggreward"
	"github.com/TeamEGEM/go-egem/consensus/progpow"
	"github.com/TeamEGEM/go-egem/miner"
	"github.com/TeamEGEM/go-egem/params"
	"github.com/TeamEGEM/go-egem/rpc"
)

func TestForgeAuxRPCExposesExactIssuedTimeAndValidityBound(t *testing.T) {
	cfg := integrationForgeConfig()
	work := &miner.AuxWork{Hash: common.HexToHash("0x1234"), Height: 10, Timestamp: 1700000000, Difficulty: big.NewInt(1), Target: big.NewInt(1)}
	out := formatAuxBlockWork(work, cfg)
	blob, err := json.Marshal(out)
	if err != nil {
		t.Fatal(err)
	}
	var fields map[string]interface{}
	if err := json.Unmarshal(blob, &fields); err != nil {
		t.Fatal(err)
	}
	if fields["timestamp"] != "0x6553f100" || fields["templateMaxValidity"] != "0x12c" || fields["workHash"] != work.Hash.Hex() {
		t.Fatalf("wrong RPC metadata: %s", blob)
	}
}

func integrationForgeConfig() *params.ChainConfig {
	return &params.ChainConfig{
		ChainId: big.NewInt(70088), ProgPoW: &params.ProgPoWConfig{Period: 10}, ForgeBlock: big.NewInt(10),
		Forge: &params.ForgeConfig{
			AuxChainID: 0x4F47, DogeAuxChainID: 0x0062, TargetBlockTime: 12,
			PreForgeGrossIssued: big.NewInt(1), InitialDifficulty: big.NewInt(1), MinimumDifficulty: big.NewInt(1),
			TemplateSigners: []common.Address{common.HexToAddress("0x1")}, TemplateQuorum: 1,
			LTCPayoutScriptHash: common.HexToHash("0x1"), DOGEPayoutScriptHash: common.HexToHash("0x2"), TemplateMaxValidity: 300,
		},
	}
}

func TestCreateConsensusEngineWrapsForgeOnlyWhenConfigured(t *testing.T) {
	powcfg := ethash.Config{PowMode: ethash.ModeFake}

	legacyCfg := *params.TestnetProgPoWChainConfig
	legacy := CreateConsensusEngine(nil, &powcfg, &legacyCfg, nil)
	if _, ok := legacy.(*progpow.ProgPoW); !ok {
		t.Fatalf("FORGE-disabled ProgPoW config wrapped unexpectedly: %T", legacy)
	}

	fcfg := integrationForgeConfig()
	wrapped := CreateConsensusEngine(nil, &powcfg, fcfg, nil)
	if _, ok := wrapped.(*forge.Hybrid); !ok {
		t.Fatalf("FORGE-enabled ProgPoW config not wrapped: %T", wrapped)
	}
}

func TestCanonicalBlockRewardRPCUsesConsensusPackage(t *testing.T) {
	cfg := integrationForgeConfig()
	cfg.ForgeBlock = big.NewInt(100)
	cfg.Forge.PreForgeGrossIssued = big.NewInt(1000000000000000000)
	for _, height := range []uint64{1, 99, 100, 101} {
		got := canonicalBlockReward(cfg, height)
		want := oggreward.RewardBreakdown(cfg, new(big.Int).SetUint64(height))
		if (*big.Int)(got.Total).Cmp(want.Total) != 0 || (*big.Int)(got.Miner).Cmp(want.Miner) != 0 ||
			(*big.Int)(got.Staking).Cmp(want.Staking) != 0 || (*big.Int)(got.Tribe).Cmp(want.Tribe) != 0 ||
			(*big.Int)(got.Maintenance).Cmp(want.Maintenance) != 0 {
			t.Fatalf("RPC reward diverged from consensus at height %d", height)
		}
	}
}

func TestForgeMiningRPCSignaturesRegister(t *testing.T) {
	server := rpc.NewServer()
	defer server.Stop()
	if err := server.RegisterName("eth", &PublicMinerAPI{}); err != nil {
		t.Fatalf("FORGE mining RPC registration failed: %v", err)
	}
}
