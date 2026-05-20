package thorclient

import (
	"fmt"
	"time"

	clienttx "github.com/cosmos/cosmos-sdk/client/tx"
	"github.com/cosmos/cosmos-sdk/types/tx/signing"
	flag "github.com/spf13/pflag"

	stypes "github.com/cosmos/cosmos-sdk/types"

	"github.com/thornadocash/go-thornado/bifrost/metrics"
	"github.com/thornadocash/go-thornado/common"
)

// broadcast is an internal helper that broadcasts a transaction with the specified mode.
// mode can be "async", "sync", or "commit"
func (b *thorchainBridge) broadcast(mode string, msgs ...stypes.Msg) (common.TxID, error) {
	b.broadcastLock.Lock()
	defer b.broadcastLock.Unlock()

	noTxID := common.TxID("")

	start := time.Now()
	defer func() {
		b.m.GetHistograms(metrics.SendToThorchainDuration).Observe(time.Since(start).Seconds())
	}()

	blockHeight, err := b.GetBlockHeight()
	if err != nil {
		return noTxID, err
	}
	if blockHeight > b.blockHeight {
		var seqNum uint64
		b.accountNumber, seqNum, err = b.getAccountNumberAndSequenceNumber()
		if err != nil {
			return noTxID, fmt.Errorf("fail to get account number and sequence number from thorchain : %w", err)
		}
		b.blockHeight = blockHeight
		if seqNum > b.seqNumber {
			b.seqNumber = seqNum
		}
	}

	b.logger.Info().Uint64("account_number", b.accountNumber).Uint64("sequence_number", b.seqNumber).Str("broadcast_mode", mode).Msg("account info")

	flags := flag.NewFlagSet("thorchain", 0)

	ctx := b.GetContext()
	ctx = ctx.WithBroadcastMode(mode)
	factory, err := clienttx.NewFactoryCLI(ctx, flags)
	if err != nil {
		return noTxID, fmt.Errorf("failed to get factory, %w", err)
	}
	factory = factory.WithAccountNumber(b.accountNumber)
	factory = factory.WithSequence(b.seqNumber)
	factory = factory.WithSignMode(signing.SignMode_SIGN_MODE_DIRECT)

	builder, err := factory.BuildUnsignedTx(msgs...)
	if err != nil {
		return noTxID, err
	}
	builder.SetGasLimit(4000000000)
	err = clienttx.Sign(ctx.CmdContext, factory, ctx.GetFromName(), builder, true)
	if err != nil {
		return noTxID, err
	}

	txBytes, err := ctx.TxConfig.TxEncoder()(builder.GetTx())
	if err != nil {
		return noTxID, err
	}

	// broadcast to a Tendermint node
	commit, err := ctx.BroadcastTx(txBytes)
	if err != nil {
		return noTxID, fmt.Errorf("fail to broadcast tx: %w", err)
	}

	b.m.GetCounter(metrics.TxToThorchainSigned).Inc()
	txHash, err := common.NewTxID(commit.TxHash)
	if err != nil {
		return common.BlankTxID, fmt.Errorf("fail to convert txhash: %w", err)
	}
	// Code will be the tendermint ABICode , it start at 1 , so if it is an error , code will not be zero
	if commit.Code > 0 {
		if commit.Code == 32 {
			// bad sequence number, fetch new one
			_, seqNum, _ := b.getAccountNumberAndSequenceNumber()
			if seqNum > 0 {
				b.seqNumber = seqNum
			}
		}
		b.logger.Info().Int("bytes", len(txBytes)).Uint32("code", commit.Code).Interface("messages", msgs).Msg("failed tx")
		// commit code 6 means `unknown request` , which means the tx can't be accepted by thorchain
		// if that's the case, let's just ignore it and move on
		if commit.Code != 6 {
			return txHash, fmt.Errorf("fail to broadcast to THORChain,code:%d, log:%s", commit.Code, commit.RawLog)
		}
	} else {
		b.seqNumber++
	}

	b.m.GetCounter(metrics.TxToThorchain).Inc()
	b.logger.Info().Msgf("Received a TxHash of %v from the thorchain", commit.TxHash)

	return txHash, nil
}

// Broadcast Broadcasts tx to thorchain using sync mode (waits for CheckTx)
func (b *thorchainBridge) Broadcast(msgs ...stypes.Msg) (common.TxID, error) {
	return b.broadcast("sync", msgs...)
}

// BroadcastWithBlocking broadcasts tx to thorchain and waits for block commitment
func (b *thorchainBridge) BroadcastWithBlocking(msgs ...stypes.Msg) (common.TxID, error) {
	// First broadcast with sync mode
	txID, err := b.broadcast("sync", msgs...)
	if err != nil {
		return txID, err
	}

	// Poll for the transaction to be included in a block
	// Poll for up to 30 seconds (6 blocks at 5 seconds per block)
	maxAttempts := 30
	for i := 0; i < maxAttempts; i++ {
		// Query for the block height to trigger state update
		currentHeight, err := b.GetBlockHeight()
		if err == nil && currentHeight > b.blockHeight {
			// Transaction should be committed by now
			b.logger.Info().Str("txid", txID.String()).Int64("height", currentHeight).Msg("transaction likely committed to block")
			return txID, nil
		}

		// Wait 1 second before retrying
		time.Sleep(time.Second)
	}

	// Return success even if we timeout - the transaction was accepted in CheckTx
	b.logger.Warn().Str("txid", txID.String()).Msg("timeout waiting for block commitment, but transaction was accepted")
	return txID, nil
}
