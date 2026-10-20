package forge

import (
	"bytes"
	"crypto/ecdsa"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math/big"
	"os"
	"sort"
	"strings"
	"testing"

	"github.com/TeamEGEM/go-egem/common"
	"github.com/TeamEGEM/go-egem/core/types"
	"github.com/TeamEGEM/go-egem/crypto"
	"github.com/TeamEGEM/go-egem/rlp"
)

func certificateTestSign(t *testing.T, template MultiTemplate, work common.Hash, indices ...int) [][]byte {
	t.Helper()
	digest, err := MultiDigest(template, work)
	if err != nil {
		t.Fatal(err)
	}
	type entry struct {
		address   common.Address
		signature []byte
	}
	var entries []entry
	for _, i := range indices {
		key, err := crypto.HexToECDSA(fmt.Sprintf("%064x", i))
		if err != nil {
			t.Fatal(err)
		}
		sig, err := crypto.Sign(digest[:], key)
		if err != nil {
			t.Fatal(err)
		}
		entries = append(entries, entry{crypto.PubkeyToAddress(key.PublicKey), sig})
	}
	sort.Slice(entries, func(i, j int) bool { return bytes.Compare(entries[i].address[:], entries[j].address[:]) < 0 })
	signatures := make([][]byte, len(entries))
	for i := range entries {
		signatures[i] = entries[i].signature
	}
	return signatures
}

