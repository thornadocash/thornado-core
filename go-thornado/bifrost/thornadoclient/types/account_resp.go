package types

// AccountResp the response from thornadoclient
type AccountResp struct {
	Account struct {
		Address       string `json:"address"`
		AccountNumber uint64 `json:"account_number,string"`
		Sequence      uint64 `json:"sequence,string"`
	} `json:"account"`
}
