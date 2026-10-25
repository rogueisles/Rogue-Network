package forge

import (
	"bytes"
	"crypto/ecdsa"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"math/big"
	"os"
	"sort"
	"strings"
	"testing"

	"github.com/TeamEGEM/go-egem/common"
	"github.com/TeamEGEM/go-egem/core/types"
	"github.com/TeamEGEM/go-egem/crypto"
	"github.com/TeamEGEM/go-egem/params"
	"github.com/TeamEGEM/go-egem/rlp"
)

type flexibleIntegrationChain struct {
	config    *params.ChainConfig
	byHash    map[common.Hash]*types.Header
	canonical map[uint64]common.Hash
}

func (c *flexibleIntegrationChain) Config() *params.ChainConfig { return c.config }
func (c *flexibleIntegrationChain) CurrentHeader() *types.Header {
	var best *types.Header
	for number, hash := range c.canonical {
		if best == nil || number > best.Number.Uint64() {
			best = c.byHash[hash]
		}
	}
	return types.CopyHeader(best)
}
func (c *flexibleIntegrationChain) GetHeader(hash common.Hash, number uint64) *types.Header {
	h := c.byHash[hash]
	if h == nil || h.Number.Uint64() != number {
		return nil
	}
	return types.CopyHeader(h)
}
func (c *flexibleIntegrationChain) GetHeaderByHash(hash common.Hash) *types.Header {
	if h := c.byHash[hash]; h != nil {
		return types.CopyHeader(h)
	}
	return nil
}
func (c *flexibleIntegrationChain) GetHeaderByNumber(number uint64) *types.Header {
	return c.GetHeaderByHash(c.canonical[number])
}
func (c *flexibleIntegrationChain) GetBlock(hash common.Hash, number uint64) *types.Block { return nil }
func (c *flexibleIntegrationChain) add(h *types.Header, canonical bool) {
	c.byHash[h.Hash()] = types.CopyHeader(h)
	if canonical {
		c.canonical[h.Number.Uint64()] = h.Hash()
	}
}

func flexibleIntegrationFixture(t testing.TB) (*Forge, *flexibleIntegrationChain, *types.Header, RegistrySnapshot, RegistryContext, []*ecdsa.PrivateKey, map[uint32][]byte) {
	t.Helper()
	s, ctx, adminKeys := registryTestFixture()
	scripts := map[uint32][]byte{0: {0x51}, 98: {0x52}, SOQAuxChainID: append([]byte{0x51, 0x20}, bytes.Repeat([]byte{0x73}, 32)...)}
	for i := range s.Coins {
		s.Coins[i].Treasury = ScriptHash(scripts[s.Coins[i].ChainID])
	}
	cfg := &params.ChainConfig{Forge: &params.ForgeConfig{
		AuxChainID: 20295, DogeAuxChainID: 98, TargetBlockTime: 12,
		PreForgeGrossIssued: big.NewInt(1), InitialDifficulty: big.NewInt(1), MinimumDifficulty: big.NewInt(1), TemplateMaxValidity: 300,
	}}
	cfg.ChainId = big.NewInt(7008914)
	cfg.ForgeBlock = big.NewInt(1)
	cfg.Forge.RegistryBlock = big.NewInt(2)
	cfg.Forge.RegistryMinDelay = 2
	cfg.Forge.RegistryMinOperationInterval = 1
	cfg.Forge.RegistryTargetBlockTimeMillis = 12500
	cfg.Forge.RetargetV2 = true
	cfg.Forge.AllowAuxTree = true
	cfg.Forge.RequireMulti = true
	cfg.Forge.RequireDGBOrDOGE = true
	cfg.Forge.TemplateSigners = ctx.WorkSigners
	cfg.Forge.TemplateQuorum = 2
	cfg.Forge.LTCPayoutScriptHash = s.Coins[0].Treasury
	cfg.Forge.DOGEPayoutScriptHash = s.Coins[1].Treasury
	cfg.Forge.PepecoinPayoutScriptHash = registryTestHash(400)
	cfg.Forge.DingocoinPayoutScriptHash = registryTestHash(401)
	cfg.Forge.BellscoinPayoutScriptHash = registryTestHash(402)
	var err error
	cfg.Forge.RegistryBootstrap, err = rlp.EncodeToBytes(s)
	if err != nil {
		t.Fatal(err)
	}
	engine, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	chain := &flexibleIntegrationChain{config: cfg, byHash: make(map[common.Hash]*types.Header), canonical: make(map[uint64]common.Hash)}
	genesis := &types.Header{Number: big.NewInt(0), Time: big.NewInt(1000), GasLimit: 8000000, Difficulty: big.NewInt(1), UncleHash: types.EmptyUncleHash, Root: registryTestHash(12345)}
	chain.add(genesis, true)
	parent := &types.Header{ParentHash: genesis.Hash(), Number: big.NewInt(1), Time: big.NewInt(1012), GasLimit: 8000000, Difficulty: big.NewInt(1), UncleHash: types.EmptyUncleHash}
	chain.add(parent, true)
	ctx.ChainID = cfg.ChainId
	ctx.Genesis = genesis.Hash()
	return engine, chain, parent, s, ctx, adminKeys, scripts
}

