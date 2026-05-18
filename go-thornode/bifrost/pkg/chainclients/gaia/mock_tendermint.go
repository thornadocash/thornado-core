package gaia

import (
	"context"
	"os"

	"github.com/cometbft/cometbft/libs/json"
	ctypes "github.com/cometbft/cometbft/rpc/core/types"
)

type TendermintRPC interface {
	Block(ctx context.Context, height *int64) (*ctypes.ResultBlock, error)
	BlockResults(ctx context.Context, height *int64) (*ctypes.ResultBlockResults, error)
}

type mockTendermintRPC struct{}

func (m *mockTendermintRPC) Block(ctx context.Context, height *int64) (*ctypes.ResultBlock, error) {
	out := new(ctypes.ResultBlock)

	path := "./test-data/latest_block.json"
	if height != nil {
		switch *height {
		case 11350886:
			// single deposit via ibc
			path = "./test-data/block_by_height_11350886.json"
		case 11350935:
			// two deposits in single ibc transaction
			path = "./test-data/block_by_height_11350935.json"
		case 26750027:
			// two ibc txs sending atom back to gaia (from osmosis & secret)
			path = "./test-data/block_by_height_26750027.json"
		case 26757930:
			// ibc usdc from noble to gaia + atom transfer
			path = "./test-data/block_by_height_26757930.json"
		default:
			path = "./test-data/block_by_height.json"
		}
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	err = json.Unmarshal(data, out)

	return out, err
}

func (m *mockTendermintRPC) BlockResults(ctx context.Context, height *int64) (*ctypes.ResultBlockResults, error) {
	out := new(ctypes.ResultBlockResults)

	path := "./test-data/tx_results_by_height.json"

	switch *height {
	case 11350886:
		// single deposit via ibc
		path = "./test-data/tx_results_by_height_11350886.json"
	case 11350935:
		// two deposits in single ibc transaction
		path = "./test-data/tx_results_by_height_11350935.json"
	case 26750027:
		// two ibc txs sending atom back to gaia (from osmosis & secret)
		path = "./test-data/tx_results_by_height_26750027.json"
	case 26757930:
		// ibc usdc from noble to gaia + atom transfer
		path = "./test-data/tx_results_by_height_26757930.json"
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	err = json.Unmarshal(data, out)

	return out, err
}
