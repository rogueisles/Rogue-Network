package forge

import (
	"encoding/json"
	"math"
	"math/big"
	"math/rand"
	"testing"
	"time"

	"github.com/TeamEGEM/go-egem/consensus"
	"github.com/TeamEGEM/go-egem/core/types"
	"github.com/TeamEGEM/go-egem/params"
)

func TestRetargetV2RegressionAndBoundaries(t *testing.T) {
	c, _, _ := testForgeConfig(t)
	p := &types.Header{Number: big.NewInt(100), Time: big.NewInt(1000), Difficulty: big.NewInt(1)}
	if got := CalcDifficulty(c, 1001, p); got.Int64() != 1 {
		t.Fatal("historical rules changed")
	}
	c.Forge.RetargetV2 = true
	if _, err := New(c); err != nil {
		t.Fatal(err)
	}
	// Fixed vectors exercise the old 5..19s dead band and low-integer escape.
	for _, tc := range []struct {
		d    int64
		span uint64
		want int64
	}{
		{1, 1, 2}, {1, 11, 2}, {1, 12, 1}, {1, 13, 1},
		{2, 11, 3}, {2, 13, 1}, {100, 1, 107}, {100, 5, 104},
		{100, 11, 101}, {100, 12, 100}, {100, 13, 99}, {100, 19, 96},
		{100, 60, 76}, {100, 1000000, 1}, {100, ^uint64(0) - 1000, 1},
	} {
		p.Difficulty.SetInt64(tc.d)
		got := CalcDifficulty(c, 1000+tc.span, p)
		if got.Int64() != tc.want {
			t.Fatalf("d=%d span=%d got %v want %d", tc.d, tc.span, got, tc.want)
		}
		if p.Difficulty.Int64() != tc.d {
			t.Fatal("mutated parent")
		}
	}
	p.Difficulty.SetInt64(100)
	c.Forge.MinimumDifficulty.SetInt64(80)
	if CalcDifficulty(c, 1060, p).Int64() != 80 {
		t.Fatal("floor not enforced")
	}
	c.Forge.InitialDifficulty.SetInt64(140)
	p.Number.SetInt64(99)
	if CalcDifficulty(c, 1012, p).Int64() != 140 {
		t.Fatal("activation difficulty ignored")
	}
	c.Forge.MinimumDifficulty.SetInt64(1)
	if CalcDifficulty(c, 1132, p).Int64() != 70 {
		t.Fatal("overdue first block cannot ease")
	}
	p.Difficulty.SetInt64(999999999)
	if CalcDifficulty(c, 1012, p).Int64() != 140 {
		t.Fatal("pre-fork work units leaked into activation")
	}
	// The new rule changes neither canonical proof targets nor chain work units.
	for _, d := range []int64{1, 140, 1400000} {
		a, _ := TargetForDifficulty(big.NewInt(d))
		b, _ := TargetForConfig(c.Forge, big.NewInt(d))
		if a.Cmp(b) != 0 {
			t.Fatal("canonical target domain changed")
		}
	}
	c.Forge.PrivateCPUTest = true
	if _, err := New(c); err == nil {
		t.Fatal("ambiguous CPU/v2 mode accepted")
	}
	c.Forge.PrivateCPUTest = false
	c.Forge.TargetBlockTime = 60
	if _, err := New(c); err == nil {
		t.Fatal("unsupported target spacing accepted")
	}
}

func TestRetargetV2DirectionPrecisionAndOverflow(t *testing.T) {
	c, _, _ := testForgeConfig(t)
	c.Forge.RetargetV2 = true
	p := &types.Header{Number: big.NewInt(100), Time: big.NewInt(1000)}
	for _, d := range []*big.Int{big.NewInt(1), big.NewInt(2), big.NewInt(10), big.NewInt(140), new(big.Int).Lsh(big.NewInt(1), 200)} {
		p.Difficulty = d
		previous := CalcDifficulty(c, 1001, p)
		for span := uint64(1); span <= 120; span++ {
			got := CalcDifficulty(c, 1000+span, p)
			if got.Cmp(previous) > 0 {
				t.Fatal("difficulty increased with longer spacing")
			}
			if span < 12 && got.Cmp(d) <= 0 {
				t.Fatal("fast-block change lost")
			}
			if span == 12 && got.Cmp(d) != 0 {
				t.Fatal("on-target drift")
			}
			if span > 12 && d.Int64() != 1 && got.Cmp(d) >= 0 {
				t.Fatal("slow-block change lost")
			}
			previous = got
		}
	}
}

