package keeperv1

import (
	"encoding/json"
	"fmt"

	"github.com/thornadocash/go-thornado/common"
	"github.com/thornadocash/go-thornado/common/cosmos"
	"github.com/thornadocash/go-thornado/x/thornado/types"
)

// InvariantRoutes returns the keeper's Thornado-specific invariant routes.
func (k KVStore) InvariantRoutes() []common.InvariantRoute {
	return []common.InvariantRoute{
		common.NewInvariantRoute("thornado", ThornadoInvariant(k)),
		common.NewInvariantRoute("deposits", DepositRecordInvariant(k)),
		common.NewInvariantRoute("shielder_vault_addresses", ShielderVaultAddressInvariant(k)),
		common.NewInvariantRoute("node_bonds", NodeBondInvariant(k)),
		common.NewInvariantRoute("node_slot_auctions", NodeSlotAuctionInvariant(k)),
		common.NewInvariantRoute("fees", FeeInvariant(k)),
	}
}

func ThornadoInvariant(k KVStore) common.Invariant {
	return func(ctx cosmos.Context) (msg []string, broken bool) {
		tcAddr := k.GetModuleAccAddress(ModuleName)
		tcCoins := k.GetBalance(ctx, tcAddr)
		if tcCoins.Empty() {
			return nil, false
		}
		for _, coin := range tcCoins {
			msg = append(msg, fmt.Sprintf("unexpected module balance: %s", coin))
		}
		return msg, true
	}
}

func DepositRecordInvariant(k KVStore) common.Invariant {
	return func(ctx cosmos.Context) (msg []string, broken bool) {
		iterator := k.GetDepositRecordIterator(ctx)
		defer iterator.Close()

		for ; iterator.Valid(); iterator.Next() {
			var deposit types.DepositRecord
			if err := json.Unmarshal(iterator.Value(), &deposit); err != nil {
				msg = append(msg, fmt.Sprintf("invalid deposit encoding: %s", string(iterator.Key())))
				broken = true
				continue
			}
			if err := deposit.Valid(); err != nil {
				msg = append(msg, fmt.Sprintf("%s: invalid deposit: %v", deposit.DepositID, err))
				broken = true
			}
			if deposit.SplitSats > deposit.AmountSats {
				msg = append(msg, fmt.Sprintf("%s: split amount exceeds deposit amount", deposit.DepositID))
				broken = true
			}
			switch deposit.Status {
			case types.DepositStatusDepositMatched:
				if deposit.Settlement != "" {
					msg = append(msg, fmt.Sprintf("%s: matched deposit has settlement %s", deposit.DepositID, deposit.Settlement))
					broken = true
				}
				if len(deposit.Commitments) != 0 {
					msg = append(msg, fmt.Sprintf("%s: matched deposit has commitments", deposit.DepositID))
					broken = true
				}
			case types.DepositStatusSettled:
				msg = append(msg, fmt.Sprintf("%s: persisted transient settlement", deposit.DepositID))
				broken = true
			case types.DepositStatusCommitted:
				if deposit.Settlement == "" {
					msg = append(msg, fmt.Sprintf("%s: committed deposit missing settlement", deposit.DepositID))
					broken = true
				}
				if len(deposit.Commitments) == 0 {
					msg = append(msg, fmt.Sprintf("%s: committed deposit missing commitments", deposit.DepositID))
					broken = true
				}
				for _, commitment := range deposit.Commitments {
					if !k.ShielderCommitmentExists(ctx, commitment) {
						msg = append(msg, fmt.Sprintf("%s: missing commitment index %s", deposit.DepositID, commitment))
						broken = true
					}
				}
			default:
				msg = append(msg, fmt.Sprintf("%s: invalid deposit status %s", deposit.DepositID, deposit.Status))
				broken = true
			}
			if deposit.Settlement == types.DepositSettlementUser && (deposit.IsNodeBond() || deposit.AuctionID != "") {
				msg = append(msg, fmt.Sprintf("%s: user settlement has operator metadata", deposit.DepositID))
				broken = true
			}
			if deposit.Settlement == types.DepositSettlementOperatorBond && !deposit.IsNodeBond() {
				msg = append(msg, fmt.Sprintf("%s: bond settlement missing node pubkey", deposit.DepositID))
				broken = true
			}
			if deposit.Settlement == types.DepositSettlementOperatorBond && deposit.Status == types.DepositStatusCommitted && !deposit.BondConfirmed {
				msg = append(msg, fmt.Sprintf("%s: committed bond deposit not confirmed", deposit.DepositID))
				broken = true
			}
			if deposit.Settlement == types.DepositSettlementOperatorSale && (deposit.AuctionID == "" || deposit.SellerPayoutSats+deposit.ProtocolBondSats != deposit.AmountSats) {
				msg = append(msg, fmt.Sprintf("%s: invalid node sale settlement amounts", deposit.DepositID))
				broken = true
			}
			if deposit.Settlement == types.DepositSettlementOperatorFee && !deposit.DepositAddress.IsNoop() {
				msg = append(msg, fmt.Sprintf("%s: fee settlement must use noop deposit address", deposit.DepositID))
				broken = true
			}
		}
		return msg, broken
	}
}

