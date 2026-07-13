.PHONY: run test vet

run:
	go run ./cmd/server

test:
	go test ./...

vet:
	go vet ./...
