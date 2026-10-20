package params

import (
	"encoding/json"
	"math/big"
	"testing"
)

func TestForgeRetargetV2Compatibility(t *testing.T) {
	old := &ChainConfig{ForgeBlock: big.NewInt(100), Forge: &ForgeConfig{}}
	updated := &ChainConfig{ForgeBlock: big.NewInt(100), Forge: &ForgeConfig{RetargetV2: true}}
	if err := old.CheckCompatible(updated, 99); err != nil {
		t.Fatal(err)
	}
	if err := old.CheckCompatible(updated, 100); err == nil || err.RewindTo != 99 {
		t.Fatal("active rule replacement accepted", err)
	}
	if err := updated.CheckCompatible(old, 100); err == nil {
		t.Fatal("active rule downgrade accepted")
	}
	var omitted ForgeConfig
	if err := json.Unmarshal([]byte(`{}`), &omitted); err != nil || omitted.RetargetV2 {
		t.Fatal("legacy default changed")
	}
	b, err := json.Marshal(updated)
	if err != nil {
		t.Fatal(err)
	}
	var roundtrip ChainConfig
	if err := json.Unmarshal(b, &roundtrip); err != nil || !roundtrip.Forge.RetargetV2 {
		t.Fatal("flag not preserved")
	}
	if MainnetChainConfig.ForgeBlock != nil {
		t.Fatal("mainnet activation changed")
	}
}
