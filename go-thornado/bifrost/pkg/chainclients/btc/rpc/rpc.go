package rpc

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/btcsuite/btcd/btcjson"
	"github.com/ethereum/go-ethereum/rpc"
	"github.com/rs/zerolog"
	"github.com/thornadocash/go-thornado/common"
)

const ZecClientTimeout = time.Second * 10

////////////////////////////////////////////////////////////////////////////////////////
// Interfaces
////////////////////////////////////////////////////////////////////////////////////////

type SerializableTx interface {
	SerializeSize() int
	Serialize(io.Writer) error
}

type rpcClient interface {
	Call(result any, method string, args ...any) error
	BatchCall(batch []rpc.BatchElem) error
	CallContext(ctx context.Context, result any, method string, args ...any) error
	BatchCallContext(ctx context.Context, batch []rpc.BatchElem) error
}

////////////////////////////////////////////////////////////////////////////////////////
// Client
////////////////////////////////////////////////////////////////////////////////////////

// Client represents a client connection to a UTXO daemon. Internally this uses the
// Ethereum JSON-RPC 2.0 implementation, which allows batching and connection reuse.
type Client struct {
	client     rpcClient
	log        zerolog.Logger
	chain      common.Chain
	maxRetries int
	timeout    time.Duration
}

// NewClient returns a client connection to a UTXO daemon.
func NewClient(host, user, password string, maxRetries int, timeout time.Duration, chain common.Chain, log zerolog.Logger) (
	*Client, error,
) {
	authFn := func(h http.Header) error {
		auth := base64.StdEncoding.EncodeToString([]byte(user + ":" + password))
		h.Set("Authorization", fmt.Sprintf("Basic %s", auth))
		return nil
	}

	// default to http if no scheme is specified
	if !strings.Contains(host, "://") {
		host = "http://" + host
	}

	client := Client{
		log:        log,
		chain:      chain,
		maxRetries: maxRetries,
		timeout:    timeout,
	}

	var err error
	client.client, err = rpc.DialOptions(context.Background(), host, rpc.WithHTTPAuth(authFn))

	if err != nil {
		return nil, err
	}

	return &client, nil
}

// GetBlockCount returns the number of blocks in the longest block chain.
func (c *Client) GetBlockCount() (int64, error) {
	var count int64
	err := c.Call(&count, "getblockcount")
	return count, extractBTCError(err)
}

// SendRawTransaction serializes and sends the transaction. The maxFeeParam differs in
// type between chains - ensure the correct variant is used.
func (c *Client) SendRawTransaction(tx SerializableTx, maxFeeParam any) (string, error) {
	txHex := ""
	if tx != nil {
		buf := bytes.NewBuffer(make([]byte, 0, tx.SerializeSize()))
		if err := tx.Serialize(buf); err != nil {
			return "", err
		}
		txHex = hex.EncodeToString(buf.Bytes())
	}

	args := []interface{}{txHex, maxFeeParam}

	var txid string
	err := c.Call(&txid, "sendrawtransaction", args...)
	return txid, extractBTCError(err)
}

// GetBlockHash returns the hash of the block in best-block-chain at the given height.
func (c *Client) GetBlockHash(height int64) (string, error) {
	var hash string
	err := c.Call(&hash, "getblockhash", height)
	return hash, extractBTCError(err)
}

// GetBlockVerbose returns information about the block with verbosity 2.
func (c *Client) GetBlockVerboseTxs(hash string) (*btcjson.GetBlockVerboseTxResult, error) {
	var block btcjson.GetBlockVerboseTxResult
	err := c.Call(&block, "getblock", hash, 2)
	return &block, extractBTCError(err)
}

// GetBlockVerbose returns information about the block with verbosity 1.
func (c *Client) GetBlockVerbose(hash string) (*btcjson.GetBlockVerboseResult, error) {
	var block btcjson.GetBlockVerboseResult
	err := c.Call(&block, "getblock", hash, 1)
	return &block, extractBTCError(err)
}

// GetBlockStats returns statistics about the block at the given hash.
func (c *Client) GetBlockStats(hash string) (*btcjson.GetBlockStatsResult, error) {
	var stats btcjson.GetBlockStatsResult
	err := c.Call(&stats, "getblockstats", hash)
	return &stats, extractBTCError(err)
}

// GetMempoolEntry returns mempool data for the given transaction.
func (c *Client) GetMempoolEntry(txid string) (*btcjson.GetMempoolEntryResult, error) {
	var entry btcjson.GetMempoolEntryResult
	err := c.Call(&entry, "getmempoolentry", txid)
	return &entry, extractBTCError(err)
}

