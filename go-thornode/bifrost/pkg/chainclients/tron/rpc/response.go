package rpc

type Response struct {
	Error  Error  `json:"error"`
	Result string `json:"result"`
}

type Error struct {
	Code    int64  `json:"code"`
	Message string `json:"message"`
	Data    string `json:"data"`
}
