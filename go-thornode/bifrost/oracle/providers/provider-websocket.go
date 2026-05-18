package providers

import (
	"context"
	"fmt"
	"time"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"
	"github.com/rs/zerolog"
	"gitlab.com/thorchain/thornode/v3/bifrost/oracle/metrics"
	"gitlab.com/thorchain/thornode/v3/bifrost/oracle/types"
	"gitlab.com/thorchain/thornode/v3/config"
)

const (
	minBackoff             = time.Second
	maxBackoff             = time.Minute * 5
	keepaliveCheckInterval = time.Second * 2
)

type websocketProvider struct {
	*baseProvider
	wsEndpoint      string
	ws              *websocket.Conn
	poll            func()
	handleWsMessage func([]byte) error
	preconnect      func() error
	ping            func() error
	getSubs         func() ([]map[string]any, error)
	backoff         time.Duration

	lastKeepalive     time.Time
	keepaliveInterval time.Duration
	keepaliveTimeout  time.Duration
}

func newWebsocketProvider(
	ctx context.Context,
	logger zerolog.Logger,
	name string,
	config config.BifrostOracleProviderConfiguration,
	metrics *metrics.Metrics,
) (*websocketProvider, error) {
	if len(config.WsEndpoints) == 0 {
		return nil, fmt.Errorf("no websocket endpoints provided")
	}

	var err error

	provider := websocketProvider{
		wsEndpoint: config.WsEndpoints[0],
		backoff:    minBackoff,
	}

	provider.baseProvider, err = newBaseProvider(ctx, logger, name, config, metrics)
	if err != nil {
		return nil, err
	}

	return &provider, nil
}

func (p *websocketProvider) init(provider types.WebsocketProvider) error {
	err := p.baseProvider.init(provider)
	if err != nil {
		p.logger.Err(err).Msg("Error initializing baseProvider")
		return err
	}

	p.poll = provider.Poll
	p.getSubs = provider.GetSubscriptionMsgs
	p.handleWsMessage = provider.HandleWsMessage
	p.preconnect = provider.PrepareConnection
	p.ping = provider.Ping
	return nil
}

func (p *websocketProvider) subscribe() error {
	msgs, err := p.getSubs()
	if err != nil {
		return err
	}

	for i, msg := range msgs {
		err := wsjson.Write(p.ctx, p.ws, msg)
		if err != nil {
			p.logger.Err(err).Msg("fail to subscribe")
			return err
		}

		if i > 0 {
			time.Sleep(time.Millisecond * 100)
		}
	}

	return nil
}

func (p *websocketProvider) connect() error {
	p.logger.Info().Msg("connecting")

	if p.ws != nil {
		_ = p.ws.Close(websocket.StatusNormalClosure, "")
		time.Sleep(time.Second)
	}

	err := p.preconnect()
	if err != nil {
		p.logger.Err(err).Msg("failed preparing websocket connection")
		return err
	}

	p.ws, _, err = websocket.Dial(p.ctx, p.wsEndpoint, nil)
	if err != nil {
		p.logger.Err(err).Msg("failed to connect")
		return err
	}

	err = p.subscribe()
	if err != nil {
		p.logger.Err(err).Msg("failed to subscribe")
		return err
	}

	p.lastKeepalive = time.Now()

	time.Sleep(time.Second)

	return nil
}

func (p *websocketProvider) reconnect() error {
	select {
	case <-p.ctx.Done():
		return fmt.Errorf("context canceled")
	case <-time.After(p.backoff):
		p.backoff = min(p.backoff*2, maxBackoff)
		return p.connect()
	}
}

func (p *websocketProvider) PrepareConnection() error {
	return nil
}

func (p *websocketProvider) Ping() error {
	return nil
}

func (p *websocketProvider) Start() {
	p.logger.Info().Msg("starting provider")
	p.wg.Add(1)

	p.poll()

	err := p.connect()
	if err != nil {
		p.logger.Err(err).Msg("failed to connect")
		return
	}

	go func() {
		defer p.ws.Close(websocket.StatusNormalClosure, "")

		var ping, check *time.Ticker
		if p.keepaliveInterval > 0 {
			ping = time.NewTicker(p.keepaliveInterval)
			defer ping.Stop()

			check = time.NewTicker(keepaliveCheckInterval)
			defer check.Stop()
		}

		for {
			select {
			case <-p.ctx.Done():
				return
			default:
			}

			if p.ws == nil {
				_ = p.reconnect()
				continue
			}

			var msg []byte
			_, msg, err = p.ws.Read(p.ctx)
			if err != nil {
				_ = p.reconnect()
				continue
			}

			if len(msg) == 0 {
				p.logger.Warn().Msg("message is empty")
				continue
			}

			err = p.handleWsMessage(msg)
			if err != nil {
				p.logger.Err(err).
					Str("msg", string(msg)).
					Msg("fail handling message")
			}

			if ping == nil {
				continue
			}

			select {
			case <-ping.C:
				err = p.ping()
				if err != nil {
					p.logger.Err(err).Msg("failed to send ping")
				}
			case <-check.C:
				if time.Since(p.lastKeepalive) > p.keepaliveTimeout {
					p.logger.Error().Msg("keepalive timeout reached")
					err = p.reconnect()
					if err != nil {
						p.logger.Err(err).Msg("failed to reconnect")
					}
				}
			default:
			}

			// reset backoff
			p.backoff = minBackoff
		}
	}()
}
