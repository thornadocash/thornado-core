package monitor

import (
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
	"github.com/stretchr/testify/assert"
)

func getGaugeVecValue(metric *prometheus.GaugeVec, label string) (float64, error) {
	m := &dto.Metric{}
	if err := metric.WithLabelValues(label).Write(m); err != nil {
		return 0, err
	}
	return m.Gauge.GetValue(), nil
}

func getCounterVec2Value(metric *prometheus.CounterVec, labels ...string) (float64, error) {
	m := &dto.Metric{}
	if err := metric.WithLabelValues(labels...).Write(m); err != nil {
		return 0, err
	}
	return m.Counter.GetValue(), nil
}

func getCounterValue(metric *prometheus.CounterVec, flag string) (float64, error) {
	m := &dto.Metric{}
	if err := metric.WithLabelValues(flag).Write(m); err != nil {
		return 0, err
	}
	return m.Counter.GetValue(), nil
}

func TestMetric_UpdateKeySign(t *testing.T) {
	metrics := NewMetric()
	testTime := time.Second
	metrics.UpdateKeySign(testTime, true)
	metrics.UpdateKeySign(testTime, true)
	metrics.UpdateKeySign(testTime, true)

	metrics.UpdateKeySign(testTime, false)
	metrics.UpdateKeySign(testTime, false)
	metrics.UpdateKeySign(testTime, false)
	metrics.UpdateKeySign(testTime, false)
	metrics.UpdateKeySign(testTime, false)

	val, err := getCounterValue(metrics.keysignCounter, "success")
	assert.Nil(t, err)
	assert.Equal(t, float64(3), val)
	val, err = getCounterValue(metrics.keysignCounter, "failure")
	assert.Nil(t, err)
	assert.Equal(t, float64(5), val)

	m := &dto.Metric{}
	err = metrics.keySignTime.Write(m)
	assert.Nil(t, err)
	val = m.Gauge.GetValue()
	assert.Equal(t, float64(time.Second), val)
}

func TestMetric_UpdateKeyGen(t *testing.T) {
	metrics := NewMetric()
	testTime := time.Second
	metrics.UpdateKeyGen(testTime, true)
	metrics.UpdateKeyGen(testTime, true)
	metrics.UpdateKeyGen(testTime, true)

	metrics.UpdateKeyGen(testTime, false)
	metrics.UpdateKeyGen(testTime, false)
	metrics.UpdateKeyGen(testTime, false)
	metrics.UpdateKeyGen(testTime, false)
	metrics.UpdateKeyGen(testTime, false)

	val, err := getCounterValue(metrics.keygenCounter, "success")
	assert.Nil(t, err)
	assert.Equal(t, float64(3), val)
	val, err = getCounterValue(metrics.keygenCounter, "failure")
	assert.Nil(t, err)
	assert.Equal(t, float64(5), val)
}

func TestMetric_KeygenJoinParty(t *testing.T) {
	metrics := NewMetric()
	testTime := 2 * time.Second

	// Test success path
	metrics.KeygenJoinParty(testTime, true)
	metrics.KeygenJoinParty(testTime, true)

	// Test failure path
	metrics.KeygenJoinParty(testTime, false)
	metrics.KeygenJoinParty(testTime, false)
	metrics.KeygenJoinParty(testTime, false)

	val, err := getCounterVec2Value(metrics.joinPartyCounter, "keygen", "success")
	assert.Nil(t, err)
	assert.Equal(t, float64(2), val)

	val, err = getCounterVec2Value(metrics.joinPartyCounter, "keygen", "failure")
	assert.Nil(t, err)
	assert.Equal(t, float64(3), val)

	val, err = getGaugeVecValue(metrics.joinPartyTime, "keygen")
	assert.Nil(t, err)
	assert.Equal(t, float64(2*time.Second), val)
}

func TestMetric_KeysignJoinParty(t *testing.T) {
	metrics := NewMetric()
	testTime := 3 * time.Second

	// Test success path
	metrics.KeysignJoinParty(testTime, true)

	// Test failure path
	metrics.KeysignJoinParty(testTime, false)
	metrics.KeysignJoinParty(testTime, false)

	val, err := getCounterVec2Value(metrics.joinPartyCounter, "keysign", "success")
	assert.Nil(t, err)
	assert.Equal(t, float64(1), val)

	val, err = getCounterVec2Value(metrics.joinPartyCounter, "keysign", "failure")
	assert.Nil(t, err)
	assert.Equal(t, float64(2), val)

	val, err = getGaugeVecValue(metrics.joinPartyTime, "keysign")
	assert.Nil(t, err)
	assert.Equal(t, float64(3*time.Second), val)
}

func TestMetric_Enable(t *testing.T) {
	// Swap default registry with a fresh one to avoid conflicts with other tests
	origReg := prometheus.DefaultRegisterer
	newReg := prometheus.NewRegistry()
	prometheus.DefaultRegisterer = newReg
	defer func() {
		prometheus.DefaultRegisterer = origReg
	}()

	metrics := NewMetric()
	// Enable registers all metrics with the default registerer
	metrics.Enable()

	// Add observations so gatherer returns metric families
	metrics.UpdateKeyGen(time.Second, true)
	metrics.UpdateKeySign(time.Second, true)
	metrics.KeygenJoinParty(time.Second, true)
	metrics.KeysignJoinParty(time.Second, true)

	mfs, err := newReg.Gather()
	assert.NoError(t, err)
	assert.Equal(t, 6, len(mfs))
}

func TestNewMetric(t *testing.T) {
	metrics := NewMetric()
	assert.NotNil(t, metrics)
	assert.NotNil(t, metrics.keygenCounter)
	assert.NotNil(t, metrics.keysignCounter)
	assert.NotNil(t, metrics.joinPartyCounter)
	assert.NotNil(t, metrics.keyGenTime)
	assert.NotNil(t, metrics.keySignTime)
	assert.NotNil(t, metrics.joinPartyTime)
}
