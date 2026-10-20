package core

import (
	"fmt"
	"math/big"
	"strings"
	"testing"

	"github.com/TeamEGEM/go-egem/common"
	"github.com/TeamEGEM/go-egem/consensus/forge"
	"github.com/TeamEGEM/go-egem/consensus/progpow"
	"github.com/TeamEGEM/go-egem/core/state"
	"github.com/TeamEGEM/go-egem/core/types"
	"github.com/TeamEGEM/go-egem/core/vm"
	"github.com/TeamEGEM/go-egem/ethdb"
	"github.com/TeamEGEM/go-egem/params"
)

// A trusted checkpoint isolates fork-choice arithmetic. Seal and native-chain
// proof validation have separate interoperability and rejection tests.
func TestRogueChainWorkCrossAlgorithmFullHeaderAndReceiptImport(t *testing.T) {
	g := rogueRehearsalGenesis(t)
	hybrid, err := forge.NewHybrid(g.Config, progpow.NewFaker())
	if err != nil {
		t.Fatal(err)
	}
	engine := rogueRehearsalEngine{hybrid}
	baseTD := big.NewInt(1000000000000)
	seed := func() (*BlockChain, *ethdb.MemDatabase, *types.Block) {
		db, err := ethdb.NewMemDatabase()
		if err != nil {
			t.Fatal(err)
		}
		genesis := g.MustCommit(db)
		parent := types.NewBlockWithHeader(&types.Header{Number: big.NewInt(params.RogueActivationHeight - 2), Time: big.NewInt(1700000000), Difficulty: big.NewInt(25000000000), GasLimit: 8000000, Root: genesis.Root(), UncleHash: types.EmptyUncleHash})
		if err := WriteBlock(db, parent); err != nil {
			t.Fatal(err)
		}
		if err := WriteTd(db, parent.Hash(), parent.NumberU64(), baseTD); err != nil {
			t.Fatal(err)
		}
		WriteCanonicalHash(db, parent.Hash(), parent.NumberU64())
		WriteHeadBlockHash(db, parent.Hash())
		WriteHeadHeaderHash(db, parent.Hash())
		WriteHeadFastBlockHash(db, parent.Hash())
		chain, err := NewBlockChain(db, &CacheConfig{Disabled: true}, g.Config, engine, vm.Config{})
		if err != nil {
			t.Fatal(err)
		}
		return chain, db, parent
	}
	chain, db, parent := seed()
	defer db.Close()
	defer chain.Stop()
	build := func(from *types.Block, seconds int64) *types.Block {
		st, err := state.New(from.Root(), state.NewDatabase(db))
		if err != nil {
			t.Fatal(err)
		}
		h := &types.Header{ParentHash: from.Hash(), Number: new(big.Int).Add(from.Number(), big.NewInt(1)), Time: new(big.Int).Add(from.Time(), big.NewInt(seconds)), Coinbase: common.HexToAddress("0xbeef"), GasLimit: from.GasLimit(), UncleHash: types.EmptyUncleHash}
		if seconds >= 12 {
			h.Coinbase = common.HexToAddress("0xcafe")
		}
		h.Difficulty = engine.CalcDifficulty(chain, h.Time.Uint64(), from.Header())
		block, err := engine.Finalize(chain, h, st, nil, nil, nil)
		if err != nil {
			t.Fatal(err)
		}
		root, err := st.Commit(true)
		if err != nil {
			t.Fatal(err)
		}
		if err := st.Database().TrieDB().Commit(root, false); err != nil {
			t.Fatal(err)
		}
		return block
	}
	fastLegacy := build(parent, 1)
	slowLegacy := build(parent, 12)
	activated := build(slowLegacy, 12)
	if fastLegacy.Difficulty().Cmp(new(big.Int).Add(slowLegacy.Difficulty(), activated.Difficulty())) <= 0 {
		t.Fatal("fixture does not expose mixed-unit failure")
	}
	if _, err := chain.InsertChain(types.Blocks{fastLegacy}); err != nil {
		t.Fatal(err)
	}
	if _, err := chain.InsertChain(types.Blocks{slowLegacy, activated}); err != nil {
		t.Fatal(err)
	}
	work, err := forge.HeaderWork(g.Config, activated.Header())
	if err != nil {
		t.Fatal(err)
	}
	want := new(big.Int).Add(new(big.Int).Add(baseTD, slowLegacy.Difficulty()), work)
	if chain.CurrentBlock().Hash() != activated.Hash() || chain.GetTd(activated.Hash(), activated.NumberU64()).Cmp(want) != 0 {
		t.Fatal("full import used incompatible work units")
	}
	headers, headersDB, otherParent := seed()
	defer headersDB.Close()
	defer headers.Stop()
	if otherParent.Hash() != parent.Hash() {
		t.Fatal("checkpoint changed")
	}
	if _, err := headers.InsertHeaderChain([]*types.Header{fastLegacy.Header()}, 1); err != nil {
		t.Fatal(err)
	}
	if _, err := headers.InsertHeaderChain([]*types.Header{slowLegacy.Header(), activated.Header()}, 1); err != nil {
		t.Fatal(err)
	}
	if headers.CurrentHeader().Hash() != activated.Hash() || headers.GetTd(activated.Hash(), activated.NumberU64()).Cmp(want) != 0 {
		t.Fatal("header import disagrees with full block work")
	}
	if _, err := headers.InsertReceiptChain(types.Blocks{slowLegacy, activated}, []types.Receipts{nil, nil}); err != nil {
		t.Fatal(err)
	}
	if headers.CurrentFastBlock().Hash() != activated.Hash() || GetTd(headersDB, activated.Hash(), activated.NumberU64()).Cmp(want) != 0 {
		t.Fatal("receipt import changed validated header work")
	}
	pruned, prunedDB, _ := seed()
	defer prunedDB.Close()
	defer pruned.Stop()
	if _, err := pruned.InsertChain(types.Blocks{fastLegacy}); err != nil {
		t.Fatal(err)
	}
	if err := pruned.WriteBlockWithoutState(slowLegacy, new(big.Int).Add(baseTD, slowLegacy.Difficulty())); err != nil {
		t.Fatal(err)
	}
	if pruned.HasState(slowLegacy.Root()) {
		t.Fatal("fixture did not retain a parent with unavailable state")
	}
	if _, err := pruned.InsertChain(types.Blocks{activated}); err != nil {
		t.Fatal("winning branch did not reconstruct its unavailable parent state", err)
	}
	if pruned.CurrentBlock().Hash() != activated.Hash() || pruned.GetTd(activated.Hash(), activated.NumberU64()).Cmp(want) != 0 || !pruned.HasState(slowLegacy.Root()) {
		t.Fatal("pruned-ancestor recovery used incompatible work units")
	}
	for _, unavailable := range []bool{false, true} {
		t.Run(fmt.Sprintf("body-check-parent-unavailable-%v", unavailable), func(t *testing.T) {
			candidate, candidateDB, _ := seed()
			defer candidateDB.Close()
			defer candidate.Stop()
			if _, err := candidate.InsertChain(types.Blocks{fastLegacy}); err != nil {
				t.Fatal(err)
			}
			if unavailable {
				if err := candidate.WriteBlockWithoutState(slowLegacy, new(big.Int).Add(baseTD, slowLegacy.Difficulty())); err != nil {
					t.Fatal(err)
				}
			} else if _, err := candidate.InsertChain(types.Blocks{slowLegacy}); err != nil {
				t.Fatal(err)
			}
			if candidate.HasState(slowLegacy.Root()) == unavailable {
				t.Fatal("incorrect parent state fixture")
			}
			badHeader := activated.Header()
			badHeader.TxHash[0] ^= 1
			bad := types.NewBlockWithHeader(badHeader).WithBody(activated.Transactions(), activated.Uncles())
			if _, err := candidate.InsertChain(types.Blocks{bad}); err == nil || !strings.Contains(err.Error(), "transaction root hash mismatch") {
				t.Fatal("body commitment check depends on parent state availability", err)
			}
			if candidate.HasBlock(bad.Hash(), bad.NumberU64()) {
				t.Fatal("invalid body retained in branch storage")
			}
			badUncles := types.NewBlockWithHeader(activated.Header()).WithBody(activated.Transactions(), []*types.Header{slowLegacy.Header()})
			if _, err := candidate.InsertChain(types.Blocks{badUncles}); err == nil || !strings.Contains(err.Error(), "may not contain uncles") {
				t.Fatal("uncle body check depends on parent state availability", err)
			}
			if unavailable && candidate.HasState(slowLegacy.Root()) {
				t.Fatal("invalid activated body triggered parent state reconstruction")
			}
		})
	}
}

