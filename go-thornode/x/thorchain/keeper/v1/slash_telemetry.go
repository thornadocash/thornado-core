package keeperv1

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"runtime"
	"strings"

	"github.com/hashicorp/go-metrics"
	"gitlab.com/thorchain/thornode/v3/common/cosmos"
	"gitlab.com/thorchain/thornode/v3/config"
	"gitlab.com/thorchain/thornode/v3/constants"
)

var (
	stackBuf          []byte
	slashTelemetryEnc *json.Encoder
)

func init() {
	config.Init()
	if config.GetThornode().Telemetry.SlashPoints {
		stackBuf = make([]byte, 4096)
		path := os.ExpandEnv("${HOME}/.thornode/slash_telemetry.json")
		file, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
		if err != nil {
			panic(err)
		}
		slashTelemetryEnc = json.NewEncoder(file)
	}
}

func getSlashCaller(fn string) string {
	written := runtime.Stack(stackBuf, false)
	stack := stackBuf[:written]
	lines := strings.Split(string(stack), "\n")
	for i, line := range lines {
		if strings.Contains(line, fn) && len(lines) > i+3 {
			return strings.Fields(strings.TrimSpace(lines[i+3]))[0]
		}
	}
	return ""
}

func slashTelemetry(ctx cosmos.Context, pts int64, addr cosmos.AccAddress, slashFnName string) {
	rawHash := sha256.Sum256(ctx.TxBytes())
	hash := hex.EncodeToString(rawHash[:])
	slash := map[string]any{
		"height":  ctx.BlockHeight(),
		"txid":    hash,
		"points":  pts,
		"address": addr.String(),
		"caller":  getSlashCaller(slashFnName),
	}
	metricLabels, _ := ctx.Context().Value(constants.CtxMetricLabels).([]metrics.Label)
	for _, label := range metricLabels {
		slash[label.Name] = label.Value
	}
	if ctx.Context().Value(constants.CtxObservedTx) != nil {
		slash["observed_txid"], _ = ctx.Context().Value(constants.CtxObservedTx).(string)
	}
	_ = slashTelemetryEnc.Encode(slash)
}
