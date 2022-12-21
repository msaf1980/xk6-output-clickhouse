//go:build test_all || test_integration
// +build test_all test_integration

package tests

import (
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/ClickHouse/clickhouse-go/v2"
	"github.com/stretchr/testify/require"
	"go.k6.io/k6/lib/testutils"
	"go.k6.io/k6/metrics"
	"go.k6.io/k6/output"

	out "github.com/msaf1980/xk6-output-clickhouse"
)

type sample struct {
	id    uint64
	start string
	ts    string
	label string
	url   string
	name  string
	tags  map[string]string
	value float64
}

func max(a, b int) int {
	if a >= b {
		return a
	}
	return b
}

func diffSamples(expected, actual []sample) string {
	maxLen := max(len(expected), len(actual))
	var sb strings.Builder
	sb.Grow(1024)
	for i := 0; i < maxLen; i++ {
		if i > len(expected) {
			sb.WriteString(fmt.Sprintf("+ [%d] = %+v\n", i, actual[i]))
		} else if i > len(actual) {
			sb.WriteString(fmt.Sprintf("- [%d] = %+v\n", i, expected[i]))
		} else if !reflect.DeepEqual(actual[i], expected[i]) {
			sb.WriteString(fmt.Sprintf("- [%d] = %+v\n", i, expected[i]))
			sb.WriteString(fmt.Sprintf("+ [%d] = %+v\n", i, actual[i]))
		}
	}
	return sb.String()
}

func TestOutputFlushMetrics(t *testing.T) {
	var (
		id     uint64
		start  time.Time
		nTests int
	)

	c, err := out.New(output.Params{
		Logger: testutils.NewLogger(t),
		Environment: map[string]string{
			"K6_OUT_CLICKHOUSE_TABLE_TESTS":   "t_k6_tests",
			"K6_OUT_CLICKHOUSE_TABLE_SAMPLES": "t_k6_samples",
			"K6_OUT_CLICKHOUSE_TESTNAME":      "carbonapi",
			"K6_OUT_CLICKHOUSE_PARAMS":        "USERS_1H_0=1 FIND=1",
			"K6_OUT_CLICKHOUSE_PUSH_INTERVAL": "1s",
		},
	})
	out_ch := c.(*out.Output)
	require.NoError(t, err)
	require.Equal(t, "t_k6_tests", out_ch.Config.TableTests())
	require.Equal(t, "t_k6_samples", out_ch.Config.TableSamples())
	for _, s := range []string{
		"DROP TABLE IF EXISTS t_k6_tests",
		"DROP TABLE IF EXISTS t_k6_samples",
	} {
		_, err = out_ch.Conn.Exec(s)
		require.NoError(t, err)
	}

	require.NoError(t, c.Start())

	defer func() {
		c.Stop()
	}()

	registry := metrics.NewRegistry()

	samples := make(metrics.Samples, 0, 11)
	samplesIn := make([]sample, 0, len(samples))

	metric, err := registry.NewMetric("test_gauge", metrics.Gauge)
	require.NoError(t, err)
	tagsInFind := map[string]string{
		"url":    "metrics/find",
		"label":  "find",
		"status": "404",
		"VU":     "20",
	}
	samples = append(samples, metrics.Sample{
		TimeSeries: metrics.TimeSeries{
			Metric: metric,
			Tags:   registry.RootTagSet().WithTagsFromMap(tagsInFind),
		},
		Time:  out_ch.Config.StartTime(),
		Value: float64(0),
	})
	samplesIn = append(samplesIn, sample{
		id:    out_ch.Config.Id(),
		start: out_ch.Config.StartTime().Format(time.RFC3339Nano),
		ts:    out_ch.Config.StartTime().Format(time.RFC3339Nano),
		label: "find",
		url:   "metrics/find",
		name:  out.TagsName(tagsInFind),
		tags:  tagsInFind,
		value: float64(0),
	})

	tagsIn := map[string]string{
		"url":    "render",
		"label":  "1h",
		"status": "200",
		"VU":     "21",
	}
	nameIn := out.TagsName(tagsIn)
	for i := 0; i < 10; i++ {
		metric, err := registry.NewMetric("test_gauge", metrics.Gauge)
		require.NoError(t, err)
		ts := out_ch.Config.StartTime().Add(time.Duration(i) * time.Second)
		v := float64(i)
		samples = append(samples, metrics.Sample{
			TimeSeries: metrics.TimeSeries{
				Metric: metric,
				Tags:   registry.RootTagSet().WithTagsFromMap(tagsIn),
			},
			Time:  ts,
			Value: v,
		})
		samplesIn = append(samplesIn, sample{
			id:    out_ch.Config.Id(),
			start: out_ch.Config.StartTime().Format(time.RFC3339Nano),
			ts:    ts.Format(time.RFC3339Nano),
			label: "1h",
			url:   "render",
			name:  nameIn,
			tags:  tagsIn,
			value: v,
		})
	}

	c.AddMetricSamples([]metrics.SampleContainer{samples})
	c.AddMetricSamples([]metrics.SampleContainer{samples})

	query := "SELECT id, ts, name, params FROM t_k6_tests ORDER BY id, ts, name"
	rows, err := out_ch.Conn.Query(query)
	require.NoError(t, err)
	defer rows.Close()
	for rows.Next() {
		var (
			name, params string
		)
		err = rows.Scan(&id, &start, &name, &params)
		require.NoError(t, err)
		nTests++
	}
	// get any error encountered during iteration
	require.NoError(t, rows.Err())
	require.Equal(t, 1, nTests, "tests count")
	require.Equal(t, out_ch.Config.StartTime(), start, "test start")
	rows.Close()

	time.Sleep(2 * time.Second)

	query = "SELECT id, start, ts, label, url, name, tags, value FROM t_k6_samples WHERE id = @Id AND start = @Time ORDER BY start, ts, url"
	rows, err = out_ch.Conn.Query(query, clickhouse.Named("Id", id), clickhouse.DateNamed("Time", start, 3))
	require.NoError(t, err)
	samplesOut := make([]sample, 0, len(samples))
	for rows.Next() {
		var (
			s      sample
			ts, st time.Time
		)
		err = rows.Scan(&s.id, &st, &ts, &s.label, &s.url, &s.name, &s.tags, &s.value)
		require.NoError(t, err)
		s.start = st.Format(time.RFC3339Nano)
		s.ts = ts.Format(time.RFC3339Nano)
		samplesOut = append(samplesOut, s)
	}

	// get any error encountered during iteration
	require.NoError(t, rows.Err())
	if diff := diffSamples(samplesIn, samplesOut); diff != "" {
		t.Errorf("samples differs:\n%s", diff)
	}
}
