package params

import (
	"math/big"
	"testing"
)

func TestRegistryFutureConfigurationDoesNotRewriteForge(t *testing.T) {
	old := &ChainConfig{ForgeBlock: big.NewInt(1100100), Forge: &ForgeConfig{}}
	updated := *old
	f := *old.Forge
	f.RegistryBlock = big.NewInt(1200000)
	f.RegistryBootstrap = []byte{1, 2, 3}
	f.RegistryMinDelay = 720
	f.RegistryMinOperationInterval = 64
	updated.Forge = &f
	for _, height := range []uint64{1100100, 1199999} {
		if err := old.CheckCompatible(&updated, height); err != nil {
			t.Fatal("future registry rewrites existing FORGE", height, err)
		}
	}
	for _, height := range []uint64{1200000, 1200001} {
		if err := old.CheckCompatible(&updated, height); err == nil || err.RewindTo != 1199999 {
			t.Fatal("active registry replacement unguarded", height, err)
		}
		if err := updated.CheckCompatible(old, height); err == nil {
			t.Fatal("active registry removed")
		}
	}
	for _, change := range []func(*ForgeConfig){
		func(c *ForgeConfig) { c.RegistryBootstrap = []byte{4} },
		func(c *ForgeConfig) { c.RegistryMinDelay++ },
		func(c *ForgeConfig) { c.RegistryMinOperationInterval++ },
		func(c *ForgeConfig) { c.RegistryTargetBlockTimeMillis = 12500 },
		func(c *ForgeConfig) { c.RegistryBlock = big.NewInt(1200001) },
	} {
		changed := updated
		fg := *updated.Forge
		changed.Forge = &fg
		change(&fg)
		if err := updated.CheckCompatible(&changed, 1200000); err == nil {
			t.Fatal("active registry profile changed")
		}
	}
	if err := updated.CheckCompatible(&updated, 1200001); err != nil {
		t.Fatal("identical registry restart refused", err)
	}
}
