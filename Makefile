APP=searchit

build:
	./build.sh

run:
	go run .

test:
	go test ./...

fmt:
	go fmt ./...

lint:
	golangci-lint run

clean:
	rm -rf dist