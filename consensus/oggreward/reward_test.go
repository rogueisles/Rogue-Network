package oggreward

import (
	"math/big"
	"testing"

	"github.com/TeamEGEM/go-egem/common"
	"github.com/TeamEGEM/go-egem/core/state"
	"github.com/TeamEGEM/go-egem/core/types"
	"github.com/TeamEGEM/go-egem/ethdb"
	"github.com/TeamEGEM/go-egem/params"
)

func bi(s string) *big.Int {
	v, ok := new(big.Int).SetString(s, 10)
	if !ok {
		panic(s)
	}
	return v
}

func TestLegacyRewardVectorBlock11465(t *testing.T) {
	got := LegacyBlockReward(big.NewInt(11465))
	want := bi("699462834763147213756")
	if got.Cmp(want) != 0 {
		t.Fatalf("legacy reward mismatch: got %s want %s", got, want)
	}
	b := SplitReward(got)
	vectors := []struct {
		name string
		got  *big.Int
		want string
	}{
		{"miner", b.Miner, "314758275643416246192"},
		{"staking", b.Staking, "279785133905258885502"},
		{"tribe", b.Tribe, "48962398433420304962"},
		{"maintenance", b.Maintenance, "55957026781051777100"},
	}
	for _, v := range vectors {
		if v.got.Cmp(bi(v.want)) != 0 {
			t.Fatalf("%s mismatch: got %s want %s", v.name, v.got, v.want)
		}
	}
}

func TestLiveRewardAddressHardFork(t *testing.T) {
	oldStaking := common.HexToAddress("0xCd442d7AC675D6c637a960e10913e341508C6672")
	oldTribe := common.HexToAddress("0xfeaD066Caa900F210B19B9df14aBc38B46a15e66")
	newStaking := common.HexToAddress("0xa47008c59f729756bEc7d01f6FE71328A242d0c4")
	newTribe := common.HexToAddress("0x085CF5da09842FA3BA01068CC02c156198b1b114")
	maintenance := common.HexToAddress("0x85ea896411EdFE9dD7fa6F4F5FaA19D2D81cdA5E")

	s, tr, m := RewardDestinations(big.NewInt(20999))
	if s != oldStaking || tr != oldTribe || m != maintenance {
		t.Fatalf("pre-fork destinations wrong: %s %s %s", s.Hex(), tr.Hex(), m.Hex())
	}
	s, tr, m = RewardDestinations(big.NewInt(21000))
	if s != newStaking || tr != newTribe || m != maintenance {
		t.Fatalf("post-fork destinations wrong: %s %s %s", s.Hex(), tr.Hex(), m.Hex())
	}
}

func TestHistoricalTribeSeedCountsTowardGrossIssuance(t *testing.T) {
	if got := HistoricalExtraIssuanceThrough(big.NewInt(21000)); got.Sign() != 0 {
		t.Fatalf("seed counted too early: %s", got)
	}
	got := HistoricalExtraIssuanceThrough(big.NewInt(21001))
	if got.Cmp(HistoricalTribeSeed) != 0 {
		t.Fatalf("seed mismatch: got %s want %s", got, HistoricalTribeSeed)
	}
}

func TestHistoricalTribeSeedAppliedToState(t *testing.T) {
	db, err := ethdb.NewMemDatabase()
	if err != nil {
		t.Fatal(err)
	}
	st, err := state.New(common.Hash{}, state.NewDatabase(db))
	if err != nil {
		t.Fatal(err)
	}
	coinbase := common.HexToAddress("0x000000000000000000000000000000000000BEEF")
	header := &types.Header{Number: big.NewInt(21001), Coinbase: coinbase}
	cfg := &params.ChainConfig{}
	AccumulateRewards(cfg, st, header)

	_, tribe, _ := RewardDestinations(header.Number)
	normal := SplitReward(LegacyBlockReward(header.Number)).Tribe
	want := new(big.Int).Add(normal, HistoricalTribeSeed)
	if got := st.GetBalance(tribe); got.Cmp(want) != 0 {
		t.Fatalf("tribe state balance mismatch: got %s want %s", got, want)
	}
}