func TestRetargetV2ExponentialReferenceAndLimits(t *testing.T) {
	c, _, _ := testForgeConfig(t)
	c.Forge.RetargetV2 = true
	p := &types.Header{Number: big.NewInt(100), Time: big.NewInt(1000), Difficulty: big.NewInt(1000000000000)}
	// Independent floating-point exponential is used ONLY as a test oracle.
	// Exercise all second residues across positive/negative exponent boundaries.
	for span := uint64(1); span <= 2400; span++ {
		want := 1e12 * math.Exp2((12-float64(span))/120)
		got, _ := new(big.Float).SetInt(CalcDifficulty(c, 1000+span, p)).Float64()
		if math.Abs(got-want) > want*0.0002+1 {
			t.Fatalf("exponential approximation at %d: got %.9f want %.9f", span, got, want)
		}
	}
	for _, span := range []uint64{132, 252, 372} {
		want := new(big.Int).Rsh(p.Difficulty, uint((span-12)/120))
		if CalcDifficulty(c, 1000+span, p).Cmp(want) != 0 {
			t.Fatal("halving is not exact", span)
		}
	}
	p.Difficulty = copyBig(diffOneTarget)
	got := CalcDifficulty(c, 1001, p)
	if got.Cmp(diffOneTarget) != 0 {
		t.Fatal("upper saturation failed")
	}
	target, err := TargetForDifficulty(got)
	if err != nil || target.Sign() <= 0 {
		t.Fatal("zero target", target, err)
	}
	c.Forge.InitialDifficulty = new(big.Int).Add(diffOneTarget, big.NewInt(1))
	if _, err := New(c); err == nil {
		t.Fatal("unmineable initial target accepted")
	}
}

func TestRetargetV2RejectsFutureTimestamp(t *testing.T) {
	c, _, _ := testForgeConfig(t)
	c.Forge.RetargetV2 = true
	f, err := New(c)
	if err != nil {
		t.Fatal(err)
	}
	p := &types.Header{Number: big.NewInt(100), Time: big.NewInt(1000), Difficulty: big.NewInt(140)}
	h := &types.Header{Number: big.NewInt(101), Time: big.NewInt(time.Now().Unix() + 3600)}
	if err := f.verifyHeader(nil, h, p, false); err != consensus.ErrFutureBlock {
		t.Fatal(err)
	}
	h.Time = new(big.Int).Lsh(big.NewInt(1), 65)
	if err := f.verifyHeader(nil, h, p, false); err == nil {
		t.Fatal("oversized time accepted")
	}
	// A wrong difficulty is rejected even without executing PoW.
	h.Time = big.NewInt(1012)
	h.Difficulty = big.NewInt(1)
	if err := f.verifyHeader(nil, h, p, false); err == nil {
		t.Fatal("wrong difficulty accepted")
	}
}

func TestRetargetV2PrepareAndIndependentHeaderValidation(t *testing.T) {
	c, _, _ := testForgeConfig(t)
	c.ForgeBlock = big.NewInt(1)
	c.Forge.RetargetV2 = true
	primary, err := New(c)
	if err != nil {
		t.Fatal(err)
	}
	blob, _ := json.Marshal(c)
	var peerConfig params.ChainConfig
	if err := json.Unmarshal(blob, &peerConfig); err != nil {
		t.Fatal(err)
	}
	peer, err := New(&peerConfig)
	if err != nil {
		t.Fatal(err)
	}
	p := &types.Header{Number: big.NewInt(0), Time: big.NewInt(1000000), Difficulty: big.NewInt(1), GasLimit: 8000000}
	chain := &hybridTestChain{config: c, headers: map[uint64]*types.Header{0: p}}
	var headers []*types.Header
	for i := int64(1); i <= 300; i++ {
		span := int64(1)
		if i > 150 {
			span = 60
		}
		h := &types.Header{Number: big.NewInt(i), Time: new(big.Int).Add(p.Time, big.NewInt(span)), ParentHash: p.Hash(), GasLimit: p.GasLimit}
		if err := primary.Prepare(chain, h); err != nil {
			t.Fatal(err)
		}
		if err := peer.VerifyHeader(chain, h, false); err != nil {
			t.Fatal(i, err)
		}
		headers = append(headers, h)
		chain.headers[uint64(i)] = h
		p = h
	}
	if headers[149].Difficulty.Cmp(big.NewInt(1)) <= 0 || p.Difficulty.Int64() != 1 {
		t.Fatal("header path did not adapt up and down")
	}
	abort, results := peer.VerifyHeaders(chain, headers, make([]bool, len(headers)))
	defer close(abort)
	for range headers {
		if err := <-results; err != nil {
			t.Fatal(err)
		}
	}
	// This is header-path validation without seals, not live PoW integration.
}

