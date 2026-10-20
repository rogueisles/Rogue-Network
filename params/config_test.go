// Copyright 2017 The go-ethereum Authors
// This file is part of the go-ethereum library.
//
// The go-ethereum library is free software: you can redistribute it and/or modify
// it under the terms of the GNU Lesser General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// The go-ethereum library is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
// GNU Lesser General Public License for more details.
//
// You should have received a copy of the GNU Lesser General Public License
// along with the go-ethereum library. If not, see <http://www.gnu.org/licenses/>.

package params

import (
	"math/big"
	"reflect"
	"testing"

	"github.com/TeamEGEM/go-egem/common"
)

func TestCheckCompatible(t *testing.T) {
	type test struct {
		stored, new *ChainConfig
		head        uint64
		wantErr     *ConfigCompatError
	}
	tests := []test{
		{stored: AllEthashProtocolChanges, new: AllEthashProtocolChanges, head: 0, wantErr: nil},
		{stored: AllEthashProtocolChanges, new: AllEthashProtocolChanges, head: 100, wantErr: nil},
		{
			stored:  &ChainConfig{EIP150Block: big.NewInt(10)},
			new:     &ChainConfig{EIP150Block: big.NewInt(20)},
			head:    9,
			wantErr: nil,
		},
		{
			stored: AllEthashProtocolChanges,
			new:    &ChainConfig{HomesteadBlock: nil},
			head:   3,
			wantErr: &ConfigCompatError{
				What:         "Homestead fork block",
				StoredConfig: big.NewInt(0),
				NewConfig:    nil,
				RewindTo:     0,
			},
		},
		{
			stored: AllEthashProtocolChanges,
			new:    &ChainConfig{HomesteadBlock: big.NewInt(1)},
			head:   3,
			wantErr: &ConfigCompatError{
				What:         "Homestead fork block",
				StoredConfig: big.NewInt(0),
				NewConfig:    big.NewInt(1),
				RewindTo:     0,
			},
		},
		{
			stored: &ChainConfig{HomesteadBlock: big.NewInt(30), EIP150Block: big.NewInt(10)},
			new:    &ChainConfig{HomesteadBlock: big.NewInt(25), EIP150Block: big.NewInt(20)},
			head:   25,
			wantErr: &ConfigCompatError{
				What:         "EIP150 fork block",
				StoredConfig: big.NewInt(10),
				NewConfig:    big.NewInt(20),
				RewindTo:     9,
			},
		},
	}

	for _, test := range tests {
		err := test.stored.CheckCompatible(test.new, test.head)
		if !reflect.DeepEqual(err, test.wantErr) {
			t.Errorf("error mismatch:\nstored: %v\nnew: %v\nhead: %v\nerr: %v\nwant: %v", test.stored, test.new, test.head, err, test.wantErr)
		}
	}
}

func TestForgeActivation(t *testing.T) {
	cfg := &ChainConfig{
		ForgeBlock: big.NewInt(1000),
		Forge: &ForgeConfig{
			AuxChainID:      0x4F47,
			DogeAuxChainID:  0x0062,
			TargetBlockTime: 12,
		},
	}
	for _, tc := range []struct {
		height uint64
		want   bool
	}{
		{0, false},
		{999, false},
		{1000, true},
		{1001, true},
	} {
		if got := cfg.IsForge(new(big.Int).SetUint64(tc.height)); got != tc.want {
			t.Fatalf("IsForge(%d) = %v, want %v", tc.height, got, tc.want)
		}
	}

	// Presence of FORGE parameters alone must not activate the fork.
	cfg.ForgeBlock = nil
	if cfg.IsForge(big.NewInt(1000000)) {
		t.Fatal("FORGE activated with nil ForgeBlock")
	}
}

