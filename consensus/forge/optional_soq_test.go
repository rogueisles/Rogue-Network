package forge

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"github.com/TeamEGEM/go-egem/common"
	"github.com/TeamEGEM/go-egem/core/types"
	"github.com/TeamEGEM/go-egem/params"
	"github.com/TeamEGEM/go-egem/rlp"
	"math/big"
	"os"
	"testing"
)

func TestOptionalSOQPoolInteropAndActivation(t *testing.T) {
	data, err := os.ReadFile("testdata/optional-soq-fixtures.json")
	if err != nil {
		t.Fatal(err)
	}
	var rows []struct {
		Mask             uint8
		Height           uint64
		HeaderRLP, Proof string
		Signers          []common.Address
	}
	if err = json.Unmarshal(data, &rows); err != nil || len(rows) != 21 {
		t.Fatal("missing 21 independent pool fixtures", err)
	}
	for _, row := range rows {
		var h types.Header
		b, _ := hex.DecodeString(row.HeaderRLP)
		if err = rlp.DecodeBytes(b, &h); err != nil {
			t.Fatal(err)
		}
		blob, _ := hex.DecodeString(row.Proof)
		p, e := DecodeProof(blob)
		if e != nil {
			t.Fatal(e)
		}
		cfg := rogueEngineConfig(t)
		dgb, _ := hex.DecodeString("0014706267a5255e3666936bcb9a777667599f387546")
		doge, _ := hex.DecodeString("76a9144b1508ebe06723842994adf33bdba36b4eba44c488ac")
		cfg.Forge.LTCPayoutScriptHash = ScriptHash(dgb)
		cfg.Forge.DOGEPayoutScriptHash = ScriptHash(doge)
		cfg.Forge.TemplateSigners = row.Signers
		cfg.Forge.TemplateQuorum = 2
		cfg.Forge.OptionalSOQBlock = big.NewInt(1125600)
		want := row.Mask&ExternalSOQ == 0 || row.Height >= 1125600
		err = verifyMultiBindings(cfg.Forge, &h, p.Multi)
		if (err == nil) != want {
			t.Fatalf("height %d mask %d accepted=%v want=%v: %v", row.Height, row.Mask, err == nil, want, err)
		}
		if want {
			reencoded, e := EncodeProof(p)
			if e != nil || !bytes.Equal(reencoded, blob) {
				t.Fatal("pool/node wire changed", e)
			}
		}
		cfg.Forge.OptionalSOQBlock = nil
		err = verifyMultiBindings(cfg.Forge, &h, p.Multi)
		if (err == nil) != (row.Mask&ExternalSOQ == 0) {
			t.Fatal("disabled feature changed historical acceptance", row.Mask, err)
		}
		cfg.Forge.OptionalSOQBlock = big.NewInt(1125600)
		if want && row.Mask&1 != 0 {
			cfg.Forge.LTCPayoutScriptHash = common.HexToHash("0xff")
			if verifyMultiBindings(cfg.Forge, &h, p.Multi) == nil {
				t.Fatal("DGB pin bypassed")
			}
			cfg.Forge.LTCPayoutScriptHash = ScriptHash(dgb)
		}
		if want && row.Mask&2 != 0 {
			cfg.Forge.DOGEPayoutScriptHash = common.HexToHash("0xff")
			if verifyMultiBindings(cfg.Forge, &h, p.Multi) == nil {
				t.Fatal("DOGE pin bypassed")
			}
		}
	}
}

func TestOptionalSOQConfigRejectsInvalidSchedule(t *testing.T) {
	for _, height := range []int64{-1, 0, params.RogueActivationHeight} {
		c := rogueEngineConfig(t)
		c.Forge.OptionalSOQBlock = big.NewInt(height)
		if validateConfig(c.Forge) == nil || c.ValidateRogueSchedule() {
			t.Fatal("invalid SOQ activation accepted", height)
		}
	}
	c := rogueEngineConfig(t)
	c.Forge.OptionalSOQBlock = big.NewInt(1125600)
	if err := validateConfig(c.Forge); err != nil || !c.ValidateRogueSchedule() {
		t.Fatal(err)
	}
	c.Forge.RewardSchedule = ""
	if validateConfig(c.Forge) == nil {
		t.Fatal("SOQ enabled on a legacy profile")
	}
}

func TestOptionalSOQDestinationShape(t *testing.T) {
	for _, script := range [][]byte{{0x51, 0x20}, {0x76, 0xa9, 0x14}, append([]byte{0x51, 0x20}, make([]byte, 32)...)} {
		cb, e := parseCoinbase(makeCoinbase([]byte{1, 1}, script, 100000, true))
		if e != nil {
			t.Fatal(e)
		}
		if validSOQOutputs(cb) != (len(script) == 34) {
			t.Fatal("unexpected destination acceptance")
		}
	}
}
