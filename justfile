set windows-shell := ["powershell.exe", "-NoLogo", "-NoProfile", "-Command"]

default:
    @just --list

build:
    go build -ldflags "-X main.version=$(git describe --tags --always --dirty)" -o ./bin/ ./cmd/naeryeo

# just는 가변 인자({{args}})를 다시 펼칠 때 따옴표를 복원하지 않으므로,
# 공백이 들어있는 인자는 안쪽에 작은따옴표를 한 번 더 감싸서 넘겨야 한다.
# 예) just run route --from "'띄워쓰기 포함된 장소'" --to "그게아닌장소"
run *args:
    @go run ./cmd/naeryeo {{args}}

fmt:
    golangci-lint fmt

lint:
    golangci-lint run

test:
    go test -race ./...

check: fmt lint test