func ShielderVaultAddressInvariant(k KVStore) common.Invariant {
	return func(ctx cosmos.Context) (msg []string, broken bool) {
		iterator := k.GetDepositAddressIterator(ctx)
		defer iterator.Close()

		for ; iterator.Valid(); iterator.Next() {
			var record types.DepositAddress
			if err := json.Unmarshal(iterator.Value(), &record); err != nil {
				msg = append(msg, fmt.Sprintf("invalid deposit address encoding: %s", string(iterator.Key())))
				broken = true
				continue
			}
			if err := record.Valid(); err != nil {
				msg = append(msg, fmt.Sprintf("%s: invalid deposit address mapping: %v", record.Address, err))
				broken = true
				continue
			}
			if !record.Address.GetChain().Equals(common.BTCChain) {
				msg = append(msg, fmt.Sprintf("%s: deposit address is not BTC", record.Address))
				broken = true
			}
			nextIndex, err := k.GetNextVaultDepositPathIndex(ctx, record.VaultPubKey)
			if err != nil {
				msg = append(msg, fmt.Sprintf("%s: missing path index cursor: %v", record.Address, err))
				broken = true
			} else if record.PathIndex >= nextIndex {
				msg = append(msg, fmt.Sprintf("%s: path index %d not below cursor %d", record.Address, record.PathIndex, nextIndex))
				broken = true
			}
		}
		return msg, broken
	}
}

func NodeBondInvariant(k KVStore) common.Invariant {
	return func(ctx cosmos.Context) (msg []string, broken bool) {
		pool, err := k.GetFeePool(ctx)
		if err != nil {
			return []string{fmt.Sprintf("fee pool error: %v", err)}, true
		}
		var activeSlots uint64
		seenSlots := make(map[uint64]string)
		iterator := k.GetShielderNodeBondIterator(ctx)
		defer iterator.Close()

		for ; iterator.Valid(); iterator.Next() {
			var bond types.ShielderNodeBond
			if err := json.Unmarshal(iterator.Value(), &bond); err != nil {
				msg = append(msg, fmt.Sprintf("invalid shielder bond encoding: %s", string(iterator.Key())))
				broken = true
				continue
			}
			if err := bond.Valid(); err != nil {
				msg = append(msg, fmt.Sprintf("%s: invalid bond: %v", bond.NodePubKey, err))
				broken = true
			}
			if previous, ok := seenSlots[bond.Slot]; ok && !bond.Sold {
				msg = append(msg, fmt.Sprintf("%s: slot %d already owned by %s", bond.NodePubKey, bond.Slot, previous))
				broken = true
			}
			if !bond.Sold {
				seenSlots[bond.Slot] = bond.NodePubKey
			}
			if bond.Sold {
				if bond.SoldAuctionID == "" || bond.FeeShareActive || bond.BondSats != 0 || bond.PendingSats != 0 {
					msg = append(msg, fmt.Sprintf("%s: invalid sold bond state", bond.NodePubKey))
					broken = true
				}
				continue
			}
			if bond.FeeShareActive {
				activeSlots++
				if bond.FeeDebtSats > pool.FeePerSlotShare {
					msg = append(msg, fmt.Sprintf("%s: fee debt exceeds pool share", bond.NodePubKey))
					broken = true
				}
			}
			node, err := k.GetNodeAccount(ctx, bond.NodeAddress)
			if err != nil {
				msg = append(msg, fmt.Sprintf("%s: node account error: %v", bond.NodePubKey, err))
				broken = true
			} else if !node.IsEmpty() && node.Bond.Uint64() != bond.BondSats {
				msg = append(msg, fmt.Sprintf("%s: node bond %s != shielder bond %d", bond.NodePubKey, node.Bond, bond.BondSats))
				broken = true
			}
		}
		if activeSlots != pool.TotalSlots {
			msg = append(msg, fmt.Sprintf("active fee slots %d != fee pool total slots %d", activeSlots, pool.TotalSlots))
			broken = true
		}
		return msg, broken
	}
}

