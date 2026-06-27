package main

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/syndtr/goleveldb/leveldb"
	"github.com/syndtr/goleveldb/leveldb/opt"
	"github.com/syndtr/goleveldb/leveldb/util"
)

type row struct {
	DB  string          `json:"db"`
	Key string          `json:"key"`
	Raw json.RawMessage `json:"raw"`
}

func dump(path string) error {
	db, err := leveldb.OpenFile(path, &opt.Options{ReadOnly: true})
	if err != nil {
		return err
	}
	defer db.Close()

	iter := db.NewIterator(util.BytesPrefix([]byte("txout-v4-")), nil)
	defer iter.Release()
	enc := json.NewEncoder(os.Stdout)
	for iter.Next() {
		var raw json.RawMessage
		if err := json.Unmarshal(iter.Value(), &raw); err != nil {
			return fmt.Errorf("%s: %w", string(iter.Key()), err)
		}
		if err := enc.Encode(row{DB: path, Key: string(iter.Key()), Raw: raw}); err != nil {
			return err
		}
	}
	return iter.Error()
}

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "usage: signerdbdump DB_PATH...")
		os.Exit(2)
	}
	for _, path := range os.Args[1:] {
		if err := dump(path); err != nil {
			fmt.Fprintf(os.Stderr, "%s: %v\n", path, err)
			os.Exit(1)
		}
	}
}
