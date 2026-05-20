package thorchain

import (
	"encoding/base64"
	"fmt"
	"strconv"
	"strings"

	"github.com/blang/semver"
	"github.com/thornadocash/go-thornado/common"
	"github.com/thornadocash/go-thornado/common/cosmos"
	"github.com/thornadocash/go-thornado/x/thorchain/keeper"
	"github.com/thornadocash/go-thornado/x/thorchain/types"
)

type parser struct {
	memo           string
	txType         TxType
	ctx            cosmos.Context
	keeper         keeper.Keeper
	parts          []string
	errs           []error
	version        semver.Version
	requiredFields int
}

func newParser(ctx cosmos.Context, keeper keeper.Keeper, version semver.Version, memo string) (parser, error) {
	if len(memo) == 0 {
		return parser{}, fmt.Errorf("memo can't be empty")
	}
	memo = strings.Split(memo, "|")[0]
	parts := strings.Split(memo, ":")
	memoType, err := StringToTxType(parts[0])
	if err != nil {
		return parser{}, err
	}

	return parser{
		memo:    memo,
		txType:  memoType,
		ctx:     ctx,
		keeper:  keeper,
		version: version,
		parts:   parts,
		errs:    make([]error, 0),
	}, nil
}

func (p *parser) getType() TxType {
	return p.txType
}

func (p *parser) incRequired(required bool) {
	if required {
		p.requiredFields += 1
	}
}

func (p *parser) parse() (mem Memo, err error) {
	defer func() {
		if err == nil {
			err = p.Error()
		}
	}()
	switch p.getType() {
	case TxLeave:
		return p.ParseLeaveMemo()
	case TxOutbound:
		return p.ParseOutboundMemo()
	case TxRefund:
		return p.ParseRefundMemo()
	case TxBond:
		return p.ParseBondMemo()
	case TxUnbond:
		return p.ParseUnbondMemo()
	case TxRebond:
		return p.ParseRebondMemo()
	case TxMigrate:
		return p.ParseMigrateMemo()
	case TxRagnarok:
		return p.ParseRagnarokMemo()
	case TxNoOp:
		return p.ParseNoOpMemo()
	case TxConsolidate:
		return p.ParseConsolidateMemo()
	case TxReferenceWriteMemo:
		return p.ParseReferenceWriteMemo()
	case TxReferenceReadMemo:
		return p.ParseReferenceReadMemo()
	case TxMaint:
		return p.ParseMaintMemo()
	case TxOperatorRotate:
		return p.ParseOperatorRotate()

	default:
		return EmptyMemo, fmt.Errorf("TxType not supported: %s", p.getType().String())
	}
}

func (p *parser) addErr(err error) {
	p.errs = append(p.errs, err)
}

func (p *parser) Error() error {
	p.hasMinParams(p.requiredFields + 1)
	if len(p.errs) == 0 {
		return nil
	}
	errStrs := make([]string, len(p.errs))
	for i, err := range p.errs {
		errStrs[i] = err.Error()
	}
	err := fmt.Errorf("MEMO: %s\nPARSE FAILURE(S): %s", p.memo, strings.Join(errStrs, "-"))
	return err
}

// check if memo has enough parameters
func (p *parser) hasMinParams(count int) {
	if len(p.parts) < count {
		p.addErr(fmt.Errorf("not enough parameters: %d/%d", len(p.parts), count))
	}
}

// Safe accessor for split memo parts - always returns empty
// string for indices that are out of bounds.
func (p *parser) get(idx int) string {
	if idx < 0 || len(p.parts) <= idx {
		return ""
	}
	return p.parts[idx]
}

// Safe accessor for split memo parts - always returns empty string for indices that are
// out of bounds. Returns the sub-index of a split part (with separator "/").
func (p *parser) getSubIndex(idx, subIdx int) string {
	if idx < 0 || len(p.parts) <= idx {
		return ""
	}
	subParts := strings.Split(p.parts[idx], "/")
	if subIdx < 0 || len(subParts) <= subIdx {
		return ""
	}
	return subParts[subIdx]
}

func (p *parser) getInt64(idx int, required bool, def int64) int64 {
	p.incRequired(required)
	value, err := strconv.ParseInt(p.get(idx), 10, 64)
	if err != nil {
		if required || p.get(idx) != "" {
			p.addErr(fmt.Errorf("cannot parse '%s' as an int64: %w", p.get(idx), err))
		}
		return def
	}
	return value
}

