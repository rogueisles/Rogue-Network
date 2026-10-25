package core

import (
	"encoding/json"
	"github.com/TeamEGEM/go-egem/core/types"
	"github.com/TeamEGEM/go-egem/ethdb"
	"github.com/TeamEGEM/go-egem/params"
	"io/ioutil"
	"math/big"
	"testing"
)

func TestOptionalSOQFutureConfigPreservesGenesisDatabaseAndHead(t *testing.T) {
	b, e := ioutil.ReadFile("../genesis.json")
	if e != nil {
		t.Fatal(e)
	}
	var old Genesis
	if e = json.Unmarshal(b, &old); e != nil {
		t.Fatal(e)
	}
	old.Config.ForgeBlock = big.NewInt(1100100)
	old.Config.Forge = &params.ForgeConfig{RewardSchedule: params.RogueRewardSchedule, RequireDGBOrDOGE: true, RequireMulti: true, AllowAuxTree: true}
	want := old.ToBlock(nil).Hash()
	db, _ := ethdb.NewMemDatabase()
	defer db.Close()
	if _, hash, e := SetupGenesisBlock(db, &old); e != nil || hash != want {
		t.Fatal(e)
	}
	head := &types.Header{Number: big.NewInt(1103300), Difficulty: big.NewInt(1), Time: big.NewInt(1700000000)}
	WriteHeader(db, head)
	WriteHeadHeaderHash(db, head.Hash())
	before := GetHeadHeaderHash(db)
	updated := old
	c := *old.Config
	f := *old.Config.Forge
	f.OptionalSOQBlock = big.NewInt(1125600)
	c.Forge = &f
	updated.Config = &c
	if updated.ToBlock(nil).Hash() != want {
		t.Fatal("optional fork changed genesis")
	}
	if cfg, hash, e := SetupGenesisBlock(db, &updated); e != nil || hash != want || cfg.Forge.OptionalSOQBlock.Cmp(big.NewInt(1125600)) != 0 {
		t.Fatal("future config rejected", e)
	}
	if GetHeadHeaderHash(db) != before {
		t.Fatal("future config rewound imported history")
	}
	if cfg, _, e := SetupGenesisBlock(db, nil); e != nil || cfg.Forge.OptionalSOQBlock == nil {
		t.Fatal("ordinary restart lost future config", e)
	}
	head.Number = big.NewInt(1125600)
	WriteHeader(db, head)
	WriteHeadHeaderHash(db, head.Hash())
	before = GetHeadHeaderHash(db)
	if _, _, e := SetupGenesisBlock(db, &old); e == nil {
		t.Fatal("active SOQ rule removed")
	}
	if GetHeadHeaderHash(db) != before {
		t.Fatal("conflicting startup rewound chain")
	}
	stored, e := GetChainConfig(db, want)
	if e != nil || stored.Forge.OptionalSOQBlock == nil {
		t.Fatal("conflict overwrote stored consensus", e)
	}
}