func flexibleIntegrationCoinbase(t testing.TB, scriptSig, payout []byte, value uint64) []byte {
	t.Helper()
	cb := makeCoinbase(scriptSig, payout, value, false)
	// Extend the single-output test coinbase with the distinct V5 certificate.
	cb[46+len(scriptSig)] = 2
	cb = cb[:len(cb)-4]
	cb = append(cb, make([]byte, 8)...)
	certificate := append([]byte{0x6a, 40}, []byte("RGNCERT5")...)
	certificate = append(certificate, make([]byte, 32)...)
	cb = append(cb, byte(len(certificate)))
	cb = append(cb, certificate...)
	return append(cb, 0, 0, 0, 0)
}

// These fixtures deliberately do not mine PoW. They test every header binding,
// including sampled-seal header synchronization, independently of the full lab.
func flexibleIntegrationHeader(t testing.TB, engine *Forge, chain *flexibleIntegrationChain, parent *types.Header, envelope *RegistryEnvelope, scripts map[uint32][]byte) *types.Header {
	t.Helper()
	h := &types.Header{ParentHash: parent.Hash(), Number: new(big.Int).Add(parent.Number, big.NewInt(1)), Time: new(big.Int).Add(parent.Time, big.NewInt(12)), GasLimit: parent.GasLimit, UncleHash: types.EmptyUncleHash, Coinbase: common.HexToAddress("0x1234"), Extra: []byte("v5-integration")}
	h.Difficulty = engine.CalcDifficulty(chain, h.Time.Uint64(), parent)
	work, err := WorkHash(h)
	if err != nil {
		t.Fatal(err)
	}
	proof := &FlexibleProof{Registry: *envelope, Multi: MultiProof{MinerExtra: append([]byte(nil), h.Extra...)}}
	template := MultiTemplate{Version: 5, ValidFrom: h.Time.Uint64(), Expires: h.Time.Uint64() + 120, ParentVersion: 1, ExtraSize: 8}
	leaves := map[uint32]common.Hash{engine.config.Forge.AuxChainID: work}
	for _, member := range envelope.Members {
		coin := member.Coin
		if coin.ChainID == 0 {
			template.Mask |= 1
			continue
		}
		if coin.ChainID == 98 {
			template.Mask |= 2
		}
		if coin.ChainID == SOQAuxChainID {
			template.Mask |= ExternalSOQ
		}
		cb := makeCoinbase([]byte{1, 1}, scripts[coin.ChainID], 5000000000, false)
		parsed, err := parseCoinbase(cb)
		if err != nil {
			t.Fatal(err)
		}
		header := makeBitcoinHeader((coin.ChainID<<16)|(1<<8)|1, registryTestHash(int64(coin.ChainID)+2), parsed.TxID, uint32(h.Time.Uint64()), 0x1d00ffff, 0)
		raw, display, err := bitcoinHeaderHashDisplay(header)
		if err != nil {
			t.Fatal(err)
		}
		leaves[coin.ChainID] = raw
		template.Aux = append(template.Aux, MultiAuxRef{ChainID: coin.ChainID, Hash: display})
		proof.Multi.Aux = append(proof.Multi.Aux, MultiAuxProof{ChainID: coin.ChainID, Header: header, Coinbase: cb})
	}
	sort.Slice(template.Aux, func(i, j int) bool { return template.Aux[i].ChainID < template.Aux[j].ChainID })
	sort.Slice(proof.Multi.Aux, func(i, j int) bool { return proof.Multi.Aux[i].ChainID < proof.Multi.Aux[j].ChainID })
	root, size, err := canonicalMultiRoot(leaves, 3)
	if err != nil {
		t.Fatal(err)
	}
	scriptSig := append([]byte{1, 1}, mergedMiningMagic...)
	scriptSig = append(scriptSig, reverse32(root[:]).Bytes()...)
	var n [4]byte
	binary.LittleEndian.PutUint32(n[:], size)
	scriptSig = append(scriptSig, n[:]...)
	binary.LittleEndian.PutUint32(n[:], 3)
	scriptSig = append(scriptSig, n[:]...)
	template.ExtraOffset = uint32(42 + len(scriptSig))
	scriptSig = append(scriptSig, make([]byte, template.ExtraSize)...)
	value := uint64(5000000000)
	payout := scripts[0]
	prev := registryTestHash(800)
	bits := uint32(0x1d00ffff)
	if template.Mask&1 == 0 {
		value = 0
		payout = []byte{0x6a}
		prev = common.Hash{}
	}
	cb := flexibleIntegrationCoinbase(t, scriptSig, payout, value)
	parsed, err := parseCoinbase(cb)
	if err != nil {
		t.Fatal(err)
	}
	parentHeader := makeBitcoinHeader(1, prev, parsed.TxID, uint32(h.Time.Uint64()), bits, 0)
	if template.Mask&1 != 0 {
		template.DGBPrevHash, _ = bitcoinHeaderPrevHashDisplay(parentHeader)
		template.DGBBits = bits
		template.ParentMinTime = uint32(h.Time.Uint64())
	}
	template.ParentCoinbaseHash = crypto.Keccak256Hash(cb)
	branch, _ := rlp.EncodeToBytes([]common.Hash{})
	template.ParentBranchHash = crypto.Keccak256Hash(branch)
	proof.Multi.Template = template
	bound, err := FlexibleSigningWork(work, *envelope)
	if err != nil {
		t.Fatal(err)
	}
	digest, err := MultiDigest(template, bound)
	if err != nil {
		t.Fatal(err)
	}
	proof.Multi.Signatures = registryTestSign(t, digest, registryTestKeys(21), engine.config.Forge.TemplateQuorum)
	certificate, err := MultiCertificateDigest(template, bound, proof.Multi.Signatures)
	if err != nil {
		t.Fatal(err)
	}
	offset, _, err := certificatePayloadOffsetVersion(parsed, 5)
	if err != nil {
		t.Fatal(err)
	}
	copy(cb[offset:offset+32], certificate[:])
	parsed, err = parseCoinbase(cb)
	if err != nil {
		t.Fatal(err)
	}
	copy(parentHeader[36:68], parsed.TxID[:])
	proof.Multi.ParentHeader, proof.Multi.ParentCoinbase = parentHeader, cb
	h.Extra, err = EncodeFlexibleProof(proof)
	if err != nil {
		t.Fatal(err)
	}
	return h
}

