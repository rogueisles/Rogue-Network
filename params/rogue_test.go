package params

import (
	"encoding/json"
	"math/big"
	"testing"
)

func TestRogueScheduleIsExplicitAndImmutable(t *testing.T) {
	old := &ChainConfig{ChainId: big.NewInt(70088)}
	updated := &ChainConfig{ChainId: big.NewInt(70088), ForgeBlock: big.NewInt(1100100), Forge: &ForgeConfig{RewardSchedule: RogueRewardSchedule}}
	if old.IsRogue(big.NewInt(1100100)) || updated.IsRogue(big.NewInt(1100099)) || !updated.IsRogue(big.NewInt(1100100)) {
		t.Fatal("wrong activation boundary")
	}
	if err := old.CheckCompatible(updated, 1100099); err != nil {
		t.Fatal(err)
	}
	if err := updated.CheckCompatible(old, 1100100); err == nil || err.RewindTo != 1100099 {
		t.Fatal("active schedule downgrade accepted", err)
	}
	b, _ := json.Marshal(updated)
	var roundtrip ChainConfig
	if err := json.Unmarshal(b, &roundtrip); err != nil {
		t.Fatal(err)
	}
	roundtrip.Forge.RewardSchedule = ""
	if err := updated.CheckCompatible(&roundtrip, 1100100); err == nil || err.What != "FORGE parameters" {
		t.Fatal("reward schedule changed after activation", err)
	}
	if MainnetChainConfig.ForgeBlock != nil {
		t.Fatal("unrelated default mainnet activated")
	}
}