func NodeSlotAuctionInvariant(k KVStore) common.Invariant {
	return func(ctx cosmos.Context) (msg []string, broken bool) {
		iterator := k.GetNodeSlotAuctionIterator(ctx)
		defer iterator.Close()

		for ; iterator.Valid(); iterator.Next() {
			var auction types.NodeSlotAuction
			if err := json.Unmarshal(iterator.Value(), &auction); err != nil {
				msg = append(msg, fmt.Sprintf("invalid node slot auction encoding: %s", string(iterator.Key())))
				broken = true
				continue
			}
			if err := auction.Valid(); err != nil {
				msg = append(msg, fmt.Sprintf("%s: invalid auction: %v", auction.AuctionID, err))
				broken = true
			}
			switch auction.Status {
			case types.NodeSlotAuctionOpen, types.NodeSlotAuctionExpired:
				if auction.SelectedBidID != "" {
					msg = append(msg, fmt.Sprintf("%s: unselected auction has selected bid", auction.AuctionID))
					broken = true
				}
			case types.NodeSlotAuctionSelected, types.NodeSlotAuctionSettled:
				if auction.SelectedBidID == "" {
					msg = append(msg, fmt.Sprintf("%s: selected auction missing bid", auction.AuctionID))
					broken = true
					continue
				}
				bid, err := k.GetNodeSlotBid(ctx, auction.SelectedBidID)
				if err != nil || bid.BidID == "" || bid.AuctionID != auction.AuctionID {
					msg = append(msg, fmt.Sprintf("%s: selected bid not found", auction.AuctionID))
					broken = true
				} else if auction.Status == types.NodeSlotAuctionSettled && !bid.Settled {
					msg = append(msg, fmt.Sprintf("%s: settled auction has unsettled bid", auction.AuctionID))
					broken = true
				}
			default:
				msg = append(msg, fmt.Sprintf("%s: invalid auction status %s", auction.AuctionID, auction.Status))
				broken = true
			}
		}
		return msg, broken
	}
}

func FeeInvariant(k KVStore) common.Invariant {
	return func(ctx cosmos.Context) (msg []string, broken bool) {
		pool, err := k.GetFeePool(ctx)
		if err != nil {
			return []string{fmt.Sprintf("fee pool error: %v", err)}, true
		}
		if pool.TotalClaimedSats > pool.TotalCollectedSats {
			msg = append(msg, fmt.Sprintf("claimed fees %d exceed collected fees %d", pool.TotalClaimedSats, pool.TotalCollectedSats))
			broken = true
		}
		if pool.TotalSlots == 0 && pool.FeePerSlotShare != 0 {
			msg = append(msg, "fee pool has share accumulator without slots")
			broken = true
		}
		return msg, broken
	}
}
