// Package forge implements the post-fork OGG FORGE Scrypt/AuxPoW consensus
// engine. It is intentionally separate from the historical ProgPoW engine;
// Phase 4 composes both engines at ForgeBlock.
package forge

import (
	"errors"
	"fmt"
	"math/big"
	"time"

	"github.com/TeamEGEM/go-egem/common"
	"github.com/TeamEGEM/go-egem/consensus"
	"github.com/TeamEGEM/go-egem/consensus/oggreward"
	"github.com/TeamEGEM/go-egem/core/state"
	"github.com/TeamEGEM/go-egem/core/types"
	"github.com/TeamEGEM/go-egem/params"
	"github.com/TeamEGEM/go-egem/rpc"
)

type Forge struct {
	config *params.ChainConfig
}

func New(config *params.ChainConfig) (*Forge, error) {
	if config == nil || config.ForgeBlock == nil {
		return nil, errors.New("FORGE activation block is not configured")
	}
	if err := validateConfig(config.Forge); err != nil {
		return nil, err
	}
	if config.Forge.RewardSchedule != "" && (config.Forge.RewardSchedule != params.RogueRewardSchedule || !config.ValidateRogueSchedule()) {
		return nil, errors.New("invalid RGN mainnet schedule, identity or anchor policy")
	}
	if config.Forge.PrivateCPUTest && (config.ChainId == nil || config.ChainId.Cmp(big.NewInt(7008902)) != 0 || config.ForgeBlock.Cmp(big.NewInt(1)) != 0) {
		return nil, errors.New("private CPU test requires chain ID 7008902 and FORGE activation at block 1")
	}
	if config.Forge.AllowAuxTree && !config.ValidateRogueSchedule() && (config.ChainId == nil || (config.ChainId.Cmp(big.NewInt(7008914)) != 0 && !(config.ChainId.Cmp(big.NewInt(7008902)) == 0 && config.Forge.PrivateCPUTest)) || config.ForgeBlock.Cmp(big.NewInt(1)) != 0) {
		return nil, errors.New("expanded auxiliary tree is isolated to D14 or private CPU lab")
	}
	return &Forge{config: config}, nil
}

// ValidateConfig exposes the fail-closed FORGE consensus configuration checks
// to integration code without constructing an engine.
func ValidateConfig(config *params.ForgeConfig) error { return validateConfig(config) }

func (f *Forge) Author(header *types.Header) (common.Address, error) { return header.Coinbase, nil }

func (f *Forge) VerifyHeader(chain consensus.ChainReader, header *types.Header, seal bool) error {
	if header == nil || header.Number == nil || header.Number.Sign() <= 0 {
		return consensus.ErrUnknownAncestor
	}
	if !f.config.IsForge(header.Number) {
		return errors.New("FORGE engine cannot verify pre-fork header")
	}
	parent := chain.GetHeader(header.ParentHash, header.Number.Uint64()-1)
	if parent == nil {
		return consensus.ErrUnknownAncestor
	}
	return f.verifyHeader(chain, header, parent, seal)
}