// A known header does not authenticate a supplied replacement body. The zero
// subsidy fixture makes a skipped finalization error observable after rollback.
func TestRogueKnownZeroRewardBlockRejectsChangedBodyAfterRollback(t *testing.T) {
	g := rogueRehearsalGenesis(t)
	db, err := ethdb.NewMemDatabase()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	genesis := g.MustCommit(db)
	parent := types.NewBlockWithHeader(&types.Header{Number: big.NewInt(33098927), Time: big.NewInt(1700000000), Difficulty: big.NewInt(140), GasLimit: 8000000, Root: genesis.Root(), UncleHash: types.EmptyUncleHash})
	if err := WriteBlock(db, parent); err != nil {
		t.Fatal(err)
	}
	if err := WriteTd(db, parent.Hash(), parent.NumberU64(), big.NewInt(1000000000000)); err != nil {
		t.Fatal(err)
	}
	WriteCanonicalHash(db, parent.Hash(), parent.NumberU64())
	WriteHeadBlockHash(db, parent.Hash())
	WriteHeadHeaderHash(db, parent.Hash())
	WriteHeadFastBlockHash(db, parent.Hash())
	hybrid, err := forge.NewHybrid(g.Config, progpow.NewFaker())
	if err != nil {
		t.Fatal(err)
	}
	engine := rogueRehearsalEngine{hybrid}
	chain, err := NewBlockChain(db, &CacheConfig{Disabled: true}, g.Config, engine, vm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	defer chain.Stop()
	st, err := state.New(parent.Root(), state.NewDatabase(db))
	if err != nil {
		t.Fatal(err)
	}
	header := &types.Header{ParentHash: parent.Hash(), Number: big.NewInt(33098928), Time: big.NewInt(1700000012), Coinbase: common.HexToAddress("0xbeef"), GasLimit: parent.GasLimit(), UncleHash: types.EmptyUncleHash}
	header.Difficulty = engine.CalcDifficulty(chain, header.Time.Uint64(), parent.Header())
	block, err := engine.Finalize(chain, header, st, nil, nil, nil)
	if err != nil || block.Root() != parent.Root() {
		t.Fatal("fixture must have no state transition or subsidy", err)
	}
	if _, err := chain.InsertChain(types.Blocks{block}); err != nil {
		t.Fatal(err)
	}
	chain.Rollback([]common.Hash{block.Hash()})
	if chain.CurrentBlock().Hash() != parent.Hash() || !chain.HasBlockAndState(block.Hash(), block.NumberU64()) {
		t.Fatal("fixture did not preserve a known block above the rolled-back head")
	}
	mutated := types.NewBlockWithHeader(block.Header()).WithBody(nil, []*types.Header{parent.Header()})
	if mutated.Hash() != block.Hash() {
		t.Fatal("fixture unexpectedly changed the authenticated header")
	}
	if _, err := chain.InsertChain(types.Blocks{mutated}); err == nil || !strings.Contains(err.Error(), "may not contain uncles") {
		t.Fatal("known-block shortcut bypassed activated body validation", err)
	}
	if chain.CurrentBlock().Hash() != parent.Hash() || len(chain.GetBlock(block.Hash(), block.NumberU64()).Uncles()) != 0 {
		t.Fatal("invalid known body changed canonical head or stored block")
	}
	if _, err := chain.InsertChain(types.Blocks{block}); err != nil || chain.CurrentBlock().Hash() != block.Hash() {
		t.Fatal("valid known block could not be reimported after rollback", err)
	}
}
