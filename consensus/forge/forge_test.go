package forge

import (
	"encoding/binary"
	"encoding/hex"
	"math/big"
	"strings"
	"testing"

	"github.com/TeamEGEM/go-egem/common"
	"github.com/TeamEGEM/go-egem/core/types"
	"github.com/TeamEGEM/go-egem/crypto"
	"github.com/TeamEGEM/go-egem/params"
)

func testKey(t *testing.T, n byte) string {
	t.Helper()
	b := make([]byte, 32)
	b[31] = n
	return hex.EncodeToString(b)
}

func testForgeConfig(t *testing.T) (*params.ChainConfig, []byte, []byte) {
	t.Helper()
	k, err := crypto.HexToECDSA(testKey(t, 1))
	if err != nil {
		t.Fatal(err)
	}
	ltcScript := []byte{0x51}
	dogeScript := []byte{0x52}
	return &params.ChainConfig{
		ChainId:    big.NewInt(70088),
		ForgeBlock: big.NewInt(100),
		Forge: &params.ForgeConfig{
			AuxChainID:           0x4F47,
			DogeAuxChainID:       0x0062,
			TargetBlockTime:      12,
			PreForgeGrossIssued:  big.NewInt(1),
			InitialDifficulty:    big.NewInt(1),
			MinimumDifficulty:    big.NewInt(1),
			TemplateSigners:      []common.Address{crypto.PubkeyToAddress(k.PublicKey)},
			TemplateQuorum:       1,
			LTCPayoutScriptHash:  ScriptHash(ltcScript),
			DOGEPayoutScriptHash: ScriptHash(dogeScript),
			TemplateMaxValidity:  300,
		},
	}, ltcScript, dogeScript
}

func TestLitecoinScryptBlock29255Vector(t *testing.T) {
	headerHex := "01000000f615f7ce3b4fc6b8f61e8f89aedb1d0852507650533a9e3b10b9bbcc30639f279fcaa86746e1ef52d3edb3c4ad8259920d509bd073605c9bf1d59983752a6b06b817bb4ea78e011d012d59d4"
	header, _ := hex.DecodeString(headerHex)
	raw, err := ScryptHash(header)
	if err != nil {
		t.Fatal(err)
	}
	display := reverse32(raw.Bytes())
	want := common.HexToHash("0x0000000110c8357966576df46f3b802ca897deb7ad18b12f1c24ecff6386ebd9")
	if display != want {
		t.Fatalf("scrypt mismatch: have %s want %s", display.Hex(), want.Hex())
	}
	// This historical block was easier than Bitcoin/Litecoin diff1; its hash is
	// therefore expected not to satisfy the canonical diff1 target.
	target, _ := TargetForDifficulty(big.NewInt(1))
	if scryptHashValue(raw).Cmp(target) <= 0 {
		t.Fatal("block 29255 unexpectedly satisfies canonical diff1 target")
	}
}

func TestAuxSlotsNeverCollideForOGGAndDOGE(t *testing.T) {
	for nonce := uint32(0); nonce < 100000; nonce++ {
		oi := ExpectedChainIndex(nonce, 0x4F47, 2)
		di := ExpectedChainIndex(nonce, 0x0062, 2)
		if oi == di {
			t.Fatalf("slot collision at nonce %d", nonce)
		}
	}
}

func TestProofEnvelopeWorkHashInvariant(t *testing.T) {
	h := &types.Header{
		ParentHash: common.HexToHash("0x01"), Coinbase: common.HexToAddress("0x01"),
		Difficulty: big.NewInt(1), Number: big.NewInt(100), GasLimit: 8000000,
		Time: big.NewInt(1000), Extra: []byte("miner-extra"), UncleHash: types.EmptyUncleHash,
	}
	before, err := WorkHash(h)
	if err != nil {
		t.Fatal(err)
	}
	p := &AuxPowProof{MinerExtra: append([]byte(nil), h.Extra...)}
	enc, err := EncodeProof(p)
	if err != nil {
		t.Fatal(err)
	}
	h2 := *h
	h2.Extra = enc
	after, err := WorkHash(&h2)
	if err != nil {
		t.Fatal(err)
	}
	if before != after {
		t.Fatalf("work hash changed after proof attach: %s != %s", before.Hex(), after.Hex())
	}
	if h.Hash() == h2.Hash() {
		t.Fatal("final block hash did not commit proof envelope")
	}

	oversized := &AuxPowProof{MinerExtra: make([]byte, MaxMinerExtra+1)}
	if _, err := EncodeProof(oversized); err == nil {
		t.Fatal("oversized miner extra accepted")
	}
	bad := append([]byte(nil), enc...)
	bad[4] = 99
	if _, err := DecodeProof(bad); err == nil {
		t.Fatal("unknown proof version accepted")
	}
	huge := &AuxPowProof{ParentCoinbase: make([]byte, MaxProofSize+1)}
	if _, err := EncodeProof(huge); err == nil {
		t.Fatal("oversized proof envelope accepted")
	}
}

