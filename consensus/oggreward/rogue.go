package oggreward

import (
	"math/big"

	"github.com/TeamEGEM/go-egem/common"
	"github.com/TeamEGEM/go-egem/params"
)

// These constants define the activated reward schedule and issuance limits.
// Burns do not reopen either the normal budget or reserved special issuance.
var (
	rogueCap              = mulOGG("10700000000")
	roguePreIssued        = mustBig(params.RoguePreForgeGrossIssuedWei)
	rogueSpecialAllowance = mulOGG("2100000000")
	rogueInitialReward    = mulOGG("1100")
	rogueDecayNumerator   = mustBig("999999807871237324305429716870248633")
	rogueDecayDenominator = mustBig("1000000000000000000000000000000000000")
	rogueTailReward       = mulOGG("100")
	// Exact sum of independently floored rewards at offsets [0,12480667).
	// The offline checker and exhaustive interval test certify this prefix.
	roguePreTailIssued = mustBig("5204842797281179394669193552")
	rogueP2EAddress    = common.HexToAddress("0x03D50e192Bd14ec9f536DF00Db7Ae6723Bdb8BcD")
)

func RogueNormalBudget() *big.Int {
	return new(big.Int).Sub(new(big.Int).Sub(rogueCap, roguePreIssued), rogueSpecialAllowance)
}

// RogueLifetimeUpperBound is exact for the finite capped schedule. The final
// partial subsidy consumes the normal allowance without touching special credits.
func RogueLifetimeUpperBound() *big.Int {
	return RogueNormalBudget()
}

func RogueBlockReward(offset *big.Int) *big.Int {
	if offset == nil || offset.Sign() < 0 {
		return new(big.Int)
	}
	tailStart := new(big.Int).SetUint64(params.RogueTailStartOffset)
	if offset.Cmp(tailStart) >= 0 {
		// Prior tail payments depend only on height. Wallet spending, burns,
		// restarts and reorganisations cannot reopen the issuance allowance.
		paid := new(big.Int).Mul(new(big.Int).Sub(offset, tailStart), rogueTailReward)
		remaining := new(big.Int).Sub(RogueNormalBudget(), roguePreTailIssued)
		remaining.Sub(remaining, paid)
		if remaining.Sign() <= 0 {
			return new(big.Int)
		}
		if remaining.Cmp(rogueTailReward) < 0 {
			return remaining
		}
		return new(big.Int).Set(rogueTailReward)
	}
	if reward, ok := certifiedGeometric(rogueInitialReward, rogueDecayNumerator, rogueDecayDenominator, offset); ok {
		return reward
	}
	// Exact fallback, as on the preserved legacy path; never use floats or an
	// uncertified fixed-point approximation to select a consensus reward.
	n := new(big.Int).Exp(rogueDecayNumerator, offset, nil)
	d := new(big.Int).Exp(rogueDecayDenominator, offset, nil)
	return n.Mul(n, rogueInitialReward).Quo(n, d)
}

// RogueSpecialIssuance is based solely on canonical height. State execution
// and reorg rollback handle credits atomically; no cron or mutable paid flag.
func RogueSpecialIssuance(height *big.Int) *big.Int {
	if height == nil || !height.IsUint64() {
		return new(big.Int)
	}
	n := height.Uint64()
	if n == params.RogueInitialAllocationHeight {
		return mulOGG("300000000")
	}
	if n <= uint64(params.RogueActivationHeight) {
		return new(big.Int)
	}
	offset := n - uint64(params.RogueActivationHeight)
	if offset%params.RogueMonthlyInterval == 0 && offset/params.RogueMonthlyInterval <= params.RogueMonthlyReleaseCount {
		return mulOGG("50000000")
	}
	return new(big.Int)
}

func splitRogueReward(total *big.Int) Breakdown {
	b := Breakdown{Total: new(big.Int), Miner: new(big.Int), Staking: new(big.Int), Tribe: new(big.Int), Maintenance: new(big.Int), P2E: new(big.Int), Treasury: new(big.Int)}
	if total == nil || total.Sign() <= 0 {
		return b
	}
	b.Total.Set(total)
	b.Staking.Quo(new(big.Int).Mul(total, big.NewInt(30)), big.NewInt(100))
	b.P2E.Quo(new(big.Int).Mul(total, big.NewInt(20)), big.NewInt(100))
	b.Treasury.Quo(new(big.Int).Mul(total, big.NewInt(10)), big.NewInt(100))
	// Miner absorbs integer wei dust, preserving the historical convention.
	b.Miner.Sub(total, b.Staking).Sub(b.Miner, b.P2E).Sub(b.Miner, b.Treasury)
	return b
}
