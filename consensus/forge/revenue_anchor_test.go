package forge

import (
 "testing"
 "math/big"
 "github.com/TeamEGEM/go-egem/common"
)

func TestRevenueAnchorCannotUseLegacyProofConfiguration(t *testing.T) {
 cfg,_,_:=testForgeConfig(t)
 cfg.Forge.RequireDGBOrDOGE=true
 if _,err:=New(cfg);err==nil {t.Fatal("anchor policy accepted legacy proof mode")}
 cfg.ChainId=big.NewInt(7008914);cfg.ForgeBlock=big.NewInt(1)
 cfg.Forge.AllowAuxTree=true;cfg.Forge.RequireMulti=true
 cfg.Forge.PepecoinPayoutScriptHash=common.HexToHash("1")
 cfg.Forge.DingocoinPayoutScriptHash=common.HexToHash("2")
 cfg.Forge.BellscoinPayoutScriptHash=common.HexToHash("3")
 if _,err:=New(cfg);err!=nil {t.Fatal("anchored multi-chain configuration rejected",err)}
}
