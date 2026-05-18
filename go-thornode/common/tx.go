package common

import (
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"fmt"
	"math"
	"regexp"
	"strings"

	"gitlab.com/thorchain/thornode/v3/common/cosmos"
	"gitlab.com/thorchain/thornode/v3/constants"
)

type (
	// TxID is a string that can uniquely represent a transaction on different
	// block chain
	TxID string
	// TxIDs is a slice of TxID
	TxIDs []TxID
)

var (
	// BlankTxID represent blank
	BlankTxID = TxID("0000000000000000000000000000000000000000000000000000000000000000")

	regexIsCosmosIndexed = regexp.MustCompile(`^[0-9a-fA-F]{64}-[0-9]+$`)
)

// NewTxID parse the input hash as TxID
func NewTxID(hash string) (TxID, error) {
	// for cosmos tx hash with appended id
	// eg: DDFB48D1A6084FD41FE1D37BB5A950234F4AED3CF8036AED12633389BDC37DB9-1
	if len(hash) > 64 {
		// TODO: check if regex is much overhead
		if regexIsCosmosIndexed.MatchString(hash) {
			return TxID(hash), nil
		}
	}

	switch len(hash) {
	case 64:
		// do nothing
	case 66: // ETH check
		if !strings.HasPrefix(hash, "0x") {
			err := fmt.Errorf("txid error: must be 66 characters (got %d)", len(hash))
			return TxID(""), err
		}
	case 87, 88:
		// SOL transaction hash, base58 is case-sensitive so do not uppercase
		return TxID(hash), nil
	default:
		err := fmt.Errorf("txid error: must be 64 characters (got %d)", len(hash))
		return TxID(""), err
	}

	return TxID(strings.ToUpper(hash)), nil
}

// Equals check whether two TxID are the same
func (tx TxID) Equals(tx2 TxID) bool {
	return strings.EqualFold(tx.String(), tx2.String())
}

func (tx TxID) Int64() int64 {
	if tx.IsEmpty() {
		return 0
	}

	s := tx.String()
	if len(s) < 8 {
		panic("tx id too short: " + s)
	}

	b := []byte(s)
	last := b[len(b)-8:]

	n := binary.BigEndian.Uint64(last)
	n &= math.MaxInt64 // ensure non-negative

	return int64(n)
}

// IsEmpty return true when the tx represent empty string
func (tx TxID) IsEmpty() bool {
	return strings.TrimSpace(tx.String()) == ""
}

func (tx TxID) IsBlank() bool {
	return tx.Equals(BlankTxID)
}

// String implement fmt.Stringer
func (tx TxID) String() string {
	return string(tx)
}

// Txs a list of Tx
type Txs []Tx

// GetRagnarokTx return a tx used for ragnarok
func GetRagnarokTx(chain Chain, fromAddr, toAddr Address) Tx {
	blankAsset := Asset{
		Chain:  chain,
		Symbol: "BLANK",
		Ticker: "BLANK",
	}
	return Tx{
		Chain:       chain,
		ID:          BlankTxID,
		FromAddress: fromAddr,
		ToAddress:   toAddr,
		Coins: Coins{
			// used for ragnarok, so doesn't really matter
			NewCoin(blankAsset, cosmos.OneUint()),
		},
		Gas: Gas{
			// used for ragnarok, so doesn't really matter
			NewCoin(blankAsset, cosmos.OneUint()),
		},
		Memo: "Ragnarok",
	}
}

// NewTx create a new instance of Tx based on the input information
func NewTx(txID TxID, from, to Address, coins Coins, gas Gas, memo string) Tx {
	var chain Chain
	for _, coin := range coins {
		chain = coin.Asset.GetChain()
		break
	}
	return Tx{
		ID:          txID,
		Chain:       chain,
		FromAddress: from,
		ToAddress:   to,
		Coins:       coins,
		Gas:         gas,
		Memo:        memo,
	}
}