// This is a Poisson-hash simulation, not an integration test or real mining.
// It calls the production CalcDifficulty for every refreshed candidate, holds
// that job's target until refresh, and includes the worker's timestamp wait.
// No stale shares, network delays, propagation or adversarial timestamps are
// modeled. Hashrate is expressed as the equilibrium difficulty at 12s.
type retargetSimulation struct {
	Scenario              string    `json:"scenario"`
	RefreshSeconds        float64   `json:"refreshSeconds"`
	Seed                  int64     `json:"seed"`
	Blocks                int       `json:"blocks"`
	InitialDifficulty     int64     `json:"initialDifficulty"`
	EquilibriumDifficulty float64   `json:"equilibriumDifficulty"`
	First100Mean          float64   `json:"first100MeanSeconds"`
	First100MaxGap        float64   `json:"first100MaxGapSeconds"`
	SteadyMean            float64   `json:"steadyMeanSeconds"`
	SteadyHeaderMean      float64   `json:"steadyHeaderMeanSeconds"`
	FinalDifficulty       string    `json:"finalDifficulty"`
	PhaseMeans            []float64 `json:"phaseSteadyMeansSeconds,omitempty"`
}

func simulateRetarget(t *testing.T, name string, initial int64, equilibrium, refresh float64, seed int64) retargetSimulation {
	c, _, _ := testForgeConfig(t)
	c.Forge.RetargetV2 = true
	c.ForgeBlock = big.NewInt(101)
	c.Forge.InitialDifficulty = big.NewInt(initial)
	p := &types.Header{Number: big.NewInt(100), Time: big.NewInt(1000000), Difficulty: big.NewInt(initial)}
	r := rand.New(rand.NewSource(seed))
	wall := float64(p.Time.Int64())
	out := retargetSimulation{Scenario: name, RefreshSeconds: refresh, Seed: seed, Blocks: 25000, InitialDifficulty: initial, EquilibriumDifficulty: equilibrium}
	for i := 0; i < out.Blocks; i++ {
		currentEquilibrium := equilibrium
		if name == "repeated-switching-50GH-to-5TH" && (i/5000)%2 == 1 {
			currentEquilibrium *= 100
		}
		start := wall
		oldTime := p.Time.Uint64()
		remainingHazard := r.ExpFloat64()
		for tries := 0; ; tries++ {
			if tries > 1000000 {
				t.Fatal("simulation failed to progress")
			}
			stamp := uint64(wall)
			if stamp <= oldTime {
				stamp = oldTime + 1
			}
			if stamp > uint64(wall)+1 {
				wall = float64(stamp)
			}
			d := CalcDifficulty(c, stamp, p)
			df, _ := new(big.Float).SetInt(d).Float64()
			rate := currentEquilibrium / (12 * df)
			if remainingHazard <= refresh*rate {
				wall += remainingHazard / rate
				p.Time.SetUint64(stamp)
				p.Difficulty = d
				p.Number.Add(p.Number, big.NewInt(1))
				break
			}
			remainingHazard -= refresh * rate
			wall += refresh
		}
		elapsed := wall - start
		if name == "repeated-switching-50GH-to-5TH" {
			phase := i / 5000
			if len(out.PhaseMeans) <= phase {
				out.PhaseMeans = append(out.PhaseMeans, 0)
			}
			if i%5000 >= 1000 {
				out.PhaseMeans[phase] += elapsed / 4000
			}
		}
		if i < 100 {
			out.First100Mean += elapsed / 100
			out.First100MaxGap = math.Max(out.First100MaxGap, elapsed)
		}
		if i >= 5000 {
			out.SteadyMean += elapsed / 20000
			out.SteadyHeaderMean += float64(p.Time.Uint64()-oldTime) / 20000
		}
	}
	out.FinalDifficulty = p.Difficulty.String()
	return out
}

func TestRetargetV2Simulation(t *testing.T) {
	for _, refresh := range []float64{1, 5} {
		for _, seed := range []int64{11, 42, 20260929} {
			for _, s := range []struct {
				name        string
				initial     int64
				equilibrium float64
			}{
				{"escape-floor-to-50GH", 1, 140},
				{"stable-50GH", 140, 140},
				{"100x-hashrate-rise", 140, 14000},
				{"100x-hashrate-drop", 14000, 140},
				{"high-initial-1000x-overestimate", 140000, 140},
				{"stable-5TH", 14000, 14000},
				{"repeated-switching-50GH-to-5TH", 140, 140},
			} {
				out := simulateRetarget(t, s.name, s.initial, s.equilibrium, refresh, seed)
				b, _ := json.Marshal(out)
				t.Log(string(b))
				// This 2% tolerance validates the simulated mean interval only.
				if out.SteadyMean < 11.76 || out.SteadyMean > 12.24 {
					t.Fatalf("outside candidate mean band: %s", b)
				}
				for _, mean := range out.PhaseMeans {
					if mean < 11.76 || mean > 12.24 {
						t.Fatalf("switching phase outside mean band: %s", b)
					}
				}
			}
		}
	}
	// Below the canonical floor's required hashrate, 12s is impossible.
	out := simulateRetarget(t, "below-minimum-hashrate", 1, 0.1, 5, 42)
	b, _ := json.Marshal(out)
	t.Log(string(b))
	if out.SteadyMean < 100 {
		t.Fatal("simulation masks minimum difficulty limit")
	}
}
