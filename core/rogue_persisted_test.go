package core

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"math/big"
	"os"
	"path/filepath"
	"testing"

	"github.com/TeamEGEM/go-egem/common"
	"github.com/TeamEGEM/go-egem/consensus"
	"github.com/TeamEGEM/go-egem/consensus/forge"
	"github.com/TeamEGEM/go-egem/consensus/oggreward"
	"github.com/TeamEGEM/go-egem/consensus/progpow"
	"github.com/TeamEGEM/go-egem/core/state"
	"github.com/TeamEGEM/go-egem/core/types"
	"github.com/TeamEGEM/go-egem/core/vm"
	"github.com/TeamEGEM/go-egem/crypto"
	"github.com/TeamEGEM/go-egem/ethdb"
	"github.com/TeamEGEM/go-egem/params"
	"github.com/syndtr/goleveldb/leveldb"
	"github.com/syndtr/goleveldb/leveldb/opt"
)

// The wrapper is TEST ONLY. It skips sealing, but retains real hybrid header
// rules, difficulty, gas checks, body, transaction, receipt and state validation.
// Missing/invalid real seals are tested separately with the unwrapped engine.
type rogueRehearsalEngine struct{ consensus.Engine }

func (e rogueRehearsalEngine) VerifyHeader(c consensus.ChainReader, h *types.Header, _ bool) error {
	return e.Engine.VerifyHeader(c, h, false)
}
func (e rogueRehearsalEngine) VerifyHeaders(c consensus.ChainReader, hs []*types.Header, _ []bool) (chan<- struct{}, <-chan error) {
	return e.Engine.VerifyHeaders(c, hs, make([]bool, len(hs)))
}

func rogueRehearsalGenesis(t *testing.T) *Genesis {
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
	g.Config.Forge = &params.ForgeConfig{
		RewardSchedule: params.RogueRewardSchedule, PreForgeGrossIssued: pre,
		RequireDGBOrDOGE: true, RequireMulti: true, AllowAuxTree: true, RetargetV2: true,
		AuxChainID: 20295, DogeAuxChainID: 98, TargetBlockTime: 12,
		InitialDifficulty: big.NewInt(140), MinimumDifficulty: big.NewInt(1),
		TemplateSigners: []common.Address{common.HexToAddress("0xcafe")}, TemplateQuorum: 1,
		LTCPayoutScriptHash: common.HexToHash("0x1"), DOGEPayoutScriptHash: common.HexToHash("0x2"), TemplateMaxValidity: 300,
	}
	return &g
}

type rogueContractFingerprint struct{ code, storage common.Hash }

func rogueContractState(s *state.StateDB, a common.Address) rogueContractFingerprint {
	f := rogueContractFingerprint{code: s.GetCodeHash(a)}
	if trie := s.StorageTrie(a); trie != nil {
		f.storage = trie.Hash()
	}
	return f
}

