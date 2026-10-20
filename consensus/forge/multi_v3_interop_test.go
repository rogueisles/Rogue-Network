package forge

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"github.com/TeamEGEM/go-egem/core/types"
	"github.com/TeamEGEM/go-egem/rlp"
	"math/big"
	"os"
	"path/filepath"
	"testing"
)

func TestMultiV3NativePoolProofInteroperability(t *testing.T) {
	path := os.Getenv("FORGE_MULTI_PROOF_FIXTURE")
	if path == "" {
		t.Skip("owned native lab fixture required")
	}
	runMultiV3Interop(t, path)
	data, e := os.ReadFile(path + ".singles")
	if e != nil {
		t.Fatal(e)
	}
	var rows []json.RawMessage
	if json.Unmarshal(data, &rows) != nil || len(rows) != 5 {
		t.Fatal("missing five native single-chain proofs")
	}
	for i, row := range rows {
		p := filepath.Join(t.TempDir(), "single.json")
		if e = os.WriteFile(p, row, 0600); e != nil {
			t.Fatal(e)
		}
		t.Run(fmt.Sprintf("native-single-%d", i), func(t *testing.T) { runMultiV3Interop(t, p) })
	}
}
func runMultiV3Interop(t *testing.T, path string) {
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
	cfg.Forge.RequireMulti = true
	pep, _ := hex.DecodeString("76a914bb0ff0797074c03e9969359b605997b53c9eaa8488ac")
	dingo, _ := hex.DecodeString("76a914e306e2a1a240f634da707d31079f18431317d7af88ac")
	bells, _ := hex.DecodeString("0014088a288ee425094ccd67779afd0c2016d010368c")
	cfg.Forge.PepecoinPayoutScriptHash = ScriptHash(pep)
	cfg.Forge.DingocoinPayoutScriptHash = ScriptHash(dingo)
	cfg.Forge.BellscoinPayoutScriptHash = ScriptHash(bells)
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
	cfg.Forge.RequireDGBOrDOGE = true
	err = engine.VerifySeal(nil, &h)
	if p.Multi.Template.Mask&3 == 0 {
		if err == nil {
			t.Fatal("real Scrypt bonus-only proof accepted under anchor rule")
		}
	} else if err != nil {
		t.Fatal("real Scrypt anchor proof rejected", err)
	}
	cfg.Forge.RequireDGBOrDOGE = false
	if p.Multi == nil || p.Multi.Template.Mask == 0 {
		t.Fatal("missing five-chain proof")
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
	reject("empty participation", func(q *AuxPowProof) { q.Multi.Template.Mask = 0 })
	if len(p.Multi.Aux) > 0 {
		reject("omitted chain", func(q *AuxPowProof) { q.Multi.Aux = q.Multi.Aux[1:] })
	}
	if len(p.Multi.Aux) > 0 {
		reject("auxiliary branch", func(q *AuxPowProof) { q.Multi.Aux[0].Header[36] ^= 1 })
	}
	reject("signature", func(q *AuxPowProof) { q.Multi.Signatures = nil })
	reject("DGB coinbase", func(q *AuxPowProof) { q.Multi.ParentCoinbase[len(q.Multi.ParentCoinbase)-5] ^= 1 })
	reject("parent version", func(q *AuxPowProof) { q.Multi.ParentHeader[0] ^= 1 })
	for i := 0; i < len(p.Multi.Aux); i++ {
		index := i
		reject("auxiliary treasury", func(q *AuxPowProof) { a := &q.Multi.Aux[index]; a.Coinbase[len(a.Coinbase)-5] ^= 1 })
	}
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
