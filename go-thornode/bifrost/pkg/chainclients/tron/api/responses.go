package api

type BroadcastResponse struct {
	Result  bool   `json:"result"`
	TxId    string `json:"txid"`
	Code    string `json:"code,omitempty"`
	Message string `json:"message,omitempty"`
}

type EstimateEnergyResponse struct {
	Result struct {
		Result bool `json:"result"`
	} `json:"result"`
	Energy int64 `json:"energy_required"`
}

type ChainParametersResponse struct {
	Parameters []struct {
		Key   string `json:"key"`
		Value int64  `json:"value"`
	} `json:"chainParameter"`
}
