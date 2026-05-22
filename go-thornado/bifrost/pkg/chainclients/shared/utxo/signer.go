package utxo

import (
	"errors"
	"fmt"

	"github.com/hashicorp/go-multierror"
	"github.com/rs/zerolog"
	"github.com/thornadocash/go-thornado/bifrost/thornadoclient"
	stypes "github.com/thornadocash/go-thornado/bifrost/thornadoclient/types"
	"github.com/thornadocash/go-thornado/bifrost/tss"
)

// SignCheckpoint is used to checkpoint the built transaction before signing, for use in
// round 7 signing errors which must reuse the same inputs.
type SignCheckpoint struct {
	UnsignedTx        []byte           `json:"unsigned_tx"`
	IndividualAmounts map[string]int64 `json:"individual_amounts"`
}

func PostKeysignFailure(
	thornadoBridge thornadoclient.ThornadoBridge,
	tx stypes.TxOutItem,
	logger zerolog.Logger,
	thornadoHeight int64,
	utxoErr error,
) error {
	// PostKeysignFailure only once per SignTx, to not broadcast duplicate messages.
	var keysignError tss.KeysignError
	if errors.As(utxoErr, &keysignError) {
		if len(keysignError.Blame.BlameNodes) == 0 {
			// TSS doesn't know which node to blame
			utxoErr = multierror.Append(utxoErr, fmt.Errorf("fail to sign UTXO"))
			return fmt.Errorf("fail to sign the message: %w", utxoErr)
		}

		// key sign error forward the keysign blame to thornado
		txID, err := thornadoBridge.PostKeysignFailure(keysignError.Blame, thornadoHeight, "", tx.Coins, tx.VaultPubKey)
		if err != nil {
			logger.Error().Err(err).Msg("fail to post keysign failure to thornado")
			utxoErr = multierror.Append(utxoErr, fmt.Errorf("fail to post keysign failure to Thornado: %w", err))
			return fmt.Errorf("fail to sign the message: %w", utxoErr)
		}
		logger.Info().Str("tx_id", txID.String()).Msgf("post keysign failure to thornado")
	}
	return utxoErr
}