// A trusted checkpoint isolates activation tests from unknown future history.
// It is explicitly not a seal-valid chain from genesis to the fork height.
// The ordinary run uses actual genesis allocations plus marked fixture accounts.
// A separate opt-in test below uses a state snapshot from verified chain replay.
func roguePersistedScenario(t *testing.T, base ethdb.Database, baseRoot common.Hash, first uint64, count int, reorg bool) {
	g := rogueRehearsalGenesis(t)
	cfg := g.Config
	dir, err := ioutil.TempDir("", "rgn-persisted-rehearsal-")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir)
	db, err := ethdb.NewLDBDatabase(filepath.Join(dir, "overlay"), 16, 16)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var store ethdb.Database = db
	if base != nil {
		store = newRogueOverlay(base, db)
	}
	g.MustCommit(store)
	if base == nil {
		baseRoot = g.ToBlock(nil).Root()
	}
	s, err := state.New(baseRoot, state.NewDatabase(store))
	if err != nil {
		t.Fatal(err)
	}
	key, err := crypto.HexToECDSA("4f3edf983ac63ad7c30118e3506c5be1735d090f83b7a1fce2f5f676f4b526f1")
	if err != nil {
		t.Fatal(err)
	}
	sender := crypto.PubkeyToAddress(key.PublicKey)
	recipient := common.HexToAddress("0x123456789abcdef0123456789abcdef0123456789")
	miner := common.HexToAddress("0x23456789abcdef0123456789abcdef0123456789a")
	staking := common.HexToAddress("0xa47008c59f729756bEc7d01f6FE71328A242d0c4")
	tribe := common.HexToAddress("0x085CF5da09842FA3BA01068CC02c156198b1b114")
	p2e := common.HexToAddress("0x03D50e192Bd14ec9f536DF00Db7Ae6723Bdb8BcD")
	treasury := common.HexToAddress("0x85ea896411EdFE9dD7fa6F4F5FaA19D2D81cdA5E")
	// This one account is explicitly funded only inside the isolated rehearsal.
	s.SetBalance(sender, new(big.Int).Exp(big.NewInt(10), big.NewInt(25), nil))
	s.SetNonce(sender, 0)
	if base == nil {
		for _, a := range []common.Address{staking, tribe} {
			s.SetCode(a, []byte{0x60, 0x00, 0x60, 0x00, 0xf3})
			s.SetState(a, common.HexToHash("0x1"), common.HexToHash("0xfeed"))
		}
	}
	fingerprints := map[common.Address]rogueContractFingerprint{staking: rogueContractState(s, staking), tribe: rogueContractState(s, tribe)}
	if base != nil {
		for a, f := range fingerprints {
			if len(s.GetCode(a)) == 0 {
				t.Fatalf("real snapshot contract missing at %s", a.Hex())
			}
			t.Logf("preserved real contract %s code=%s storage=%s", a.Hex(), f.code.Hex(), f.storage.Hex())
		}
	}
	initial := make(map[common.Address]*big.Int)
	for _, a := range []common.Address{miner, staking, tribe, p2e, treasury, recipient} {
		initial[a] = new(big.Int).Set(s.GetBalance(a))
	}
	root, err := s.Commit(true)
	if err != nil {
		t.Fatal(err)
	}
	if err = s.Database().TrieDB().Commit(root, false); err != nil {
		t.Fatal(err)
	}
	parent := types.NewBlockWithHeader(&types.Header{Number: new(big.Int).SetUint64(first - 1), Time: big.NewInt(1700000000), Difficulty: big.NewInt(140), GasLimit: 8000000, Root: root, UncleHash: types.EmptyUncleHash})
	if err = WriteBlock(store, parent); err != nil {
		t.Fatal(err)
	}
	if err = WriteTd(store, parent.Hash(), parent.NumberU64(), big.NewInt(1000000000000)); err != nil {
		t.Fatal(err)
	}
	WriteCanonicalHash(store, parent.Hash(), parent.NumberU64())
	WriteHeadBlockHash(store, parent.Hash())
	WriteHeadHeaderHash(store, parent.Hash())
	WriteHeadFastBlockHash(store, parent.Hash())
	h, err := forge.NewHybrid(cfg, progpow.NewFaker())
	if err != nil {
		t.Fatal(err)
	}
	engine := rogueRehearsalEngine{h}
	open := func() *BlockChain {
		chain, e := NewBlockChain(store, &CacheConfig{Disabled: true}, cfg, engine, vm.Config{})
		if e != nil {
			t.Fatal(e)
		}
		return chain
	}
	chain := open()
	defer func() { chain.Stop() }()
	build := func(from *types.Block, n int, spacing uint64, marker string) types.Blocks {
		blocks := make(types.Blocks, 0, n)
		for i := 0; i < n; i++ {
			st, e := state.New(from.Root(), state.NewDatabase(store))
			if e != nil {
				t.Fatal(e)
			}
			hd := &types.Header{ParentHash: from.Hash(), Number: new(big.Int).Add(from.Number(), big.NewInt(1)), Time: new(big.Int).Add(from.Time(), new(big.Int).SetUint64(spacing)), Coinbase: miner, GasLimit: from.GasLimit(), UncleHash: types.EmptyUncleHash, Extra: []byte(marker)}
			hd.Difficulty = engine.CalcDifficulty(chain, hd.Time.Uint64(), from.Header())
			tx := types.NewTransaction(st.GetNonce(sender), recipient, big.NewInt(12345), 21000, big.NewInt(1), nil)
			tx, e = types.SignTx(tx, types.NewEIP155Signer(cfg.ChainId), key)
			if e != nil {
				t.Fatal(e)
			}
			st.Prepare(tx.Hash(), common.Hash{}, 0)
			used := uint64(0)
			r, _, e := ApplyTransaction(cfg, chain, &miner, new(GasPool).AddGas(hd.GasLimit), st, hd, tx, &used, vm.Config{})
			if e != nil {
				t.Fatal(e)
			}
			hd.GasUsed = used
			block, e := engine.Finalize(chain, hd, st, []*types.Transaction{tx}, nil, types.Receipts{r})
			if e != nil {
				t.Fatal(e)
			}
			root, e := st.Commit(true)
			if e != nil {
				t.Fatal(e)
			}
			if root != block.Root() {
				t.Fatal("generated root mismatch")
			}
			if e = st.Database().TrieDB().Commit(root, false); e != nil {
				t.Fatal(e)
			}
			blocks = append(blocks, block)
			from = block
		}
		return blocks
	}
	check := func(branch types.Blocks) {
		totalWork := big.NewInt(1000000000000)
		for _, b := range branch {
			work, err := forge.HeaderWork(cfg, b.Header())
			if err != nil {
				t.Fatal(err)
			}
			totalWork.Add(totalWork, work)
		}
		head := chain.CurrentBlock()
		if got := GetTd(store, head.Hash(), head.NumberU64()); got == nil || got.Cmp(totalWork) != 0 {
			t.Fatalf("persisted total work mismatch: %v / %s", got, totalWork)
		}
		st, e := chain.State()
		if e != nil {
			t.Fatal(e)
		}
		expected := make(map[common.Address]*big.Int)
		for a, v := range initial {
			expected[a] = new(big.Int).Set(v)
		}
		for _, b := range branch {
			r := oggreward.RewardBreakdown(cfg, b.Number())
			expected[miner].Add(expected[miner], r.Miner).Add(expected[miner], big.NewInt(21000))
			expected[staking].Add(expected[staking], r.Staking)
			if cfg.IsRogue(b.Number()) {
				expected[p2e].Add(expected[p2e], r.P2E)
				expected[treasury].Add(expected[treasury], r.Treasury).Add(expected[treasury], oggreward.RogueSpecialIssuance(b.Number()))
			} else {
				expected[tribe].Add(expected[tribe], r.Tribe)
				expected[treasury].Add(expected[treasury], r.Maintenance)
			}
			expected[recipient].Add(expected[recipient], big.NewInt(12345))
			rc := GetBlockReceipts(store, b.Hash(), b.NumberU64())
			if len(rc) != 1 || rc[0].Status != 1 || rc[0].GasUsed != 21000 {
				t.Fatal("persisted signed transfer receipt mismatch")
			}
		}
		for a, want := range expected {
			if got := st.GetBalance(a); got.Cmp(want) != 0 {
				t.Fatalf("balance %s at %s: got %s want %s", a.Hex(), chain.CurrentBlock().Number(), got, want)
			}
		}
		if st.GetNonce(sender) != uint64(len(branch)) {
			t.Fatal("canonical sender nonce mismatch")
		}
		for a, want := range fingerprints {
			if rogueContractState(st, a) != want {
				t.Fatal("existing contract code/storage changed", a.Hex())
			}
		}
	}
	blocks := build(parent, count, 12, "rehearsal-main")
	if i, err := chain.InsertChain(blocks); err != nil {
		t.Fatalf("import %d: %v", i, err)
	}
	check(blocks)
	if reorg {
		// Keep the same historical predecessor, then replace the slower suffix
		// with a heavier, shorter FORGE branch crossing the issuance boundary.
		common := blocks[0]
		alternative := build(common, count-2, 1, "rehearsal-heavier")
		if i, err := chain.InsertChain(alternative); err != nil {
			t.Fatalf("reorg import %d: %v", i, err)
		}
		if chain.CurrentBlock().Hash() != alternative[len(alternative)-1].Hash() {
			t.Fatal("heavier shorter branch did not replace issuance boundary")
		}
		canonical := append(types.Blocks{common}, alternative...)
		check(canonical) // Removed branch issuance must be rolled back.
		replacement := build(chain.CurrentBlock(), 2, 12, "rehearsal-reallocation")
		if _, err := chain.InsertChain(replacement); err != nil {
			t.Fatal(err)
		}
		canonical = append(canonical, replacement...)
		check(canonical) // Issuance is applied once on the replacement branch.
		blocks = canonical
	}
	before := chain.CurrentBlock().Hash()
	chain.Stop()
	db.Close()
	db, err = ethdb.NewLDBDatabase(filepath.Join(dir, "overlay"), 16, 16)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	store = db
	if base != nil {
		store = newRogueOverlay(base, db)
	}
	restarted, _, err := SetupGenesisBlock(store, nil)
	if err != nil || !restarted.ValidateRogueSchedule() {
		t.Fatal("restart lost fork configuration", err)
	}
	chain = open()
	if chain.CurrentBlock().Hash() != before {
		t.Fatal("persisted canonical head changed after restart")
	}
	check(blocks)
}