// Hash calculates an internal hash based on chain, from address, coins, to address and block height.
func (tx Tx) Hash(blockHeight int64) string {
	str := fmt.Sprintf("%s|%s|%s|%s|%d", tx.Chain, tx.FromAddress, tx.Coins, tx.ToAddress, blockHeight)
	return fmt.Sprintf("%X", sha256.Sum256([]byte(str)))
}

// String implement fmt.Stringer return a string representation of the tx
func (tx Tx) String() string {
	return fmt.Sprintf("%s: %s ==> %s (Memo: %s) %s (gas: %s)", tx.ID, tx.FromAddress, tx.ToAddress, tx.Memo, tx.Coins, tx.Gas)
}

// IsEmpty check whether the ID field is empty or not
func (tx Tx) IsEmpty() bool {
	return tx.ID.IsEmpty()
}

// EqualsEx compare two Tx to see whether they represent the same Tx
// This method will not change the original tx & tx2
func (tx Tx) EqualsEx(tx2 Tx) bool {
	if !tx.ID.Equals(tx2.ID) {
		return false
	}
	if !tx.Chain.Equals(tx2.Chain) {
		return false
	}
	if !tx.FromAddress.Equals(tx2.FromAddress) {
		return false
	}
	if !tx.ToAddress.Equals(tx2.ToAddress) {
		return false
	}
	if !tx.Coins.EqualsEx(tx2.Coins) {
		return false
	}
	if !tx.Gas.Equals(tx2.Gas) {
		return false
	}
	if !strings.EqualFold(tx.Memo, tx2.Memo) {
		return false
	}
	return true
}

// EqualsExIgnoreGas compare two Tx to see whether they represent the same Tx,
// ignoring gas differences. Used for matching re-observations after reorg where
// gas may differ due to changed block conditions (different base fees or tips).
func (tx Tx) EqualsExIgnoreGas(tx2 Tx) bool {
	if !tx.ID.Equals(tx2.ID) {
		return false
	}
	if !tx.Chain.Equals(tx2.Chain) {
		return false
	}
	if !tx.FromAddress.Equals(tx2.FromAddress) {
		return false
	}
	if !tx.ToAddress.Equals(tx2.ToAddress) {
		return false
	}
	if !tx.Coins.EqualsEx(tx2.Coins) {
		return false
	}
	if !strings.EqualFold(tx.Memo, tx2.Memo) {
		return false
	}
	return true
}

// Valid do some data sanity check , if the tx contains invalid information
// it will return an none nil error
func (tx Tx) Valid() error {
	if tx.ID.IsEmpty() {
		return errors.New("Tx ID cannot be empty")
	}
	if tx.FromAddress.IsEmpty() {
		return errors.New("from address cannot be empty")
	}
	if tx.ToAddress.IsEmpty() {
		return errors.New("to address cannot be empty")
	}
	if tx.Chain.IsEmpty() {
		return errors.New("chain cannot be empty")
	}
	if len(tx.Coins) == 0 {
		return errors.New("must have at least 1 coin")
	}
	if err := tx.Coins.Valid(); err != nil {
		return err
	}
	if !tx.Chain.Equals(THORChain) && len(tx.Gas) == 0 {
		return errors.New("must have at least 1 gas coin")
	}
	if err := tx.Gas.Valid(); err != nil {
		return err
	}
	// relax this check from 150 -> 180
	if len([]byte(tx.Memo)) > constants.MaxMemoSize {
		return fmt.Errorf("memo must not exceed %d bytes: %d", constants.MaxMemoSize, len([]byte(tx.Memo)))
	}
	return nil
}

// ToAttributes push all the tx fields into a slice of cosmos Attribute(key value pairs)
func (tx Tx) ToAttributes() []cosmos.Attribute {
	return []cosmos.Attribute{
		cosmos.NewAttribute("id", tx.ID.String()),
		cosmos.NewAttribute("chain", tx.Chain.String()),
		cosmos.NewAttribute("from", tx.FromAddress.String()),
		cosmos.NewAttribute("to", tx.ToAddress.String()),
		cosmos.NewAttribute("coin", tx.Coins.String()),
		cosmos.NewAttribute("memo", tx.Memo),
	}
}
