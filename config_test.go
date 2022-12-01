package timescaledb

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
			"K6_CLICKHOUSE_PUSH_INTERVAL": "2s",
			"K6_CLICKHOUSE_NAME":          "test",
		})
	require.NoError(t, err)
	assert.Equal(t, config{
		URL:          "http://127.0.0.1:8124/k6?dial_timeout=200ms&max_execution_time=60",
		PushInterval: Duration(2 * time.Second),
		Name:         "test",
		dbName:       "k6",
		id:           time.Unix(1669909784, 10),
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
		Name:         "2022-12-01T15:49:44.000000010Z",
		id:           time.Unix(1669909784, 10),
		dbName:       "default",
	}, actualConfig)
}

func Test_getConsolidatedConfig_FromEnvVariables(t *testing.T) {
	timeNow = func() time.Time {
		return time.Unix(1669909784, 10)
	}
	actualConfig, err := getConsolidatedConfig(
		nil,
		map[string]string{
			"K6_CLICKHOUSE_PUSH_INTERVAL": "2s",
		})

	assert.NoError(t, err)
	assert.Equal(t, config{
		URL:          "http://localhost:8123/default?dial_timeout=200ms&max_execution_time=60",
		PushInterval: Duration(2 * time.Second),
		Name:         "2022-12-01T15:49:44.000000010Z",
		id:           time.Unix(1669909784, 10),
		dbName:       "default",
	}, actualConfig)
}

func Test_getConsolidatedConfig_EnvVariableTakesPrecedenceWithoutConfigArg(t *testing.T) {
	timeNow = func() time.Time {
		return time.Unix(1669909784, 10)
	}
	actualConfig, err := getConsolidatedConfig(
		[]byte(`{"url":"http://user:password@127.0.0.1:8124/default?dial_timeout=200ms&max_execution_time=60","pushInterval":"3s"}`),
		map[string]string{
			"K6_CLICKHOUSE_PUSH_INTERVAL": "2s",
		})

	assert.NoError(t, err)
	assert.Equal(t, config{
		URL:          "http://user:password@127.0.0.1:8124/default?dial_timeout=200ms&max_execution_time=60",
		PushInterval: Duration(2 * time.Second),
		Name:         "2022-12-01T15:49:44.000000010Z",
		id:           time.Unix(1669909784, 10),
		dbName:       "default",
	}, actualConfig)
}
