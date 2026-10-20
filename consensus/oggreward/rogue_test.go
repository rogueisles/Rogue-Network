package oggreward

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io/ioutil"
	"math/big"
	"testing"

	"github.com/TeamEGEM/go-egem/common"
	"github.com/TeamEGEM/go-egem/core/state"
	"github.com/TeamEGEM/go-egem/core/types"
	"github.com/TeamEGEM/go-egem/ethdb"
	"github.com/TeamEGEM/go-egem/params"
)

func TestRogueIndependentLongRangeVectors(t *testing.T) {
	data, err := ioutil.ReadFile("testdata/rogue-vectors.json")
	if err != nil {
		t.Fatal(err)
	}
	var fixture struct {
		Vectors []struct {
			Offset    int64
			RewardWei string
		}
	}
	if err = json.Unmarshal(data, &fixture); err != nil {
		t.Fatal(err)
	}
	for _, v := range fixture.Vectors {
		if got := RogueBlockReward(big.NewInt(v.Offset)); got.String() != v.RewardWei {
			t.Fatalf("offset %d got %s want %s", v.Offset, got, v.RewardWei)
		}
	}
}

// This is an issuance-prefix verification, not transaction/state replay.
func TestRogueHistoricalIssuancePrefix(t *testing.T) {
	if testing.Short() {
		t.Skip("full 1,100,099 reward prefix requested separately")
	}
	sum := new(big.Int)
	for n := int64(1); n < params.RogueActivationHeight; n++ {
		reward, certified := certifiedGeometric(LegacyInitialReward, LegacyDecayNumerator, LegacyDecayDenominator, big.NewInt(n))
		if !certified {
			t.Fatalf("uncertified historical reward at %d", n)
		}
		sum.Add(sum, reward)
	}
	sum.Add(sum, GenesisGrossIssued).Add(sum, HistoricalTribeSeed)
	if sum.Cmp(roguePreIssued) != 0 {
		t.Fatalf("historical issuance snapshot mismatch: got %s want %s", sum, roguePreIssued)
	}
}

func rogueConfig() *params.ChainConfig {
	return &params.ChainConfig{ChainId: big.NewInt(70088), EIP158Block: big.NewInt(0), ForgeBlock: big.NewInt(1100100), Forge: &params.ForgeConfig{RewardSchedule: params.RogueRewardSchedule, PreForgeGrossIssued: new(big.Int).Set(roguePreIssued)}}
}

func TestRogueRewardsAndHistoricalBoundary(t *testing.T) {
	cfg := rogueConfig()
	if got := BlockReward(cfg, big.NewInt(1100099)); got.Cmp(bi("650290966654543546603")) != 0 {
		t.Fatalf("legacy fork predecessor changed: %s", got)
	}
	b := RewardBreakdown(cfg, big.NewInt(1100100))
	for name, v := range map[string]struct {
		got  *big.Int
		want string
	}{
		"total": {b.Total, "1100"}, "mining": {b.Miner, "440"}, "staking": {b.Staking, "330"}, "p2e": {b.P2E, "220"}, "treasury": {b.Treasury, "110"}, "tribe": {b.Tribe, "0"}, "maintenance": {b.Maintenance, "0"},
	} {
		if v.got.Cmp(mulOGG(v.want)) != 0 {
			t.Fatalf("%s=%s", name, v.got)
		}
	}
	if BlockReward(cfg, big.NewInt(1100101)).Cmp(b.Total) >= 0 {
		t.Fatal("subsidy did not decline")
	}
	legacy := RewardBreakdown(cfg, big.NewInt(1100099))
	if legacy.P2E.Sign() != 0 || legacy.Treasury.Sign() != 0 || legacy.Tribe.Sign() == 0 {
		t.Fatal("new split leaked into history")
	}
	// A configuration without the new schedule retains its reward curve and split.
	d14 := &params.ChainConfig{ForgeBlock: big.NewInt(1), Forge: &params.ForgeConfig{PreForgeGrossIssued: GenesisGrossIssued}}
	d14b := RewardBreakdown(d14, big.NewInt(1100100))
	if d14b.Tribe.Sign() == 0 || d14b.P2E.Sign() != 0 {
		t.Fatal("D14 rewards changed")
	}
}

