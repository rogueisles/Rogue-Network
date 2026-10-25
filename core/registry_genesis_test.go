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

func TestRegistrySchedulePreservesGenesisAndNeverRewindsMainnet(t *testing.T) {
	raw, err := ioutil.ReadFile("../genesis.json")
	if err != nil {
		t.Fatal(err)
	}
	var old Genesis
	if err = json.Unmarshal(raw, &old); err != nil {
		t.Fatal(err)
	}
	old.Config.ForgeBlock = big.NewInt(1100100)
	old.Config.Forge = &params.ForgeConfig{RewardSchedule: params.RogueRewardSchedule, RequireDGBOrDOGE: true, RequireMulti: true, AllowAuxTree: true}
	db, _ := ethdb.NewMemDatabase()
	defer db.Close()
	_, genesis, err := SetupGenesisBlock(db, &old)
	if err != nil {
		t.Fatal(err)
	}
	head := &types.Header{Number: big.NewInt(1150000), Difficulty: big.NewInt(1), Time: big.NewInt(1700000000)}
	WriteHeader(db, head)
	WriteHeadHeaderHash(db, head.Hash())
	updated := old
	c := *old.Config
	f := *old.Config.Forge
	f.RegistryBlock = big.NewInt(1200000)
	f.RegistryBootstrap = []byte{1, 2, 3}
	f.RegistryMinDelay = 720
	f.RegistryMinOperationInterval = 64
	c.Forge = &f
	updated.Config = &c
	if updated.ToBlock(nil).Hash() != genesis {
		t.Fatal("registry profile changed genesis")
	}
	if _, _, err = SetupGenesisBlock(db, &updated); err != nil {
		t.Fatal(err)
	}
	if GetHeadHeaderHash(db) != head.Hash() {
		t.Fatal("scheduling registry rewound current head")
	}
	head.Number = big.NewInt(1200000)
	WriteHeader(db, head)
	WriteHeadHeaderHash(db, head.Hash())
	if _, _, err = SetupGenesisBlock(db, &old); err == nil {
		t.Fatal("active registry can be removed")
	}
	if GetHeadHeaderHash(db) != head.Hash() {
		t.Fatal("incompatible startup rewound history")
	}
	stored, err := GetChainConfig(db, genesis)
	if err != nil || stored.Forge.RegistryBlock == nil {
		t.Fatal("refused startup overwrote stored profile", err)
	}
}