// BatchGetMempoolEntry returns mempool data for the given transactions.
func (c *Client) BatchGetMempoolEntry(txids []string) ([]*btcjson.GetMempoolEntryResult, []error, error) {
	// create batch request
	batch := []rpc.BatchElem{}
	for _, txid := range txids {
		batch = append(batch, rpc.BatchElem{
			Method: "getmempoolentry",
			Args:   []interface{}{txid},
			Result: &btcjson.GetMempoolEntryResult{},
		})
	}

	// call batch request
	err := c.BatchCall(batch)
	if err != nil {
		return nil, nil, err
	}

	// collect results
	errs := make([]error, len(txids))
	results := make([]*btcjson.GetMempoolEntryResult, len(txids))
	var ok bool
	for i, b := range batch {
		results[i], ok = b.Result.(*btcjson.GetMempoolEntryResult)
		if !ok {
			return nil, nil, fmt.Errorf("unexpected type returned from batch call: %T", b.Result)
		}
		errs[i] = extractBTCError(b.Error)
	}

	return results, errs, nil
}

// GetRawMempool returns all transaction ids in the mempool.
func (c *Client) GetRawMempool() ([]string, error) {
	var txids []string
	err := c.Call(&txids, "getrawmempool")
	return txids, extractBTCError(err)
}

// GetRawTransactionVerbose returns a raw transaction for the transaction id.
func (c *Client) GetRawTransactionVerbose(txid string) (*btcjson.TxRawResult, error) {
	var tx btcjson.TxRawResult
	err := c.Call(&tx, "getrawtransaction", txid, true)
	return &tx, extractBTCError(err)
}

// GetRawTransaction returns a raw transaction string for the transaction id.
func (c *Client) GetRawTransaction(txid string) (string, error) {
	var tx string
	err := c.Call(&tx, "getrawtransaction", txid, false)
	return tx, extractBTCError(err)
}

// GetTxOut returns information about an unspent transaction output. A nil result
// means Bitcoin Core knows the outpoint but it is spent, or the outpoint is absent.
func (c *Client) GetTxOut(txid string, vout uint32, includeMempool bool) (*btcjson.GetTxOutResult, error) {
	var result *btcjson.GetTxOutResult
	err := c.Call(&result, "gettxout", txid, vout, includeMempool)
	return result, extractBTCError(err)
}

// BatchGetRawTransactionVerbose returns a raw transaction for given transaction ids.
func (c *Client) BatchGetRawTransactionVerbose(txids []string) ([]*btcjson.TxRawResult, []error, error) {
	// create batch request
	batch := make([]rpc.BatchElem, 0, len(txids))
	for _, txid := range txids {
		args := []interface{}{txid, true}
		batch = append(batch, rpc.BatchElem{
			Method: "getrawtransaction",
			Args:   args,
			Result: &btcjson.TxRawResult{},
		})
	}

	// call batch request
	err := c.BatchCall(batch)
	if err != nil {
		return nil, nil, err
	}

	// collect results
	errs := make([]error, len(txids))
	results := make([]*btcjson.TxRawResult, len(txids))
	var ok bool
	for i, b := range batch {
		results[i], ok = b.Result.(*btcjson.TxRawResult)
		if !ok {
			return nil, nil, fmt.Errorf("unexpected type returned from batch call: %T", b.Result)
		}
		errs[i] = extractBTCError(b.Error)
	}

	return results, errs, nil
}

// ImportAddress imports the address with no rescan.
func (c *Client) ImportAddress(address string) error {
	err := c.Call(nil, "importaddress", address, "", false)
	return extractBTCError(err)
}

// AddressKnown reports whether this wallet already tracks the address.
func (c *Client) AddressKnown(address string) (bool, error) {
	var info struct {
		IsMine      bool `json:"ismine"`
		IsWatchOnly bool `json:"iswatchonly"`
		Solvable    bool `json:"solvable"`
	}
	err := c.Call(&info, "getaddressinfo", address)
	if err != nil {
		return false, extractBTCError(err)
	}
	return info.IsMine || info.IsWatchOnly || info.Solvable, nil
}

// ImportDescriptorAddress imports a descriptor wallet watch-only address.
func (c *Client) ImportDescriptorAddress(address string) error {
	desc := fmt.Sprintf("addr(%s)", address)
	var descriptorInfo struct {
		Descriptor string `json:"descriptor"`
	}
	err := c.Call(&descriptorInfo, "getdescriptorinfo", desc)
	if err != nil {
		return extractBTCError(err)
	}
	if descriptorInfo.Descriptor != "" {
		desc = descriptorInfo.Descriptor
	}

	var results []struct {
		Success bool              `json:"success"`
		Error   *btcjson.RPCError `json:"error,omitempty"`
	}
	err = c.Call(&results, "importdescriptors", []map[string]any{
		{
			"desc":      desc,
			"timestamp": "now",
		},
	})
	if err != nil {
		return extractBTCError(err)
	}
	if len(results) != 1 {
		return fmt.Errorf("unexpected importdescriptors response length: %d", len(results))
	}
	if !results[0].Success {
		if results[0].Error != nil {
			return results[0].Error
		}
		return errors.New("importdescriptors failed")
	}
	return nil
}

// ImportAddressRescan imports the address with rescan.
func (c *Client) ImportAddressRescan(address string) error {
	err := c.Call(nil, "importaddress", address, "", true)
	return extractBTCError(err)
}

