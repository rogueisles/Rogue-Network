// Package oggreward contains the canonical OGG native issuance and reward split
// rules shared by every consensus engine. Keeping issuance here avoids subtle
// drift between the historical ProgPoW engine and the post-fork FORGE engine.
package oggreward

import (
	"math/big"

	"github.com/TeamEGEM/go-egem/common"
	"github.com/TeamEGEM/go-egem/core/state"
	"github.com/TeamEGEM/go-egem/core/types"
	"github.com/TeamEGEM/go-egem/params"
)

var (
	weiPerOGG = mustBig("1000000000000000000")

	// GrossSupplyCap is a lifetime issuance ceiling. Burns never reopen this
	// allowance; it is intentionally based on gross creation, not totalSupply.
	GrossSupplyCap = mulOGG("10700000000")

	// GenesisGrossIssued is the OGG created in genesis and therefore already
	// consumes part of the lifetime gross issuance cap.
	GenesisGrossIssued = mulOGG("700000000")

	// Historical S3 Enhanced ProgPoW emission.
	LegacyInitialReward    = mulOGG("700")
	LegacyDecayNumerator   = big.NewInt(999999933042)
	LegacyDecayDenominator = big.NewInt(1000000000000)

	// FORGE uses Q18 fixed-point decay. This factor gives an approximately
	// three-year half-life at a 12-second target spacing.
	ForgeDecayNumerator   = mustBig("999999912142000000")
	ForgeDecayDenominator = mustBig("1000000000000000000")

	// OGG reward destinations before and after the live block-21000 migration.
	legacyStakingAddress = common.HexToAddress("0xCd442d7AC675D6c637a960e10913e341508C6672")
	legacyTribeAddress   = common.HexToAddress("0xfeaD066Caa900F210B19B9df14aBc38B46a15e66")
	stakingAddress       = common.HexToAddress("0xa47008c59f729756bEc7d01f6FE71328A242d0c4")
	tribeAddress         = common.HexToAddress("0x085CF5da09842FA3BA01068CC02c156198b1b114")
	maintenanceAddress   = common.HexToAddress("0x85ea896411EdFE9dD7fa6F4F5FaA19D2D81cdA5E")

	// The live chain made a one-time 950,000 OGG allocation to the Tribe at
	// block 21001. It is native issuance and must therefore count toward the
	// 10.7B gross cap when PreForgeGrossIssued is frozen at activation.
	HistoricalTribeSeed = mulOGG("950000")
)

const (
	RewardAddressForkBlock   uint64 = 21000
	HistoricalTribeSeedBlock uint64 = 21001
)

// Breakdown is the deterministic four-way split of one normal block reward.
// Historical one-time issuance is deliberately not folded into Total.
type Breakdown struct {
	Total       *big.Int
	Miner       *big.Int
	Staking     *big.Int
	Tribe       *big.Int
	Maintenance *big.Int
	P2E         *big.Int
	Treasury    *big.Int
}

func mustBig(s string) *big.Int {
	v, ok := new(big.Int).SetString(s, 10)
	if !ok {
		panic("invalid OGG consensus integer: " + s)
	}
	return v
}

func mulOGG(s string) *big.Int {
	v := mustBig(s)
	return new(big.Int).Mul(v, weiPerOGG)
}

// LegacyBlockReward reproduces the historical OGG reward curve exactly:
// 700 OGG * (999999933042 / 1e12)^blockNumber, floored to wei.
func LegacyBlockReward(blockNum *big.Int) *big.Int {
	if blockNum == nil || blockNum.Sign() < 0 {
		return new(big.Int)
	}
	if reward, certified := certifiedGeometric(LegacyInitialReward, LegacyDecayNumerator, LegacyDecayDenominator, blockNum); certified {
		return reward
	}
	return legacyBlockRewardExact(blockNum)
}

// The certified fast path proves the same wei result as this original exact
// rational path. The fallback is retained to preserve historical consensus.
func legacyBlockRewardExact(blockNum *big.Int) *big.Int {
	if blockNum == nil || blockNum.Sign() < 0 {
		return new(big.Int)
	}
	reward := new(big.Int).Set(LegacyInitialReward)
	if blockNum.Sign() == 0 {
		return reward
	}

	// Preserve the original exact rational binary exponentiation. Do not use
	// floating point or fixed-point approximation on the historical path.
	numPow := big.NewInt(1)
	denPow := big.NewInt(1)
	numBase := new(big.Int).Set(LegacyDecayNumerator)
	denBase := new(big.Int).Set(LegacyDecayDenominator)
	n := new(big.Int).Set(blockNum)
	zero := new(big.Int)
	for n.Cmp(zero) > 0 {
		if n.Bit(0) == 1 {
			numPow.Mul(numPow, numBase)
			denPow.Mul(denPow, denBase)
		}
		n.Rsh(n, 1)
		if n.Sign() > 0 {
			numBase.Mul(numBase, numBase)
			denBase.Mul(denBase, denBase)
		}
	}
	reward.Mul(reward, numPow)
	reward.Div(reward, denPow)
	return reward
}

