package timescaledb

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"

	_ "github.com/ClickHouse/clickhouse-go/v2"
)

type Duration time.Duration

func (d *Duration) Parse(v string) error {
	duration, err := time.ParseDuration(v)
	if err == nil {
		*d = Duration(duration)
	}
	return err
}

func (d *Duration) UnmarshalJSON(b []byte) error {
	var v interface{}
	if err := json.Unmarshal(b, &v); err != nil {
		return err
	}
	switch value := v.(type) {
	case float64:
		*d = Duration(value)
		return nil
	case string:
		err := d.Parse(value)
		if err != nil {
			return err
		}
		return nil
	default:
		return errors.New("invalid duration")
	}
}

type config struct {
	URL          string   `json:"url"`
	PushInterval Duration `json:"pushInterval"`
	Name         string   `json:"name"`
	dbName       string
	id           uint64
	ts           time.Time
	params       string
}

func newConfig() config {
	return config{
		URL:          "http://localhost:8123/default?dial_timeout=200ms&max_execution_time=60",
		PushInterval: Duration(10 * time.Second),
		dbName:       "default",
	}
}

func (c config) apply(modifiedConf config) (config, error) {
	if modifiedConf.URL != "" {
		if u, dbName, err := parseURL(modifiedConf.URL); err == nil {
			c.URL, c.dbName = u, dbName
		} else {
			return config{}, err
		}
	}
	if modifiedConf.PushInterval > 0 {
		c.PushInterval = modifiedConf.PushInterval
	}
	return c, nil
}

func parseURL(text string) (dbUrl, dbName string, err error) {
	var u *url.URL
	u, err = url.Parse(text)
	if err != nil {
		return
	}

	if u.Host == "" || u.Scheme == "" {
		return "", "", errors.New("empty host url")
	}
	if dbName = strings.TrimPrefix(u.Path, "/"); dbName == "" {
		dbName = "default"
	}
	dbUrl = text

	return
}

func getConsolidatedConfig(jsonRawConf json.RawMessage, env map[string]string) (config, error) {
	consolidatedConf := newConfig()
	var err error

	if jsonRawConf != nil {
		var jsonConf config
		if err := json.Unmarshal(jsonRawConf, &jsonConf); err != nil {
			return config{}, fmt.Errorf("problem unmarshalling JSON: %w", err)
		}
		if consolidatedConf, err = consolidatedConf.apply(jsonConf); err != nil {
			return config{}, fmt.Errorf("problem apply config: %w", err)
		}
	}

	envPushInterval, ok := env["K6_OUT_CLICKHOUSE_PUSH_INTERVAL"]
	if ok {
		var pushInterval Duration
		err := pushInterval.Parse(envPushInterval)
		if err != nil {
			return config{}, fmt.Errorf("invalid K6_OUT_CLICKHOUSE_PUSH_INTERVAL: %w", err)
		}
		if consolidatedConf, err = consolidatedConf.apply(config{PushInterval: pushInterval}); err != nil {
			return config{}, fmt.Errorf("problem apply config from K6_OUT_CLICKHOUSE_PUSH_INTERVAL: %w", err)
		}
	}

	name := env["K6_OUT_CLICKHOUSE_TESTNAME"]
	consolidatedConf.ts = timeNow().UTC()
	consolidatedConf.id = uint64(consolidatedConf.ts.UnixNano())
	if name == "" {
		consolidatedConf.Name = consolidatedConf.ts.Format(time.RFC3339Nano)
	} else {
		consolidatedConf.Name = name + " " + consolidatedConf.ts.Format(time.RFC3339Nano)
	}
	consolidatedConf.params = env["K6_OUT_CLICKHOUSE_PARAMS"]

	return consolidatedConf, nil
}