func (p *parser) getUint(idx int, required bool, def uint64) cosmos.Uint {
	p.incRequired(required)
	value, err := cosmos.ParseUint(p.get(idx))
	if err != nil {
		if required || p.get(idx) != "" {
			p.addErr(fmt.Errorf("cannot parse '%s' as an uint: %w", p.get(idx), err))
		}
		return cosmos.NewUint(def)
	}
	return value
}

func (p *parser) getUintWithScientificNotation(idx int, required bool, def uint64) cosmos.Uint {
	p.incRequired(required)
	s := p.get(idx)
	if s == "" {
		if required {
			p.addErr(fmt.Errorf("required index idx value is empty"))
		}
		return cosmos.NewUint(def)
	}

	// Handle scientific notation without floating-point precision loss
	if strings.Contains(s, "e") || strings.Contains(s, "E") {
		// Parse scientific notation manually to avoid float precision issues
		parts := strings.Split(strings.ToLower(s), "e")
		if len(parts) != 2 {
			if required || s != "" {
				p.addErr(fmt.Errorf("cannot parse '%s' as scientific notation", s))
			}
			return cosmos.NewUint(def)
		}

		// Parse the coefficient
		coeff, err := cosmos.ParseUint(parts[0])
		if err != nil {
			if required || s != "" {
				p.addErr(fmt.Errorf("cannot parse coefficient '%s' in scientific notation: %w", parts[0], err))
			}
			return cosmos.NewUint(def)
		}

		// Parse the exponent
		exp, err := strconv.ParseInt(parts[1], 10, 64)
		if err != nil {
			if required || s != "" {
				p.addErr(fmt.Errorf("cannot parse exponent '%s' in scientific notation: %w", parts[1], err))
			}
			return cosmos.NewUint(def)
		}

		if exp < 0 {
			if required || s != "" {
				p.addErr(fmt.Errorf("the memo parser does not support fractional values: %s", s))
			}
			return cosmos.NewUint(def)
		}

		// Set maximum exponent limit based on 256-bit capacity
		const maxExponent = 77 // 10^77 is still within 256-bit range

		if exp > maxExponent {
			if required || s != "" {
				p.addErr(fmt.Errorf("exponent %d exceeds maximum allowed value of %d: %s", exp, maxExponent, s))
			}
			return cosmos.NewUint(def)
		}

		// Calculate 10^exp (the multiplier)
		multiplier := cosmos.NewUint(1)
		for i := int64(0); i < exp; i++ {
			multiplier = multiplier.MulUint64(10)
		}

		// Get the maximum 256-bit value
		// For cosmos.Uint, this would be 2^256 - 1
		maxUint256 := cosmos.NewUintFromString("115792089237316195423570985008687907853269984665640564039457584007913129639935") // 2^256 - 1

		// Check if coefficient * multiplier would overflow
		if !coeff.IsZero() && coeff.GT(maxUint256.Quo(multiplier)) {
			if required || s != "" {
				p.addErr(fmt.Errorf("coefficient %s would cause overflow when multiplied by 10^%d: %s", coeff.String(), exp, s))
			}
			return cosmos.NewUint(def)
		}

		return coeff.Mul(multiplier)
	}

	// Fallback to regular uint parsing for non-scientific notation
	result, err := cosmos.ParseUint(s)
	if err != nil {
		if required || s != "" {
			p.addErr(fmt.Errorf("cannot parse '%s' as an uint: %w", s, err))
		}
		return cosmos.NewUint(def)
	}
	return result
}

func (p *parser) getUintWithMaxValue(idx int, required bool, def, max uint64) cosmos.Uint {
	value := p.getUint(idx, required, def)
	if value.GT(cosmos.NewUint(max)) {
		if required || p.get(idx) != "" {
			p.addErr(fmt.Errorf("%s cannot exceed '%d'", p.get(idx), max))
		}
		return cosmos.NewUint(max)
	}
	return value
}

func (p *parser) getAccAddress(idx int, required bool, def cosmos.AccAddress) cosmos.AccAddress {
	p.incRequired(required)
	value, err := cosmos.AccAddressFromBech32(p.get(idx))
	if err != nil {
		if required || p.get(idx) != "" {
			p.addErr(fmt.Errorf("cannot parse '%s' as an AccAddress: %w", p.get(idx), err))
		}
		return def
	}
	return value
}

