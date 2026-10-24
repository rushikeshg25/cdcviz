.PHONY: build test race vet fmt demo clean

BIN := bin/cdcviz

build:
	go build -o $(BIN) ./cmd/cdcviz

test:
	go test ./...

race:
	go test -race ./...

vet:
	go vet ./...

fmt:
	gofmt -w $$(find . -name '*.go' -not -path './bin/*')

demo: build
	./$(BIN) demo

clean:
	rm -rf bin
