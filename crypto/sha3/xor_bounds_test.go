package sha3

import (
	"bytes"
	"encoding/hex"
	"hash"
	"runtime"
	"testing"
)

func TestXorExactBackingAllocationMatchesGeneric(t *testing.T) {
	for _, rate := range []int{72, 104, 136, 144, 168} {
		for offset := 0; offset < 64; offset++ {
			// Keep the input at the very end of the backing allocation. The
			// optimized implementation must never cast past this boundary.
			backing := make([]byte, offset+rate)
			input := backing[offset : offset+rate : offset+rate]
			for i := range input {
				input[i] = byte(i*31 + offset)
			}
			var got, want state
			for i := range got.a {
				got.a[i] = uint64(i+1) * 0x102030405060708
			}
			want = got
			xorInGeneric(&want, input)
			xorInUnaligned(&got, input)
			if got.a != want.a {
				t.Fatalf("rate=%d offset=%d optimized XOR differs", rate, offset)
			}
			runtime.KeepAlive(backing)
		}
	}
}

func TestKeccakLegacyKnownVectorsAndExactTails(t *testing.T) {
	vectors := []struct {
		newHash func() hash.Hash
		message string
		digest  string
	}{
		{NewKeccak256, "", "c5d2460186f7233c927e7db2dcc703c0e500b653ca82273b7bfad8045d85a470"},
		{NewKeccak256, "abc", "4e03657aea45a94fc7d47ba826c8d667c0d1e6e33a64a036ec44f58fa12d6c45"},
		{NewKeccak512, "", "0eab42de4c3ceb9235fc91acffe746b29c29a8c366b7c60e4e67c466f36a4304c00fa9caf9d87976ba469bcbe06713b435f091ef2769fb160cdab33d3670680e"},
	}
	for _, vector := range vectors {
		h := vector.newHash()
		h.Write([]byte(vector.message))
		if got := hex.EncodeToString(h.Sum(nil)); got != vector.digest {
			t.Fatalf("legacy Keccak known vector differs: got=%s want=%s", got, vector.digest)
		}
	}
	// Exercise short heap tails in every supported sponge rate, comparing the
	// complete optimized hash with the portable reference implementation.
	testUnalignedAndGeneric(t, func(implementation string) {
		for _, newHash := range []func() hash.Hash{New224, New256, New384, New512, NewKeccak256, NewKeccak512} {
			for blocks := 1; blocks <= 5; blocks++ {
				h := newHash()
				message := make([]byte, blocks*h.BlockSize()+7)
				for i := range message {
					message[i] = byte(i * 13)
				}
				got := h
				got.Write(message)
				// Clone the exact algorithm parameters without swapping global
				// function pointers during an individual hash operation.
				want := *newHash().(*state)
				for len(message) >= want.rate {
					xorInGeneric(&want, message[:want.rate])
					keccakF1600(&want.a)
					message = message[want.rate:]
				}
				want.buf = append(want.storage[:0], message...)
				if !bytes.Equal(got.Sum(nil), want.Sum(nil)) {
					t.Fatalf("implementation=%s blocks=%d exact-tail hash differs", implementation, blocks)
				}
			}
		}
	})
}
