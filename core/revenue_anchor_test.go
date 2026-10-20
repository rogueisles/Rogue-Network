package core

import (
	"github.com/TeamEGEM/go-egem/core/types"
	"github.com/TeamEGEM/go-egem/ethdb"
	"github.com/TeamEGEM/go-egem/params"
	"math/big"
	"testing"
)

func TestRevenueAnchorGenesisConfigCannotChangeAfterBlockOne(t *testing.T) {
	db, _ := ethdb.NewMemDatabase()
	defer db.Close()
	g := &Genesis{Config: &params.ChainConfig{ChainId: big.NewInt(7008914), ForgeBlock: big.NewInt(1), Forge: &params.ForgeConfig{RequireMulti: true, AllowAuxTree: true}}, Difficulty: big.NewInt(1), GasLimit: 8000000}
	block := g.MustCommit(db)
	updated := *g
	cfg := *g.Config
	forge := *cfg.Forge
	forge.RequireDGBOrDOGE = true
	cfg.Forge = &forge
	updated.Config = &cfg
	if _, hash, err := SetupGenesisBlock(db, &updated); err != nil || hash != block.Hash() {
		t.Fatal("genesis-only policy update failed", err)
	}
	header := &types.Header{Number: big.NewInt(1), Difficulty: big.NewInt(1), Time: big.NewInt(1)}
	WriteHeader(db, header)
	WriteHeadHeaderHash(db, header.Hash())
	if _, _, err := SetupGenesisBlock(db, g); err == nil {
		t.Fatal("active anchor policy disabled through init")
	}
	stored, err := GetChainConfig(db, block.Hash())
	if err != nil || !stored.Forge.RequireDGBOrDOGE {
		t.Fatal("rejected init overwrote stored anchor policy", err)
	}
}
