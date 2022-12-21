package clickhouse

import (
	"database/sql"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/ClickHouse/clickhouse-go/v2"
	"github.com/sirupsen/logrus"

	"go.k6.io/k6/metrics"
	"go.k6.io/k6/output"

	_ "github.com/mailru/go-clickhouse/v2"
)

func init() {
	output.RegisterExtension("clickhouse", New)
}

var (
	// _       interface{ output.WithThresholds } = &Output{}
	timeNow = time.Now
)

type Output struct {
	output.SampleBuffer
	periodicFlusher *output.PeriodicFlusher
	Conn            *sql.DB
	Config          config

	thresholds map[string][]*dbThreshold

	logger logrus.FieldLogger
}

func (o *Output) Description() string {
	return "Clickhouse"
}

func New(params output.Params) (output.Output, error) {
	config, err := getConsolidatedConfig(params.JSONConfig, params.Environment)
	if err != nil {
		return nil, fmt.Errorf("problem parsing config: %w", err)
	}

	conn, err := sql.Open("clickhouse", config.URL)
	// conn, err := sql.Open("chhttp", config.URL)
	if err != nil {
		return nil, fmt.Errorf("clickhouse: unable to create connection: %w", err)
	}

	o := &Output{
		Conn:   conn,
		Config: config,
		logger: params.Logger.WithFields(logrus.Fields{
			"output": "Clickhouse",
		}),
	}

	return o, nil
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

func (o *Output) Start() error {
	sql := "CREATE DATABASE IF NOT EXISTS " + o.Config.dbName
	_, err := o.Conn.Exec(sql)
	if err != nil {
		o.logger.WithError(err).WithField("sql", sql).Debug("Start: Couldn't create database; most likely harmless")
	}

	schema := []string{
		`CREATE TABLE IF NOT EXISTS ` + o.Config.tableSamples + `(
			id UInt64,
			start DateTime64(9, 'UTC'),
			ts DateTime64(9, 'UTC'),
			metric String,
			url String,
			label String,
			status String,
			name String,
			tags Map(String, String),
			value Float64
		) ENGINE = ReplacingMergeTree(start)
		PARTITION BY toYYYYMM(start)
		ORDER BY (id, start, ts, metric, url, label, status, name);`,
		`CREATE TABLE IF NOT EXISTS ` + o.Config.tableTests + ` (
			id UInt64,
			ts DateTime64(9, 'UTC'),
			name String,
			params String
		) ENGINE = ReplacingMergeTree(ts)
		PARTITION BY toYYYYMM(ts)
		ORDER BY (id, ts, name);`,
	}

	for _, s := range schema {
		if _, err = o.Conn.Exec(s); err != nil {
			o.logger.WithError(err).WithField("sql", s).Debug("Start: Couldn't create database schema; most likely harmless")
			return err
		}
	}
	_, err = o.Conn.Exec(
		"INSERT INTO "+o.Config.tableTests+" (id, ts, name, params) VALUES (@Id, @Time, @Name, @Params)",
		clickhouse.Named("Id", o.Config.id),
		clickhouse.DateNamed("Time", o.Config.ts, clickhouse.NanoSeconds),
		clickhouse.Named("Name", o.Config.Name),
		clickhouse.Named("Params", o.Config.params),
		// "INSERT INTO "+o.Config.tableTests+" (id, ts, name, params) VALUES ($1, $2, $3, $4)",
		// o.Config.id,
		// o.Config.ts,
		// o.Config.Name,
		// o.Config.params,
	)
	if err != nil {
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

func TagsName(tags map[string]string) string {
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

	start := time.Now()

	tx, err := o.Conn.Begin()
	if err != nil {
		o.logger.Error(err)
		return
	}

	stmt, err := tx.Prepare("INSERT INTO " + o.Config.tableSamples + " (id, start, ts, metric, url, label, status, name, tags, value)")
	if err != nil {
		o.logger.Error(err)
		return
	}

	for _, sc := range samplesContainer {
		samples := sc.GetSamples()
		for _, s := range samples {
			tags := s.Tags.Map()
			name := TagsName(tags)
			url := tags["url"]
			label := tags["label"]
			status := tags["status"]
			if _, err = stmt.Exec(o.Config.id, o.Config.ts, s.Time.UTC(), s.Metric.Name, url, label, status, name, tags, s.Value); err != nil {
				o.logger.Error(err)
				return
			}
		}
	}

	if err = tx.Commit(); err != nil {
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