// Pool-generated fixtures contain synthetic native templates and unmined
// canonical-target headers. They establish encoding and binding interoperability.
func TestV4CertificatePoolInteropAndMutationRejection(t *testing.T) {
	data, err := os.ReadFile("testdata/v4-proof-fixtures.json")
	if err != nil {
		t.Fatal(err)
	}
	var rows []struct {
		Mask                                          uint8
		HeaderRLP, WorkHash, Proof, CertificateDigest string
		Signers                                       []common.Address
		PrivateCPU                                    bool
	}
	if err := json.Unmarshal(data, &rows); err != nil || len(rows) != 3 {
		t.Fatal("missing V4 fixtures", err)
	}
	seen := map[uint8]bool{}
	for _, row := range rows {
		t.Run(fmt.Sprint(row.Mask), func(t *testing.T) {
			seen[row.Mask] = true
			if row.PrivateCPU {
				t.Fatal("canonical fixture unexpectedly uses private target")
			}
			headerBytes, err := hex.DecodeString(row.HeaderRLP)
			if err != nil {
				t.Fatal(err)
			}
			var issued types.Header
			if err := rlp.DecodeBytes(headerBytes, &issued); err != nil {
				t.Fatal(err)
			}
			work, err := WorkHash(&issued)
			if err != nil || work.Hex() != row.WorkHash {
				t.Fatal("pool/node work mismatch", err)
			}
			blob, err := hex.DecodeString(row.Proof)
			if err != nil {
				t.Fatal(err)
			}
			p, err := DecodeProof(blob)
			if err != nil || p.Multi == nil || p.Multi.Template.Version != 4 {
				t.Fatal("invalid V4 fixture", err)
			}
			encoded, err := EncodeProof(p)
			if err != nil || !bytes.Equal(encoded, blob) {
				t.Fatal("pool/node envelope mismatch", err)
			}
			cfg := rogueEngineConfig(t)
			dgb, _ := hex.DecodeString("0014706267a5255e3666936bcb9a777667599f387546")
			doge, _ := hex.DecodeString("76a9144b1508ebe06723842994adf33bdba36b4eba44c488ac")
			cfg.Forge.LTCPayoutScriptHash, cfg.Forge.DOGEPayoutScriptHash = ScriptHash(dgb), ScriptHash(doge)
			cfg.Forge.TemplateSigners = append([]common.Address(nil), row.Signers...)
			cfg.Forge.TemplateQuorum = 2
			h := issued
			h.Extra = blob
			if err := verifyMultiBindings(cfg.Forge, &h, p.Multi); err != nil {
				t.Fatal("pool V4 binding rejected", err)
			}
			certificate, err := MultiCertificateDigest(p.Multi.Template, work, p.Multi.Signatures)
			if err != nil || certificate.Hex() != row.CertificateDigest {
				t.Fatal("pool/node certificate digest mismatch", err)
			}
			engine, err := New(cfg)
			if err != nil {
				t.Fatal(err)
			}
			if err := engine.VerifySeal(nil, &h); err == nil || !strings.Contains(err.Error(), "Scrypt proof-of-work") {
				t.Fatal("unmined canonical proof bypassed work gate", err)
			}
			headerRLP, err := rlp.EncodeToBytes(&h)
			if err != nil || h.Hash() != crypto.Keccak256Hash(headerRLP) {
				t.Fatal("Ethereum header identity changed", err)
			}
			bound, err := WorkHash(&h)
			if err != nil || bound != work {
				t.Fatal("proof attachment changed unsigned work", err)
			}
			reject := func(name string, change func(*MultiProof), repairMerkle bool) {
				t.Helper()
				q, err := DecodeProof(blob)
				if err != nil {
					t.Fatal(err)
				}
				change(q.Multi)
				if repairMerkle {
					cb, err := parseCoinbase(q.Multi.ParentCoinbase)
					if err != nil {
						t.Fatal(err)
					}
					root := merkleRoot(cb.TxID, q.Multi.ParentMerkleBranch, 0)
					copy(q.Multi.ParentHeader[36:68], root[:])
				}
				extra, err := EncodeProof(q)
				if err != nil {
					t.Fatal(err)
				}
				changed := issued
				changed.Extra = extra
				if err := verifyMultiBindings(cfg.Forge, &changed, q.Multi); err == nil {
					t.Fatal("accepted mutated " + name)
				}
			}
			reject("surplus signature", func(q *MultiProof) { q.Signatures = append(q.Signatures, make([]byte, 65)) }, false)
			reject("duplicate signature", func(q *MultiProof) { q.Signatures[1] = q.Signatures[0] }, false)
			reject("signature order", func(q *MultiProof) { q.Signatures[0], q.Signatures[1] = q.Signatures[1], q.Signatures[0] }, false)
			reject("different valid quorum", func(q *MultiProof) {
				q.Signatures = certificateTestSign(t, q.Template, work, 2, 3)
				if err := ValidateMultiSignatures(q.Template, work, q.Signatures, cfg.Forge.TemplateSigners, 2, issued.Time.Uint64()); err != nil {
					t.Fatal(err)
				}
			}, false)
			reject("different valid signature nonce", func(q *MultiProof) {
				digest, err := MultiDigest(q.Template, work)
				if err != nil {
					t.Fatal(err)
				}
				originalPub, err := crypto.SigToPub(digest[:], q.Signatures[0])
				if err != nil {
					t.Fatal(err)
				}
				address := crypto.PubkeyToAddress(*originalPub)
				var key *ecdsa.PrivateKey
				for i := 1; i <= 3; i++ {
					candidate, e := crypto.HexToECDSA(fmt.Sprintf("%064x", i))
					if e != nil {
						t.Fatal(e)
					}
					if crypto.PubkeyToAddress(candidate.PublicKey) == address {
						key = candidate
						break
					}
				}
				if key == nil {
					t.Fatal("unknown fixture signer")
				}
				r, s, err := ecdsa.Sign(rand.Reader, key, digest[:])
				if err != nil {
					t.Fatal(err)
				}
				order := crypto.S256().Params().N
				if s.Cmp(new(big.Int).Rsh(new(big.Int).Set(order), 1)) > 0 {
					s.Sub(order, s)
				}
				alternative := append(common.LeftPadBytes(r.Bytes(), 32), common.LeftPadBytes(s.Bytes(), 32)...)
				alternative = append(alternative, 0)
				found := false
				for v := byte(0); v < 2; v++ {
					alternative[64] = v
					pub, e := crypto.SigToPub(digest[:], alternative)
					if e == nil && crypto.PubkeyToAddress(*pub) == address {
						found = true
						break
					}
				}
				if !found || bytes.Equal(alternative, q.Signatures[0]) {
					t.Fatal("failed to construct a distinct valid fixture signature")
				}
				q.Signatures[0] = alternative
				if err := ValidateMultiSignatures(q.Template, work, q.Signatures, cfg.Forge.TemplateSigners, 2, issued.Time.Uint64()); err != nil {
					t.Fatal(err)
				}
			}, false)
			reject("new signed expiry", func(q *MultiProof) {
				q.Template.Expires--
				q.Signatures = certificateTestSign(t, q.Template, work, 1, 2)
			}, false)
			if row.Mask&1 != 0 {
				reject("new signed minimum time", func(q *MultiProof) {
					q.Template.ParentMinTime--
					q.Signatures = certificateTestSign(t, q.Template, work, 1, 2)
				}, false)
			}
			reject("certificate payload", func(q *MultiProof) {
				cb, err := parseCoinbase(q.ParentCoinbase)
				if err != nil {
					t.Fatal(err)
				}
				offset, _, err := certificatePayloadOffset(cb)
				if err != nil {
					t.Fatal(err)
				}
				q.ParentCoinbase[offset] ^= 1
			}, true)
			reject("positive certificate output", func(q *MultiProof) {
				cb, err := parseCoinbase(q.ParentCoinbase)
				if err != nil {
					t.Fatal(err)
				}
				_, index, err := certificatePayloadOffset(cb)
				if err != nil {
					t.Fatal(err)
				}
				q.ParentCoinbase[cb.Outputs[index].ScriptOffset-9] = 1
			}, true)
			reject("duplicate certificate output", func(q *MultiProof) {
				cb, err := parseCoinbase(q.ParentCoinbase)
				if err != nil {
					t.Fatal(err)
				}
				_, index, err := certificatePayloadOffset(cb)
				if err != nil {
					t.Fatal(err)
				}
				countOffset := 42 + len(cb.ScriptSig) + 4
				q.ParentCoinbase[countOffset]++
				output := append(make([]byte, 8), byte(42))
				output = append(output, cb.Outputs[index].Script...)
				end := len(q.ParentCoinbase) - 4
				q.ParentCoinbase = append(append(append([]byte{}, q.ParentCoinbase[:end]...), output...), q.ParentCoinbase[end:]...)
			}, true)
			legacy, err := DecodeProof(blob)
			if err != nil {
				t.Fatal(err)
			}
			legacy.Multi.Template.Version = 3
			h.Extra, err = EncodeProof(legacy)
			if err != nil {
				t.Fatal(err)
			}
			if err := engine.VerifySeal(nil, &h); err == nil || !strings.Contains(err.Error(), "requires a mined V4 certificate") {
				t.Fatal("activated profile accepted V3 envelope", err)
			}
			wrongEnvelope := append([]byte(nil), blob...)
			wrongEnvelope[4] = 3
			if _, err := DecodeProof(wrongEnvelope); err == nil {
				t.Fatal("version mismatch decoded")
			}
		})
	}
	if !seen[1] || !seen[2] || !seen[3] {
		t.Fatal("missing participation modes")
	}
}
