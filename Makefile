K6_VERSION ?= "v0.41.0"

MAKEFLAGS += --silent

CLICKHOUSE_VERSION ?= "clickhouse/clickhouse-server:latest"
CLICKHOUSE_CONTAINER ?= "xk6_output_clickhouse"

DOCKER ?= docker
GO ?= go

all: clean format test build

## help: Prints a list of available build targets.
help:
	echo "Usage: make <OPTIONS> ... <TARGETS>"
	echo ""
	echo "Available targets are:"
	echo ''
	sed -n 's/^##//p' ${PWD}/Makefile | column -t -s ':' | sed -e 's/^/ /'
	echo
	echo "Targets run by default are: `sed -n 's/^all: //p' ./Makefile | sed -e 's/ /, /g' | sed -e 's/\(.*\), /\1, and /'`"

## clean: Removes any previously created build artifacts.
clean:
	rm -f ./k6

prep:
	go install go.k6.io/xk6/cmd/xk6@latest

## build: Builds a custom 'k6' with the local extension. 
build:
	xk6 build ${K6_VERSION} --with $(shell go list -m)=.

## format: Applies Go formatting to code.
format:
	${GO} fmt ./...

## test: Executes any unit tests.
test:
	${GO} test -cover -race ./...

up:
	${DOCKER} run -d -it --rm --name "${CLICKHOUSE_CONTAINER}" -p 127.0.0.1:8123:8123 -p 127.0.0.1:9000:9000 ${CLICKHOUSE_VERSION}

down:
	${DOCKER} stop "${CLICKHOUSE_CONTAINER}"

clear:
	${DOCKER} exec -ti "${CLICKHOUSE_CONTAINER}" clickhouse-client -q "DROP TABLE IF EXISTS k6_tests"
	${DOCKER} exec -ti "${CLICKHOUSE_CONTAINER}" clickhouse-client -q "DROP TABLE IF EXISTS k6_samples"

cli:
	${DOCKER} exec -it "${CLICKHOUSE_CONTAINER}" clickhouse-client

integrations:
	${GO} test -count=1 -tags=test_integration ./tests
	K6_OUT_CLICKHOUSE_TABLE_TESTS="t_k6_tests" K6_OUT_CLICKHOUSE_TABLE_SAMPLES="t_k6_samples" K6_CLICKHOUSE_PARAMS="USERS_1H_0=10 USERS_7D_0=1" K6_OUT="clickhouse=http://localhost:8123/default?dial_timeout=200ms&max_execution_time=60" ./k6 run tests/http.js -v

dump:
	echo "tests id                        name                            params"
	${DOCKER} exec -ti "${CLICKHOUSE_CONTAINER}" clickhouse-client -q "SELECT id, name, params FROM t_k6_tests"
	echo
	echo "samples                         count"
	${DOCKER} exec -ti "${CLICKHOUSE_CONTAINER}" clickhouse-client -q "SELECT id, count(1) AS samples FROM t_k6_samples GROUP BY id"

.PHONY: build clean format help test