func TestRoguePersistedForkAndAllocationReorg(t *testing.T) {
	roguePersistedScenario(t, nil, common.Hash{}, uint64(params.RogueActivationHeight)-1, 12, true)
}

func TestRoguePersistedTailAndExhaustionReorg(t *testing.T) {
	for _, boundary := range []uint64{13580767, 33098927} {
		t.Run(fmt.Sprint(boundary), func(t *testing.T) {
			roguePersistedScenario(t, nil, common.Hash{}, boundary-1, 3, false)
			roguePersistedScenario(t, nil, common.Hash{}, boundary-11, 12, true)
		})
	}
}

func TestRoguePersistedEveryMonthlyBoundary(t *testing.T) {
	// Every due height and both neighbors, including the absent 37th release.
	for j := uint64(1); j <= 37; j++ {
		due := uint64(params.RogueActivationHeight) + j*params.RogueMonthlyInterval
		t.Run(fmt.Sprintf("release-%02d", j), func(t *testing.T) { roguePersistedScenario(t, nil, common.Hash{}, due-1, 3, false) })
	}
}

// A disk-backed overlay keeps source snapshot state/chain records immutable.
// Tombstones are prefixed separately, and persist across overlay reopening.
type rogueOverlay struct{ base, changes ethdb.Database }