func TestForgeConfigCompatibility(t *testing.T) {
	stored := &ChainConfig{ForgeBlock: nil}
	planned := &ChainConfig{ForgeBlock: big.NewInt(1000)}

	// A not-yet-active fork may be scheduled without requiring a rewind.
	if err := stored.CheckCompatible(planned, 999); err != nil {
		t.Fatalf("future FORGE fork unexpectedly incompatible: %v", err)
	}

	// Once the head reaches the scheduled fork, changing from disabled to active
	// is a consensus change and must be rejected unless the chain is rewound.
	err := stored.CheckCompatible(planned, 1000)
	want := &ConfigCompatError{
		What:         "FORGE fork block",
		StoredConfig: nil,
		NewConfig:    big.NewInt(1000),
		RewindTo:     999,
	}
	if !reflect.DeepEqual(err, want) {
		t.Fatalf("FORGE compatibility mismatch: got %#v want %#v", err, want)
	}
}

func TestForgeDisabledInCurrentOGGConfig(t *testing.T) {
	if TestnetProgPoWChainConfig.ForgeBlock != nil || TestnetProgPoWChainConfig.Forge != nil {
		t.Fatal("current OGG ProgPoW config must keep FORGE disabled")
	}
	if !ForgeDevChainConfig.IsForge(big.NewInt(0)) {
		t.Fatal("FORGE development config should activate at block 0")
	}
}

func TestForgeParametersBecomeImmutableAtActivation(t *testing.T) {
	preIssued := new(big.Int).Mul(big.NewInt(1256950000), big.NewInt(1000000000000000000))
	stored := &ChainConfig{
		ForgeBlock: big.NewInt(1000),
		Forge: &ForgeConfig{
			AuxChainID:          0x4F47,
			DogeAuxChainID:      0x0062,
			TargetBlockTime:     12,
			PreForgeGrossIssued: new(big.Int).Set(preIssued),
		},
	}
	changed := &ChainConfig{
		ForgeBlock: big.NewInt(1000),
		Forge: &ForgeConfig{
			AuxChainID:          0x4F47,
			DogeAuxChainID:      0x0062,
			TargetBlockTime:     12,
			PreForgeGrossIssued: new(big.Int).Add(preIssued, big.NewInt(1)),
		},
	}

	if err := stored.CheckCompatible(changed, 999); err != nil {
		t.Fatalf("FORGE params should be changeable before activation: %v", err)
	}
	if err := stored.CheckCompatible(changed, 1000); err == nil || err.What != "FORGE parameters" || err.RewindTo != 999 {
		t.Fatalf("active FORGE gross-issued snapshot mismatch not rejected correctly: %#v", err)
	}
}

func TestForgeSecurityParametersBecomeImmutableAtActivation(t *testing.T) {
	baseForge := &ForgeConfig{
		AuxChainID:           0x4F47,
		DogeAuxChainID:       0x0062,
		TargetBlockTime:      12,
		PreForgeGrossIssued:  big.NewInt(1),
		InitialDifficulty:    big.NewInt(100),
		MinimumDifficulty:    big.NewInt(1),
		TemplateSigners:      []common.Address{common.HexToAddress("0x1")},
		TemplateQuorum:       1,
		LTCPayoutScriptHash:  common.HexToHash("0x11"),
		DOGEPayoutScriptHash: common.HexToHash("0x22"),
		TemplateMaxValidity:  120,
	}
	stored := &ChainConfig{ForgeBlock: big.NewInt(1000), Forge: baseForge}
	changedForge := *baseForge
	changedForge.TemplateMaxValidity = 121
	changed := &ChainConfig{ForgeBlock: big.NewInt(1000), Forge: &changedForge}

	if err := stored.CheckCompatible(changed, 999); err != nil {
		t.Fatalf("FORGE security params should be changeable before activation: %v", err)
	}
	if err := stored.CheckCompatible(changed, 1000); err == nil || err.What != "FORGE parameters" || err.RewindTo != 999 {
		t.Fatalf("active FORGE security parameter mismatch not rejected correctly: %#v", err)
	}
}

func TestPrivateCPUCompatibility(t *testing.T) {
	old := &ChainConfig{ChainId: big.NewInt(7008902), ForgeBlock: big.NewInt(1), Forge: &ForgeConfig{}}
	changed := *old
	fc := *old.Forge
	fc.PrivateCPUTest = true
	changed.Forge = &fc
	if err := old.CheckCompatible(&changed, 1); err == nil {
		t.Fatal("CPU domain change after activation accepted")
	}
	if MainnetChainConfig.ForgeBlock != nil {
		t.Fatal("mainnet FORGE enabled")
	}
}
