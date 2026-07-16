.PHONY: build run test race benchmark chaos profile audit release clean coverage install

APP=searchit
STATICCHECK=$(shell go env GOPATH)/bin/staticcheck

build:
	go build -o bin/$(APP) .

run:
	go run .

test:
	go test ./...

race:
	go test -race -count=1 ./...

benchmark:
	go test -bench=Benchmark -run=^$$ ./...

chaos:
	go test -v ./internal/recursion/ -run=TestChaos

profile:
	go test -v -bench=BenchmarkProfile -run=^$$ ./internal/fingerprint/

audit:
	go vet ./...
	$(STATICCHECK) ./...

release:
	mkdir -p dist
	GOOS=linux GOARCH=amd64 go build -ldflags="-s -w" -o dist/$(APP)-linux-amd64 .
	GOOS=darwin GOARCH=amd64 go build -ldflags="-s -w" -o dist/$(APP)-darwin-amd64 .
	GOOS=windows GOARCH=amd64 go build -ldflags="-s -w" -o dist/$(APP)-windows-amd64.exe .

clean:
	rm -rf dist bin coverage.out coverage.html

coverage:
	go test -coverprofile=coverage.out ./...
	go tool cover -html=coverage.out -o coverage.html

install:
	go install .