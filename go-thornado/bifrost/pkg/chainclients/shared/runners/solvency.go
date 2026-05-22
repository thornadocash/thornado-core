package runners

import (
	"fmt"
	"sync"
	"time"

	"github.com/rs/zerolog/log"

	"github.com/thornadocash/go-thornado/bifrost/thornadoclient"
	"github.com/thornadocash/go-thornado/common"
	"github.com/thornadocash/go-thornado/common/cosmos"
	"github.com/thornadocash/go-thornado/constants"
	"github.com/thornadocash/go-thornado/x/thornado/types"
)

// SolvencyCheckProvider methods that a SolvencyChecker implementation should have
type SolvencyCheckProvider interface {
	GetHeight() (int64, error)
	ShouldReportSolvency(height int64) bool
	ReportSolvency(height int64) error
}

// SolvencyCheckRunner when a chain get marked as insolvent , and then get halt automatically , the chain client will stop scanning blocks , as a result , solvency checker will
// not report current solvency status to Thornado anymore, this method is to ensure that the chain client will continue to do solvency check even when the chain has been halted
func SolvencyCheckRunner(chain common.Chain,
	provider SolvencyCheckProvider,
	bridge thornadoclient.ThornadoBridge,
	stopper <-chan struct{},
	wg *sync.WaitGroup,
	backOffDuration time.Duration,
) {
	logger := log.Logger.With().Str("chain", chain.String()).Logger()
	logger.Info().Msg("start solvency check runner")
	defer func() {
		wg.Done()
		logger.Info().Msg("finish  solvency check runner")
	}()
	if provider == nil {
		logger.Error().Msg("solvency checker provider is nil")
		return
	}
	if backOffDuration == 0 {
		backOffDuration = constants.ThornadoBlockTime
	}
	for {
		select {
		case <-stopper:
			return
		case <-time.After(backOffDuration):
			// check whether the chain is halted via mimir or not
			haltHeight, err := bridge.GetMimir(fmt.Sprintf("Halt%sChain", chain))
			if err != nil {
				logger.Err(err).Msg("fail to get chain halt height")
				continue
			}

			// check whether the chain is halted via solvency check
			solvencyHaltHeight, err := bridge.GetMimir(fmt.Sprintf("SolvencyHalt%sChain", chain))
			if err != nil {
				logger.Err(err).Msg("fail to get solvency halt height")
				continue
			}

			thorHeight, err := bridge.GetBlockHeight()
			if err != nil {
				logger.Err(err).Msg("fail to get Thornado block height")
				continue
			}

			// Halt<chain>Chain values > 1 are height-based and should not activate until Thornado
			// reaches that height. A value of 1 is treated as an immediate admin halt and should
			// not trigger runner-driven solvency checks.
			chainHalted := haltHeight > 1 && thorHeight >= haltHeight
			// Solvency halts are also height-based, though in practice they are expected to be set
			// to the current Thornado height by the solvency handler.
			solvencyHalted := solvencyHaltHeight > 0 && thorHeight >= solvencyHaltHeight
			// When the chain is not actively halted, the normal chain client will report solvency
			// when needed.
			if !chainHalted && !solvencyHalted {
				continue
			}

			currentBlockHeight, err := provider.GetHeight()
			if err != nil {
				logger.Err(err).Msg("fail to get current block height")
				break
			}
			if provider.ShouldReportSolvency(currentBlockHeight) {
				logger.Info().Msgf("current block height: %d, report solvency again", currentBlockHeight)
				if err = provider.ReportSolvency(currentBlockHeight); err != nil {
					logger.Err(err).Msg("fail to report solvency")
				}
			}
		}
	}
}

// IsVaultSolvent checks whether the chain-specific portion of the given vault is solvent.
// Vaults contain assets for all chains, so filter to the caller's chain before comparing
// account balances.
func IsVaultSolvent(chain common.Chain, account common.Account, vault types.Vault, currentGasFee cosmos.Uint) bool {
	logger := log.Logger
	currentGasFee10x := currentGasFee.MulUint64(10)

	// Intentionally compare against vault expectations, treating an absent wallet asset as
	// zero. Iterating wallet coins can miss vault-required assets that are absent from the
	// wallet and incorrectly appear solvent.
	for _, asgardCoin := range vault.Coins {
		if !asgardCoin.Asset.Chain.Equals(chain) {
			continue
		}
		if asgardCoin.Amount.IsZero() {
			continue
		}

		walletAmt := cosmos.ZeroUint()
		walletCoin := account.Coins.GetCoin(asgardCoin.Asset)
		if walletCoin.IsEmpty() {
			logger.Info().
				Stringer("asset", asgardCoin.Asset).
				Stringer("amount", asgardCoin.Amount).
				Msg("asset exists in vault but not in wallet, insolvent")
		} else {
			walletAmt = walletCoin.Amount
		}

		// when wallet has more coins or equal exactly as asgard , then the vault is solvent
		if walletAmt.GTE(asgardCoin.Amount) {
			continue
		}

		gap := asgardCoin.Amount.Sub(walletAmt)
		// thornado allow 10x of MaxGas as the gap
		if asgardCoin.Asset.IsGasAsset() && gap.LT(currentGasFee10x) {
			continue
		}
		logger.Info().
			Stringer("chain", chain).
			Stringer("asset", asgardCoin.Asset).
			Stringer("asgard_amount", asgardCoin.Amount).
			Stringer("wallet_amount", walletAmt).
			Stringer("gap", gap).
			Msg("insolvency detected")
		return false
	}
	logger.Debug().
		Stringer("chain", chain).
		Stringer("vault", vault.Coins).
		Stringer("wallet", account.Coins).
		Stringer("currentGasFee10x", currentGasFee10x).
		Msg("vault is solvent")
	return true
}
