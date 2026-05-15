.PHONY: build test fmt vet clean install

build:
	go build -o bin/caveira ./cmd/caveira
	go build -o bin/cav ./cmd/cav

test:
	go test ./...

fmt:
	gofmt -w .

vet:
	go vet ./...

install:
	go install ./cmd/caveira ./cmd/cav

clean:
	rm -rf bin
