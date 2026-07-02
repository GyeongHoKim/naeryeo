# naeryeo cloud track (PlayMCP in KC) — Streamable HTTP MCP server.
#
# PlayMCP in KC's "Git source build" builds this file from the repository
# root and requires a linux/amd64 image (arm64 images fail to activate).
# Build locally with:  docker build --platform linux/amd64 -t naeryeo-cloud .
# Run with:            docker run -p 8080:8080 -e NAERYEO_MOTIS_URL=... naeryeo-cloud
#
# See specs/005-playmcp-cloud-server/ (research.md §5, contracts/http-server.md).

FROM golang:1.26 AS build

WORKDIR /src

# Cache module downloads separately from source changes.
COPY go.mod go.sum ./
RUN go mod download

COPY . .

ARG VERSION=docker
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
    go build -trimpath -ldflags "-s -w -X main.version=${VERSION}" \
    -o /out/naeryeo ./cmd/naeryeo

# distroless/static ships ca-certificates and tzdata (TLS to the MOTIS
# backend, Asia/Seoul time rendering) with no shell or package manager.
FROM gcr.io/distroless/static-debian12:nonroot

COPY --from=build /out/naeryeo /naeryeo

# The MOTIS backend base URL is mandatory; the server fails fast without it.
# ENV NAERYEO_MOTIS_URL=

EXPOSE 8080

ENTRYPOINT ["/naeryeo", "mcp", "--http"]