func TestRogueSpecialAllocationBoundaries(t *testing.T) {
	total := new(big.Int)
	for _, n := range []int64{1100099, 1100100, 1100109, 1100111, 1319249, 1319251, 8989499, 8989501, 9208650} {
		if RogueSpecialIssuance(big.NewInt(n)).Sign() != 0 {
			t.Fatalf("unexpected allocation at %d", n)
		}
	}
	if got := RogueSpecialIssuance(big.NewInt(1100110)); got.Cmp(mulOGG("300000000")) != 0 {
		t.Fatal(got)
	}
	total.Add(total, mulOGG("300000000"))
	for j := uint64(1); j <= 36; j++ {
		n := uint64(1100100) + j*219150
		got := RogueSpecialIssuance(new(big.Int).SetUint64(n))
		if got.Cmp(mulOGG("50000000")) != 0 {
			t.Fatalf("release %d=%s", j, got)
		}
		total.Add(total, got)
	}
	if total.Cmp(rogueSpecialAllowance) != 0 {
		t.Fatal("special issuance total", total)
	}
	for _, n := range []*big.Int{nil, big.NewInt(-1), new(big.Int).Lsh(big.NewInt(1), 80)} {
		if RogueSpecialIssuance(n).Sign() != 0 {
			t.Fatal("invalid height minted coins")
		}
	}
}

func TestRogueLifetimeCapProof(t *testing.T) {
	budget := RogueNormalBudget()
	if budget.Cmp(bi("7156658799291511116642406018")) != 0 {
		t.Fatal("normal budget", budget)
	}
	remaining := new(big.Int).Sub(budget, roguePreTailIssued)
	full, partial := new(big.Int), new(big.Int)
	full.QuoRem(remaining, rogueTailReward, partial)
	if full.String() != "19518160" || partial.String() != "2010331721973212466" {
		t.Fatal("tail budget partition changed", full, partial)
	}
	paid := new(big.Int).Mul(full, rogueTailReward)
	paid.Add(paid, partial).Add(paid, roguePreTailIssued)
	if paid.Cmp(budget) != 0 {
		t.Fatal("finite schedule does not exhaust normal allowance")
	}
	issued := new(big.Int).Add(roguePreIssued, rogueSpecialAllowance)
	issued.Add(issued, RogueLifetimeUpperBound())
	if issued.Cmp(rogueCap) != 0 {
		t.Fatal("finite schedule does not exactly reach gross cap")
	}
	// Returning a budget value must not allow a caller to alter constants.
	budget.SetInt64(0)
	if RogueNormalBudget().Sign() <= 0 {
		t.Fatal("mutable budget exposed")
	}
}

func TestRogueCappedTailBoundaries(t *testing.T) {
	start := new(big.Int).SetUint64(params.RogueTailStartOffset)
	before := new(big.Int).Sub(start, big.NewInt(1))
	if params.RogueTailStartHeight != 13580767 || RogueBlockReward(before).String() != "100000012649744717650" {
		t.Fatal("last declining reward changed")
	}
	curve, certified := certifiedGeometric(rogueInitialReward, rogueDecayNumerator, rogueDecayDenominator, start)
	if !certified || curve.String() != "99999993436866019700" || curve.Cmp(rogueTailReward) > 0 {
		t.Fatal("tail does not start at first threshold crossing")
	}
	for _, offset := range []int64{12480667, 12480668, 26298000, 31998826} {
		if RogueBlockReward(big.NewInt(offset)).Cmp(rogueTailReward) != 0 {
			t.Fatal("tail not fixed at 100", offset)
		}
	}
	if RogueBlockReward(big.NewInt(31998827)).String() != "2010331721973212466" {
		t.Fatal("wrong final partial reward")
	}
	for _, offset := range []*big.Int{big.NewInt(31998828), big.NewInt(400000000), new(big.Int).Lsh(big.NewInt(1), 100)} {
		if RogueBlockReward(offset).Sign() != 0 {
			t.Fatal("subsidy reopened after exhaustion", offset)
		}
	}
	copyReward := RogueBlockReward(start)
	copyReward.SetInt64(0)
	if RogueBlockReward(start).Cmp(rogueTailReward) != 0 {
		t.Fatal("caller mutated tail constant")
	}
}

