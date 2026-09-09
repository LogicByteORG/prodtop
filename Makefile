.PHONY: run build vet test fmt tidy

run:
	go run ./cmd/prodtop

build:
	go build -o bin/prodtop ./cmd/prodtop

vet:
	go vet ./...

test:
	go test ./...

fmt:
	gofmt -l .

tidy:
	go mod tidy