func flexibleIntegrationClone(t *testing.T, h *types.Header) (*types.Header, *FlexibleProof) {
	t.Helper()
	out := types.CopyHeader(h)
	decoded, err := DecodeProof(out.Extra)
	if err != nil || decoded.Flexible == nil {
		t.Fatal("not a V5 fixture", err)
	}
	return out, decoded.Flexible
}

func TestFlexibleHeaderActivationAndAllCoreSubsets(t *testing.T) {
	engine, chain, parent, s, _, _, scripts := flexibleIntegrationFixture(t)
	for _, slots := range [][]uint8{{1}, {2}, {3}, {1, 2}, {1, 3}, {2, 3}, {1, 2, 3}} {
		h := flexibleIntegrationHeader(t, engine, chain, parent, registryTestEnvelope(t, s, slots...), scripts)
		if err := engine.VerifyHeader(chain, h, false); err != nil {
			t.Fatalf("core slots=%v rejected: %v", slots, err)
		}
		if err := engine.VerifySeal(chain, h); err == nil || !strings.Contains(err.Error(), "Scrypt proof-of-work") {
			t.Fatalf("unmined V5 fixture bypassed PoW: %v", err)
		}
		work, _ := WorkHash(h)
		unsigned := types.CopyHeader(h)
		unsigned.Extra = []byte("v5-integration")
		want, _ := WorkHash(unsigned)
		if work != want {
			t.Fatal("V5 attachment changed RGN work leaf")
		}
	}
	valid := flexibleIntegrationHeader(t, engine, chain, parent, registryTestEnvelope(t, s, 1, 2, 3), scripts)
	pre := types.CopyHeader(valid)
	pre.Number = big.NewInt(1)
	if err := engine.VerifySeal(chain, pre); err == nil || !strings.Contains(err.Error(), "before activation") {
		t.Fatal("accepted V5 before activation", err)
	}
	for _, seal := range []bool{false, true} {
		old := types.CopyHeader(valid)
		payload, _ := rlp.EncodeToBytes(MultiProof{Template: MultiTemplate{Version: 4}})
		old.Extra = append([]byte{'O', 'G', 'G', 'F', 4}, payload...)
		if err := engine.VerifyHeader(chain, old, seal); err == nil || !strings.Contains(err.Error(), "requires V5") {
			t.Fatal("legacy proof bypassed postactivation registry", seal, err)
		}
	}
	unknown := types.CopyHeader(valid)
	unknown.ParentHash = registryTestHash(999)
	if err := engine.VerifyHeader(chain, unknown, false); err == nil {
		t.Fatal("accepted unknown parent")
	}
	if err := engine.VerifySeal(nil, valid); err == nil {
		t.Fatal("validated registry without exact parent chain")
	}
}

