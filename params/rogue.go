package params

import (
	"errors"
	"github.com/TeamEGEM/go-egem/common"
	"math/big"
)

// CheckRogueHistoricalRules rejects a reward profile paired with an incompatible
// historical engine or EVM configuration before it can be stored or selected.
func (c *ChainConfig) CheckRogueHistoricalRules() error {
	if c != nil && c.Forge != nil && c.Forge.RewardSchedule == RogueRewardSchedule && !c.HasRogueHistoricalRules() {
		return errors.New("RGN schedule requires the preserved historical ProgPoW and EVM configuration")
	}
	return nil
}

// Branding is metadata. Existing signing IDs, genesis, addresses and wire
// domains are intentionally retained; an RGN unit is the former OGG unit.
const (
	RogueNetworkName                    = "Rogue Network"
	RogueCurrencySymbol                 = "RGN"
	RogueCurrencyDecimals               = 18
	RogueMainnetChainID          int64  = 70088
	RogueActivationHeight        int64  = 1100100
	RogueInitialAllocationHeight uint64 = 1100110
	RogueMonthlyInterval         uint64 = 219150
	RogueMonthlyReleaseCount     uint64 = 36
	RogueRewardSchedule                 = "rgn-mainnet-v2"
	RogueTailStartOffset         uint64 = 12480667
	RogueTailStartHeight         uint64 = uint64(RogueActivationHeight) + RogueTailStartOffset
	RoguePreForgeGrossIssuedWei         = "1443341200708488883357593982"
)

func (c *ChainConfig) IsRogue(num *big.Int) bool {
	return c != nil && c.IsForge(num) && c.Forge.RewardSchedule == RogueRewardSchedule
}

// ValidateRogueSchedule protects the chain identity and fixed issuance
// snapshot. Proof signers, payout commitments and difficulty still require
// the full engine validation. Default configurations remain unchanged.
func (c *ChainConfig) ValidateRogueSchedule() bool {
	return c.HasRogueHistoricalRules() &&
		c.ForgeBlock != nil && c.ForgeBlock.Cmp(big.NewInt(RogueActivationHeight)) == 0 &&
		c.Forge != nil && c.Forge.RewardSchedule == RogueRewardSchedule &&
		c.Forge.PreForgeGrossIssued != nil && c.Forge.PreForgeGrossIssued.String() == RoguePreForgeGrossIssuedWei &&
		!c.Forge.PrivateCPUTest && c.Forge.AllowAuxTree && c.Forge.RequireMulti &&
		c.Forge.RequireDGBOrDOGE && c.Forge.RetargetV2 && c.Forge.TargetBlockTime == 12
}

// The genesis block hash does not commit ChainConfig. Check the live historical
// configuration as well, so a correct genesis cannot disguise changed old rules.
func (c *ChainConfig) HasRogueHistoricalRules() bool {
	zero := func(n *big.Int) bool { return n != nil && n.Sign() == 0 }
	return c != nil && c.ChainId != nil && c.ChainId.Cmp(big.NewInt(RogueMainnetChainID)) == 0 &&
		zero(c.HomesteadBlock) && c.DAOForkBlock == nil && !c.DAOForkSupport &&
		zero(c.EIP150Block) && c.EIP150Hash == (common.Hash{}) && zero(c.EIP155Block) && zero(c.EIP158Block) &&
		zero(c.ByzantiumBlock) && zero(c.ConstantinopleBlock) &&
		c.ProgPoW != nil && c.ProgPoW.Period == 10 && c.Ethash == nil && c.Clique == nil
}
