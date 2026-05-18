package rpc

import (
	"encoding/json"
	"strconv"
)

////////////////////////////////////////////////////////////////////////////////////////
// rpc_types contains the structs for the responses from the Solana RPC
////////////////////////////////////////////////////////////////////////////////////////

type RPCMeta struct {
	ComputeUnitsConsumed uint64        `json:"computeUnitsConsumed"`
	Err                  interface{}   `json:"err"`
	Fee                  uint64        `json:"fee"`
	InnerInstructions    []interface{} `json:"innerInstructions"`
	LoadedAddresses      RPCAddrList   `json:"loadedAddresses"`
	LogMessages          []string      `json:"logMessages"`
	PostBalances         []uint64      `json:"postBalances"`
	PostTokenBalances    []interface{} `json:"postTokenBalances"`
	PreBalances          []uint64      `json:"preBalances"`
	PreTokenBalances     []interface{} `json:"preTokenBalances"`
	Rewards              interface{}   `json:"rewards"`
	Status               RPCTxnStatus  `json:"status"`
}

type RPCTxnData struct {
	Message    RPCMessage `json:"message"`
	Signatures []string   `json:"signatures"`
}

type RPCAddrList struct {
	Readonly []interface{} `json:"readonly"`
	Writable []interface{} `json:"writable"`
}

type RPCTxnStatus struct {
	Ok interface{} `json:"Ok"`
}

type RPCMessage struct {
	AccountKeys         []string                 `json:"accountKeys"`
	AddressLookupTables []RPCAddressLookupTables `json:"addressTableLookups"`
	Header              RPCHeader                `json:"header"`
	Instructions        []RPCInstruction         `json:"instructions"`
	RecentBlockhash     string                   `json:"recentBlockhash"`
}

type RPCAddressLookupTables struct {
	AccountKey      string `json:"accountKey"`
	WritableIndexes []int  `json:"writableIndexes"`
	ReadonlyIndexes []int  `json:"readonlyIndexes"`
}

type RPCHeader struct {
	NumReadonlySignedAccounts   int `json:"numReadonlySignedAccounts"`
	NumReadonlyUnsignedAccounts int `json:"numReadonlyUnsignedAccounts"`
	NumRequiredSignatures       int `json:"numRequiredSignatures"`
}

type RPCInstruction struct {
	Accounts       []int       `json:"accounts"`
	Data           string      `json:"data"`
	ProgramIdIndex int         `json:"programIdIndex"`
	StackHeight    interface{} `json:"stackHeight"`
}

type RPCError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// Custom type for Version to handle both string and numeric values
type RPCVersion struct {
	Value string
}

func (v *RPCVersion) UnmarshalJSON(b []byte) error {
	if b[0] == '"' {
		return json.Unmarshal(b, &v.Value)
	}
	var num int
	if err := json.Unmarshal(b, &num); err != nil {
		return err
	}
	v.Value = strconv.Itoa(num)
	return nil
}

////////////////////////////////////////////////////////////////////////////////////////
// Helpers
////////////////////////////////////////////////////////////////////////////////////////

func (r *RPCTxnData) GetInstructions() []RPCInstruction {
	return r.Message.Instructions
}

func (r *RPCTxnData) GetAccountAtIndex(i int) string {
	if i >= len(r.Message.AccountKeys) || len(r.Message.AccountKeys) == 0 {
		return ""
	}
	return r.Message.AccountKeys[i]
}