func TestFlexibleRegistryAndMinedCertificateMutationsFailWithoutSeal(t *testing.T) {
	engine, chain, parent, s, _, _, scripts := flexibleIntegrationFixture(t)
	valid := flexibleIntegrationHeader(t, engine, chain, parent, registryTestEnvelope(t, s, 1, 2, 3), scripts)
	tests := []struct {
		name   string
		change func(*FlexibleProof)
	}{
		{"registry root", func(p *FlexibleProof) { p.Registry.Root = registryTestHash(999) }},
		{"registry checkpoint", func(p *FlexibleProof) { p.Registry.Checkpoint = 1 }},
		{"treasury membership", func(p *FlexibleProof) { p.Registry.Members[0].Coin.Treasury = registryTestHash(999) }},
		{"membership branch", func(p *FlexibleProof) { p.Registry.Members[0].Branch[0] = registryTestHash(999) }},
		{"missing used member", func(p *FlexibleProof) { p.Registry.Members = p.Registry.Members[:2] }},
		{"duplicate work signer", func(p *FlexibleProof) { p.Multi.Signatures[1] = p.Multi.Signatures[0] }},
		{"signature removed", func(p *FlexibleProof) { p.Multi.Signatures = nil }},
		{"native previous hash", func(p *FlexibleProof) { p.Multi.ParentHeader[4] ^= 1 }},
		{"native coinbase", func(p *FlexibleProof) { p.Multi.ParentCoinbase[len(p.Multi.ParentCoinbase)-1] ^= 1 }},
		{"auxiliary evidence", func(p *FlexibleProof) { p.Multi.Aux[0].Header[4] ^= 1 }},
		{"hidden auxiliary", func(p *FlexibleProof) { p.Multi.Aux = p.Multi.Aux[:1] }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			h, p := flexibleIntegrationClone(t, valid)
			test.change(p)
			raw, err := rlp.EncodeToBytes(p)
			if err != nil {
				t.Fatal(err)
			}
			h.Extra = append([]byte{'O', 'G', 'G', 'F', 5}, raw...)
			if err := engine.VerifyHeader(chain, h, false); err == nil {
				t.Fatal("invalid proof accepted when seal sampling is disabled")
			}
		})
	}
	oversized := types.CopyHeader(valid)
	oversized.Extra = append(oversized.Extra, make([]byte, MaxProofSize)...)
	if err := engine.VerifyHeader(chain, oversized, false); err == nil {
		t.Fatal("oversized V5 proof accepted")
	}
	malformed := types.CopyHeader(valid)
	malformed.Extra = []byte{'O', 'G', 'G', 'F', 5, 0xc0}
	if err := engine.VerifyHeader(chain, malformed, false); err == nil {
		t.Fatal("malformed V5 envelope accepted")
	}
}

