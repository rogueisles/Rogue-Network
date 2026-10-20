package forge

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"github.com/TeamEGEM/go-egem/common"
	"github.com/TeamEGEM/go-egem/core/types"
	"github.com/TeamEGEM/go-egem/crypto"
	"github.com/TeamEGEM/go-egem/rlp"
	"math/big"
	"os"
	"strings"
	"testing"
)

func TestMultiV3AllSubsetConsensus(t *testing.T) {
	path := os.Getenv("FORGE_MULTI_MATRIX_PROOFS")
	if path == "" {
		t.Skip("owned private matrix lab required")
	}
	var rows []struct {
		Mask             uint8
		Proof, HeaderRLP string
	}
	raw, e := os.ReadFile(path)
	if e != nil || json.Unmarshal(raw, &rows) != nil || len(rows) != 31 {
		t.Fatal("missing complete matrix")
	}
	cfg, _, _ := testForgeConfig(t)
	cfg.ChainId = big.NewInt(7008902)
	cfg.ForgeBlock = big.NewInt(1)
	cfg.Forge.PrivateCPUTest = true
	cfg.Forge.AllowAuxTree = true
	cfg.Forge.RequireMulti = true
	cfg.Forge.MinimumDifficulty = big.NewInt(65536)
	cfg.Forge.InitialDifficulty = big.NewInt(65536)
	hashes := []*common.Hash{&cfg.Forge.LTCPayoutScriptHash, &cfg.Forge.DOGEPayoutScriptHash, &cfg.Forge.PepecoinPayoutScriptHash, &cfg.Forge.DingocoinPayoutScriptHash, &cfg.Forge.BellscoinPayoutScriptHash}
	scripts := []string{"0014706267a5255e3666936bcb9a777667599f387546", "76a9144b1508ebe06723842994adf33bdba36b4eba44c488ac", "76a914bb0ff0797074c03e9969359b605997b53c9eaa8488ac", "76a914e306e2a1a240f634da707d31079f18431317d7af88ac", "0014088a288ee425094ccd67779afd0c2016d010368c"}
	for i, s := range scripts {
		b, _ := hex.DecodeString(s)
		*hashes[i] = ScriptHash(b)
	}
	engine, e := New(cfg)
	if e != nil {
		t.Fatal(e)
	}
	key, _ := crypto.HexToECDSA(strings.Repeat("0", 63) + "1")
	for _, row := range rows {
		t.Run(fmt.Sprintf("mask-%02x", row.Mask), func(t *testing.T) {
			b, _ := hex.DecodeString(row.HeaderRLP)
			var h types.Header
			if e = rlp.DecodeBytes(b, &h); e != nil {
				t.Fatal(e)
			}
			if h.Difficulty.Cmp(big.NewInt(65536)) != 0 {
				t.Fatal("private fixture difficulty changed")
			}
			original, _ := hex.DecodeString(row.Proof)
			h.Extra = original
			parsed, err := DecodeProof(original)
			if err != nil {
				t.Fatal(err)
			}
			if e = verifyMultiBindings(cfg.Forge, &h, parsed.Multi); e != nil {
				t.Fatal("valid subset rejected", e)
			}
			reject := func(name string, change func(*MultiProof), resign bool) {
				t.Helper()
				q, err := DecodeProof(original)
				if err != nil {
					t.Fatal(err)
				}
				change(q.Multi)
				if resign {
					work, err := WorkHash(&h)
					if err != nil {
						t.Fatal(err)
					}
					digest, err := MultiDigest(q.Multi.Template, work)
					if err != nil {
						t.Fatal(err)
					}
					sig, err := crypto.Sign(digest.Bytes(), key)
					if err != nil {
						t.Fatal(err)
					}
					q.Multi.Signatures = [][]byte{sig}
				}
				h.Extra, err = EncodeProof(q)
				if err != nil {
					t.Fatal(err)
				}
				if verifyMultiBindings(cfg.Forge, &h, q.Multi) == nil {
					t.Fatal("accepted " + name)
				}
				h.Extra = original
			}
			reject("zero participation", func(p *MultiProof) { p.Template.Mask = 0 }, false)
			reject("signature replay", func(p *MultiProof) { p.Template.Expires-- }, false)
			reject("parent body", func(p *MultiProof) { p.ParentCoinbase[len(p.ParentCoinbase)-1] ^= 1 }, false)
			reject("parent branch", func(p *MultiProof) { p.ParentMerkleBranch = append(p.ParentMerkleBranch, common.Hash{}) }, false)
			reject("legacy proof bypass", func(p *MultiProof) { p.Template.Version = 2 }, false)
			if len(parsed.Multi.Aux) > 0 {
				bit := MultiChainBit(parsed.Multi.Aux[0].ChainID)
				if row.Mask & ^bit != 0 {
					reject("hidden undeclared auxiliary leaf", func(p *MultiProof) { p.Template.Mask &^= bit; p.Template.Aux = p.Template.Aux[1:]; p.Aux = p.Aux[1:] }, true)
				}
			}
			// A valid template signature does not override the chain's treasury pins.
			for i, bit := range []uint8{1, 2, 4, 8, 16} {
				if row.Mask&bit == 0 {
					continue
				}
				previous := *hashes[i]
				*hashes[i] = common.Hash{1}
				if verifyMultiBindings(cfg.Forge, &h, parsed.Multi) == nil {
					t.Fatal("wrong treasury pin accepted")
				}
				*hashes[i] = previous
			}
			if len(row.Proof) > 0 {
				q, _ := DecodeProof(original)
				q.Multi = nil
				q.Template.Version = 0x21
				h.Extra, _ = EncodeProof(q)
				if engine.VerifySeal(nil, &h) == nil {
					t.Fatal("legacy envelope bypass accepted")
				}
				h.Extra = original
			}
			cfg.Forge.PrivateCPUTest = false
			if engine.VerifySeal(nil, &h) == nil {
				t.Fatal("CPU matrix work accepted at production target")
			}
			cfg.Forge.PrivateCPUTest = true
		})
	}
}