// CreateWallet creates a new wallet.
func (c *Client) CreateWallet(name string) error {
	descriptors := c.chain == common.BTCChain
	disablePrivateKeys := c.chain == common.BTCChain
	err := c.Call(nil, "createwallet", name, disablePrivateKeys, false, "", false, descriptors)
	err = extractBTCError(err)

	// ignore code -4 (wallet already exists)
	if rpcErr, ok := err.(*btcjson.RPCError); ok && rpcErr.Code == btcjson.ErrRPCWallet {
		return nil
	}
	if name == "" && err != nil && strings.Contains(err.Error(), "Wallet name cannot be empty") {
		return nil
	}

	return err
}

// ListUnspent returns all unspent outputs with between min and max confirmations.
func (c *Client) ListUnspent(address string) ([]btcjson.ListUnspentResult, error) {
	var unspent []btcjson.ListUnspentResult
	const minConfirm = 0
	const maxConfirm = 9999999
	err := c.Call(&unspent, "listunspent", minConfirm, maxConfirm, []string{address})

	return unspent, extractBTCError(err)
}

func (c *Client) GetNetworkInfo() (*btcjson.GetNetworkInfoResult, error) {
	type networkInfoCompat struct {
		btcjson.GetNetworkInfoResult
		Warnings json.RawMessage `json:"warnings"`
	}
	var info networkInfoCompat
	err := c.Call(&info, "getnetworkinfo")
	return &info.GetNetworkInfoResult, extractBTCError(err)
}

func (c *Client) Call(result any, method string, args ...interface{}) error {
	fn := func() error {
		if c.timeout <= 0 {
			return c.client.Call(result, method, args...)
		}
		ctx, cancel := context.WithTimeout(context.Background(), c.timeout)
		defer cancel()
		return c.client.CallContext(ctx, result, method, args...)
	}

	err := c.retry(fn)
	return extractBTCError(err)
}

func (c *Client) BatchCall(batch []rpc.BatchElem) error {
	err := c.retry(func() error {
		return c.batchCall(batch)
	})
	return extractBTCError(err)
}

func (c *Client) batchCall(batch []rpc.BatchElem) error {
	if c.timeout <= 0 {
		return c.client.BatchCall(batch)
	}
	ctx, cancel := context.WithTimeout(context.Background(), c.timeout)
	defer cancel()
	return c.client.BatchCallContext(ctx, batch)
}

////////////////////////////////////////////////////////////////////////////////////////
// Helpers
////////////////////////////////////////////////////////////////////////////////////////

// Ethereum RPC returns an error with the response appended to the HTTP status like:
// 404 Not Found: {"error":{"code":-32601,"message":"Method not found"},"id":1}
//
// This makes best effort to extract and return the error as a btcjson.RPCError.
func extractBTCError(err error) error {
	if err == nil {
		return nil
	}

	// split the error into the HTTP status and the JSON response
	parts := strings.SplitN(err.Error(), ": ", 2)
	if len(parts) != 2 {
		return err
	}

	// parse the JSON response
	var response struct {
		Error struct {
			Code    btcjson.RPCErrorCode `json:"code"`
			Message string               `json:"message"`
		} `json:"error"`
	}
	if jsonErr := json.Unmarshal([]byte(parts[1]), &response); jsonErr != nil {
		return err
	}

	// return the error message
	return btcjson.NewRPCError(response.Error.Code, response.Error.Message)
}

// retry will wrap the function call with a retry loop on specific errors - namely 500
// response when the daemon is overloaded the the work queue is exceeded. We use a
// simple static backoff of 1 second for now.
func (c *Client) retry(fn func() error) error {
	var err error
	backoff := time.Second
	for i := 0; i <= c.maxRetries; i++ {
		err = fn()
		if err == nil {
			break
		}

		errStr := strings.ToLower(err.Error())

		// check in order of most to least likely based on log sampling
		retry := strings.Contains(errStr, "connect: connection refused") ||
			strings.Contains(errStr, "work queue depth exceeded") ||
			(strings.HasPrefix(errStr, "post") && strings.HasSuffix(errStr, "eof")) ||
			strings.Contains(errStr, "loading block index") || // daemon startup
			strings.Contains(errStr, "verifying wallet") || // daemon startup
			strings.Contains(errStr, "verifying wallet") || // daemon startup
			strings.Contains(errStr, "currently rescanning") || // rescanning wallet
			strings.HasPrefix(errStr, "503 service unavailable")

		// break if not a retryable error
		if !retry {
			break
		}

		// break if this was the last retry
		if i == c.maxRetries {
			break
		}

		// backoff and retry
		c.log.Err(err).
			Str("backoff", backoff.String()).
			Msgf("retrying %d/%d after backoff", i+1, c.maxRetries)
		time.Sleep(backoff)

		// exponential backoff, max 10 seconds
		backoff *= 2
		if backoff > time.Second*10 {
			backoff = time.Second * 10
		}
	}
	return err
}