func TestFlexibleTreasuryRotationSurvivesPeerBatchAndRejectsOldPayout(t *testing.T) {
	engine, chain, parent, s, ctx, keys, scripts := flexibleIntegrationFixture(t)
	change := RegistryChange{Kind: RegistryRotateTreasury, Sequence: 1, Slot: 1, ActivationHeight: 4, Treasury: ScriptHash([]byte{0x53})}
	e, after := registryTestOperation(t, s, ctx, 2, change, keys, nil)
	h2 := flexibleIntegrationHeader(t, engine, chain, parent, e, scripts)
	h3 := flexibleIntegrationHeader(t, engine, chain, h2, registryTestEnvelope(t, after, 1, 2, 3), scripts)
	rotatedScripts := make(map[uint32][]byte)
	for id, script := range scripts {
		rotatedScripts[id] = script
	}
	rotatedScripts[0] = []byte{0x53}
	h4 := flexibleIntegrationHeader(t, engine, chain, h3, registryTestEnvelope(t, after, 1, 2, 3), rotatedScripts)
	abort, results := engine.VerifyHeaders(chain, []*types.Header{h2, h3, h4}, []bool{false, false, false})
	defer close(abort)
	for i := 0; i < 3; i++ {
		if err := <-results; err != nil {
			t.Fatalf("header-only peer batch item%d failed: %v", i, err)
		}
	}
	chain.add(h2, true)
	chain.add(h3, true)
	badPayout := flexibleIntegrationHeader(t, engine, chain, h3, registryTestEnvelope(t, after, 1, 2, 3), scripts)
	if err := engine.VerifyHeader(chain, badPayout, false); err == nil || !strings.Contains(err.Error(), "treasury") {
		t.Fatal("work-signing quorum bypassed rotated treasury", err)
	}
	if err := engine.VerifyHeader(chain, h4, false); err != nil {
		t.Fatal(err)
	}
	chain.add(h4, true)
	got, _, root, checkpoint, err := RegistrySnapshotAtParent(chain.config, chain, h4)
	wantRoot, _ := RegistrySnapshotRoot(after)
	if err != nil || checkpoint != 2 || root != wantRoot || got.Coins[0].NextTreasury != change.Treasury {
		t.Fatal("RPC checkpoint reconstruction failed", err)
	}
	if _, _, _, _, err = RegistrySnapshotAtParent(chain.config, chain, parent); err != nil {
		t.Fatal("bootstrap checkpoint RPC failed", err)
	}
	// Altering an operation in a header whose PoW is deliberately not checked
	// must still fail before its registry state can enter the peer overlay.
	bad, p := flexibleIntegrationClone(t, h2)
	p.Registry.Operations[0].Signatures = nil
	bad.Extra, _ = EncodeFlexibleProof(p)
	abort2, results2 := engine.VerifyHeaders(chain, []*types.Header{bad}, []bool{false})
	defer close(abort2)
	if err := <-results2; err == nil {
		t.Fatal("sampled header sync accepted unauthorized treasury transition")
	}
}