// RewardDestinations returns the consensus reward destinations active at the
// supplied height. The maintenance address did not change at the migration.
func RewardDestinations(blockNum *big.Int) (common.Address, common.Address, common.Address) {
	if blockNum != nil && blockNum.Sign() >= 0 && blockNum.Uint64() >= RewardAddressForkBlock {
		return stakingAddress, tribeAddress, maintenanceAddress
	}
	return legacyStakingAddress, legacyTribeAddress, maintenanceAddress
}

// Recipients identifies the destinations for normal subsidy allocations.
// Inactive allocations have the zero address.
type Recipients struct {
	Staking     common.Address
	Tribe       common.Address
	Maintenance common.Address
	P2E         common.Address
	Treasury    common.Address
}

// RewardRecipients uses the same height and configuration selection as reward
// execution, including the historical destination migration.
func RewardRecipients(config *params.ChainConfig, blockNum *big.Int) Recipients {
	if config != nil && config.IsRogue(blockNum) {
		return Recipients{Staking: stakingAddress, P2E: rogueP2EAddress, Treasury: maintenanceAddress}
	}
	staking, tribe, maintenance := RewardDestinations(blockNum)
	return Recipients{Staking: staking, Tribe: tribe, Maintenance: maintenance}
}

// HistoricalExtraIssuanceThrough returns native one-time issuance created on
// or before height. This is used when calculating the frozen pre-FORGE gross
// issuance value for the 10.7B lifetime cap.
func HistoricalExtraIssuanceThrough(height *big.Int) *big.Int {
	if height != nil && height.Sign() >= 0 && height.Uint64() >= HistoricalTribeSeedBlock {
		return new(big.Int).Set(HistoricalTribeSeed)
	}
	return new(big.Int)
}

// q18Pow computes base^exp in Q18 fixed point using exponentiation by squaring.
// Every multiplication floors deterministically, which is conservative for the
// capped emission schedule.
func q18Pow(base *big.Int, exp *big.Int) *big.Int {
	if exp == nil || exp.Sign() <= 0 {
		return new(big.Int).Set(ForgeDecayDenominator)
	}
	result := new(big.Int).Set(ForgeDecayDenominator)
	b := new(big.Int).Set(base)
	n := new(big.Int).Set(exp)
	for n.Sign() > 0 {
		if n.Bit(0) == 1 {
			result.Mul(result, b)
			result.Div(result, ForgeDecayDenominator)
		}
		n.Rsh(n, 1)
		if n.Sign() > 0 {
			b.Mul(b, b)
			b.Div(b, ForgeDecayDenominator)
		}
	}
	return result
}

// ForgeRemainingBudget returns the native issuance room available at the fork.
// A nil or out-of-range pre-issued value is treated conservatively as zero room.
func ForgeRemainingBudget(preForgeGrossIssued *big.Int) *big.Int {
	if preForgeGrossIssued == nil || preForgeGrossIssued.Sign() < 0 || preForgeGrossIssued.Cmp(GrossSupplyCap) >= 0 {
		return new(big.Int)
	}
	return new(big.Int).Sub(GrossSupplyCap, preForgeGrossIssued)
}

// ForgeInitialReward derives the first FORGE block reward from the remaining
// gross issuance budget. For an infinite geometric series, R0 = B*(1-q), so
// the full post-fork schedule cannot exceed B even before per-block flooring.
func ForgeInitialReward(preForgeGrossIssued *big.Int) *big.Int {
	budget := ForgeRemainingBudget(preForgeGrossIssued)
	if budget.Sign() == 0 {
		return budget
	}
	delta := new(big.Int).Sub(ForgeDecayDenominator, ForgeDecayNumerator)
	r := new(big.Int).Mul(budget, delta)
	r.Div(r, ForgeDecayDenominator)
	return r
}

// ForgeLifetimeEmissionUpperBound returns the geometric-series upper bound for
// the configured post-fork emission. It must never exceed RemainingBudget.
func ForgeLifetimeEmissionUpperBound(preForgeGrossIssued *big.Int) *big.Int {
	initial := ForgeInitialReward(preForgeGrossIssued)
	if initial.Sign() == 0 {
		return initial
	}
	delta := new(big.Int).Sub(ForgeDecayDenominator, ForgeDecayNumerator)
	upper := new(big.Int).Mul(initial, ForgeDecayDenominator)
	upper.Div(upper, delta)
	return upper
}

