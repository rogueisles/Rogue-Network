package forge

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"github.com/TeamEGEM/go-egem/core/types"
	"github.com/TeamEGEM/go-egem/rlp"
	"math/big"
	"os"
	"testing"
)

func TestMultiNativePoolProofInteroperability(t *testing.T) {
	path := os.Getenv("FORGE_MULTI_PROOF_FIXTURE")
	if path == "" {
		t.Skip("owned native lab fixture required")
	}
	var v struct {
		HeaderRLP, WorkHash, Proof string
		Private                    bool `json:"privateCpuTest"`
	}
	blob, err := os.ReadFile(path)
	if err != nil || json.Unmarshal(blob, &v) != nil || !v.Private {
		t.Fatal("private lab fixture required")
	}
	headerBytes, err := hex.DecodeString(v.HeaderRLP)
	if err != nil {
		t.Fatal(err)
	}
	var h types.Header
	if err := rlp.DecodeBytes(headerBytes, &h); err != nil {
		t.Fatal(err)
	}
	proofBytes, err := hex.DecodeString(v.Proof)
	if err != nil {
		t.Fatal(err)
	}
	p, err := DecodeProof(proofBytes)
	if err != nil {
		t.Fatal(err)
	}
	again, err := EncodeProof(p)
	if err != nil || !bytes.Equal(again, proofBytes) {
		t.Fatal("pool/node proof encoding mismatch", err)
	}
	cfg, _, _ := testForgeConfig(t)
	cfg.ChainId = big.NewInt(7008902)
	cfg.ForgeBlock = big.NewInt(1)
	cfg.Forge.PrivateCPUTest = true
	cfg.Forge.AllowAuxTree = true
	cfg.Forge.InitialDifficulty = big.NewInt(65536)
	cfg.Forge.MinimumDifficulty = big.NewInt(65536)
	dgb, _ := hex.DecodeString("0014706267a5255e3666936bcb9a777667599f387546")
	doge, _ := hex.DecodeString("76a9144b1508ebe06723842994adf33bdba36b4eba44c488ac")
	cfg.Forge.LTCPayoutScriptHash = ScriptHash(dgb)
	cfg.Forge.DOGEPayoutScriptHash = ScriptHash(doge)
	engine, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	h.Extra = proofBytes
	if err := engine.VerifySeal(nil, &h); err != nil {
		t.Fatal("real pool proof rejected by node:", err)
	}
	if len(p.OGGChainBranch) != 5 || len(p.DOGEChainBranch) != 5 {
		t.Fatal("wrong tree depth")
	}
	reject := func(name string, change func(*AuxPowProof)) {
		t.Helper()
		q, e := DecodeProof(proofBytes)
		if e != nil {
			t.Fatal(e)
		}
		change(q)
		h.Extra, e = EncodeProof(q)
		if e != nil {
			t.Fatal(e)
		}
		if engine.VerifySeal(nil, &h) == nil {
			t.Fatal("accepted tampered " + name)
		}
	}
	reject("OGG branch", func(q *AuxPowProof) { q.OGGChainBranch[0][0] ^= 1 })
	reject("DOGE branch", func(q *AuxPowProof) { q.DOGEChainBranch[0][0] ^= 1 })
	reject("signature", func(q *AuxPowProof) { q.Signatures = nil })
	reject("DGB coinbase", func(q *AuxPowProof) { q.ParentCoinbase[len(q.ParentCoinbase)-5] ^= 1 })
	h.Extra = proofBytes
	cfg.Forge.AllowAuxTree = false
	if engine.VerifySeal(nil, &h) == nil {
		t.Fatal("expanded proof accepted when disabled")
	}
	cfg.Forge.AllowAuxTree = true
	cfg.Forge.PrivateCPUTest = false
	if engine.VerifySeal(nil, &h) == nil {
		t.Fatal("CPU lab work accepted under production difficulty")
	}
	t.Log("native five-chain solution also passes OGG node verification; branch, signature, payout and production-target tampering rejected")
}
