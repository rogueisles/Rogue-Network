package params

import (
	"math/big"
	"testing"
)

func TestOptionalSOQConfigCompatibility(t *testing.T) {
	base := &ChainConfig{ForgeBlock: big.NewInt(1100100), Forge: &ForgeConfig{}}
	updated := *base
	f := *base.Forge
	f.OptionalSOQBlock = big.NewInt(1125600)
	updated.Forge = &f
	for _, head := range []uint64{1100100, 1125599} {
		if e := base.CheckCompatible(&updated, head); e != nil {
			t.Fatal("future upgrade incorrectly rewinds old fork", head, e)
		}
	}
	for _, head := range []uint64{1125600, 1125601} {
		e := base.CheckCompatible(&updated, head)
		if e == nil || e.RewindTo != 1125599 {
			t.Fatal("active change not guarded", head, e)
		}
	}
	changed := updated
	g := *updated.Forge
	g.OptionalSOQBlock = big.NewInt(1125700)
	changed.Forge = &g
	if e := updated.CheckCompatible(&changed, 1125600); e == nil || e.RewindTo != 1125599 {
		t.Fatal("active activation rescheduled", e)
	}
	if e := updated.CheckCompatible(base, 1125600); e == nil || e.RewindTo != 1125599 {
		t.Fatal("active optional rule removed", e)
	}
	if e := updated.CheckCompatible(&updated, 1125601); e != nil {
		t.Fatal(e)
	}
}
