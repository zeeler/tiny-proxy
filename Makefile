.PHONY: build run test clean

BINARY=tiny-proxy

build:
	go build -o $(BINARY) .

run: build
	./$(BINARY)

test:
	go test ./... -v -count=1

clean:
	rm -f $(BINARY)
