package types

type Provider interface {
	Start()
	GetTickers() (map[string]Ticker, error)

	GetAvailablePairs() ([]string, error)
	ToProviderSymbol(string, string) string
}

type PollingProvider interface {
	Provider
	Poll()
}

type WebsocketProvider interface {
	Provider
	Poll()
	HandleWsMessage([]byte) error
	GetSubscriptionMsgs() ([]map[string]any, error)
	PrepareConnection() error
	Ping() error
}
