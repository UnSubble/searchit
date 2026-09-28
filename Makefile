.PHONY: build run fmt vet test test-race check check-full \
        benchmark chaos profile audit lint \
        smoke determinism stress pipeline compatibility \
        ci verify release clean coverage install

APP        = searchit
GOFLAGS   ?=
STATICCHECK = $(shell go env GOPATH)/bin/staticcheck

# ---------------------------------------------------------------------------
# Development
# ---------------------------------------------------------------------------

## build: compile the binary to bin/$(APP) (no version injection)
build:
	go build $(GOFLAGS) -o bin/$(APP) .

## run: run from source
run:
	go run .

## install: install to $GOPATH/bin
install:
	go install .

## fmt: format all Go source files in-place
fmt:
	gofmt -w .

## vet: run go vet
vet:
	go vet ./...

## test: run unit tests
test:
	BENCHMARK_LEVEL=$${BENCHMARK_LEVEL:-1} go test $(GOFLAGS) ./...

## test-race: run unit tests with the race detector
test-race:
	BENCHMARK_LEVEL=$${BENCHMARK_LEVEL:-1} go test -race -count=1 $(GOFLAGS) ./...

## check: fast local quality gate — format check + vet + unit tests + lint
check: lint test

## check-full: full local quality gate — check + race detector
check-full: lint test test-race

# ---------------------------------------------------------------------------
# Extended / Scientific Tests
# ---------------------------------------------------------------------------

## benchmark: run Go benchmarks
benchmark:
	BENCHMARK_LEVEL=$${BENCHMARK_LEVEL:-1} go test -bench=Benchmark -run=^$$ ./...

## chaos: run chaos tests against the recursion engine
chaos:
	go test -v ./internal/recursion/ -run=TestChaos

## profile: run fingerprint profiling benchmarks
profile:
	go test -v -bench=BenchmarkProfile -run=^$$ ./internal/fingerprint/

## smoke: build binary + run binary smoke tests
smoke: build
	./scripts/ci/binary_smoke_test.sh

## determinism: build binary + run determinism verification (requires dist/searchit)
determinism: build
	./scripts/ci/determinism_test.sh

## stress: run stress & race safety tests
stress:
	./scripts/ci/stress_test.sh

## pipeline: run pipeline & reconciliation tests
pipeline:
	./scripts/ci/pipeline_test.sh

## compatibility: run output & profile compatibility tests
compatibility:
	./scripts/ci/compatibility_test.sh

# ---------------------------------------------------------------------------
# Lint / Static Analysis
# ---------------------------------------------------------------------------

## audit: run go vet + staticcheck (mirrors CI lint, no gofmt check)
audit: vet
	$(STATICCHECK) ./...

## lint: full lint check via CI lint script (gofmt + go vet + staticcheck)
lint:
	./scripts/ci/lint.sh

# ---------------------------------------------------------------------------
# Full CI / Verification
# ---------------------------------------------------------------------------

## ci: run the full local CI suite (all checks and scientific tests)
ci: check-full smoke stress determinism pipeline compatibility
	./scripts/ci/coverage_report.sh
	./scripts/ci/benchmark_perf_test.sh
	./scripts/ci/generate_verification_report.sh

## verify: alias for ci
verify: ci

# ---------------------------------------------------------------------------
# Release Build
# ---------------------------------------------------------------------------

## release: cross-compile release binaries with proper ldflags (use build.sh for official releases)
release:
	chmod +x build.sh
	./build.sh

# ---------------------------------------------------------------------------
# Coverage
# ---------------------------------------------------------------------------

## coverage: generate HTML coverage report
coverage:
	go test -coverprofile=coverage.out ./...
	go tool cover -html=coverage.out -o coverage.html

# ---------------------------------------------------------------------------
# Cleanup
# ---------------------------------------------------------------------------

## clean: remove build artifacts
clean:
	rm -rf dist bin coverage.out coverage.html benchmark performance determinism pipeline compatibility verification