func (p *parser) getAddress(idx int, required bool, def common.Address) common.Address {
	p.incRequired(required)
	value, err := common.NewAddress(p.get(idx))
	if err != nil {
		if required || p.get(idx) != "" {
			p.addErr(fmt.Errorf("cannot parse '%s' as an Address: %w", p.get(idx), err))
		}
		return def
	}
	return value
}

func (p *parser) getThorAddress(idx int, required bool, def common.Address) common.Address {
	p.incRequired(required)
	value, err := common.NewAddress(p.get(idx))
	if err != nil {
		if required || p.get(idx) != "" {
			p.addErr(fmt.Errorf("cannot parse '%s' as an Address: %w", p.get(idx), err))
		}
		return def
	}

	if !value.IsChain(common.THORChain) && required {
		p.addErr(fmt.Errorf("cannot parse '%s' as a THOR Address: %w", p.get(idx), err))
		return def
	}
	return value
}

func (p *parser) getAddressWithKeeper(idx int, required bool, def common.Address, chain common.Chain) common.Address {
	p.incRequired(required)
	if p.keeper == nil {
		return p.getAddress(idx, required, common.NoAddress)
	}
	addr, err := FetchAddress(p.ctx, p.keeper, p.get(idx), chain)
	if err != nil {
		if required || p.get(idx) != "" {
			p.addErr(fmt.Errorf("cannot parse '%s' as an Address: %w", p.get(idx), err))
		}
	}
	return addr
}

func (p *parser) getString(idx int, required bool, def string) string {
	p.incRequired(required)
	value := p.get(idx)
	if value == "" {
		if required {
			p.addErr(fmt.Errorf("cannot parse '%s' as a string", p.get(idx)))
		}
		return def
	}
	return value
}

func (p *parser) getStringArrayBySeparator(idx int, required bool, separator string) []string {
	p.incRequired(required)
	value := p.get(idx)
	if value == "" {
		return []string{}
	}
	return strings.Split(value, separator)
}

func (p *parser) getUintArrayBySeparator(idx int, required bool, separator string) []cosmos.Uint {
	p.incRequired(required)
	value := p.get(idx)
	if value == "" {
		return []cosmos.Uint{}
	}
	strArray := strings.Split(value, separator)
	result := make([]cosmos.Uint, 0, len(strArray))
	for _, str := range strArray {
		u, err := cosmos.ParseUint(str)
		if err != nil {
			if required || str != "" {
				p.addErr(fmt.Errorf("cannot parse '%s' as an uint: %w", str, err))
			}
			return []cosmos.Uint{}
		}
		result = append(result, u)
	}
	return result
}

func (p *parser) getAddressAndRefundAddressWithKeeper(idx int, required bool, def common.Address, chain common.Chain) (common.Address, common.Address) {
	p.incRequired(required)

	//nolint:ineffassign
	destination := common.NoAddress
	refundAddress := common.NoAddress
	addresses := p.get(idx)

	if strings.Contains(addresses, "/") {
		parts := strings.SplitN(addresses, "/", 2)
		if p.keeper == nil {
			dest, err := common.NewAddress(parts[0])
			if err != nil {
				if required || parts[0] != "" {
					p.addErr(fmt.Errorf("cannot parse '%s' as an Address: %w", parts[0], err))
				}
			}
			destination = dest
		} else {
			destination = p.getAddressFromString(parts[0], chain, required)
		}
		if len(parts) > 1 {
			refundAddress, _ = common.NewAddress(parts[1])
		}
	} else {
		destination = p.getAddressWithKeeper(idx, false, common.NoAddress, chain)
	}

	if destination.IsEmpty() && !refundAddress.IsEmpty() {
		p.addErr(fmt.Errorf("refund address is set but destination address is empty"))
	}

	return destination, refundAddress
}

func (p *parser) getAddressFromString(val string, chain common.Chain, required bool) common.Address {
	addr, err := FetchAddress(p.ctx, p.keeper, val, chain)
	if err != nil {
		if required || val != "" {
			p.addErr(fmt.Errorf("cannot parse '%s' as an Address: %w", val, err))
		}
	}
	return addr
}

func (p *parser) getChain(idx int, required bool, def common.Chain) common.Chain {
	p.incRequired(required)
	value, err := common.NewChain(p.get(idx))
	if err != nil {
		if required || p.get(idx) != "" {
			p.addErr(fmt.Errorf("cannot parse '%s' as a chain: %w", p.get(idx), err))
		}
		return def
	}
	return value
}

