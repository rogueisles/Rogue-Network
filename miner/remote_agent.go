// Copyright 2015 The go-ethereum Authors
// This file is part of the go-ethereum library.
//
// The go-ethereum library is free software: you can redistribute it and/or modify
// it under the terms of the GNU Lesser General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// The go-ethereum library is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
// GNU Lesser General Public License for more details.
//
// You should have received a copy of the GNU Lesser General Public License
// along with the go-ethereum library. If not, see <http://www.gnu.org/licenses/>.

package miner

import (
	"errors"
	"fmt"
	"math/big"
	"sync"
	"sync/atomic"
	"time"

	"github.com/TeamEGEM/go-egem/common"
	"github.com/TeamEGEM/go-egem/consensus"
	"github.com/TeamEGEM/go-egem/consensus/ethash"
	"github.com/TeamEGEM/go-egem/consensus/forge"
	"github.com/TeamEGEM/go-egem/core/types"
	"github.com/TeamEGEM/go-egem/log"
	"github.com/TeamEGEM/go-egem/rlp"
)

type hashrate struct {
	ping time.Time
	rate uint64
}

// AuxWork is the FORGE work package issued to an external Scrypt merged miner.
// Target is the integer Scrypt target for the OGG candidate.
type AuxWork struct {
	HeaderRLP         []byte
	Hash              common.Hash
	Target            *big.Int
	Height            uint64
	Timestamp         uint64
	Difficulty        *big.Int
	PreviousBlockHash common.Hash
}

type auxWorkItem struct {
	work      *Work
	block     *types.Block
	createdAt time.Time
}

type RemoteAgent struct {
	mu sync.Mutex

	quitCh   chan struct{}
	workCh   chan *Work
	returnCh chan<- *Result

	chain       consensus.ChainReader
	engine      consensus.Engine
	currentWork *Work
	work        map[common.Hash]*Work
	auxWork     map[common.Hash]*auxWorkItem

	hashrateMu sync.RWMutex
	hashrate   map[common.Hash]hashrate

	running int32 // running indicates whether the agent is active. Call atomically
}

func NewRemoteAgent(chain consensus.ChainReader, engine consensus.Engine) *RemoteAgent {
	return &RemoteAgent{
		chain:    chain,
		engine:   engine,
		work:     make(map[common.Hash]*Work),
		auxWork:  make(map[common.Hash]*auxWorkItem),
		hashrate: make(map[common.Hash]hashrate),
	}
}

func (a *RemoteAgent) SubmitHashrate(id common.Hash, rate uint64) {
	a.hashrateMu.Lock()
	defer a.hashrateMu.Unlock()

	a.hashrate[id] = hashrate{time.Now(), rate}
}

func (a *RemoteAgent) Work() chan<- *Work {
	return a.workCh
}

func (a *RemoteAgent) SetReturnCh(returnCh chan<- *Result) {
	a.returnCh = returnCh
}

func (a *RemoteAgent) Start() {
	if !atomic.CompareAndSwapInt32(&a.running, 0, 1) {
		return
	}
	a.quitCh = make(chan struct{})
	a.workCh = make(chan *Work, 1)
	go a.loop(a.workCh, a.quitCh)
}

func (a *RemoteAgent) Stop() {
	if !atomic.CompareAndSwapInt32(&a.running, 1, 0) {
		return
	}
	close(a.quitCh)
	close(a.workCh)
}

// GetHashRate returns the accumulated hashrate of all identifier combined
func (a *RemoteAgent) GetHashRate() (tot int64) {
	a.hashrateMu.RLock()
	defer a.hashrateMu.RUnlock()

	// this could overflow
	for _, hashrate := range a.hashrate {
		tot += int64(hashrate.rate)
	}
	return
}

func (a *RemoteAgent) GetWork() ([4]string, error) {
	a.mu.Lock()
	defer a.mu.Unlock()

	var res [4]string

	if a.currentWork != nil {
		block := a.currentWork.Block
		if a.chain.Config().IsForge(block.Number()) {
			return res, errors.New("legacy getWork disabled after FORGE activation")
		}

		res[0] = block.HashNoNonce().Hex()
		// Use the engine's SeedHash if it implements consensus.SeedHasher
		// (works for both Ethash and ProgPoW), else fall back to ethash.SeedHash.
		var seedHash []byte
		if sh, ok := a.engine.(consensus.SeedHasher); ok {
			seedHash = sh.SeedHash(block.NumberU64())
		} else {
			seedHash = ethash.SeedHash(block.NumberU64())
		}
		res[1] = common.BytesToHash(seedHash).Hex()
		// Calculate the "target" to be returned to the external miner
		n := big.NewInt(1)
		n.Lsh(n, 255)
		n.Div(n, block.Difficulty())
		n.Lsh(n, 1)
		res[2] = common.BytesToHash(n.Bytes()).Hex()
		res[3] = fmt.Sprintf("0x%x", block.NumberU64())

		a.work[block.HashNoNonce()] = a.currentWork
		return res, nil
	}
	return res, errors.New("No work available yet, don't panic.")
}

