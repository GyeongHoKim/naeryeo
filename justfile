default:
    @just --list

fmt:
    golangci-lint fmt

lint:
    golangci-lint run

test:
    go test -race ./...

check: fmt lint test