func (p *parser) getAsset(idx int, required bool, def common.Asset) common.Asset {
	p.incRequired(required)
	value, err := common.NewAssetWithShortCodes(p.version, p.get(idx))
	if err != nil && (required || p.get(idx) != "") {
		p.addErr(fmt.Errorf("cannot parse '%s' as an asset: %w", p.get(idx), err))
		return def
	}
	return value
}

func (p *parser) getCoin(idx int, required bool, def common.Coin) common.Coin {
	p.incRequired(required)
	coinStr := p.get(idx)

	// Check if the coin string contains special separators for trade assets (~),
	// synths (/), or secured assets (-). These are not valid in cosmos.ParseCoins
	// but are valid THORChain assets that need custom parsing via common.ParseCoin.
	if strings.ContainsAny(coinStr, "~/-") {
		// common.ParseCoin expects format "<amount> <asset>" with a space,
		// but memo format is "<amount><asset>" without a space.
		// Find where digits end and insert a space.
		i := 0
		for i < len(coinStr) && (coinStr[i] >= '0' && coinStr[i] <= '9') {
			i++
		}

		if i == 0 || i == len(coinStr) {
			if required || coinStr != "" {
				p.addErr(fmt.Errorf("cannot parse '%s' as coin: invalid format", coinStr))
			}
			return def
		}

		// Insert space between amount and asset
		coinStrWithSpace := coinStr[:i] + " " + coinStr[i:]

		// Use common.ParseCoin which handles trade assets, synths, and secured assets
		coin, err := common.ParseCoin(coinStrWithSpace)
		if err != nil {
			if required || coinStr != "" {
				p.addErr(fmt.Errorf("cannot parse '%s' as coin: %w", coinStr, err))
			}
			return def
		}

		return coin
	}

	// Use standard cosmos.ParseCoins for regular assets (backward compatibility)
	coins, err := cosmos.ParseCoins(coinStr)
	if err != nil {
		if required || coinStr != "" {
			p.addErr(fmt.Errorf("cannot parse '%s' as coins: %w", p.get(idx), err))
		}
		return def
	}

	// Should have exactly one coin
	if len(coins) != 1 {
		if required || coinStr != "" {
			p.addErr(fmt.Errorf("expected exactly one coin, got %d", len(coins)))
		}
		return def
	}

	// Convert cosmos.Coin to common.Coin
	cosmosCoin := coins[0]

	// Parse the denom back to an asset
	asset, err := common.NewAssetWithShortCodes(p.version, cosmosCoin.Denom)
	if err != nil {
		if required || coinStr != "" {
			p.addErr(fmt.Errorf("cannot parse denom '%s' as asset: %w", cosmosCoin.Denom, err))
		}
		return def
	}

	return common.NewCoin(asset, cosmos.NewUintFromBigInt(cosmosCoin.Amount.BigInt()))
}

func (p *parser) getTxID(idx int, required bool, def common.TxID) common.TxID {
	p.incRequired(required)
	value, err := common.NewTxID(p.get(idx))
	if err != nil {
		if required || p.get(idx) != "" {
			p.addErr(fmt.Errorf("cannot parse '%s' as tx hash: %w", p.get(idx), err))
		}
		return def
	}
	return value
}

func (p *parser) getTHORName(idx int, required bool, def types.THORName, subIndex int) types.THORName {
	p.incRequired(required)
	name := p.get(idx)
	if subIndex >= 0 {
		name = p.getSubIndex(idx, subIndex)
	}
	if p.keeper == nil {
		return def
	}
	if p.keeper.THORNameExists(p.ctx, name) {
		tn, err := p.keeper.GetTHORName(p.ctx, name)
		if err != nil {
			if required || p.get(idx) != "" {
				p.addErr(fmt.Errorf("cannot parse '%s' as a THORName: %w", p.get(idx), err))
			}
		}
		return tn
	}
	return def
}

func (p *parser) getBase64Bytes(idx int, required bool, def []byte) []byte {
	p.incRequired(required)
	value, err := base64.StdEncoding.DecodeString(p.get(idx))
	if err != nil {
		if required || p.get(idx) != "" {
			p.addErr(fmt.Errorf("cannot parse '%s' as a base64 string: %w", p.get(idx), err))
		}
		return def
	}
	return value
}
