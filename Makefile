.PHONY: test check build bench

test:
	CGO_ENABLED=0 go test -tags noasm ./...

check: test
	CGO_ENABLED=0 go vet -tags noasm ./...
	CGO_ENABLED=0 go run -tags noasm ./cmd/purecheck

build:
	CGO_ENABLED=0 go build -tags noasm -o bin/gomath ./cmd/gomath

bench:
	CGO_ENABLED=0 go run ./tests/cmd/run -run '^$$' -bench . -benchmem
