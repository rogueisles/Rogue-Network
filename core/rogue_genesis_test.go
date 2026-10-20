package core

import (
	"encoding/json"
	"github.com/TeamEGEM/go-egem/common"
	"github.com/TeamEGEM/go-egem/core/types"
	"github.com/TeamEGEM/go-egem/ethdb"
	"github.com/TeamEGEM/go-egem/params"
	"io/ioutil"
	"math/big"
	"testing"
)

func TestRogueActualMainnetGenesisAndConfigGuard(t *testing.T) {
	blob, err := ioutil.ReadFile("../genesis.json")
	if err != nil {
		t.Fatal(err)
	}
	var g Genesis
	if err = json.Unmarshal(blob, &g); err != nil {
		t.Fatal(err)
	}
	want := common.HexToHash("0xdc4d38c0ef41d3fdf50642fdf5fc99e221eadd09e73bbe616724f55a136270ab")
	if g.ToBlock(nil).Hash() != want {
		t.Fatal("actual mainnet genesis changed")
	}
	db, _ := ethdb.NewMemDatabase()
	defer db.Close()
	if _, hash, err := SetupGenesisBlock(db, &g); err != nil || hash != want {
		t.Fatal(hash, err)
	}
	updated := g
	cfg := *g.Config
	cfg.ForgeBlock = big.NewInt(1100100)
	cfg.Forge = &params.ForgeConfig{RewardSchedule: params.RogueRewardSchedule}
	updated.Config = &cfg
	head := &types.Header{Number: big.NewInt(1100099), Difficulty: big.NewInt(1), Time: big.NewInt(1700000000)}
	WriteHeader(db, head)
	WriteHeadHeaderHash(db, head.Hash())
	if _, hash, err := SetupGenesisBlock(db, &updated); err != nil || hash != want {
		t.Fatal("pre-fork configuration rejected", hash, err)
	}
	head.Number = big.NewInt(1100100)
	WriteHeader(db, head)
	WriteHeadHeaderHash(db, head.Hash())
	if _, _, err := SetupGenesisBlock(db, &g); err == nil {
		t.Fatal("active mainnet fork could be silently removed")
	} else if _, rewindable := err.(*params.ConfigCompatError); rewindable {
		t.Fatal("active RGN downgrade returned an auto-rewindable startup error")
	}
	stored, err := GetChainConfig(db, want)
	if err != nil || stored.Forge.RewardSchedule != params.RogueRewardSchedule {
		t.Fatal("failed downgrade overwrote fork config", err)
	}
	for name, change := range map[string]func(*params.ChainConfig){
		"changed proof parameters": func(c *params.ChainConfig) { c.Forge.MinimumDifficulty = big.NewInt(123) },
		"moved activation":         func(c *params.ChainConfig) { c.ForgeBlock = big.NewInt(1100101) },
		"changed historical rules": func(c *params.ChainConfig) { c.HomesteadBlock = big.NewInt(1) },
		"added clique":             func(c *params.ChainConfig) { c.Clique = &params.CliqueConfig{Period: 12, Epoch: 30000} },
		"removed progpow":          func(c *params.ChainConfig) { c.ProgPoW = nil },
		"changed progpow period":   func(c *params.ChainConfig) { c.ProgPoW.Period = 11 },
		"added ethash":             func(c *params.ChainConfig) { c.Ethash = &params.EthashConfig{} },
	} {
		t.Run(name, func(t *testing.T) {
			b, _ := json.Marshal(updated.Config)
			var changed params.ChainConfig
			if err := json.Unmarshal(b, &changed); err != nil {
				t.Fatal(err)
			}
			change(&changed)
			next := updated
			next.Config = &changed
			_, _, err := SetupGenesisBlock(db, &next)
			if err == nil {
				t.Fatal("active consensus conflict accepted")
			}
			if _, rewindable := err.(*params.ConfigCompatError); rewindable {
				t.Fatal("conflict permits automatic rewind")
			}
			if GetHeadHeaderHash(db) != head.Hash() {
				t.Fatal("head changed on rejected configuration")
			}
			after, _ := GetChainConfig(db, want)
			x, _ := json.Marshal(stored)
			y, _ := json.Marshal(after)
			if string(x) != string(y) {
				t.Fatal("stored config changed after rejection")
			}
		})
	}
}