// ForgeBlockReward returns the reward at offset blocks after activation.
func ForgeBlockReward(preForgeGrossIssued *big.Int, offset *big.Int) *big.Int {
	if offset == nil || offset.Sign() < 0 {
		return new(big.Int)
	}
	initial := ForgeInitialReward(preForgeGrossIssued)
	if initial.Sign() == 0 || offset.Sign() == 0 {
		return initial
	}
	factor := q18Pow(ForgeDecayNumerator, offset)
	reward := new(big.Int).Mul(initial, factor)
	reward.Div(reward, ForgeDecayDenominator)
	return reward
}

// BlockReward selects the exact historical or FORGE schedule at the hard-fork
// boundary. A configured FORGE activation requires a frozen gross-issued value.
func BlockReward(config *params.ChainConfig, blockNum *big.Int) *big.Int {
	if config != nil && config.IsRogue(blockNum) {
		return RogueBlockReward(new(big.Int).Sub(blockNum, config.ForgeBlock))
	}
	if config == nil || !config.IsForge(blockNum) {
		return LegacyBlockReward(blockNum)
	}
	if config.Forge == nil || config.ForgeBlock == nil || config.Forge.PreForgeGrossIssued == nil {
		return new(big.Int)
	}
	offset := new(big.Int).Sub(new(big.Int).Set(blockNum), config.ForgeBlock)
	return ForgeBlockReward(config.Forge.PreForgeGrossIssued, offset)
}

// SplitReward performs the immutable 45/40/7/8 split. The miner receives the
// remainder after the three fixed integer percentages and therefore absorbs
// any rounding dust.
func SplitReward(total *big.Int) Breakdown {
	if total == nil || total.Sign() <= 0 {
		z := new(big.Int)
		return Breakdown{Total: new(big.Int), Miner: new(big.Int), Staking: new(big.Int), Tribe: new(big.Int), Maintenance: z, P2E: new(big.Int), Treasury: new(big.Int)}
	}
	staking := new(big.Int).Mul(total, big.NewInt(40))
	staking.Div(staking, big.NewInt(100))
	tribe := new(big.Int).Mul(total, big.NewInt(7))
	tribe.Div(tribe, big.NewInt(100))
	maintenance := new(big.Int).Mul(total, big.NewInt(8))
	maintenance.Div(maintenance, big.NewInt(100))
	miner := new(big.Int).Set(total)
	miner.Sub(miner, staking)
	miner.Sub(miner, tribe)
	miner.Sub(miner, maintenance)
	return Breakdown{
		Total:       new(big.Int).Set(total),
		Miner:       miner,
		Staking:     staking,
		Tribe:       tribe,
		Maintenance: maintenance,
		P2E:         new(big.Int),
		Treasury:    new(big.Int),
	}
}

// RewardBreakdown returns the normal per-block reward split for RPC/tests.
func RewardBreakdown(config *params.ChainConfig, blockNum *big.Int) Breakdown {
	if config != nil && config.IsRogue(blockNum) {
		return splitRogueReward(BlockReward(config, blockNum))
	}
	return SplitReward(BlockReward(config, blockNum))
}

// AccumulateRewards applies the canonical normal reward and the historical
// one-time Tribe issuance. OGG has no uncle/ommer reward in this path.
func AccumulateRewards(config *params.ChainConfig, statedb *state.StateDB, header *types.Header) {
	b := RewardBreakdown(config, header.Number)
	if config != nil && config.IsRogue(header.Number) {
		statedb.AddBalance(header.Coinbase, b.Miner)
		statedb.AddBalance(stakingAddress, b.Staking)
		statedb.AddBalance(rogueP2EAddress, b.P2E)
		treasury := new(big.Int).Add(b.Treasury, RogueSpecialIssuance(header.Number))
		statedb.AddBalance(maintenanceAddress, treasury)
		return
	}
	stakingDest, tribeDest, maintenanceDest := RewardDestinations(header.Number)
	statedb.AddBalance(header.Coinbase, b.Miner)
	statedb.AddBalance(stakingDest, b.Staking)
	statedb.AddBalance(tribeDest, b.Tribe)
	statedb.AddBalance(maintenanceDest, b.Maintenance)

	if header.Number != nil && header.Number.Sign() >= 0 && header.Number.Uint64() == HistoricalTribeSeedBlock {
		statedb.AddBalance(tribeDest, new(big.Int).Set(HistoricalTribeSeed))
	}
}
