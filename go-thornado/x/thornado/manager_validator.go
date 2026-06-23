package thornado

// findCountToRemove - find the number of node accounts to remove
func findCountToRemove(active NodeAccounts) (toRemove int) {
	// count number of node accounts that are a candidate to leaving
	var candidateCount int
	for _, na := range active {
		if na.LeaveScore > 0 {
			candidateCount++
			continue
		}
	}

	maxRemove := findMaxAbleToLeave(len(active))
	if len(active) > 0 {
		if maxRemove == 0 {
			// we can't remove any mathematically, but we always leave room for
			// node accounts requesting to leave or are being banned
			if active[0].ForcedToLeave || active[0].RequestedToLeave {
				toRemove = 1
			}
		} else {
			if candidateCount > maxRemove {
				toRemove = maxRemove
			} else {
				toRemove = candidateCount
			}
		}
	}
	return
}

func findCountToRemoveWithReplacements(active NodeAccounts, replacementCount, targetCount int, minimumNodesForBFT int64) int {
	toRemove := findCountToRemove(active)
	if toRemove > 0 {
		return toRemove
	}
	if replacementCount <= 0 || !hasLeaveCandidate(active) || len(active) < targetCount {
		return 0
	}
	if len(active)-1+replacementCount < int(minimumNodesForBFT) {
		return 0
	}
	return 1
}

func hasLeaveCandidate(active NodeAccounts) bool {
	for _, na := range active {
		if na.LeaveScore > 0 || na.ForcedToLeave || na.RequestedToLeave {
			return true
		}
	}
	return false
}

// findMaxAbleToLeave - given number of current active node account, figure out
// the max number of individuals we can allow to leave in a single churn event
func findMaxAbleToLeave(count int) int {
	majority := (count * 2 / 3) + 1 // add an extra 1 to "round up" for security
	max := count - majority

	// we don't want to loose BFT by accident (only when someone leaves)
	if count-max < 4 {
		max = count - 4
		if max < 0 {
			max = 0
		}
	}

	return max
}