func TestDigiShieldAndForkBoundary(t *testing.T) {
	cfg, _, _ := testForgeConfig(t)
	cfg.Forge.InitialDifficulty = big.NewInt(100)
	cfg.Forge.MinimumDifficulty = big.NewInt(50)
	parent := &types.Header{Number: big.NewInt(99), Time: big.NewInt(1000), Difficulty: big.NewInt(999999)}
	if got := CalcDifficulty(cfg, 1012, parent); got.Cmp(big.NewInt(100)) != 0 {
		t.Fatalf("first FORGE difficulty = %v want 100", got)
	}
	parent.Number = big.NewInt(100)
	parent.Difficulty = big.NewInt(100)
	if got := CalcDifficulty(cfg, 1012, parent); got.Cmp(big.NewInt(100)) != 0 {
		t.Fatalf("on-target difficulty = %v want 100", got)
	}
	if got := CalcDifficulty(cfg, 1001, parent); got.Cmp(big.NewInt(109)) != 0 {
		t.Fatalf("fast-block difficulty = %v want 109", got)
	}
	if got := CalcDifficulty(cfg, 1120, parent); got.Cmp(big.NewInt(66)) != 0 {
		t.Fatalf("slow-block difficulty = %v want 66", got)
	}
	parent.Difficulty = big.NewInt(50)
	if got := CalcDifficulty(cfg, 1120, parent); got.Cmp(big.NewInt(50)) != 0 {
		t.Fatalf("minimum difficulty clamp = %v want 50", got)
	}
}

func putVarInt(dst []byte, v uint64) []byte {
	if v < 0xfd {
		return append(dst, byte(v))
	}
	panic("test helper only supports small varints")
}

func makeCoinbase(scriptSig, payoutScript []byte, value uint64, zeroOpReturn bool) []byte {
	b := make([]byte, 0, 128)
	var x4 [4]byte
	binary.LittleEndian.PutUint32(x4[:], 1)
	b = append(b, x4[:]...)
	b = putVarInt(b, 1)
	b = append(b, make([]byte, 32)...)
	binary.LittleEndian.PutUint32(x4[:], 0xffffffff)
	b = append(b, x4[:]...)
	b = putVarInt(b, uint64(len(scriptSig)))
	b = append(b, scriptSig...)
	b = append(b, 0xff, 0xff, 0xff, 0xff)
	count := uint64(1)
	if zeroOpReturn {
		count = 2
	}
	b = putVarInt(b, count)
	var x8 [8]byte
	binary.LittleEndian.PutUint64(x8[:], value)
	b = append(b, x8[:]...)
	b = putVarInt(b, uint64(len(payoutScript)))
	b = append(b, payoutScript...)
	if zeroOpReturn {
		b = append(b, make([]byte, 8)...)
		opret := []byte{0x6a, 0x01, 0x00}
		b = putVarInt(b, uint64(len(opret)))
		b = append(b, opret...)
	}
	b = append(b, 0, 0, 0, 0)
	return b
}

func makeBitcoinHeader(version uint32, prevRaw common.Hash, merkleRaw common.Hash, when, bits, nonce uint32) []byte {
	b := make([]byte, 80)
	binary.LittleEndian.PutUint32(b[0:4], version)
	copy(b[4:36], prevRaw.Bytes())
	copy(b[36:68], merkleRaw.Bytes())
	binary.LittleEndian.PutUint32(b[68:72], when)
	binary.LittleEndian.PutUint32(b[72:76], bits)
	binary.LittleEndian.PutUint32(b[76:80], nonce)
	return b
}

