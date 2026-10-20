package forge

import (
	"encoding/binary"
	"github.com/TeamEGEM/go-egem/common"
	"github.com/TeamEGEM/go-egem/core/types"
	"math/big"
	"testing"
)

func TestPrivateCPUConfigurationAndTargets(t *testing.T) {
	c, _, _ := testForgeConfig(t)
	legacy, _ := TargetForDifficulty(big.NewInt(65536))
	got, _ := TargetForConfig(c.Forge, big.NewInt(65536))
	if got.Cmp(legacy) != 0 {
		t.Fatal("legacy changed")
	}
	c.Forge.PrivateCPUTest = true
	c.Forge.MinimumDifficulty = big.NewInt(65536)
	c.Forge.InitialDifficulty = big.NewInt(131072)
	if _, err := New(c); err == nil {
		t.Fatal("wrong chain accepted")
	}
	c.ChainId = big.NewInt(7008902)
	if _, err := New(c); err == nil {
		t.Fatal("late activation accepted")
	}
	c.ForgeBlock = big.NewInt(1)
	if _, err := New(c); err != nil {
		t.Fatal(err)
	}
	got, err := TargetForConfig(c.Forge, big.NewInt(65536))
	if err != nil || got.Cmp(new(big.Int).Lsh(legacy, 32)) != 0 || got.BitLen() > 256 {
		t.Fatal(got, err)
	}
	for _, d := range []*big.Int{nil, big.NewInt(0), big.NewInt(-1), big.NewInt(65535), new(big.Int).Lsh(big.NewInt(1), 257)} {
		if _, err := TargetForConfig(c.Forge, d); err == nil {
			t.Fatal("invalid difficulty accepted", d)
		}
	}
	c.Forge.MinimumDifficulty = big.NewInt(1)
	if _, err := New(c); err == nil {
		t.Fatal("rounding floor accepted")
	}
}

func TestPrivateCPURetargetPrecisionAndBounds(t *testing.T) {
	c, _, _ := testForgeConfig(t)
	c.Forge.PrivateCPUTest = true
	c.ForgeBlock = big.NewInt(1)
	c.Forge.InitialDifficulty = big.NewInt(131072)
	c.Forge.MinimumDifficulty = big.NewInt(65536)
	p := &types.Header{Number: big.NewInt(0), Time: big.NewInt(1000), Difficulty: big.NewInt(1)}
	if CalcDifficulty(c, 1012, p).Int64() != 131072 {
		t.Fatal("first block")
	}
	p.Number = big.NewInt(1)
	p.Difficulty = big.NewInt(131072)
	for _, span := range []uint64{1, 5, 11, 12, 13, 19, 60, 100000} {
		got := CalcDifficulty(c, 1000+span, p)
		bounded := span
		if bounded > 60 {
			bounded = 60
		}
		want := int64(131072 * 96 / (84 + bounded))
		if got.Int64() != want {
			t.Fatal(span, got, want)
		}
	}
	// Minimum has enough precision to increase; slow blocks cannot fall below it.
	p.Difficulty = big.NewInt(65536)
	if CalcDifficulty(c, 1001, p).Cmp(p.Difficulty) <= 0 {
		t.Fatal("stuck at floor")
	}
	if CalcDifficulty(c, ^uint64(0), p).Int64() != 65536 {
		t.Fatal("overflow/floor")
	}
	// Deterministic response to sustained fast and slow spacing.
	for i := 0; i < 30; i++ {
		p.Difficulty = CalcDifficulty(c, 1001, p)
	}
	if p.Difficulty.Int64() < 655360 {
		t.Fatal("no fast response")
	}
	for i := 0; i < 30; i++ {
		p.Difficulty = CalcDifficulty(c, 1060, p)
	}
	if p.Difficulty.Int64() != 65536 {
		t.Fatal("no slow response")
	}
}

func TestPrivateCPURealScryptAndProofChecks(t *testing.T) {
	c, h, p, _, _ := makeStructuralProof(t)
	c.Forge.PrivateCPUTest = true
	c.Forge.InitialDifficulty = big.NewInt(65536)
	c.Forge.MinimumDifficulty = big.NewInt(65536)
	h.Difficulty = big.NewInt(65536)
	// Changing the native header changes the merged leaf: rebuild its commitment.
	leaf, _ := WorkHash(h)
	dogeRaw, _, _ := bitcoinHeaderHashDisplay(p.DogeHeader)
	root, _, _, _ := BuildAuxRoot(leaf, dogeRaw, 3, c.Forge.AuxChainID, c.Forge.DogeAuxChainID)
	commit := append([]byte{1, 1}, mergedMiningMagic...)
	commit = append(commit, reverse32(root.Bytes()).Bytes()...)
	commit = append(commit, 2, 0, 0, 0, 3, 0, 0, 0)
	p.ParentCoinbase = makeCoinbase(commit, []byte{0x51}, 5000000000, true)
	cb, _ := parseCoinbase(p.ParentCoinbase)
	copy(p.ParentHeader[36:68], cb.TxID.Bytes())
	target, _ := TargetForConfig(c.Forge, h.Difficulty)
	found := false
	for nonce := uint32(0); nonce < 2000000; nonce++ {
		binary.LittleEndian.PutUint32(p.ParentHeader[76:], nonce)
		hash, _ := ScryptHash(p.ParentHeader)
		if scryptHashValue(hash).Cmp(target) <= 0 {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("no solution")
	}
	if err := verifyAuxPow(c.Forge, h, p); err != nil {
		t.Fatal(err)
	}
	c.Forge.PrivateCPUTest = false
	if err := verifyAuxPow(c.Forge, h, p); err == nil {
		t.Fatal("CPU solution accepted under default rules")
	}
	c.Forge.PrivateCPUTest = true
	sig := p.Signatures
	p.Signatures = nil
	if err := verifyAuxPow(c.Forge, h, p); err == nil {
		t.Fatal("unsigned accepted")
	}
	p.Signatures = sig
	payout := c.Forge.LTCPayoutScriptHash
	c.Forge.LTCPayoutScriptHash = common.HexToHash("0x1234")
	if err := verifyAuxPow(c.Forge, h, p); err == nil {
		t.Fatal("diverted payout accepted")
	}
	c.Forge.LTCPayoutScriptHash = payout
	for nonce := uint32(0); ; nonce++ {
		binary.LittleEndian.PutUint32(p.ParentHeader[76:], nonce)
		hash, _ := ScryptHash(p.ParentHeader)
		if scryptHashValue(hash).Cmp(target) > 0 {
			break
		}
	}
	if err := verifyAuxPow(c.Forge, h, p); err == nil || err.Error() != "invalid FORGE Scrypt proof-of-work" {
		t.Fatal("invalid PoW not rejected", err)
	}
}