func TestFlexibleCompatibleExtraRequiresApprovedRegistration(t *testing.T) {
	engine, chain, parent, s, ctx, keys, scripts := flexibleIntegrationFixture(t)
	extra := registryTestExtra(4, 4)
	extra.ChainID = 63
	extra.Treasury = ScriptHash([]byte{0x54})
	change := RegistryChange{Kind: RegistryRegister, Sequence: 1, ActivationHeight: 4, Coin: extra}
	e, after := registryTestOperation(t, s, ctx, 2, change, keys, nil)
	h2 := flexibleIntegrationHeader(t, engine, chain, parent, e, scripts)
	if err := engine.VerifyHeader(chain, h2, false); err != nil {
		t.Fatal(err)
	}
	chain.add(h2, true)
	h3 := flexibleIntegrationHeader(t, engine, chain, h2, registryTestEnvelope(t, after, 1, 2, 3), scripts)
	if err := engine.VerifyHeader(chain, h3, false); err != nil {
		t.Fatal(err)
	}
	chain.add(h3, true)
	scripts[63] = []byte{0x54}
	h4 := flexibleIntegrationHeader(t, engine, chain, h3, registryTestEnvelope(t, after, 1, 2, 3, 4), scripts)
	if err := engine.VerifyHeader(chain, h4, false); err != nil {
		t.Fatal("registered compatible extra rejected", err)
	}
	if chain.config.Forge.PepecoinPayoutScriptHash == extra.Treasury {
		t.Fatal("fixture accidentally used historical extra treasury pin")
	}
	// A correctly signed candidate cannot redirect a registered bonus reward.
	scripts[63] = []byte{0x55}
	bad := flexibleIntegrationHeader(t, engine, chain, h3, registryTestEnvelope(t, after, 1, 2, 3, 4), scripts)
	if err := engine.VerifyHeader(chain, bad, false); err == nil || !strings.Contains(err.Error(), "treasury") {
		t.Fatal("registered extra diverted its reward", err)
	}
	// New registration remains optional: all three established paths work.
	for _, slot := range []uint8{1, 2, 3} {
		h := flexibleIntegrationHeader(t, engine, chain, h3, registryTestEnvelope(t, after, slot), scripts)
		if err := engine.VerifyHeader(chain, h, false); err != nil {
			t.Fatal("optional extra obstructed core fallback", slot, err)
		}
	}
}

func TestFlexibleExactParentContextAndForeignBranchCheckpoint(t *testing.T) {
	engine, chain, parent, s, ctx, keys, scripts := flexibleIntegrationFixture(t)
	changeA := registryTestRotate(1, 1, 4, 900)
	changeB := registryTestRotate(1, 1, 4, 901)
	ea, sa := registryTestOperation(t, s, ctx, 2, changeA, keys, nil)
	eb, sb := registryTestOperation(t, s, ctx, 2, changeB, keys, nil)
	a := flexibleIntegrationHeader(t, engine, chain, parent, ea, scripts)
	b := flexibleIntegrationHeader(t, engine, chain, parent, eb, scripts)
	for _, h := range []*types.Header{a, b} {
		if err := engine.VerifyHeader(chain, h, false); err != nil {
			t.Fatal(err)
		}
	}
	chain.add(a, true)
	chain.add(b, false)
	childB := flexibleIntegrationHeader(t, engine, chain, b, registryTestEnvelope(t, sb, 1), scripts)
	if err := engine.VerifyHeader(chain, childB, false); err != nil {
		t.Fatal("side-branch exact-parent validation consulted canonical registry", err)
	}
	wrong := flexibleIntegrationHeader(t, engine, chain, b, registryTestEnvelope(t, sa, 1), scripts)
	if err := engine.VerifyHeader(chain, wrong, false); err == nil {
		t.Fatal("canonical snapshot substituted for exact side-branch parent")
	}
	if _, _, _, _, err := RegistrySnapshotAtParent(chain.config, chain, b); err == nil {
		t.Fatal("RPC silently returned canonical state for incompatible side branch")
	}
}

func BenchmarkV5TwoSourceFullVerification(b *testing.B) {
	benchmarkFlexibleVerification(b, []uint8{1, 2}, true, false)
}

func BenchmarkV5ThreeSourceFullVerification(b *testing.B) {
	benchmarkFlexibleVerification(b, []uint8{1, 2, 3}, true, false)
}