func makeStructuralProof(t *testing.T) (*params.ChainConfig, *types.Header, *AuxPowProof, []byte, []byte) {
	t.Helper()
	cfg, ltcScript, dogeScript := testForgeConfig(t)
	oggHeader := &types.Header{
		ParentHash: common.HexToHash("0x1234"), UncleHash: types.EmptyUncleHash,
		Coinbase: common.HexToAddress("0x1234"), Difficulty: big.NewInt(1), Number: big.NewInt(100),
		GasLimit: 8000000, Time: big.NewInt(1000), Extra: []byte("rig-1"),
	}
	oggWork, err := WorkHash(oggHeader)
	if err != nil {
		t.Fatal(err)
	}

	dogeCB := makeCoinbase([]byte{0x01, 0x01}, dogeScript, 5000000000, false)
	dogeParsed, err := parseCoinbase(dogeCB)
	if err != nil {
		t.Fatal(err)
	}
	dogeVersion := (cfg.Forge.DogeAuxChainID << 16) | (1 << 8) | 1
	dogeHeader := makeBitcoinHeader(dogeVersion, common.HexToHash("0x22"), dogeParsed.TxID, 990, 0x1d00ffff, 7)
	dogeRaw, dogeDisplay, err := bitcoinHeaderHashDisplay(dogeHeader)
	if err != nil {
		t.Fatal(err)
	}

	auxNonce := uint32(3)
	root, _, _, err := BuildAuxRoot(oggWork, dogeRaw, auxNonce, cfg.Forge.AuxChainID, cfg.Forge.DogeAuxChainID)
	if err != nil {
		t.Fatal(err)
	}
	commit := append([]byte{0x01, 0x01}, mergedMiningMagic...)
	commit = append(commit, reverse32(root.Bytes()).Bytes()...)
	var x4 [4]byte
	binary.LittleEndian.PutUint32(x4[:], 2)
	commit = append(commit, x4[:]...)
	binary.LittleEndian.PutUint32(x4[:], auxNonce)
	commit = append(commit, x4[:]...)
	parentCB := makeCoinbase(commit, ltcScript, 5000000000, true)
	parentParsed, err := parseCoinbase(parentCB)
	if err != nil {
		t.Fatal(err)
	}
	prevRaw := common.HexToHash("0x3333")
	parentHeader := makeBitcoinHeader(1, prevRaw, parentParsed.TxID, 995, 0x1d00ffff, 0)
	ltcPrev, _ := bitcoinHeaderPrevHashDisplay(parentHeader)

	tmpl := ExternalTemplate{Version: TemplateVersion, LTCPrevHash: ltcPrev, LTCBits: 0x1d00ffff, DogeAuxHash: dogeDisplay, ValidFrom: 900, Expires: 1100}
	key, err := crypto.HexToECDSA(testKey(t, 1))
	if err != nil {
		t.Fatal(err)
	}
	sig, err := crypto.Sign(TemplateDigest(tmpl).Bytes(), key)
	if err != nil {
		t.Fatal(err)
	}
	p := &AuxPowProof{
		MinerExtra:   append([]byte(nil), oggHeader.Extra...),
		ParentHeader: parentHeader, ParentCoinbase: parentCB, ParentMerkleIndex: 0,
		DogeHeader: dogeHeader, DogeCoinbase: dogeCB, DogeMerkleIndex: 0,
		Template: tmpl, Signatures: [][]byte{sig},
	}
	return cfg, oggHeader, p, ltcScript, dogeScript
}

func TestAuxPowStructuralValidationReachesPoW(t *testing.T) {
	cfg, header, p, _, _ := makeStructuralProof(t)
	enc, err := EncodeProof(p)
	if err != nil {
		t.Fatal(err)
	}
	header.Extra = enc
	if err := verifyAuxPow(cfg.Forge, header, p); err == nil || err.Error() != "invalid FORGE Scrypt proof-of-work" {
		t.Fatalf("structurally valid proof stopped before expected PoW gate: %v", err)
	}
}

func TestExternalPayoutDiversionRejected(t *testing.T) {
	cfg, header, p, _, _ := makeStructuralProof(t)
	badScript := []byte{0x53}
	badCB := makeCoinbase(p.ParentCoinbase[:0], badScript, 5000000000, false)
	_ = badCB
	// Mutating the configured commitment is enough to prove fail-closed payout
	// validation without disturbing the otherwise valid Merkle structure.
	cfg.Forge.LTCPayoutScriptHash = ScriptHash(badScript)
	if err := verifyAuxPow(cfg.Forge, header, p); err == nil || !strings.Contains(err.Error(), "Litecoin payout") {
		t.Fatalf("diverted payout not rejected at payout gate: %v", err)
	}
}

func TestTemplateQuorumDistinctAndFresh(t *testing.T) {
	cfg, _, p, _, _ := makeStructuralProof(t)
	k2, err := crypto.HexToECDSA(testKey(t, 2))
	if err != nil {
		t.Fatal(err)
	}
	cfg.Forge.TemplateSigners = append(cfg.Forge.TemplateSigners, crypto.PubkeyToAddress(k2.PublicKey))
	cfg.Forge.TemplateQuorum = 2
	if err := verifyTemplate(cfg.Forge, p.Template, [][]byte{p.Signatures[0], p.Signatures[0]}, 1000); err == nil {
		t.Fatal("duplicate signer counted twice toward quorum")
	}
	sig2, err := crypto.Sign(TemplateDigest(p.Template).Bytes(), k2)
	if err != nil {
		t.Fatal(err)
	}
	if err := verifyTemplate(cfg.Forge, p.Template, [][]byte{p.Signatures[0], sig2}, 1000); err != nil {
		t.Fatalf("valid two-signer quorum rejected: %v", err)
	}
	if err := verifyTemplate(cfg.Forge, p.Template, [][]byte{p.Signatures[0], sig2}, 1200); err == nil {
		t.Fatal("expired external template accepted")
	}
}

func TestConfigFailsClosedAndNoUncles(t *testing.T) {
	cfg, _, _ := testForgeConfig(t)
	if _, err := New(cfg); err != nil {
		t.Fatalf("complete FORGE config rejected: %v", err)
	}
	bad := *cfg.Forge
	bad.LTCPayoutScriptHash = common.Hash{}
	if err := ValidateConfig(&bad); err == nil {
		t.Fatal("missing payout commitment accepted")
	}

	eng, _ := New(cfg)
	h := &types.Header{UncleHash: types.EmptyUncleHash}
	uncle := &types.Header{Number: big.NewInt(1)}
	block := types.NewBlockWithHeader(h).WithBody(nil, []*types.Header{uncle})
	if err := eng.VerifyUncles(nil, block); err == nil {
		t.Fatal("FORGE uncle accepted")
	}
}