// ReadOnly also prevents LevelDB recovery/compaction writes to the public
// snapshot. All rehearsal writes belong to the independent overlay database.
type rogueReadOnlySnapshot struct{ *leveldb.DB }

func (s *rogueReadOnlySnapshot) Get(k []byte) ([]byte, error) { return s.DB.Get(k, nil) }
func (s *rogueReadOnlySnapshot) Has(k []byte) (bool, error)   { return s.DB.Has(k, nil) }
func (s *rogueReadOnlySnapshot) Put([]byte, []byte) error {
	return fmt.Errorf("read-only replay snapshot")
}
func (s *rogueReadOnlySnapshot) Delete([]byte) error { return fmt.Errorf("read-only replay snapshot") }
func (s *rogueReadOnlySnapshot) Close()              { s.DB.Close() }
func (s *rogueReadOnlySnapshot) NewBatch() ethdb.Batch {
	panic("cannot batch writes to read-only replay snapshot")
}

func newRogueOverlay(base, changes ethdb.Database) *rogueOverlay { return &rogueOverlay{base, changes} }
func rogueOverlayKey(prefix byte, k []byte) []byte               { return append([]byte{prefix}, k...) }
func (o *rogueOverlay) Get(k []byte) ([]byte, error) {
	if yes, _ := o.changes.Has(rogueOverlayKey(1, k)); yes {
		return nil, fmt.Errorf("deleted overlay key")
	}
	if v, e := o.changes.Get(rogueOverlayKey(0, k)); e == nil {
		return v, nil
	}
	return o.base.Get(k)
}
func (o *rogueOverlay) Has(k []byte) (bool, error) { _, e := o.Get(k); return e == nil, nil }
func (o *rogueOverlay) Put(k, v []byte) error {
	if e := o.changes.Delete(rogueOverlayKey(1, k)); e != nil {
		return e
	}
	return o.changes.Put(rogueOverlayKey(0, k), v)
}
func (o *rogueOverlay) Delete(k []byte) error {
	if e := o.changes.Delete(rogueOverlayKey(0, k)); e != nil {
		return e
	}
	return o.changes.Put(rogueOverlayKey(1, k), []byte{1})
}
func (o *rogueOverlay) Close()                {}
func (o *rogueOverlay) NewBatch() ethdb.Batch { return &rogueOverlayBatch{o: o} }

