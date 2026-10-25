package forge

import (
	"bytes"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/TeamEGEM/go-egem/common"
	"github.com/TeamEGEM/go-egem/core/types"
	"github.com/TeamEGEM/go-egem/crypto"
	"github.com/TeamEGEM/go-egem/rlp"
)

// This test uses only existing, public fixture identities. It does not change
// mainnet rules or contact a production node. A new SOQ leaf is fully rebound
// and re-signed so rejection cannot be attributed to a stale certificate.
func TestSOQAdditionalLeafRejectedByUnchangedMainnet(t *testing.T) {
	data, err := os.ReadFile("testdata/v4-proof-fixtures.json")
	if err != nil {
		t.Fatal(err)
	}
	var rows []struct {
		Mask             uint8
		HeaderRLP, Proof string
		Signers          []common.Address
	}
	if err = json.Unmarshal(data, &rows); err != nil {
		t.Fatal(err)
	}
	for _, row := range rows {
		if row.Mask != 3 {
			continue
		}
		hb, err := hex.DecodeString(row.HeaderRLP)
		if err != nil {
			t.Fatal(err)
		}
		var h types.Header
		if err = rlp.DecodeBytes(hb, &h); err != nil {
			t.Fatal(err)
		}
		blob, err := hex.DecodeString(row.Proof)
		if err != nil {
			t.Fatal(err)
		}
		proof, err := DecodeProof(blob)
		if err != nil || proof.Multi == nil {
			t.Fatal("missing public V4 fixture", err)
		}
		p := proof.Multi
		cfg := rogueEngineConfig(t)
		dgb, _ := hex.DecodeString("0014706267a5255e3666936bcb9a777667599f387546")
		doge, _ := hex.DecodeString("76a9144b1508ebe06723842994adf33bdba36b4eba44c488ac")
		cfg.Forge.LTCPayoutScriptHash, cfg.Forge.DOGEPayoutScriptHash = ScriptHash(dgb), ScriptHash(doge)
		cfg.Forge.TemplateSigners = row.Signers
		cfg.Forge.TemplateQuorum = 2
		if err = verifyMultiBindings(cfg.Forge, &h, p); err != nil {
			t.Fatal("baseline RGN+DGB+DOGE fixture rejected", err)
		}
		work, err := WorkHash(&h)
		if err != nil {
			t.Fatal(err)
		}
		cb, err := parseCoinbase(p.ParentCoinbase)
		if err != nil {
			t.Fatal(err)
		}
		_, _, nonce, err := parseMergedMiningCommitment(cb.ScriptSig)
		if err != nil {
			t.Fatal(err)
		}
		leaves := map[uint32]common.Hash{cfg.Forge.AuxChainID: work, 21329: common.HexToHash("0x534f51")}
		for _, a := range p.Aux {
			raw, _, err := bitcoinHeaderHashDisplay(a.Header)
			if err != nil {
				t.Fatal(err)
			}
			leaves[a.ChainID] = raw
		}
		root, size, err := canonicalMultiRoot(leaves, nonce)
		if err != nil {
			t.Fatal(err)
		}
		magic := bytes.Index(cb.ScriptSig, mergedMiningMagic)
		if magic < 0 {
			t.Fatal("fixture has no merged-mining commitment")
		}
		off := 42 + magic + 4
		for i := 0; i < 32; i++ {
			p.ParentCoinbase[off+i] = root[31-i]
		}
		binary.LittleEndian.PutUint32(p.ParentCoinbase[off+32:off+36], size)
		certOffset, _, err := certificatePayloadOffset(cb)
		if err != nil {
			t.Fatal(err)
		}
		normalized := append([]byte(nil), p.ParentCoinbase...)
		for i := certOffset; i < certOffset+32; i++ {
			normalized[i] = 0
		}
		for i := int(p.Template.ExtraOffset); i < int(p.Template.ExtraOffset+p.Template.ExtraSize); i++ {
			normalized[i] = 0
		}
		p.Template.ParentCoinbaseHash = crypto.Keccak256Hash(normalized)
		p.Signatures = certificateTestSign(t, p.Template, work, 1, 2)
		if err = ValidateMultiSignatures(p.Template, work, p.Signatures, cfg.Forge.TemplateSigners, 2, h.Time.Uint64()); err != nil {
			t.Fatal(err)
		}
		cert, err := MultiCertificateDigest(p.Template, work, p.Signatures)
		if err != nil {
			t.Fatal(err)
		}
		copy(p.ParentCoinbase[certOffset:certOffset+32], cert[:])
		cb, err = parseCoinbase(p.ParentCoinbase)
		if err != nil {
			t.Fatal(err)
		}
		parentRoot := merkleRoot(cb.TxID, p.ParentMerkleBranch, 0)
		copy(p.ParentHeader[36:68], parentRoot[:])
		err = verifyMultiBindings(cfg.Forge, &h, p)
		if err == nil || !strings.Contains(err.Error(), "noncanonical or hidden auxiliary commitment") {
			t.Fatal("expected extra SOQ leaf rejection after valid signatures/body binding", err)
		}
		t.Log("Baseline accepted; adding SOQ chain 21329 rejected even after valid fixture quorum re-signing:", err)
		return
	}
	t.Fatal("missing combined DGB+DOGE V4 fixture")
}