// Outward-rounded recurrence encloses every exact rational reward. Equality of
// the two integer floors certifies each term of the compiled issuance prefix.
func TestRogueExactPreTailIssuancePrefix(t *testing.T) {
	if testing.Short() {
		t.Skip("exhaustive declining-prefix certification requested separately")
	}
	const bits = 384
	lo := new(big.Int).Lsh(new(big.Int).Set(rogueInitialReward), bits)
	hi := new(big.Int).Set(lo)
	lower, upper, total := new(big.Int), new(big.Int), new(big.Int)
	ceil := new(big.Int).Sub(rogueDecayDenominator, big.NewInt(1))
	digest := sha256.New()
	var encoded [16]byte
	for n := uint64(0); n < params.RogueTailStartOffset; n++ {
		lower.Rsh(lo, bits)
		upper.Rsh(hi, bits)
		if lower.Cmp(upper) != 0 {
			t.Fatalf("ambiguous floor at %d", n)
		}
		total.Add(total, lower)
		lower.FillBytes(encoded[:])
		digest.Write(encoded[:])
		if n%10000 == 0 && RogueBlockReward(new(big.Int).SetUint64(n)).Cmp(lower) != 0 {
			t.Fatalf("runtime reward differs at %d", n)
		}
		lo.Mul(lo, rogueDecayNumerator).Quo(lo, rogueDecayDenominator)
		hi.Mul(hi, rogueDecayNumerator).Add(hi, ceil).Quo(hi, rogueDecayDenominator)
	}
	if total.Cmp(roguePreTailIssued) != 0 {
		t.Fatal("compiled prefix differs from exact floors", total)
	}
	if hex.EncodeToString(digest.Sum(nil)) != "682939cf440811061341524ab7cdb092d705a1fd11e62ce71c838031156b7171" {
		t.Fatal("per-block reward sequence changed")
	}
}

