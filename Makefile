.PHONY: build run test race benchmark chaos profile audit release clean coverage install lint smoke determinism stress pipeline compatibility ci verify

APP=searchit
STATICCHECK=$(shell go env GOPATH)/bin/staticcheck

build:
	go build -o bin/$(APP) .

run:
	go run .

test:
	BENCHMARK_LEVEL=$${BENCHMARK_LEVEL:-1} go test ./...

race:
	BENCHMARK_LEVEL=$${BENCHMARK_LEVEL:-1} go test -race -count=1 ./...

benchmark:
	BENCHMARK_LEVEL=$${BENCHMARK_LEVEL:-1} go test -bench=Benchmark -run=^$$ ./...

chaos:
	go test -v ./internal/recursion/ -run=TestChaos

profile:
	go test -v -bench=BenchmarkProfile -run=^$$ ./internal/fingerprint/

audit:
	go vet ./...
	$(STATICCHECK) ./...

lint:
	./scripts/ci/lint.sh

smoke: build
	./scripts/ci/binary_smoke_test.sh

determinism: build
	./scripts/ci/determinism_test.sh

stress:
	./scripts/ci/stress_test.sh

pipeline:
	./scripts/ci/pipeline_test.sh

compatibility:
	./scripts/ci/compatibility_test.sh

ci: lint smoke stress determinism pipeline compatibility
	./scripts/ci/coverage_report.sh
	./scripts/ci/benchmark_perf_test.sh
	./scripts/ci/generate_verification_report.sh

verify: ci

release:
	mkdir -p dist
	GOOS=linux GOARCH=amd64 go build -ldflags="-s -w" -o dist/$(APP)-linux-amd64 .
	GOOS=darwin GOARCH=amd64 go build -ldflags="-s -w" -o dist/$(APP)-darwin-amd64 .
	GOOS=windows GOARCH=amd64 go build -ldflags="-s -w" -o dist/$(APP)-windows-amd64.exe .

clean:
	rm -rf dist bin coverage.out coverage.html benchmark performance determinism pipeline compatibility verification

coverage:
	go test -coverprofile=coverage.out ./...
	go tool cover -html=coverage.out -o coverage.html

install:
	go install .