func TestForgeBoundaryAndBudget(t *testing.T) {
	pre := mulOGG("1256950000")
	cfg := &params.ChainConfig{
		ForgeBlock: big.NewInt(1000),
		Forge: &params.ForgeConfig{
			AuxChainID:          0x4F47,
			DogeAuxChainID:      0x0062,
			TargetBlockTime:     12,
			PreForgeGrossIssued: new(big.Int).Set(pre),
		},
	}
	if got, want := BlockReward(cfg, big.NewInt(999)), LegacyBlockReward(big.NewInt(999)); got.Cmp(want) != 0 {
		t.Fatalf("pre-fork reward changed: got %s want %s", got, want)
	}
	initial := ForgeInitialReward(pre)
	if got := BlockReward(cfg, big.NewInt(1000)); got.Cmp(initial) != 0 {
		t.Fatalf("activation reward mismatch: got %s want %s", got, initial)
	}
	if next := BlockReward(cfg, big.NewInt(1001)); next.Cmp(initial) >= 0 {
		t.Fatalf("FORGE reward did not decay: initial %s next %s", initial, next)
	}

	budget := ForgeRemainingBudget(pre)
	upper := ForgeLifetimeEmissionUpperBound(pre)
	if upper.Cmp(budget) > 0 {
		t.Fatalf("post-FORGE upper bound exceeds remaining budget: upper %s budget %s", upper, budget)
	}
	if new(big.Int).Add(new(big.Int).Set(pre), upper).Cmp(GrossSupplyCap) > 0 {
		t.Fatal("gross issuance can exceed 10.7B cap")
	}
}

func TestForgeThreeYearHalfLife(t *testing.T) {
	pre := GenesisGrossIssued
	initial := ForgeInitialReward(pre)
	// q was chosen for ~3 years at 12 seconds/block: ~7,889,403 blocks.
	half := ForgeBlockReward(pre, big.NewInt(7889403))
	// Integer fixed-point squaring floors slightly; require within 0.1% of half.
	twice := new(big.Int).Mul(half, big.NewInt(2))
	diff := new(big.Int).Sub(twice, initial)
	if diff.Sign() < 0 {
		diff.Neg(diff)
	}
	permille := new(big.Int).Mul(diff, big.NewInt(1000))
	permille.Div(permille, initial)
	if permille.Cmp(big.NewInt(1)) > 0 {
		t.Fatalf("half-life outside 0.1%%: initial=%s half=%s", initial, half)
	}
}

func TestForgeCapExhaustionFailsClosed(t *testing.T) {
	if got := ForgeBlockReward(GrossSupplyCap, big.NewInt(0)); got.Sign() != 0 {
		t.Fatalf("reward after exhausted cap: %s", got)
	}
	cfg := &params.ChainConfig{
		ForgeBlock: big.NewInt(10),
		Forge:      &params.ForgeConfig{PreForgeGrossIssued: nil},
	}
	if got := BlockReward(cfg, big.NewInt(10)); got.Sign() != 0 {
		t.Fatalf("missing frozen gross issuance must fail closed, got %s", got)
	}
}

func TestRewardSplitAlwaysReconciles(t *testing.T) {
	for _, total := range []*big.Int{big.NewInt(1), big.NewInt(99), big.NewInt(100), bi("829701234567890123456")} {
		b := SplitReward(total)
		sum := new(big.Int).Add(b.Miner, b.Staking)
		sum.Add(sum, b.Tribe)
		sum.Add(sum, b.Maintenance)
		if sum.Cmp(total) != 0 {
			t.Fatalf("split does not reconcile for %s: got %s", total, sum)
		}
	}
}
