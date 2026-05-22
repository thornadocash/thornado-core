package constants

import (
	"fmt"

	"github.com/blang/semver"
)

// ConstantName the name we used to get constant values.
//
//go:generate stringer -type=ConstantName
type ConstantName int

const (
	BlocksPerYear ConstantName = iota
	MinimumNodesForBFT
	DesiredNodeSet
	ChurnInterval
	ChurnRetryInterval
	MissingBlockChurnOut
	MaxMissingBlockChurnOut
	MaxTrackMissingBlock
	BadNodeRedline
	LackOfObservationPenalty
	SigningTransactionPeriod
	DoubleSignMaxAge
	SlashPenalty
	PauseOnSlashThreshold
	FailKeygenSlashPoints
	FailKeysignSlashPoints
	ObserveSlashPoints
	DoubleBlockSignSlashPoints
	MissBlockSignSlashPoints
	ObservationDelayFlexibility
	JailTimeKeygen
	JailTimeKeysign
	NodePauseChainBlocks
	MinPenaltyPointsForBadNode
	KeygenRetryInterval
	OperationalVotesMin
)

// ConstantValues define methods used to get constant values
type ConstantValues interface {
	fmt.Stringer
	GetInt64Value(name ConstantName) int64
	GetBoolValue(name ConstantName) bool
	GetStringValue(name ConstantName) string
	GetConstantValsByKeyname() ConstantValsByKeyname
}

// GetConstantValues will return an  implementation of ConstantValues which provide ways to get constant values
// TODO hard fork remove unused version parameter
func GetConstantValues(_ semver.Version) ConstantValues {
	return NewConstantValue()
}
