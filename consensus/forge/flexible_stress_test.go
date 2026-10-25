package forge

import (
	"bytes"
	"crypto/ecdsa"
	"os"
	"runtime"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/TeamEGEM/go-egem/core/types"
	"github.com/TeamEGEM/go-egem/crypto"
)

func flexibleStressKeys(start int64) []*ecdsa.PrivateKey {
	keys := make([]*ecdsa.PrivateKey, RegistryMaxAdminKeys)
	for i := range keys {
		keys[i] = registryTestKey(start + int64(i))
	}
	sort.Slice(keys, func(i, j int) bool {
		a, b := crypto.PubkeyToAddress(keys[i].PublicKey), crypto.PubkeyToAddress(keys[j].PublicKey)
		return bytes.Compare(a[:], b[:]) < 0
	})
	return keys
}

// Reach the largest registry through verified transitions from the permitted
// three-core bootstrap, rather than seeding an impossible bootstrap snapshot.
func flexibleStressFixture(t testing.TB) (*Forge, *flexibleIntegrationChain, *types.Header, RegistrySnapshot, RegistryContext, []*ecdsa.PrivateKey, map[uint32][]byte) {
	t.Helper()
	engine, chain, parent, snapshot, ctx, keys, scripts := flexibleIntegrationFixture(t)
	nextKeys := flexibleStressKeys(101)
	height := parent.Number.Uint64() + 1
	change := RegistryChange{Kind: RegistryRotateAdmins, Sequence: 1, ActivationHeight: height + ctx.MinDelay, Keys: registryTestAddresses(nextKeys), Quorum: 5}
	envelope, after := registryTestOperation(t, snapshot, ctx, height, change, keys, nextKeys)
	parent = flexibleIntegrationHeader(t, engine, chain, parent, envelope, scripts)
	if err := engine.VerifyHeader(chain, parent, false); err != nil {
		t.Fatal(err)
	}
	chain.add(parent, true)
	snapshot = after
	for parent.Number.Uint64() < change.ActivationHeight {
		parent = flexibleIntegrationHeader(t, engine, chain, parent, registryTestEnvelope(t, snapshot, 1, 2, 3), scripts)
		if err := engine.VerifyHeader(chain, parent, false); err != nil {
			t.Fatal(err)
		}
		chain.add(parent, true)
	}
	keys = nextKeys
	for slot := uint8(4); slot <= RegistryMaxCoins; slot++ {
		height = parent.Number.Uint64() + 1
		coin := registryTestExtra(slot, height+ctx.MinDelay)
		script := []byte{0x51, byte(slot)}
		coin.Treasury = ScriptHash(script)
		scripts[coin.ChainID] = script
		change = RegistryChange{Kind: RegistryRegister, Sequence: snapshot.Admin.Sequence + 1, Coin: coin, ActivationHeight: coin.EnabledFrom}
		envelope, after = registryTestOperation(t, snapshot, ctx, height, change, keys, nil)
		parent = flexibleIntegrationHeader(t, engine, chain, parent, envelope, scripts)
		if err := engine.VerifyHeader(chain, parent, false); err != nil {
			t.Fatal(err)
		}
		chain.add(parent, true)
		snapshot = after
	}
	for i := uint64(0); i < ctx.MinDelay; i++ {
		parent = flexibleIntegrationHeader(t, engine, chain, parent, registryTestEnvelope(t, snapshot, 1, 2, 3), scripts)
		if err := engine.VerifyHeader(chain, parent, false); err != nil {
			t.Fatal(err)
		}
		chain.add(parent, true)
	}
	return engine, chain, parent, snapshot, ctx, keys, scripts
}

func flexibleStressMemberships(t testing.TB, snapshot RegistrySnapshot, envelope *RegistryEnvelope) {
	t.Helper()
	all := registryTestEnvelope(t, snapshot, 1, 2, 3, 4, 5, 6)
	envelope.Members = all.Members
}

