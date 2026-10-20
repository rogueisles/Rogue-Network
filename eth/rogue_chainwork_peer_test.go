package eth

import (
	"math/big"
	"testing"

	"github.com/TeamEGEM/go-egem/consensus/forge"
	"github.com/TeamEGEM/go-egem/core"
	"github.com/TeamEGEM/go-egem/core/types"
	"github.com/TeamEGEM/go-egem/eth/downloader"
	"github.com/TeamEGEM/go-egem/p2p"
	"github.com/TeamEGEM/go-egem/p2p/discover"
	"github.com/TeamEGEM/go-egem/params"
)

// Trusted parent records isolate wire total-work arithmetic from seal/import
// validation, which has separate consensus and persisted-chain coverage.
func TestActivatedPeerBlockWorkPropagationAndRejection(t *testing.T) {
	pm, db := newTestProtocolManagerMust(t, downloader.FullSync, 0, nil, nil)
	defer db.Close()
	defer pm.Stop()
	cfg := *pm.chainconfig
	cfg.ForgeBlock = big.NewInt(params.RogueActivationHeight)
	cfg.Forge = &params.ForgeConfig{RewardSchedule: params.RogueRewardSchedule}
	pm.chainconfig = &cfg
	parentTD := big.NewInt(100)
	for _, height := range []int64{params.RogueActivationHeight - 1, params.RogueActivationHeight} {
		parent := types.NewBlockWithHeader(&types.Header{
			Number: big.NewInt(height - 1), Difficulty: big.NewInt(25000000000),
			Root: pm.blockchain.Genesis().Root(), GasLimit: 8000000,
		})
		if err := core.WriteBlock(db, parent); err != nil {
			t.Fatal(err)
		}
		if err := core.WriteTd(db, parent.Hash(), parent.NumberU64(), parentTD); err != nil {
			t.Fatal(err)
		}
		difficulty := big.NewInt(25000000000)
		if height == params.RogueActivationHeight {
			difficulty = big.NewInt(140)
		}
		block := types.NewBlockWithHeader(&types.Header{ParentHash: parent.Hash(), Number: big.NewInt(height), Difficulty: difficulty})
		work, err := forge.HeaderWork(&cfg, block.Header())
		if err != nil {
			t.Fatal(err)
		}
		want := new(big.Int).Add(parentTD, work)
		if height == params.RogueActivationHeight && work.Cmp(difficulty) == 0 {
			t.Fatal("fixture does not distinguish work domains")
		}
		cases := []struct {
			name  string
			td    *big.Int
			valid bool
		}{
			{"correct", want, true},
			{"underflow", new(big.Int).Sub(work, big.NewInt(1)), false},
			{"inflated-parent", new(big.Int).Add(want, big.NewInt(1)), false},
		}
		if height == params.RogueActivationHeight {
			cases = append(cases, struct {
				name  string
				td    *big.Int
				valid bool
			}{"raw-scrypt-difficulty", new(big.Int).Add(parentTD, difficulty), false})
		}
		for _, row := range cases {
			t.Run(block.Number().String()+"/"+row.name, func(t *testing.T) {
				local, remote := p2p.MsgPipe()
				defer local.Close()
				defer remote.Close()
				peer := newPeer(63, p2p.NewPeer(discover.NodeID{1}, "work-peer", nil), local)
				peer.td = new(big.Int)
				sent := make(chan error, 1)
				go func() { sent <- p2p.Send(remote, NewBlockMsg, &newBlockData{Block: block, TD: row.td}) }()
				err := pm.handleMsg(peer)
				if sendErr := <-sent; sendErr != nil {
					t.Fatal(sendErr)
				}
				if row.valid {
					if err != nil {
						t.Fatal(err)
					}
					head, td := peer.Head()
					if head != parent.Hash() || td.Cmp(parentTD) != 0 || !peer.knownBlocks.Has(block.Hash()) {
						t.Fatal("peer parent total work was derived in the wrong domain")
					}
				} else if err == nil || peer.knownBlocks.Has(block.Hash()) {
					t.Fatal("invalid peer total work reached import admission")
				}
			})
		}
		local, remote := p2p.MsgPipe()
		peer := newPeer(63, p2p.NewPeer(discover.NodeID{2}, "broadcast-peer", nil), local)
		peer.td = new(big.Int)
		if err := pm.peers.Register(peer); err != nil {
			t.Fatal(err)
		}
		done := make(chan struct{})
		go func() { pm.BroadcastBlock(block, true); close(done) }()
		message, err := remote.ReadMsg()
		if err != nil {
			t.Fatal(err)
		}
		var propagated newBlockData
		if message.Code != NewBlockMsg || message.Decode(&propagated) != nil || propagated.TD.Cmp(want) != 0 || propagated.Block.Hash() != block.Hash() {
			t.Fatal("broadcast used incompatible total-work units")
		}
		message.Discard()
		<-done
		pm.peers.Unregister(peer.id)
		local.Close()
		remote.Close()
	}
}
