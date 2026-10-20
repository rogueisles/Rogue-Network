// rogue-replay validates public canonical history in an isolated local database.
// It never starts networking, reads a live datadir, signs or sends transactions.
package main

import (
	"compress/gzip"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/TeamEGEM/go-egem/common"
	"github.com/TeamEGEM/go-egem/consensus"
	"github.com/TeamEGEM/go-egem/consensus/forge"
	"github.com/TeamEGEM/go-egem/consensus/progpow"
	"github.com/TeamEGEM/go-egem/core"
	"github.com/TeamEGEM/go-egem/core/types"
	"github.com/TeamEGEM/go-egem/core/vm"
	"github.com/TeamEGEM/go-egem/ethdb"
	"github.com/TeamEGEM/go-egem/rlp"
)

func run() error {
	genesisFile := flag.String("genesis", "", "candidate genesis with preserved mainnet history")
	blocksFile := flag.String("blocks", "", "public canonical RLP or RLP.gz archive")
	directory := flag.String("db", "", "NEW isolated replay directory; existing directories refused")
	dagDirectory := flag.String("dag-dir", "", "optional existing canonical replay DAG cache directory")
	height := flag.Uint64("height", 0, "frozen last height to verify")
	expectedHash := flag.String("hash", "", "frozen last canonical block hash")
	expectedRoot := flag.String("state-root", "", "independently observed frozen state root")
	seals := flag.Bool("verify-seals", true, "verify every historical ProgPoW seal; false is explicitly STATE-ONLY qualification")
	flag.Parse()
	if *genesisFile == "" || *blocksFile == "" || *directory == "" || *height == 0 || *expectedHash == "" || *expectedRoot == "" {
		return fmt.Errorf("all genesis/blocks/db/height/hash/state-root arguments are required")
	}
	if *dagDirectory != "" {
		info, e := os.Stat(*dagDirectory)
		if !filepath.IsAbs(*dagDirectory) || e != nil || !info.IsDir() {
			return fmt.Errorf("dag-dir must name an existing absolute canonical cache directory")
		}
	}
	blob, err := ioutil.ReadFile(*genesisFile)
	if err != nil {
		return err
	}
	var g core.Genesis
	if err = json.Unmarshal(blob, &g); err != nil {
		return err
	}
	actualGenesis := common.HexToHash("0xdc4d38c0ef41d3fdf50642fdf5fc99e221eadd09e73bbe616724f55a136270ab")
	if g.ToBlock(nil).Hash() != actualGenesis || !g.Config.ValidateRogueSchedule() {
		return fmt.Errorf("expected unchanged actual mainnet genesis and explicit RGN schedule")
	}
	if err = os.Mkdir(*directory, 0700); err != nil {
		return fmt.Errorf("replay requires a NEW directory: %v", err)
	}
	marker := []byte("Isolated public-history qualification; not a live node database.\n")
	if err = ioutil.WriteFile(filepath.Join(*directory, "REPLAY-ONLY.txt"), marker, 0600); err != nil {
		return err
	}
	db, err := ethdb.NewLDBDatabase(filepath.Join(*directory, "chaindata"), 384, 64)
	if err != nil {
		return err
	}
	defer db.Close()
	cfg, hash, err := core.SetupGenesisBlock(db, &g)
	if err != nil || hash != actualGenesis {
		return fmt.Errorf("genesis setup: %v", err)
	}
	var legacy consensus.Engine
	if *seals {
		dag := *dagDirectory
		if dag == "" {
			dag = filepath.Join(*directory, "dag")
		}
		legacy = progpow.New(progpow.Config{CacheDir: filepath.Join(*directory, "cache"), CachesInMem: 1, CachesOnDisk: 2, DatasetDir: dag, DatasetsInMem: 1, DatasetsOnDisk: 2})
	} else {
		legacy = progpow.NewFaker()
	}
	engine, err := forge.NewHybrid(cfg, legacy)
	if err != nil {
		return err
	}
	chain, err := core.NewBlockChain(db, &core.CacheConfig{TrieNodeLimit: 256, TrieTimeLimit: time.Minute}, cfg, engine, vm.Config{})
	if err != nil {
		return err
	}
	defer chain.Stop()
	fh, err := os.Open(*blocksFile)
	if err != nil {
		return err
	}
	defer fh.Close()
	var reader io.Reader = fh
	if strings.HasSuffix(*blocksFile, ".gz") {
		gz, e := gzip.NewReader(fh)
		if e != nil {
			return e
		}
		defer gz.Close()
		reader = gz
	}
	stream := rlp.NewStream(reader, 0)
	batch := make(types.Blocks, 0, 128)
	count, transactions := uint64(0), uint64(0)
	started := time.Now()
	emit := func(v interface{}) { json.NewEncoder(os.Stdout).Encode(v) }
	flush := func() error {
		if len(batch) == 0 {
			return nil
		}
		if index, e := chain.InsertChain(batch); e != nil {
			return fmt.Errorf("historical import failed at height %d: %v", batch[index].NumberU64(), e)
		}
		last := batch[len(batch)-1]
		if last.NumberU64()%4096 == 0 || last.NumberU64() == *height {
			emit(map[string]interface{}{"replayedHeight": last.NumberU64(), "hash": last.Hash(), "transactions": transactions, "verifySeals": *seals, "elapsedSeconds": time.Since(started).Seconds()})
		}
		batch = batch[:0]
		return nil
	}
	for {
		var b types.Block
		err = stream.Decode(&b)
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("RLP decode after %d blocks: %v", count, err)
		}
		if b.NumberU64() != count {
			return fmt.Errorf("archive height discontinuity at %d", count)
		}
		count++
		if b.NumberU64() == 0 {
			if b.Hash() != actualGenesis {
				return fmt.Errorf("archive genesis mismatch")
			}
			continue
		}
		if b.NumberU64() > *height {
			return fmt.Errorf("archive exceeds the frozen checkpoint")
		}
		transactions += uint64(len(b.Transactions()))
		batch = append(batch, &b)
		if len(batch) == cap(batch) {
			if err = flush(); err != nil {
				return err
			}
		}
	}
	if err = flush(); err != nil {
		return err
	}
	last := chain.CurrentBlock()
	if count != *height+1 || last.NumberU64() != *height || last.Hash() != common.HexToHash(*expectedHash) || last.Root() != common.HexToHash(*expectedRoot) {
		return fmt.Errorf("final canonical height/hash/state root mismatch")
	}
	report := map[string]interface{}{"replayComplete": true, "height": last.NumberU64(), "hash": last.Hash(), "stateRoot": last.Root(), "transactionsExecuted": transactions, "allStateRootsAndReceiptsValidated": true, "allHistoricalSealsVerified": *seals, "elapsedSeconds": time.Since(started).Seconds(), "chainId": cfg.ChainId, "activationHeight": cfg.ForgeBlock, "scope": "Existing canonical history only; future activation rehearsal and production proof settings remain separate."}
	data, _ := json.MarshalIndent(report, "", "  ")
	if err = ioutil.WriteFile(filepath.Join(*directory, "report.json"), append(data, '\n'), 0600); err != nil {
		return err
	}
	emit(report)
	return nil
}
func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