// Opt-in computational/retention stress, not full database synchronization,
// block execution, networking or a proof-of-work success measurement.
func TestV5BoundedHeaderValidationStress(t *testing.T) {
	if os.Getenv("RGN_RUN_VALIDATION_STRESS") != "1" {
		t.Skip("set RGN_RUN_VALIDATION_STRESS=1 for bounded 10000-header validation/retention stress")
	}
	const iterations, window, warmup, sampleEvery = 10000, 256, 1000, 1000
	engine, chain, parent, snapshot, ctx, keys, scripts := flexibleStressFixture(t)
	originalScript, alternateScript := []byte{0x51}, []byte{0x53}
	start := time.Now()
	var baseline, final runtime.MemStats
	var maximumHeap, maximumPostGC, operations uint64
	for i := 1; i <= iterations; i++ {
		height := parent.Number.Uint64() + 1
		envelope := registryTestEnvelope(t, snapshot, 1, 2, 3, 4, 5, 6)
		if i%64 == 0 {
			current, err := RegistryTreasury(snapshot.Coins[0], height)
			if err != nil {
				t.Fatal(err)
			}
			next := alternateScript
			if current == ScriptHash(alternateScript) {
				next = originalScript
			}
			change := RegistryChange{Kind: RegistryRotateTreasury, Sequence: snapshot.Admin.Sequence + 1, Slot: 1, Treasury: ScriptHash(next), ActivationHeight: height + ctx.MinDelay}
			var after RegistrySnapshot
			envelope, after = registryTestOperation(t, snapshot, ctx, height, change, keys, nil)
			snapshot = after
			flexibleStressMemberships(t, snapshot, envelope)
			operations++
		}
		payout, err := RegistryTreasury(snapshot.Coins[0], height)
		if err != nil {
			t.Fatal(err)
		}
		if payout == ScriptHash(originalScript) {
			scripts[0] = originalScript
		} else if payout == ScriptHash(alternateScript) {
			scripts[0] = alternateScript
		} else {
			t.Fatal("unexpected stress treasury")
		}
		header := flexibleIntegrationHeader(t, engine, chain, parent, envelope, scripts)
		if err := engine.VerifyHeader(chain, header, false); err != nil {
			t.Fatalf("header %d: %v", i, err)
		}
		chain.add(header, true)
		parent = header
		// Retain a constant-size chain-reader window and immutable genesis.
		if height > window {
			oldHeight := height - window
			delete(chain.byHash, chain.canonical[oldHeight])
			delete(chain.canonical, oldHeight)
		}
		var sample runtime.MemStats
		runtime.ReadMemStats(&sample)
		if sample.HeapAlloc > maximumHeap {
			maximumHeap = sample.HeapAlloc
		}
		if i%sampleEvery == 0 {
			runtime.GC()
			runtime.ReadMemStats(&sample)
			if sample.HeapAlloc > maximumPostGC {
				maximumPostGC = sample.HeapAlloc
			}
			if i == warmup {
				baseline = sample
			}
			final = sample
		}
	}
	if len(chain.byHash) > window+2 || len(chain.canonical) > window+2 {
		t.Fatalf("test chain window grew: hashes=%d canonical=%d", len(chain.byHash), len(chain.canonical))
	}
	// Leave room for runtime caches/map growth while detecting retained data
	// proportional to the thousands of retired header proofs.
	if final.HeapAlloc > baseline.HeapAlloc+8*1024*1024 {
		t.Fatalf("retained heap grew by more than 8 MiB after warmup: start=%d end=%d", baseline.HeapAlloc, final.HeapAlloc)
	}
	t.Logf("validation-only headers=%d admin_operations=%d registered_coins=%d sources=6 window=%d elapsed_ms=%d baseline_post_gc_heap=%d final_post_gc_heap=%d peak_sampled_heap=%d peak_post_gc_heap=%d total_allocated_after_warmup=%d gc_after_warmup=%d", iterations, operations, len(snapshot.Coins), window, time.Since(start).Milliseconds(), baseline.HeapAlloc, final.HeapAlloc, maximumHeap, maximumPostGC, final.TotalAlloc-baseline.TotalAlloc, final.NumGC-baseline.NumGC)
}

func BenchmarkV5MaximumSixSourceFullVerification(b *testing.B) {
	engine, chain, parent, snapshot, _, _, scripts := flexibleStressFixture(b)
	header := flexibleIntegrationHeader(b, engine, chain, parent, registryTestEnvelope(b, snapshot, 1, 2, 3, 4, 5, 6), scripts)
	benchmarkFlexibleMaximum(b, engine, chain, header)
}

func BenchmarkV5MaximumAdministratorRotationFullVerification(b *testing.B) {
	engine, chain, parent, snapshot, ctx, keys, scripts := flexibleStressFixture(b)
	height := parent.Number.Uint64() + 1
	nextKeys := flexibleStressKeys(201)
	change := RegistryChange{Kind: RegistryRotateAdmins, Sequence: snapshot.Admin.Sequence + 1, ActivationHeight: height + ctx.MinDelay, Keys: registryTestAddresses(nextKeys), Quorum: 5}
	envelope, after := registryTestOperation(b, snapshot, ctx, height, change, keys, nextKeys)
	flexibleStressMemberships(b, after, envelope)
	header := flexibleIntegrationHeader(b, engine, chain, parent, envelope, scripts)
	benchmarkFlexibleMaximum(b, engine, chain, header)
}

func benchmarkFlexibleMaximum(b *testing.B, engine *Forge, chain *flexibleIntegrationChain, header *types.Header) {
	b.Helper()
	if err := engine.VerifyHeader(chain, header, false); err != nil {
		b.Fatal(err)
	}
	b.ReportAllocs()
	b.SetBytes(int64(len(header.Extra)))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if err := engine.VerifySeal(chain, header); err == nil || !strings.Contains(err.Error(), "Scrypt proof-of-work") {
			b.Fatal(err)
		}
	}
}
