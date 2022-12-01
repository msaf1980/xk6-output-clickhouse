package timescaledb

import (
	"database/sql"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/sirupsen/logrus"

	"go.k6.io/k6/metrics"
	"go.k6.io/k6/output"
)

func init() {
	output.RegisterExtension("clickhouse", newOutput)
}

var (
	_       interface{ output.WithThresholds } = &Output{}
	timeNow                                    = time.Now
)

type Output struct {
	output.SampleBuffer
	periodicFlusher *output.PeriodicFlusher
	Conn            *sql.DB
	Config          config
	id              time.Time

	thresholds map[string][]*dbThreshold

	logger logrus.FieldLogger
}

func (o *Output) Description() string {
	return "Clickhouse"
}

func newOutput(params output.Params) (output.Output, error) {
	config, err := getConsolidatedConfig(params.JSONConfig, params.Environment)
	if err != nil {
		return nil, fmt.Errorf("problem parsing config: %w", err)
	}

	conn, err := sql.Open("clickhouse", config.URL)
	if err != nil {
		return nil, fmt.Errorf("clickhouse: unable to create connection: %w", err)
	}

	o := Output{
		Conn:   conn,
		Config: config,
		logger: params.Logger.WithFields(logrus.Fields{
			"output": "Clickhouse",
		}),
	}

	return &o, nil
}

func (o *Output) SetThresholds(thresholds map[string]metrics.Thresholds) {
	ths := make(map[string][]*dbThreshold)
	for metric, fullTh := range thresholds {
		for _, t := range fullTh.Thresholds {
			ths[metric] = append(ths[metric], &dbThreshold{
				id:        -1,
				threshold: t,
			})
		}
	}

	o.thresholds = ths
}

type dbThreshold struct {
	id        int
	threshold *metrics.Threshold
}

var schema = []string{
	`CREATE TABLE IF NOT EXISTS k6_samples (
        id DateTime64(9),
        ts DateTime64(9),
        metric String,
        name String,
        tags Map(String, String),
        value Float64,
        version DateTime64(9)
    ) ENGINE = ReplacingMergeTree(version)
    PARTITION BY toYYYYMM(id)
    ORDER BY (id, ts, metric, name);`,
	`CREATE TABLE IF NOT EXISTS k6_tests (
        id DateTime64(9),
        name String
	) ENGINE = ReplacingMergeTree(id)
    PARTITION BY toYYYYMM(id)
    ORDER BY (id);`,
}

func (o *Output) Start() error {
	sql := "CREATE DATABASE IF NOT EXISTS " + o.Config.dbName
	_, err := o.Conn.Exec(sql)
	if err != nil {
		o.logger.WithError(err).WithField("sql", sql).Debug("Start: Couldn't create database; most likely harmless")
	}

	for _, s := range schema {
		_, err = o.Conn.Exec(s)
		if err != nil {
			o.logger.WithError(err).WithField("sql", s).Debug("Start: Couldn't create database schema; most likely harmless")
			return err
		}
	}
	o.id = timeNow()
	name := os.Getenv("K6_TESTNAME")
	if name == "" {
		name = o.id.Format(time.RFC3339Nano)
	}
	if _, err = o.Conn.Exec("INSERT INTO k6_tests (id, name) VALUES (?, ?)", o.id, name); err != nil {
		o.logger.WithError(err).Debug("Start: Failed to insert test")
		return err
	}

	pf, err := output.NewPeriodicFlusher(time.Duration(o.Config.PushInterval), o.flushMetrics)
	if err != nil {
		return err
	}

	o.logger.Debug("Start: Running!")
	o.periodicFlusher = pf

	return nil
}

func tagsName(tags map[string]string) string {
	tagsSlice := make([]string, 0, len(tags))
	for k, v := range tags {
		tagsSlice = append(tagsSlice, k+"="+v)
	}
	sort.Strings(tagsSlice)
	return strings.Join(tagsSlice, " ")
}

func (o *Output) flushMetrics() {
	samplesContainer := o.GetBufferedSamples()
	if len(samplesContainer) == 0 {
		return
	}
	start := timeNow()

	scope, err := o.Conn.Begin()
	if err != nil {
		o.logger.Error(err)
		return
	}
	batch, err := scope.Prepare(`INSERT INTO k6_samples (id, ts, metric, name, tags, value, version)`)
	if err != nil {
		o.logger.Error(err)
		return
	}

	for _, sc := range samplesContainer {
		samples := sc.GetSamples()
		for _, s := range samples {
			tags := s.Tags.Map()
			name := tagsName(tags)
			if _, err = batch.Exec(start, s.Time, s.Metric.Name, name, tags, s.Value, o.id); err != nil {
				o.logger.Error(err)
				scope.Rollback()
				return
			}
		}
	}

	err = scope.Commit()
	if err != nil {
		o.logger.Error(err)
		return
	}

	t := time.Since(start)
	o.logger.WithField("time_since_start", t).Debug("flushMetrics: Samples committed!")
}

func (o *Output) Stop() error {
	o.logger.Debug("Stopping...")
	defer o.logger.Debug("Stopped!")
	o.periodicFlusher.Stop()
	o.Conn.Close()
	return nil
}
