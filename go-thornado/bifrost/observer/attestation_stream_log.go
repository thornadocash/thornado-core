package observer

import (
	"strings"

	"github.com/libp2p/go-libp2p-core/peer"
	"github.com/rs/zerolog"
)

func logAttestationStreamOpenError(logger zerolog.Logger, err error, peerID peer.ID, protocol string) {
	if strings.Contains(err.Error(), "protocol not supported") {
		logger.Debug().
			Err(err).
			Str("peer", peerID.String()).
			Str("protocol", protocol).
			Msg("peer attestation protocol not ready")
		return
	}
	logger.Error().
		Err(err).
		Str("peer", peerID.String()).
		Str("protocol", protocol).
		Msg("fail to create attestation stream")
}
