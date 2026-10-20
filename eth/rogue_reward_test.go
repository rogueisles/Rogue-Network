package eth

import (
	"github.com/TeamEGEM/go-egem/params"
	"math/big"
	"testing"
)

func TestRogueRewardRPCSeparatesSpecialIssuance(t *testing.T) {
	cfg := integrationForgeConfig()
	cfg.ForgeBlock = big.NewInt(1100100)
	cfg.Forge.RewardSchedule = params.RogueRewardSchedule
	for _, row := range []struct {
		height  uint64
		special string
	}{{1100100, "0"}, {1100110, "300000000000000000000000000"}, {1319250, "50000000000000000000000000"}, {8989500, "50000000000000000000000000"}, {8989501, "0"}} {
		r := canonicalBlockReward(cfg, row.height)
		if (*big.Int)(r.SpecialIssuance).String() != row.special || r.CurrencySymbol != "RGN" || (*big.Int)(r.Tribe).Sign() != 0 || (*big.Int)(r.Maintenance).Sign() != 0 {
			t.Fatal("RPC changed issuance semantics", row.height)
		}
		sum := new(big.Int).Add((*big.Int)(r.Miner), (*big.Int)(r.Staking))
		sum.Add(sum, (*big.Int)(r.P2E)).Add(sum, (*big.Int)(r.Treasury))
		if sum.Cmp((*big.Int)(r.Total)) != 0 {
			t.Fatal("RPC subsidy includes special allocation", row.height)
		}
	}
	if canonicalBlockReward(cfg, 1100099).CurrencySymbol != "OGG" {
		t.Fatal("historical metadata changed")
	}
}