func TestRogueInvalidHistoricalEngineNeverStored(t *testing.T) {
	blob, err := ioutil.ReadFile("../genesis.json")
	if err != nil {
		t.Fatal(err)
	}
	for _, height := range []int64{0, 1100099, 1100100} {
		for _, removeProfile := range []bool{false, true} {
			var g Genesis
			if err := json.Unmarshal(blob, &g); err != nil {
				t.Fatal(err)
			}
			g.Config.ForgeBlock = big.NewInt(params.RogueActivationHeight)
			g.Config.Forge = &params.ForgeConfig{RewardSchedule: params.RogueRewardSchedule}
			db, _ := ethdb.NewMemDatabase()
			defer db.Close()
			_, hash, err := SetupGenesisBlock(db, &g)
			if err != nil {
				t.Fatal(err)
			}
			before, _ := GetChainConfig(db, hash)
			head := &types.Header{Number: big.NewInt(height), Difficulty: big.NewInt(1), Time: big.NewInt(1700000000)}
			WriteHeader(db, head)
			WriteHeadHeaderHash(db, head.Hash())
			g.Config.Clique = &params.CliqueConfig{Period: 12, Epoch: 30000}
			if removeProfile {
				g.Config.Forge, g.Config.ForgeBlock = nil, nil
			}
			if _, _, err := SetupGenesisBlock(db, &g); err == nil {
				t.Fatalf("incompatible historical engine accepted at %d", height)
			} else if _, ok := err.(*params.ConfigCompatError); ok {
				t.Fatal("historical engine error permits automatic rewind")
			}
			after, _ := GetChainConfig(db, hash)
			a, _ := json.Marshal(before)
			b, _ := json.Marshal(after)
			if string(a) != string(b) || GetHeadHeaderHash(db) != head.Hash() {
				t.Fatal("rejected config changed stored chain")
			}
			if !removeProfile {
				fresh, _ := ethdb.NewMemDatabase()
				if _, err := g.Commit(fresh); err == nil || GetCanonicalHash(fresh, 0) != (common.Hash{}) {
					t.Fatal("direct genesis commit stored invalid engine")
				}
				fresh.Close()
			}
		}
	}
}

// An ordinary startup supplies no genesis. Preserve an explicitly installed
// future RGN schedule instead of resetting it to the legacy default configuration.
func TestRogueRestartRetainsInstalledFutureSchedule(t *testing.T) {
	blob, err := ioutil.ReadFile("../genesis.json")
	if err != nil {
		t.Fatal(err)
	}
	var g Genesis
	if err = json.Unmarshal(blob, &g); err != nil {
		t.Fatal(err)
	}
	pre, _ := new(big.Int).SetString(params.RoguePreForgeGrossIssuedWei, 10)
	g.Config.ForgeBlock = big.NewInt(params.RogueActivationHeight)
	g.Config.Forge = &params.ForgeConfig{RewardSchedule: params.RogueRewardSchedule, PreForgeGrossIssued: pre,
		AllowAuxTree: true, RequireMulti: true, RequireDGBOrDOGE: true, RetargetV2: true, TargetBlockTime: 12}
	db, _ := ethdb.NewMemDatabase()
	defer db.Close()
	if _, _, err := SetupGenesisBlock(db, &g); err != nil {
		t.Fatal(err)
	}
	for _, height := range []int64{0, 1066800, 1100099, 1100100, 1100110} {
		head := &types.Header{Number: big.NewInt(height), Difficulty: big.NewInt(1), Time: big.NewInt(1700000000)}
		WriteHeader(db, head)
		WriteHeadHeaderHash(db, head.Hash())
		config, _, err := SetupGenesisBlock(db, nil)
		if err != nil {
			t.Fatalf("ordinary restart at %d: %v", height, err)
		}
		if config.Forge == nil || !config.ValidateRogueSchedule() {
			t.Fatalf("ordinary restart discarded installed RGN schedule at %d", height)
		}
	}
}
