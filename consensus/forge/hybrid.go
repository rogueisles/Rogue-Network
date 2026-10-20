package forge

import (
	"errors"
	"math/big"

	"github.com/TeamEGEM/go-egem/common"
	"github.com/TeamEGEM/go-egem/consensus"
	"github.com/TeamEGEM/go-egem/core/state"
	"github.com/TeamEGEM/go-egem/core/types"
	"github.com/TeamEGEM/go-egem/params"
	"github.com/TeamEGEM/go-egem/rpc"
)

// Hybrid preserves the historical consensus engine before ForgeBlock and
// switches atomically to FORGE for ForgeBlock and every block after it.
type Hybrid struct {
	config *params.ChainConfig
	legacy consensus.Engine
	forge  *Forge
}

func NewHybrid(config *params.ChainConfig, legacy consensus.Engine) (*Hybrid, error) {
	if config == nil {
		return nil, errors.New("nil chain config")
	}
	if legacy == nil {
		return nil, errors.New("nil legacy consensus engine")
	}
	if config.ForgeBlock == nil || config.Forge == nil {
		return nil, errors.New("FORGE is not configured")
	}
	post, err := New(config)
	if err != nil {
		return nil, err
	}
	return &Hybrid{config: config, legacy: legacy, forge: post}, nil
}

func (h *Hybrid) IsForge(number *big.Int) bool {
	return h != nil && h.config != nil && h.config.IsForge(number)
}

func (h *Hybrid) engineFor(number *big.Int) consensus.Engine {
	if h.IsForge(number) {
		return h.forge
	}
	return h.legacy
}

func (h *Hybrid) Author(header *types.Header) (common.Address, error) {
	if header == nil {
		return common.Address{}, errors.New("nil header")
	}
	return h.engineFor(header.Number).Author(header)
}

func (h *Hybrid) VerifyHeader(chain consensus.ChainReader, header *types.Header, seal bool) error {
	if header == nil || header.Number == nil {
		return consensus.ErrUnknownAncestor
	}
	return h.engineFor(header.Number).VerifyHeader(chain, header, seal)
}

// batchChain overlays already-verified batch headers on the backing chain so a
// VerifyHeaders request can safely cross the ProgPoW -> FORGE fork boundary.
type batchChain struct {
	consensus.ChainReader
	byHash   map[common.Hash]*types.Header
	byNumber map[uint64]*types.Header
}

func newBatchChain(chain consensus.ChainReader) *batchChain {
	return &batchChain{
		ChainReader: chain,
		byHash:      make(map[common.Hash]*types.Header),
		byNumber:    make(map[uint64]*types.Header),
	}
}

func (c *batchChain) add(header *types.Header) {
	if header == nil || header.Number == nil {
		return
	}
	cpy := types.CopyHeader(header)
	c.byHash[cpy.Hash()] = cpy
	c.byNumber[cpy.Number.Uint64()] = cpy
}

func (c *batchChain) GetHeader(hash common.Hash, number uint64) *types.Header {
	if h := c.byHash[hash]; h != nil && h.Number.Uint64() == number {
		return types.CopyHeader(h)
	}
	return c.ChainReader.GetHeader(hash, number)
}

func (c *batchChain) GetHeaderByNumber(number uint64) *types.Header {
	if h := c.byNumber[number]; h != nil {
		return types.CopyHeader(h)
	}
	return c.ChainReader.GetHeaderByNumber(number)
}

func (c *batchChain) GetHeaderByHash(hash common.Hash) *types.Header {
	if h := c.byHash[hash]; h != nil {
		return types.CopyHeader(h)
	}
	return c.ChainReader.GetHeaderByHash(hash)
}

func (h *Hybrid) VerifyHeaders(chain consensus.ChainReader, headers []*types.Header, seals []bool) (chan<- struct{}, <-chan error) {
	abort := make(chan struct{})
	results := make(chan error, len(headers))
	go func() {
		overlay := newBatchChain(chain)
		for i, header := range headers {
			seal := i < len(seals) && seals[i]
			var err error
			if header == nil || header.Number == nil {
				err = consensus.ErrUnknownAncestor
			} else {
				err = h.engineFor(header.Number).VerifyHeader(overlay, header, seal)
				if err == nil {
					overlay.add(header)
				}
			}
			select {
			case <-abort:
				return
			case results <- err:
			}
		}
	}()
	return abort, results
}

func (h *Hybrid) VerifyUncles(chain consensus.ChainReader, block *types.Block) error {
	if block == nil {
		return errors.New("nil block")
	}
	return h.engineFor(block.Number()).VerifyUncles(chain, block)
}

func (h *Hybrid) VerifySeal(chain consensus.ChainReader, header *types.Header) error {
	if header == nil || header.Number == nil {
		return consensus.ErrUnknownAncestor
	}
	return h.engineFor(header.Number).VerifySeal(chain, header)
}

func (h *Hybrid) Prepare(chain consensus.ChainReader, header *types.Header) error {
	if header == nil || header.Number == nil {
		return consensus.ErrUnknownAncestor
	}
	return h.engineFor(header.Number).Prepare(chain, header)
}

func (h *Hybrid) Finalize(chain consensus.ChainReader, header *types.Header, statedb *state.StateDB, txs []*types.Transaction, uncles []*types.Header, receipts []*types.Receipt) (*types.Block, error) {
	if header == nil || header.Number == nil {
		return nil, consensus.ErrUnknownAncestor
	}
	return h.engineFor(header.Number).Finalize(chain, header, statedb, txs, uncles, receipts)
}

func (h *Hybrid) Seal(chain consensus.ChainReader, block *types.Block, stop <-chan struct{}) (*types.Block, error) {
	if block == nil {
		return nil, errors.New("nil block")
	}
	return h.engineFor(block.Number()).Seal(chain, block, stop)
}

func (h *Hybrid) CalcDifficulty(chain consensus.ChainReader, timestamp uint64, parent *types.Header) *big.Int {
	if parent == nil || parent.Number == nil {
		return new(big.Int)
	}
	next := new(big.Int).Add(parent.Number, big.NewInt(1))
	return h.engineFor(next).CalcDifficulty(chain, timestamp, parent)
}

func (h *Hybrid) APIs(chain consensus.ChainReader) []rpc.API {
	apis := append([]rpc.API(nil), h.legacy.APIs(chain)...)
	apis = append(apis, h.forge.APIs(chain)...)
	return apis
}

func (h *Hybrid) Hashrate() float64 {
	if pow, ok := h.legacy.(consensus.PoW); ok {
		return pow.Hashrate()
	}
	return 0
}

// SetThreads preserves the legacy miner-control interface. It only controls the
// historical ProgPoW engine; FORGE itself is externally merged mined.
func (h *Hybrid) SetThreads(threads int) {
	if threaded, ok := h.legacy.(interface{ SetThreads(int) }); ok {
		threaded.SetThreads(threads)
	}
}

// SeedHash exists only for legacy GetWork compatibility. FORGE mining never
// uses a DAG/seed hash and the remote agent rejects legacy GetWork post-fork.
func (h *Hybrid) SeedHash(blockNumber uint64) []byte {
	if sh, ok := h.legacy.(consensus.SeedHasher); ok {
		return sh.SeedHash(blockNumber)
	}
	return nil
}
