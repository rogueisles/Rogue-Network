package forge

import (
	"math/big"
	"testing"

	"github.com/TeamEGEM/go-egem/common"
	"github.com/TeamEGEM/go-egem/core/types"
	"github.com/TeamEGEM/go-egem/params"
)

func TestActivatedChainWorkUsesInclusiveTarget(t *testing.T) {
	cfg := rogueEngineConfig(t)
	for _, row := range []struct {
		height           int64
		difficulty, want string
	}{
		{1100099, "25000000000", "25000000000"},
		{1100100, "1", "4295032833"},
		{1100100, "140", "601304596620"},
	} {
		d, _ := new(big.Int).SetString(row.difficulty, 10)
		h := &types.Header{Number: big.NewInt(row.height), Difficulty: d}
		got, err := HeaderWork(cfg, h)
		if err != nil || got.String() != row.want {
			t.Fatalf("height %d work: %v %v", row.height, got, err)
		}
		if d.String() != row.difficulty {
			t.Fatal("header difficulty mutated")
		}
	}
	cfg.Forge.PrivateCPUTest = true
	if _, err := HeaderWork(cfg, &types.Header{Number: big.NewInt(params.RogueActivationHeight), Difficulty: big.NewInt(1)}); err == nil {
		t.Fatal("private target accepted for activated chainwork")
	}
	cfg.Forge.PrivateCPUTest = false
	tooLarge := new(big.Int).Add(diffOneTarget, big.NewInt(1))
	if _, err := HeaderWork(cfg, &types.Header{Number: big.NewInt(params.RogueActivationHeight), Difficulty: tooLarge}); err == nil {
		t.Fatal("zero target work accepted")
	}
	for _, h := range []*types.Header{nil, {Number: big.NewInt(1)}, {Number: big.NewInt(1), Difficulty: big.NewInt(0)}} {
		if _, err := HeaderWork(cfg, h); err == nil {
			t.Fatal("malformed header accepted")
		}
	}
}

func TestActivatedSignerConfigurationFitsCertificate(t *testing.T) {
	for _, count := range []int{3, MaxTemplateSigners, MaxTemplateSigners + 1, 300} {
		cfg := rogueEngineConfig(t)
		cfg.Forge.TemplateSigners = nil
		for i := 1; i <= count; i++ {
			cfg.Forge.TemplateSigners = append(cfg.Forge.TemplateSigners, common.BigToAddress(big.NewInt(int64(i))))
		}
		cfg.Forge.TemplateQuorum = uint64(count)
		err := ValidateConfig(cfg.Forge)
		if (err == nil) != (count <= MaxTemplateSigners) {
			t.Fatalf("count %d acceptance: %v", count, err)
		}
	}
}