func TestRogueStateCreditsReorgAndContractPreservation(t *testing.T) {
	db, err := ethdb.NewMemDatabase()
	if err != nil {
		t.Fatal(err)
	}
	st, err := state.New(common.Hash{}, state.NewDatabase(db))
	if err != nil {
		t.Fatal(err)
	}
	cfg := rogueConfig()
	miner := common.HexToAddress("0xBEEF")
	st.SetCode(stakingAddress, []byte{0x60, 0x00})
	st.SetCode(tribeAddress, []byte{0x60, 0x01})
	key, value := common.HexToHash("0x123"), common.HexToHash("0x456")
	st.SetState(stakingAddress, key, value)
	st.SetState(tribeAddress, key, value)
	st.AddBalance(tribeAddress, big.NewInt(777))
	st.AddBalance(stakingAddress, big.NewInt(888))
	apply := func(n int64) { AccumulateRewards(cfg, st, &types.Header{Number: big.NewInt(n), Coinbase: miner}) }
	apply(1100100)
	if got := st.GetBalance(maintenanceAddress); got.Cmp(mulOGG("110")) != 0 {
		t.Fatal("300M credited at activation", got)
	}
	for _, n := range []int64{1100110, 1319250, 8989500} {
		before := st.GetBalance(maintenanceAddress)
		snap := st.Snapshot()
		apply(n)
		expected := new(big.Int).Add(before, RewardBreakdown(cfg, big.NewInt(n)).Treasury)
		expected.Add(expected, RogueSpecialIssuance(big.NewInt(n)))
		if st.GetBalance(maintenanceAddress).Cmp(expected) != 0 {
			t.Fatal("special credit missing", n)
		}
		root1 := st.Copy().IntermediateRoot(true)
		st.RevertToSnapshot(snap)
		if st.GetBalance(maintenanceAddress).Cmp(before) != 0 {
			t.Fatal("reorg retained allocation", n)
		}
		apply(n)
		root2 := st.Copy().IntermediateRoot(true)
		if root1 != root2 {
			t.Fatal("reexecution differs", n)
		}
	}
	if st.GetBalance(tribeAddress).Cmp(big.NewInt(777)) != 0 {
		t.Fatal("Tribe balance altered")
	}
	if st.GetState(stakingAddress, key) != value || st.GetState(tribeAddress, key) != value {
		t.Fatal("contract storage altered")
	}
	if len(st.GetCode(stakingAddress)) != 2 || len(st.GetCode(tribeAddress)) != 2 {
		t.Fatal("contract code altered")
	}
	// Commit/reopen simulates restart at the resulting state, without a paid flag.
	root, err := st.Commit(true)
	if err != nil {
		t.Fatal(err)
	}
	if err := st.Database().TrieDB().Commit(root, false); err != nil {
		t.Fatal(err)
	}
	reopened, err := state.New(root, state.NewDatabase(db))
	if err != nil {
		t.Fatal(err)
	}
	if reopened.GetBalance(maintenanceAddress).Cmp(st.GetBalance(maintenanceAddress)) != 0 {
		t.Fatal("restart balance changed")
	}
}

func TestCertifiedLegacyRewardMatchesExact(t *testing.T) {
	for _, n := range []int64{0, 1, 2, 114, 115, 4099, 4100, 11465, 20999, 21000, 21001, 21002} {
		got := LegacyBlockReward(big.NewInt(n))
		want := legacyBlockRewardExact(big.NewInt(n))
		if got.Cmp(want) != 0 {
			t.Fatalf("height %d got %s want %s", n, got, want)
		}
	}
}

func TestRogueExactArithmeticAndTail(t *testing.T) {
	for _, n := range []int64{0, 1, 2, 10, 100, 1000} {
		v, ok := certifiedGeometric(rogueInitialReward, rogueDecayNumerator, rogueDecayDenominator, big.NewInt(n))
		if !ok {
			t.Fatal("uncertified", n)
		}
		num := new(big.Int).Exp(rogueDecayNumerator, big.NewInt(n), nil)
		den := new(big.Int).Exp(rogueDecayDenominator, big.NewInt(n), nil)
		want := num.Mul(num, rogueInitialReward).Quo(num, den)
		if v.Cmp(want) != 0 {
			t.Fatal("exact reward mismatch", n)
		}
	}
	prev := new(big.Int).Set(rogueInitialReward)
	for n := int64(0); n <= 400000000; n += 1000000 {
		got := RogueBlockReward(big.NewInt(n))
		if got.Sign() < 0 || got.Cmp(prev) > 0 {
			t.Fatal("reward increased", n)
		}
		prev = got
	}
	if RogueBlockReward(big.NewInt(400000000)).Sign() != 0 {
		t.Fatal("tail not exhausted to wei")
	}
	for _, total := range []*big.Int{big.NewInt(1), big.NewInt(99), big.NewInt(100), bi("1100000000000000000000")} {
		b := splitRogueReward(total)
		sum := new(big.Int).Add(b.Miner, b.Staking)
		sum.Add(sum, b.P2E).Add(sum, b.Treasury)
		if sum.Cmp(total) != 0 || b.Tribe.Sign() != 0 || b.Maintenance.Sign() != 0 {
			t.Fatal("split mismatch")
		}
	}
}
