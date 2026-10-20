package miner

import (
	"math/big"
	"testing"
	"time"

	"github.com/TeamEGEM/go-egem/accounts"
	"github.com/TeamEGEM/go-egem/common"
	"github.com/TeamEGEM/go-egem/consensus/forge"
	"github.com/TeamEGEM/go-egem/core"
	"github.com/TeamEGEM/go-egem/core/types"
	"github.com/TeamEGEM/go-egem/core/vm"
	"github.com/TeamEGEM/go-egem/ethdb"
	"github.com/TeamEGEM/go-egem/event"
)

type forgeRefreshBackend struct {
	chain *core.BlockChain
	pool  *core.TxPool
	db    ethdb.Database
}

func (b *forgeRefreshBackend) AccountManager() *accounts.Manager { return nil }

func (b *forgeRefreshBackend) BlockChain() *core.BlockChain { return b.chain }
func (b *forgeRefreshBackend) TxPool() *core.TxPool         { return b.pool }
func (b *forgeRefreshBackend) ChainDb() ethdb.Database      { return b.db }

func TestForgeQuietChainRefreshRebuildsStateAndPreservesIssuedWork(t *testing.T) {
	cfg := remoteForgeConfig()
	db, err := ethdb.NewMemDatabase()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	genesis := &core.Genesis{Config: cfg, Timestamp: uint64(time.Now().Unix() - 60), GasLimit: 8000000, Difficulty: big.NewInt(1)}
	genesis.MustCommit(db)
	engine, err := forge.New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	chain, err := core.NewBlockChain(db, nil, cfg, engine, vm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	defer chain.Stop()
	poolCfg := core.DefaultTxPoolConfig
	poolCfg.Journal = ""
	pool := core.NewTxPool(poolCfg, cfg, chain)
	defer pool.Stop()
	backend := &forgeRefreshBackend{chain: chain, pool: pool, db: db}
	w := &worker{config: cfg, engine: engine, chain: chain, chainDb: db, eth: backend, mux: new(event.TypeMux),
		coinbase: common.HexToAddress("0x1234"), agents: make(map[Agent]struct{}), possibleUncles: make(map[common.Hash]*types.Block),
		unconfirmed: newUnconfirmedBlocks(chain, miningLogAtDepth), mining: 1}
	w.commitNewWork()
	if w.current == nil || w.current.Block == nil {
		t.Fatal("no initial candidate")
	}
	if w.refreshForgeWork(time.Now()) {
		t.Fatal("fresh work rebuilt unnecessarily")
	}
	old := w.current
	oldHeader := old.Block.Header()
	oldHeader.Time = big.NewInt(time.Now().Unix() - 20)
	old.Block = old.Block.WithSeal(oldHeader)
	oldHash := old.Block.Hash()
	// A mutation unique to the stale execution context must not survive a real
	// rebuild from the parent state. This catches a header-only timestamp patch.
	account, slot, marker := common.HexToAddress("0xab"), common.HexToHash("0xcd"), common.HexToHash("0xef")
	old.state.SetState(account, slot, marker)
	if !w.refreshForgeWork(time.Now()) {
		t.Fatal("quiet FORGE chain did not refresh")
	}
	if w.current == old || w.current.Block.Time().Cmp(old.Block.Time()) <= 0 {
		t.Fatal("candidate execution not rebuilt")
	}
	if w.current.state.GetState(account, slot) != (common.Hash{}) {
		t.Fatal("stale execution state was reused")
	}
	if old.Block.Hash() != oldHash {
		t.Fatal("already-issued work mutated")
	}
	if w.current.Block.ParentHash() != old.Block.ParentHash() {
		t.Fatal("refresh unexpectedly changed parent")
	}
	// Neither stopped mining nor pre-fork work gets the FORGE refresh behavior.
	w.mining = 0
	if w.refreshForgeWork(time.Now().Add(time.Minute)) {
		t.Fatal("stopped worker refreshed")
	}
	w.mining = 1
	cfg.ForgeBlock = big.NewInt(100)
	if w.refreshForgeWork(time.Now().Add(time.Minute)) {
		t.Fatal("legacy candidate refreshed through FORGE policy")
	}
}
