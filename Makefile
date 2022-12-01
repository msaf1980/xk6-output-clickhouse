K6_VERSION ?= "v0.41.0"

MAKEFLAGS += --silent

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
	go fmt ./...

## test: Executes any unit tests.
test:
	go test -cover -race

ch:
	docker run -d -it --rm --name xk6_output_clickhouse -p 127.0.0.1:8123:8123 -p 127.0.0.1:9000:9000 clickhouse/clickhouse-server:latest

ch_stop:
	docker stop xk6_output_clickhouse

integrations:
	K6_OUT="clickhouse=http://localhost:8123/default?dial_timeout=200ms&max_execution_time=60" ./k6 run tests/http.js -v

dump:
	echo "tests id                        name"
	docker exec -ti xk6_output_clickhouse clickhouse-client -q "SELECT id, name FROM k6_tests"
	echo
	echo "samples"
	docker exec -ti xk6_output_clickhouse clickhouse-client -q "SELECT count(1) AS samples FROM k6_samples"

.PHONY: build clean format help test
