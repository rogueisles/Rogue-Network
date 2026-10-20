package forge

import (
	"errors"
	"math/big"
	"testing"

	"github.com/TeamEGEM/go-egem/common"
	"github.com/TeamEGEM/go-egem/consensus"
	"github.com/TeamEGEM/go-egem/core/state"
	"github.com/TeamEGEM/go-egem/core/types"
	"github.com/TeamEGEM/go-egem/params"
	"github.com/TeamEGEM/go-egem/rpc"
)

type hybridTestChain struct {
	config  *params.ChainConfig
	headers map[uint64]*types.Header
}

func (c *hybridTestChain) Config() *params.ChainConfig { return c.config }
func (c *hybridTestChain) CurrentHeader() *types.Header {
	var best *types.Header
	for _, h := range c.headers {
		if best == nil || h.Number.Cmp(best.Number) > 0 {
			best = h
		}
	}
	return best
}
func (c *hybridTestChain) GetHeader(hash common.Hash, number uint64) *types.Header {
	h := c.headers[number]
	if h != nil && h.Hash() == hash {
		return types.CopyHeader(h)
	}
	return nil
}
func (c *hybridTestChain) GetHeaderByNumber(number uint64) *types.Header {
	if h := c.headers[number]; h != nil {
		return types.CopyHeader(h)
	}
	return nil
}
func (c *hybridTestChain) GetHeaderByHash(hash common.Hash) *types.Header {
	for _, h := range c.headers {
		if h.Hash() == hash {
			return types.CopyHeader(h)
		}
	}
	return nil
}
func (c *hybridTestChain) GetBlock(hash common.Hash, number uint64) *types.Block { return nil }

type hybridLegacyStub struct {
	verified []uint64
}

func (e *hybridLegacyStub) Author(h *types.Header) (common.Address, error) { return h.Coinbase, nil }
func (e *hybridLegacyStub) VerifyHeader(chain consensus.ChainReader, h *types.Header, seal bool) error {
	if h.Number == nil || h.Number.Sign() <= 0 {
		return consensus.ErrUnknownAncestor
	}
	if chain.GetHeader(h.ParentHash, h.Number.Uint64()-1) == nil {
		return consensus.ErrUnknownAncestor
	}
	e.verified = append(e.verified, h.Number.Uint64())
	return nil
}
func (e *hybridLegacyStub) VerifyHeaders(chain consensus.ChainReader, hs []*types.Header, seals []bool) (chan<- struct{}, <-chan error) {
	abort := make(chan struct{})
	out := make(chan error, len(hs))
	go func() {
		for i, h := range hs {
			seal := i < len(seals) && seals[i]
			out <- e.VerifyHeader(chain, h, seal)
		}
	}()
	return abort, out
}
func (e *hybridLegacyStub) VerifyUncles(consensus.ChainReader, *types.Block) error { return nil }
func (e *hybridLegacyStub) VerifySeal(consensus.ChainReader, *types.Header) error  { return nil }
func (e *hybridLegacyStub) Prepare(consensus.ChainReader, *types.Header) error     { return nil }
func (e *hybridLegacyStub) Finalize(consensus.ChainReader, *types.Header, *state.StateDB, []*types.Transaction, []*types.Header, []*types.Receipt) (*types.Block, error) {
	return nil, errors.New("unused")
}
func (e *hybridLegacyStub) Seal(consensus.ChainReader, *types.Block, <-chan struct{}) (*types.Block, error) {
	return nil, errors.New("unused")
}
func (e *hybridLegacyStub) CalcDifficulty(consensus.ChainReader, uint64, *types.Header) *big.Int {
	return big.NewInt(777)
}
func (e *hybridLegacyStub) APIs(consensus.ChainReader) []rpc.API { return nil }

func TestHybridVerifyHeadersAcrossForgeBoundary(t *testing.T) {
	cfg, _, _ := testForgeConfig(t)
	cfg.ForgeBlock = big.NewInt(2)
	cfg.Forge.InitialDifficulty = big.NewInt(100)
	cfg.Forge.MinimumDifficulty = big.NewInt(1)

	genesis := &types.Header{
		Number: big.NewInt(0), Time: big.NewInt(1000), Difficulty: big.NewInt(9),
		GasLimit: 8000000, UncleHash: types.EmptyUncleHash,
	}
	chain := &hybridTestChain{config: cfg, headers: map[uint64]*types.Header{0: genesis}}
	legacy := new(hybridLegacyStub)
	hybrid, err := NewHybrid(cfg, legacy)
	if err != nil {
		t.Fatal(err)
	}

	h1 := &types.Header{
		ParentHash: genesis.Hash(), Number: big.NewInt(1), Time: big.NewInt(1010),
		Difficulty: big.NewInt(777), GasLimit: 8000000, UncleHash: types.EmptyUncleHash,
	}
	h2 := &types.Header{
		ParentHash: h1.Hash(), Number: big.NewInt(2), Time: big.NewInt(1022),
		Difficulty: big.NewInt(100), GasLimit: 8000000, UncleHash: types.EmptyUncleHash,
	}
	abort, results := hybrid.VerifyHeaders(chain, []*types.Header{h1, h2}, []bool{false, false})
	defer close(abort)
	if err := <-results; err != nil {
		t.Fatalf("legacy side of cross-fork batch rejected: %v", err)
	}
	if err := <-results; err != nil {
		t.Fatalf("FORGE side of cross-fork batch rejected: %v", err)
	}
	if len(legacy.verified) != 1 || legacy.verified[0] != 1 {
		t.Fatalf("legacy engine routing = %v, want only block 1", legacy.verified)
	}
	if got := hybrid.CalcDifficulty(chain, 1010, genesis); got.Cmp(big.NewInt(777)) != 0 {
		t.Fatalf("pre-fork difficulty routed incorrectly: %v", got)
	}
	if got := hybrid.CalcDifficulty(chain, 1022, h1); got.Cmp(big.NewInt(100)) != 0 {
		t.Fatalf("first FORGE difficulty = %v want 100", got)
	}
}

func TestHybridRejectsForgeUncles(t *testing.T) {
	cfg, _, _ := testForgeConfig(t)
	cfg.ForgeBlock = big.NewInt(1)
	hybrid, err := NewHybrid(cfg, new(hybridLegacyStub))
	if err != nil {
		t.Fatal(err)
	}
	h := &types.Header{Number: big.NewInt(1), UncleHash: types.EmptyUncleHash}
	uncle := &types.Header{Number: big.NewInt(0)}
	block := types.NewBlockWithHeader(h).WithBody(nil, []*types.Header{uncle})
	if err := hybrid.VerifyUncles(nil, block); err == nil {
		t.Fatal("hybrid accepted uncle after FORGE activation")
	}
}

func TestForgeDevelopmentPresetIsComplete(t *testing.T) {
	if err := ValidateConfig(params.ForgeDevChainConfig.Forge); err != nil {
		t.Fatalf("FORGE development preset is incomplete: %v", err)
	}
}
