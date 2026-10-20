package forge

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"math/big"
	"os"
	"strings"
	"testing"

	"github.com/TeamEGEM/go-egem/common"
	"github.com/TeamEGEM/go-egem/core/types"
	"github.com/TeamEGEM/go-egem/rlp"
)

func TestPoolProofInterop(t *testing.T) {
	path := os.Getenv("FORGE_POOL_FIXTURE")
	if path == "" {
		t.Skip("run pool/scripts/test-forge-node.sh")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var fixture struct{ HeaderRLP, WorkHash, Proof, LateProof, TemplateDigest, ScryptHash, Signer string }
	if err := json.Unmarshal(data, &fixture); err != nil {
		t.Fatal(err)
	}
	decode := func(s string) []byte {
		b, err := hex.DecodeString(s)
		if err != nil {
			t.Fatal(err)
		}
		return b
	}
	var issued types.Header
	if err := rlp.DecodeBytes(decode(fixture.HeaderRLP), &issued); err != nil {
		t.Fatal(err)
	}
	work, err := WorkHash(&issued)
	if err != nil || work.Hex() != fixture.WorkHash {
		t.Fatal("pool/node issued work mismatch", err)
	}
	blob := decode(fixture.Proof)
	proof, err := DecodeProof(blob)
	if err != nil {
		t.Fatal(err)
	}
	roundtrip, err := EncodeProof(proof)
	if err != nil || !bytes.Equal(roundtrip, blob) {
		t.Fatal("pool/node proof encoding mismatch", err)
	}
	if TemplateDigest(proof.Template).Hex() != fixture.TemplateDigest {
		t.Fatal("pool/node template digest mismatch")
	}
	scrypt, err := ScryptHash(proof.ParentHeader)
	if err != nil || hex.EncodeToString(scrypt[:]) != fixture.ScryptHash {
		t.Fatal("pool/node Scrypt result mismatch", err)
	}
	cfg, _, _ := testForgeConfig(t)
	if cfg.Forge.TemplateSigners[0] != common.HexToAddress(fixture.Signer) {
		t.Fatal("wrong DEV fixture signer")
	}
	engine, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	sealed := issued
	sealed.Extra = blob
	bound, err := WorkHash(&sealed)
	if err != nil || bound != work || sealed.Hash() == issued.Hash() {
		t.Fatal("proof attachment changed WorkHash or failed to commit final hash")
	}
	// The exact real verifier must pass signatures, time, payouts, headers,
	// coinbases, branches and OGG/DOGE commitments, then reject this unmined
	// nonce at the production PoW target. No easier target or fake seal is used.
	if err := engine.VerifySeal(nil, &sealed); err == nil || err.Error() != "invalid FORGE Scrypt proof-of-work" {
		t.Fatalf("pool proof failed before PoW gate: %v", err)
	}
	late := issued
	late.Extra = decode(fixture.LateProof)
	if err := engine.VerifySeal(nil, &late); err == nil || !strings.Contains(err.Error(), "not valid at block timestamp") {
		t.Fatalf("old submission-time bug not reproduced: %v", err)
	}
	for _, tc := range []struct {
		name, want string
		mutate     func(*AuxPowProof)
	}{
		{"parent-index", "index zero", func(p *AuxPowProof) { p.ParentMerkleIndex = 1 }},
		{"doge-index", "index zero", func(p *AuxPowProof) { p.DogeMerkleIndex = 1 }},
		{"parent-header", "previous hash", func(p *AuxPowProof) { p.ParentHeader[4] ^= 1 }},
		{"doge-header", "signed template hash", func(p *AuxPowProof) { p.DogeHeader[76] ^= 1 }},
		{"signature", "insufficient external template signatures", func(p *AuxPowProof) { p.Signatures = nil }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p, err := DecodeProof(blob)
			if err != nil {
				t.Fatal(err)
			}
			tc.mutate(p)
			extra, err := EncodeProof(p)
			if err != nil {
				t.Fatal(err)
			}
			h := issued
			h.Extra = extra
			if err := engine.VerifySeal(nil, &h); err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("wrong rejection: %v", err)
			}
		})
	}
	substituted := sealed
	substituted.Number = new(big.Int).Add(issued.Number, big.NewInt(1))
	if err := engine.VerifySeal(nil, &substituted); err == nil || !strings.Contains(err.Error(), "merged-mining") {
		t.Fatalf("WorkHash substitution not rejected at commitment: %v", err)
	}
}
