package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/syndtr/goleveldb/leveldb"
	"github.com/syndtr/goleveldb/leveldb/util"
)

const txOutPrefix = "txout-v4-"

type txOutStoreItem struct {
	TxOutItem struct {
		Height      int64  `json:"height"`
		TxType      string `json:"tx_type"`
		VaultPubKey string `json:"vault_pub_key"`
		InHash      string `json:"in_hash"`
	} `json:"TxOutItem"`
	Height              int64  `json:"Height"`
	DeferredUntilHeight int64  `json:"DeferredUntilHeight"`
	Round7Retry         bool   `json:"Round7Retry"`
	Checkpoint          []byte `json:"Checkpoint"`
	Status              int    `json:"Status"`
}

func parseKeepHeights(raw string) map[int64]struct{} {
	keep := make(map[int64]struct{})
	for _, part := range strings.Split(raw, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		height, err := strconv.ParseInt(part, 10, 64)
		if err != nil {
			fmt.Fprintf(os.Stderr, "invalid -keep height %q: %v\n", part, err)
			os.Exit(1)
		}
		keep[height] = struct{}{}
	}
	return keep
}

func main() {
	dbPath := flag.String("db", "", "path to signer leveldb")
	keepHeights := flag.String("keep", "3991,10011,10071,10161", "comma-separated txout heights to keep")
	pruneNotKept := flag.Bool("prune-not-kept", false, "remove any txout height not listed in -keep")
	resetDefer := flag.Bool("reset-defer", false, "clear DeferredUntilHeight/Checkpoint on kept items")
	dryRun := flag.Bool("dry-run", false, "only print actions")
	flag.Parse()
	if *dbPath == "" {
		fmt.Fprintln(os.Stderr, "missing -db")
		os.Exit(1)
	}

	keep := parseKeepHeights(*keepHeights)

	db, err := leveldb.OpenFile(*dbPath, nil)
	if err != nil {
		fmt.Fprintf(os.Stderr, "open db: %v\n", err)
		os.Exit(1)
	}
	defer db.Close()

	var scanned, removed, reset int
	iter := db.NewIterator(util.BytesPrefix([]byte(txOutPrefix)), nil)
	defer iter.Release()
	batch := new(leveldb.Batch)
	for iter.Next() {
		scanned++
		key := append([]byte(nil), iter.Key()...)
		var item txOutStoreItem
		if err := json.Unmarshal(iter.Value(), &item); err != nil {
			continue
		}
		_, isKept := keep[item.Height]
		if isKept && *resetDefer {
			if item.DeferredUntilHeight != 0 || len(item.Checkpoint) > 0 || item.Round7Retry {
				reset++
				fmt.Printf("reset height=%d deferred=%d checkpoint=%t round7=%t key=%s\n",
					item.Height, item.DeferredUntilHeight, len(item.Checkpoint) > 0, item.Round7Retry, string(key))
				item.DeferredUntilHeight = 0
				item.Checkpoint = nil
				item.Round7Retry = false
				if !*dryRun {
					buf, err := json.Marshal(item)
					if err != nil {
						fmt.Fprintf(os.Stderr, "marshal height=%d: %v\n", item.Height, err)
						os.Exit(1)
					}
					batch.Put(key, buf)
				}
			}
			continue
		}
		if isKept {
			continue
		}
		remove := *pruneNotKept
		if !remove {
			remove = item.Status == 2 // TxUnavailable
		}
		if !remove {
			remove = item.DeferredUntilHeight > 1_000_000
		}
		if !remove && item.TxOutItem.TxType == "refund" {
			remove = true
		}
		if !remove {
			continue
		}
		removed++
		fmt.Printf("remove height=%d type=%s vault=%s deferred=%d key=%s\n",
			item.Height, item.TxOutItem.TxType, item.TxOutItem.VaultPubKey, item.DeferredUntilHeight, string(key))
		if !*dryRun {
			batch.Delete(key)
		}
	}
	if !*dryRun && (removed > 0 || reset > 0) {
		if err := db.Write(batch, nil); err != nil {
			fmt.Fprintf(os.Stderr, "write batch: %v\n", err)
			os.Exit(1)
		}
	}
	fmt.Printf("scanned=%d removed=%d reset=%d dry_run=%v\n", scanned, removed, reset, *dryRun)
}