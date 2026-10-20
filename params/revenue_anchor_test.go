package params

import (
	"github.com/TeamEGEM/go-egem/common"
	"math/big"
	"testing"
)

func TestRevenueAnchorAndMultiTreasuryConfigImmutable(t *testing.T) {
	for name, mutate := range map[string]func(*ForgeConfig){
		"anchor":         func(c *ForgeConfig) { c.RequireDGBOrDOGE = true },
		"multi":          func(c *ForgeConfig) { c.RequireMulti = true },
		"tree":           func(c *ForgeConfig) { c.AllowAuxTree = true },
		"pep treasury":   func(c *ForgeConfig) { c.PepecoinPayoutScriptHash = common.HexToHash("1") },
		"dingo treasury": func(c *ForgeConfig) { c.DingocoinPayoutScriptHash = common.HexToHash("2") },
		"bells treasury": func(c *ForgeConfig) { c.BellscoinPayoutScriptHash = common.HexToHash("3") },
	} {
		t.Run(name, func(t *testing.T) {
			old := &ChainConfig{ForgeBlock: big.NewInt(1), Forge: &ForgeConfig{}}
			changed := *old
			fc := *old.Forge
			mutate(&fc)
			changed.Forge = &fc
			if err := old.CheckCompatible(&changed, 0); err != nil {
				t.Fatal("pre-activation change rejected", err)
			}
			if err := old.CheckCompatible(&changed, 1); err == nil {
				t.Fatal("active consensus change accepted")
			}
		})
	}
}
