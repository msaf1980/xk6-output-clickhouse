package clickhouse

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_getConsolidatedConfig_Succeeds(t *testing.T) {
	timeNow = func() time.Time {
		return time.Unix(1669909784, 10)
	}
	actualConfig, err := getConsolidatedConfig(
		[]byte(`{"url":"http://127.0.0.1:8124/k6?dial_timeout=200ms&max_execution_time=60","pushInterval":"3s"}`),
		map[string]string{
			"K6_OUT_CLICKHOUSE_PUSH_INTERVAL": "2s",
			"K6_OUT_CLICKHOUSE_TESTNAME":      "test",
			"K6_OUT_CLICKHOUSE_PARAMS":        "USERS_1H_0=10 USERS_7D_0=1",
		})
	require.NoError(t, err)
	assert.Equal(t, config{
		URL:          "http://127.0.0.1:8124/k6?dial_timeout=200ms&max_execution_time=60",
		PushInterval: Duration(2 * time.Second),
		Name:         "test 2022-12-01T15:49:44.00000001Z",
		id:           uint64(time.Unix(1669909784, 10).UnixNano()),
		ts:           time.Unix(1669909784, 10).UTC(),
		dbName:       "k6",
		params:       "USERS_1H_0=10 USERS_7D_0=1",
		tableTests:   "k6_tests",
		tableSamples: "k6_samples",
	}, actualConfig)
}

func Test_getConsolidatedConfig_FromJsonAndPopulatesConfigFieldsFromJsonUrl(t *testing.T) {
	timeNow = func() time.Time {
		return time.Unix(1669909784, 10)
	}
	actualConfig, err := getConsolidatedConfig(
		[]byte(`{"url":"http://127.0.0.1:8124/default?dial_timeout=200ms&max_execution_time=60"}`),
		nil)
	assert.NoError(t, err)
	assert.Equal(t, config{
		URL:          "http://127.0.0.1:8124/default?dial_timeout=200ms&max_execution_time=60",
		PushInterval: Duration(10 * time.Second),
		Name:         "2022-12-01T15:49:44.00000001Z",
		id:           uint64(time.Unix(1669909784, 10).UnixNano()),
		ts:           time.Unix(1669909784, 10).UTC(),
		dbName:       "default",
		tableTests:   "k6_tests",
		tableSamples: "k6_samples",
	}, actualConfig)
}

func Test_getConsolidatedConfig_FromEnvVariables(t *testing.T) {
	timeNow = func() time.Time {
		return time.Unix(1669909784, 10)
	}
	actualConfig, err := getConsolidatedConfig(
		nil,
		map[string]string{
			"K6_OUT_CLICKHOUSE_PUSH_INTERVAL": "2s",
		})

	assert.NoError(t, err)
	assert.Equal(t, config{
		URL:          "http://localhost:8123/default?dial_timeout=1s&max_execution_time=60",
		PushInterval: Duration(2 * time.Second),
		Name:         "2022-12-01T15:49:44.00000001Z",
		id:           uint64(time.Unix(1669909784, 10).UnixNano()),
		ts:           time.Unix(1669909784, 10).UTC(),
		dbName:       "default",
		tableTests:   "k6_tests",
		tableSamples: "k6_samples",
	}, actualConfig)
}

func Test_getConsolidatedConfig_EnvVariableTakesPrecedenceWithoutConfigArg(t *testing.T) {
	timeNow = func() time.Time {
		return time.Unix(1669909784, 1)
	}
	actualConfig, err := getConsolidatedConfig(
		[]byte(`{"url":"http://user:password@127.0.0.1:8124/default?dial_timeout=200ms&max_execution_time=60","pushInterval":"3s"}`),
		map[string]string{
			"K6_OUT_CLICKHOUSE_PUSH_INTERVAL": "2s",
		})

	assert.NoError(t, err)
	assert.Equal(t, config{
		URL:          "http://user:password@127.0.0.1:8124/default?dial_timeout=200ms&max_execution_time=60",
		PushInterval: Duration(2 * time.Second),
		Name:         "2022-12-01T15:49:44.000000001Z",
		id:           uint64(time.Unix(1669909784, 1).UnixNano()),
		ts:           time.Unix(1669909784, 1).UTC(),
		dbName:       "default",
		tableTests:   "k6_tests",
		tableSamples: "k6_samples",
	}, actualConfig)
}
