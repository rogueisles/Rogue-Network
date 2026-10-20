package forge

import (
	"encoding/hex"
	"encoding/json"
	"github.com/TeamEGEM/go-egem/common"
	"github.com/TeamEGEM/go-egem/core/types"
	"github.com/TeamEGEM/go-egem/params"
	"github.com/TeamEGEM/go-egem/rlp"
	"io/ioutil"
	"math/big"
	"os"
	"testing"
)

// Binding validation uses independently generated native-lab proofs. Their
// reduced difficulty does not establish validity at production work targets.
func TestRogueNativeBindingsMatrix(t *testing.T) {
	path := os.Getenv("FORGE_MULTI_MATRIX_PROOFS")
	if path == "" {
		t.Skip("native matrix fixture required")
	}
	blob, err := ioutil.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var rows []struct {
		Mask             uint8
		Proof, HeaderRLP string
	}
	if err = json.Unmarshal(blob, &rows); err != nil || len(rows) != 31 {
		t.Fatal("invalid fixture", err)
	}
	c := rogueEngineConfig(t)
	dgb, _ := hex.DecodeString("0014706267a5255e3666936bcb9a777667599f387546")
	doge, _ := hex.DecodeString("76a9144b1508ebe06723842994adf33bdba36b4eba44c488ac")
	c.Forge.LTCPayoutScriptHash = ScriptHash(dgb)
	c.Forge.DOGEPayoutScriptHash = ScriptHash(doge)
	accepted := 0
	for _, row := range rows {
		hb, _ := hex.DecodeString(row.HeaderRLP)
		var h types.Header
		if err = rlp.DecodeBytes(hb, &h); err != nil {
			t.Fatal(err)
		}
		pb, _ := hex.DecodeString(row.Proof)
		p, e := DecodeProof(pb)
		if e != nil {
			t.Fatal(e)
		}
		e = verifyMultiBindings(c.Forge, &h, p.Multi)
		want := row.Mask == 1 || row.Mask == 2 || row.Mask == 3
		if (e == nil) != want {
			t.Fatalf("mask %d acceptance wrong: %v", row.Mask, e)
		}
		if e == nil {
			accepted++
		}
	}
	if accepted != 3 {
		t.Fatal("missing DGB-only, DOGE-only or both bindings")
	}
}

func rogueEngineConfig(t *testing.T) *params.ChainConfig {
	c, _, _ := testForgeConfig(t)
	c.ProgPoW = &params.ProgPoWConfig{Period: 10}
	c.HomesteadBlock = big.NewInt(0)
	c.EIP150Block = big.NewInt(0)
	c.EIP155Block = big.NewInt(0)
	c.EIP158Block = big.NewInt(0)
	c.ByzantiumBlock = big.NewInt(0)
	c.ConstantinopleBlock = big.NewInt(0)
	c.ForgeBlock = big.NewInt(params.RogueActivationHeight)
	c.Forge.RewardSchedule = params.RogueRewardSchedule
	c.Forge.PreForgeGrossIssued, _ = new(big.Int).SetString(params.RoguePreForgeGrossIssuedWei, 10)
	c.Forge.AllowAuxTree = true
	c.Forge.RequireMulti = true
	c.Forge.RequireDGBOrDOGE = true
	c.Forge.RetargetV2 = true
	return c
}

func TestRogueEngineConfigurationFailsClosed(t *testing.T) {
	c := rogueEngineConfig(t)
	if _, err := New(c); err != nil {
		t.Fatal(err)
	}
	for name, change := range map[string]func(*params.ChainConfig){
		"changed historical EVM fork": func(c *params.ChainConfig) { c.EIP158Block = big.NewInt(100) },
		"missing historical engine":   func(c *params.ChainConfig) { c.ProgPoW = nil },
		"testnet identity":            func(c *params.ChainConfig) { c.ChainId = big.NewInt(7008914) },
		"wrong activation":            func(c *params.ChainConfig) { c.ForgeBlock = big.NewInt(1100101) },
		"wrong historical issuance": func(c *params.ChainConfig) {
			c.Forge.PreForgeGrossIssued.Add(c.Forge.PreForgeGrossIssued, big.NewInt(1))
		},
		"anchor policy disabled": func(c *params.ChainConfig) { c.Forge.RequireDGBOrDOGE = false },
		"old proof enabled":      func(c *params.ChainConfig) { c.Forge.RequireMulti = false },
		"old retarget":           func(c *params.ChainConfig) { c.Forge.RetargetV2 = false },
		"private CPU":            func(c *params.ChainConfig) { c.Forge.PrivateCPUTest = true },
		"bonus chain":            func(c *params.ChainConfig) { c.Forge.PepecoinPayoutScriptHash = common.HexToHash("0x1") },
		"unknown schedule":       func(c *params.ChainConfig) { c.Forge.RewardSchedule = "typo" },
		"superseded schedule":    func(c *params.ChainConfig) { c.Forge.RewardSchedule = "rgn-mainnet-v1" },
	} {
		t.Run(name, func(t *testing.T) {
			cfg := rogueEngineConfig(t)
			change(cfg)
			if _, err := New(cfg); err == nil {
				t.Fatal("invalid mainnet configuration accepted")
			}
		})
	}
}

func TestRogueRejectsMissingAndLegacySealsAtActivation(t *testing.T) {
	cfg := rogueEngineConfig(t)
	legacy := new(hybridLegacyStub)
	hybrid, err := NewHybrid(cfg, legacy)
	if err != nil {
		t.Fatal(err)
	}
	for _, n := range []int64{1100100, 1100101} {
		h := &types.Header{Number: big.NewInt(n), Difficulty: big.NewInt(1), Time: big.NewInt(1700000000), Extra: []byte("old miner extra")}
		if err := hybrid.VerifySeal(nil, h); err == nil {
			t.Fatal("legacy proof admitted after fork")
		}
		if len(legacy.verified) != 0 {
			t.Fatal("post-fork header reached legacy engine")
		}
	}
	if !hybrid.IsForge(big.NewInt(1100100)) || hybrid.IsForge(big.NewInt(1100099)) {
		t.Fatal("hybrid fork boundary changed")
	}
}