// SubmitWork tries to inject a pow solution into the remote agent, returning
// whether the solution was accepted or not (not can be both a bad pow as well as
// any other error, like no work pending).
func (a *RemoteAgent) SubmitWork(nonce types.BlockNonce, mixDigest, hash common.Hash) bool {
	a.mu.Lock()
	defer a.mu.Unlock()

	// Make sure the work submitted is present
	work := a.work[hash]
	if work == nil {
		log.Info("Work submitted but none pending", "hash", hash)
		return false
	}
	if a.chain.Config().IsForge(work.Block.Number()) {
		log.Info("Legacy work submitted after FORGE activation", "hash", hash)
		return false
	}
	// Make sure the Engine solutions is indeed valid
	result := work.Block.Header()
	result.Nonce = nonce
	result.MixDigest = mixDigest

	if err := a.engine.VerifySeal(a.chain, result); err != nil {
		log.Warn("Invalid proof-of-work submitted", "hash", hash, "err", err)
		return false
	}
	block := work.Block.WithSeal(result)

	// Solutions seems to be valid, return to the miner and notify acceptance
	a.returnCh <- &Result{work, block}
	delete(a.work, hash)

	return true
}

// CreateAuxWork issues the current post-FORGE block as an OGG AuxPoW leaf.
// MinerExtra is deliberately bounded by consensus and is part of the stable
// WorkHash. Phase 4 RPC issues an empty MinerExtra; pool-specific use can supply
// a bounded value later without changing the proof format.
func (a *RemoteAgent) CreateAuxWork(minerExtra []byte) (*AuxWork, error) {
	if len(minerExtra) > forge.MaxMinerExtra {
		return nil, fmt.Errorf("FORGE miner extra too long: %d > %d", len(minerExtra), forge.MaxMinerExtra)
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.currentWork == nil || a.currentWork.Block == nil {
		return nil, errors.New("no FORGE work available yet")
	}
	if !a.chain.Config().IsForge(a.currentWork.Block.Number()) {
		return nil, errors.New("FORGE is not active for current work")
	}
	header := a.currentWork.Block.Header()
	if header.Time == nil || !header.Time.IsUint64() || header.Time.Sign() <= 0 {
		return nil, errors.New("invalid FORGE issued header timestamp")
	}
	header.Extra = append([]byte(nil), minerExtra...)
	header.UncleHash = types.EmptyUncleHash
	block := a.currentWork.Block.WithSeal(header)
	hash, err := forge.WorkHash(header)
	if err != nil {
		return nil, err
	}
	target, err := forge.TargetForConfig(a.chain.Config().Forge, header.Difficulty)
	if err != nil {
		return nil, err
	}
	a.auxWork[hash] = &auxWorkItem{work: a.currentWork, block: block, createdAt: time.Now()}
	headerRLP, err := rlp.EncodeToBytes(header)
	if err != nil {
		return nil, err
	}
	return &AuxWork{
		HeaderRLP:         headerRLP,
		Hash:              hash,
		Target:            new(big.Int).Set(target),
		Height:            header.Number.Uint64(),
		Timestamp:         header.Time.Uint64(),
		Difficulty:        new(big.Int).Set(header.Difficulty),
		PreviousBlockHash: header.ParentHash,
	}, nil
}

// SubmitAuxWork validates an issued FORGE work item, attaches the bounded AuxPoW
// envelope, verifies the Scrypt/AuxPoW seal, and routes the accepted block back
// through the normal miner result path. Unknown or malformed work fails closed.
func (a *RemoteAgent) SubmitAuxWork(hash common.Hash, proofBlob []byte) bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	item := a.auxWork[hash]
	if item == nil || item.block == nil || item.work == nil {
		log.Info("FORGE work submitted but none pending", "hash", hash)
		return false
	}
	proof, err := forge.DecodeProof(proofBlob)
	if err != nil {
		log.Warn("Invalid FORGE proof envelope submitted", "hash", hash, "err", err)
		return false
	}
	header := item.block.Header()
	header.Extra = append([]byte(nil), proofBlob...)
	recomputed, err := forge.WorkHash(header)
	if err != nil || recomputed != hash {
		log.Warn("FORGE proof does not match issued work", "hash", hash, "recomputed", recomputed, "err", err)
		return false
	}
	// WorkHash replaces the proof with proof.MinerExtra. This explicit check makes
	// the issued miner-extra binding obvious and rejects substitutions early.
	issuedHeader := item.block.Header()
	if string(issuedHeader.Extra) != string(proof.MinerExtra) {
		log.Warn("FORGE miner extra does not match issued work", "hash", hash)
		return false
	}
	if err := a.engine.VerifySeal(a.chain, header); err != nil {
		log.Warn("Invalid FORGE AuxPoW submitted", "hash", hash, "err", err)
		return false
	}
	block := item.block.WithSeal(header)
	a.returnCh <- &Result{item.work, block}
	delete(a.auxWork, hash)
	return true
}

// loop monitors mining events on the work and quit channels, updating the internal
// state of the remote miner until a termination is requested.
//
// Note, the reason the work and quit channels are passed as parameters is because
// RemoteAgent.Start() constantly recreates these channels, so the loop code cannot
// assume data stability in these member fields.
func (a *RemoteAgent) loop(workCh chan *Work, quitCh chan struct{}) {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-quitCh:
			return
		case work := <-workCh:
			a.mu.Lock()
			a.currentWork = work
			a.mu.Unlock()
		case <-ticker.C:
			// cleanup
			a.mu.Lock()
			for hash, work := range a.work {
				if time.Since(work.createdAt) > 7*(12*time.Second) {
					delete(a.work, hash)
				}
			}
			for hash, work := range a.auxWork {
				if time.Since(work.createdAt) > 7*(12*time.Second) {
					delete(a.auxWork, hash)
				}
			}
			a.mu.Unlock()

			a.hashrateMu.Lock()
			for id, hashrate := range a.hashrate {
				if time.Since(hashrate.ping) > 10*time.Second {
					delete(a.hashrate, id)
				}
			}
			a.hashrateMu.Unlock()
		}
	}
}