type rogueOverlayBatch struct {
	o            *rogueOverlay
	keys, values [][]byte
	size         int
}

func (b *rogueOverlayBatch) Put(k, v []byte) error {
	b.keys = append(b.keys, common.CopyBytes(k))
	b.values = append(b.values, common.CopyBytes(v))
	b.size += len(v)
	return nil
}
func (b *rogueOverlayBatch) ValueSize() int { return b.size }
func (b *rogueOverlayBatch) Reset()         { b.keys = nil; b.values = nil; b.size = 0 }
func (b *rogueOverlayBatch) Write() error {
	for i, k := range b.keys {
		if e := b.o.Put(k, b.values[i]); e != nil {
			return e
		}
	}
	return nil
}

func TestRogueReplayedMainnetStateForkRehearsal(t *testing.T) {
	path := os.Getenv("RGN_REHEARSAL_SNAPSHOT")
	if path == "" {
		t.Skip("explicit isolated replay snapshot required; no live datadir may be used")
	}
	if b, err := ioutil.ReadFile(filepath.Join(path, "REPLAY-ONLY.txt")); err != nil || !bytes.Contains(b, []byte("Isolated public-history qualification")) {
		t.Fatal("not an explicitly marked isolated replay database")
	}
	report, err := ioutil.ReadFile(filepath.Join(path, "report.json"))
	if err != nil {
		t.Fatal("completed replay report required", err)
	}
	var proof struct {
		ReplayComplete  bool
		Height          uint64
		Hash, StateRoot common.Hash
	}
	if err = json.Unmarshal(report, &proof); err != nil || !proof.ReplayComplete || proof.Height != 1066800 || proof.Hash != common.HexToHash("0x72ae77232348d5af1cf6eeb4723389901d4a310993ef2baa3f1e4f0ba2522533") || proof.StateRoot != common.HexToHash("0xd4c58a3339ab7c43b3aa405766ca66e2d912f53cacd8440e8b231b96ae519b67") {
		t.Fatal("frozen replay checkpoint differs", err)
	}
	raw, err := leveldb.OpenFile(filepath.Join(path, "chaindata"), &opt.Options{ReadOnly: true})
	if err != nil {
		t.Fatal(err)
	}
	base := &rogueReadOnlySnapshot{raw}
	defer base.Close()
	head := GetHeadBlockHash(base)
	roguePersistedScenario(t, base, proof.StateRoot, uint64(params.RogueActivationHeight)-1, 12, true)
	if GetHeadBlockHash(base) != head {
		t.Fatal("base snapshot head was modified")
	}
	t.Log("Real replayed state preserved through a trusted future-height checkpoint; signed transfers, native reward routing, allocation reorg, and restart passed. Real post-fork PoW sealing was intentionally not exercised.")
}