func (f *Forge) VerifyHeaders(chain consensus.ChainReader, headers []*types.Header, seals []bool) (chan<- struct{}, <-chan error) {
	abort := make(chan struct{})
	results := make(chan error, len(headers))
	go func() {
		for i, header := range headers {
			var parent *types.Header
			if i == 0 {
				if header.Number != nil && header.Number.Sign() > 0 {
					parent = chain.GetHeader(header.ParentHash, header.Number.Uint64()-1)
				}
			} else {
				parent = headers[i-1]
			}
			var err error
			if parent == nil {
				err = consensus.ErrUnknownAncestor
			} else if !f.config.IsForge(header.Number) {
				err = errors.New("FORGE engine cannot verify pre-fork header")
			} else {
				seal := i < len(seals) && seals[i]
				err = f.verifyHeader(chain, header, parent, seal)
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

func (f *Forge) verifyHeader(chain consensus.ChainReader, header, parent *types.Header, seal bool) error {
	if header.Time == nil || parent.Time == nil || header.Time.Cmp(parent.Time) <= 0 {
		return errors.New("timestamp not greater than parent")
	}
	// Candidate timestamps drive the retarget. Reject fabricated distant-future
	// headers instead of allowing arbitrary wall-clock-free difficulty drops.
	// Preserve historical verification behavior when v2 is not selected.
	if f.config.Forge.RetargetV2 {
		if !header.Time.IsUint64() || !parent.Time.IsUint64() {
			return errors.New("FORGE timestamp outside uint64 range")
		}
		if header.Time.Uint64() > uint64(time.Now().Unix())+15 {
			return consensus.ErrFutureBlock
		}
	}
	if d := new(big.Int).Sub(header.Number, parent.Number); d.Cmp(big.NewInt(1)) != 0 {
		return consensus.ErrInvalidNumber
	}
	expected := f.CalcDifficulty(chain, header.Time.Uint64(), parent)
	if expected == nil || header.Difficulty == nil || expected.Cmp(header.Difficulty) != 0 {
		return fmt.Errorf("invalid FORGE difficulty: have %v want %v", header.Difficulty, expected)
	}
	cap := uint64(0x7fffffffffffffff)
	if header.GasLimit > cap {
		return fmt.Errorf("invalid gasLimit: have %v max %v", header.GasLimit, cap)
	}
	if header.GasUsed > header.GasLimit {
		return fmt.Errorf("invalid gasUsed: have %d gasLimit %d", header.GasUsed, header.GasLimit)
	}
	delta := int64(parent.GasLimit) - int64(header.GasLimit)
	if delta < 0 {
		delta = -delta
	}
	limit := parent.GasLimit / params.GasLimitBoundDivisor
	if uint64(delta) >= limit || header.GasLimit < params.MinGasLimit {
		return fmt.Errorf("invalid gas limit: have %d want %d += %d", header.GasLimit, parent.GasLimit, limit)
	}
	if len(header.Extra) > MaxProofSize {
		return fmt.Errorf("FORGE extra-data too long: %d > %d", len(header.Extra), MaxProofSize)
	}
	if _, err := DecodeProof(header.Extra); err == ErrNoProof && len(header.Extra) > MaxMinerExtra {
		return fmt.Errorf("FORGE miner extra too long: %d > %d", len(header.Extra), MaxMinerExtra)
	} else if err != nil && err != ErrNoProof {
		return err
	}
	if seal {
		return f.VerifySeal(chain, header)
	}
	return nil
}

func (f *Forge) VerifyUncles(chain consensus.ChainReader, block *types.Block) error {
	if len(block.Uncles()) != 0 {
		return errors.New("FORGE blocks may not contain uncles")
	}
	if block.Header().UncleHash != types.EmptyUncleHash {
		return errors.New("invalid FORGE uncle hash")
	}
	return nil
}

func (f *Forge) VerifySeal(chain consensus.ChainReader, header *types.Header) error {
	p, err := DecodeProof(header.Extra)
	if err == ErrNoProof {
		return errors.New("missing FORGE AuxPoW proof")
	}
	if err != nil {
		return err
	}
	return verifyAuxPow(f.config.Forge, header, p)
}

func (f *Forge) Prepare(chain consensus.ChainReader, header *types.Header) error {
	if header == nil || header.Number == nil || header.Number.Sign() <= 0 {
		return consensus.ErrUnknownAncestor
	}
	parent := chain.GetHeader(header.ParentHash, header.Number.Uint64()-1)
	if parent == nil {
		return consensus.ErrUnknownAncestor
	}
	header.Difficulty = f.CalcDifficulty(chain, header.Time.Uint64(), parent)
	header.UncleHash = types.EmptyUncleHash
	return nil
}

func (f *Forge) Finalize(chain consensus.ChainReader, header *types.Header, statedb *state.StateDB, txs []*types.Transaction, uncles []*types.Header, receipts []*types.Receipt) (*types.Block, error) {
	if len(uncles) != 0 {
		return nil, errors.New("FORGE blocks may not contain uncles")
	}
	header.UncleHash = types.EmptyUncleHash
	oggreward.AccumulateRewards(chain.Config(), statedb, header)
	header.Root = statedb.IntermediateRoot(chain.Config().IsEIP158(header.Number))
	return types.NewBlock(header, txs, nil, receipts), nil
}

func (f *Forge) Seal(chain consensus.ChainReader, block *types.Block, stop <-chan struct{}) (*types.Block, error) {
	return nil, errors.New("FORGE sealing is performed by external merged-mining workers")
}

func (f *Forge) CalcDifficulty(chain consensus.ChainReader, timestamp uint64, parent *types.Header) *big.Int {
	return CalcDifficulty(f.config, timestamp, parent)
}

func (f *Forge) APIs(chain consensus.ChainReader) []rpc.API { return nil }
func (f *Forge) Hashrate() float64                          { return 0 }