func BenchmarkV5ThreeSourceBindings(b *testing.B) {
	benchmarkFlexibleVerification(b, []uint8{1, 2, 3}, false, false)
}

func BenchmarkV5AdministratorOperationFullVerification(b *testing.B) {
	benchmarkFlexibleVerification(b, []uint8{1, 2, 3}, true, true)
}

// Full-verification benchmarks use deliberately invalid nonces. They exercise
// all authorization, native proof and Scrypt work before the final target fails;
// they are computational measurements, not successful mining evidence.
func benchmarkFlexibleVerification(b *testing.B, slots []uint8, pow, operation bool) {
	engine, chain, parent, snapshot, ctx, keys, scripts := flexibleIntegrationFixture(b)
	envelope := registryTestEnvelope(b, snapshot, slots...)
	if operation {
		envelope, _ = registryTestOperation(b, snapshot, ctx, 2, registryTestRotate(1, 1, 4, 900), keys, nil)
	}
	header := flexibleIntegrationHeader(b, engine, chain, parent, envelope, scripts)
	if err := engine.verifyFlexible(chain, header, false); err != nil {
		b.Fatal(err)
	}
	b.ReportAllocs()
	b.SetBytes(int64(len(header.Extra)))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		err := engine.verifyFlexible(chain, header, pow)
		if pow {
			if err == nil || !strings.Contains(err.Error(), "Scrypt proof-of-work") {
				b.Fatal(err)
			}
		} else if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkV4TwoSourceFullVerification(b *testing.B) {
	raw, err := os.ReadFile("testdata/v4-proof-fixtures.json")
	if err != nil {
		b.Fatal(err)
	}
	var rows []struct {
		Mask             uint8
		HeaderRLP, Proof string
		Signers          []common.Address
	}
	if err = json.Unmarshal(raw, &rows); err != nil {
		b.Fatal(err)
	}
	var selected bool
	var header types.Header
	var signers []common.Address
	for _, row := range rows {
		if row.Mask == 3 {
			encoded, e := hex.DecodeString(row.HeaderRLP)
			if e != nil {
				b.Fatal(e)
			}
			if e = rlp.DecodeBytes(encoded, &header); e != nil {
				b.Fatal(e)
			}
			header.Extra, e = hex.DecodeString(row.Proof)
			if e != nil {
				b.Fatal(e)
			}
			signers = row.Signers
			selected = true
			break
		}
	}
	if !selected {
		b.Fatal("two-source V4 fixture missing")
	}
	// Direct construction avoids network initialization; VerifySeal uses the
	// same immutable rules and full V4 binding routine as a running node.
	dgb, _ := hex.DecodeString("0014706267a5255e3666936bcb9a777667599f387546")
	doge, _ := hex.DecodeString("76a9144b1508ebe06723842994adf33bdba36b4eba44c488ac")
	engine := &Forge{config: &params.ChainConfig{Forge: &params.ForgeConfig{RequireMulti: true, AllowAuxTree: true, RequireDGBOrDOGE: true, RewardSchedule: params.RogueRewardSchedule, AuxChainID: 20295, DogeAuxChainID: 98, TemplateSigners: signers, TemplateQuorum: 2, TemplateMaxValidity: 300, LTCPayoutScriptHash: ScriptHash(dgb), DOGEPayoutScriptHash: ScriptHash(doge)}}}
	engine.config.Forge.TargetBlockTime = 12
	engine.config.Forge.InitialDifficulty = big.NewInt(1)
	engine.config.Forge.MinimumDifficulty = big.NewInt(1)
	engine.config.Forge.PreForgeGrossIssued = big.NewInt(1)
	if err = engine.VerifySeal(nil, &header); err == nil || !strings.Contains(err.Error(), "Scrypt proof-of-work") {
		b.Fatal(err)
	}
	b.ReportAllocs()
	b.SetBytes(int64(len(header.Extra)))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if err = engine.VerifySeal(nil, &header); err == nil || !strings.Contains(err.Error(), "Scrypt proof-of-work") {
			b.Fatal(err)
		}
	}
}
