package log

import (
	"strings"

	sdklog "cosmossdk.io/log"
	"github.com/rs/zerolog"
	"gitlab.com/thorchain/thornode/v3/config"
)

var _ sdklog.Logger = (*SdkLogWrapper)(nil)

// SdkLogWrapper provides a wrapper around a zerolog.Logger instance. It implements
// cosmos sdk's Logger interface.
type SdkLogWrapper struct {
	*zerolog.Logger
}

// Info implements cosmos sdk's Logger interface and logs with level INFO. A set
// of key/value tuples may be provided to add context to the log. The number of
// tuples must be even and the key of the tuple must be a string.
func (z SdkLogWrapper) Info(msg string, keyVals ...any) {
	if z.filter(msg) {
		return
	}
	z.Logger.Info().Fields(getLogFields(keyVals...)).Msg(msg)
}

// Warn implements cosmos sdk's Logger interface and logs with level WARN. A set
// of key/value tuples may be provided to add context to the log. The number of
// tuples must be even and the key of the tuple must be a string.
func (z SdkLogWrapper) Warn(msg string, keyVals ...any) {
	if z.filter(msg) {
		return
	}
	z.Logger.Error().Fields(getLogFields(keyVals...)).Msg(msg)
}

// Error implements cosmos sdk's Logger interface and logs with level ERR. A set
// of key/value tuples may be provided to add context to the log. The number of
// tuples must be even and the key of the tuple must be a string.
func (z SdkLogWrapper) Error(msg string, keyVals ...any) {
	if z.filter(msg) {
		return
	}
	z.Logger.Error().Fields(getLogFields(keyVals...)).Msg(msg)
}

// Debug implements cosmos sdk's Logger interface and logs with level DEBUG. A set
// of key/value tuples may be provided to add context to the log. The number of
// tuples must be even and the key of the tuple must be a string.
func (z SdkLogWrapper) Debug(msg string, keyVals ...any) {
	if z.filter(msg) {
		return
	}
	z.Logger.Debug().Fields(getLogFields(keyVals...)).Msg(msg)
}

// With returns a new wrapped logger with additional context provided by a set
// of key/value tuples. The number of tuples must be even and the key of the
// tuple must be a string.
func (z SdkLogWrapper) With(keyVals ...interface{}) sdklog.Logger {
	// skip filtering modules if unexpected keyVals or debug level
	if len(keyVals)%2 != 0 || z.Logger.GetLevel() <= zerolog.DebugLevel {
		logger := z.Logger.With().Fields(getLogFields(keyVals...)).Logger()
		return SdkLogWrapper{
			Logger: &logger,
		}
	}

	for i := 0; i < len(keyVals); i += 2 {
		name, ok := keyVals[i].(string)
		if !ok {
			z.Logger.Error().Interface("key", keyVals[i]).Msg("non-string logging key provided")
		}
		if name != "module" {
			continue
		}
		value, ok := keyVals[i+1].(string)
		if !ok {
			continue
		}
		for _, item := range config.GetThornode().LogFilter.Modules {
			if strings.EqualFold(item, value) {
				logger := z.Logger.Level(zerolog.WarnLevel).With().Fields(getLogFields(keyVals...)).Logger()
				return SdkLogWrapper{
					Logger: &logger,
				}
			}
		}
	}

	logger := z.Logger.With().Fields(getLogFields(keyVals...)).Logger()
	return SdkLogWrapper{
		Logger: &logger,
	}
}

// Impl returns the underlying logger implementation.
// It is used to access the full functionalities of the underlying logger.
// Advanced users can type cast the returned value to the actual logger.
func (z SdkLogWrapper) Impl() any {
	return z.Logger
}

func getLogFields(keyVals ...any) map[string]any {
	if len(keyVals)%2 != 0 {
		return nil
	}

	fields := make(map[string]any)
	for i := 0; i < len(keyVals); i += 2 {
		val, ok := keyVals[i].(string)
		if ok {
			fields[val] = keyVals[i+1]
		}
	}

	return fields
}

func (z SdkLogWrapper) filter(msg string) bool {
	if z.Logger.GetLevel() > zerolog.DebugLevel {
		for _, filter := range config.GetThornode().LogFilter.Messages {
			if filter == msg {
				return true
			}
		}
	}
	return false
}
