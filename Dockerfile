# naeryeo cloud track (PlayMCP in KC) — Streamable HTTP MCP server.
#
# PlayMCP in KC's "Git source build" builds this file from the repository
# root and requires a linux/amd64 image (arm64 images fail to activate).
#
# PlayMCP in KC does not yet support runtime env var/secret injection
# (official FAQ, checked 2026-07-05; native support is slated for the
# second week of July 2026). Per that FAQ, the sanctioned workaround is to
# bake the provider secret into the image at build time and deploy it via
# a PRIVATE GitHub repo or PRIVATE Docker registry only — never public,
# since the secret becomes part of the image layers.
#
# Build locally with:
#   docker build --platform linux/amd64 \
#     --build-arg NAERYEO_PROVIDER=tmap \
#     --build-arg NAERYEO_TMAP_APP_KEY=... \
#     -t naeryeo-cloud .
# Run with:            docker run -p 8080:8080 naeryeo-cloud
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

# Baked in at build time (see header comment) until PlayMCP in KC ships
# native env var injection. Pass only the --build-arg pair for whichever
# provider you're deploying; the rest stay empty and unused.
ARG NAERYEO_PROVIDER=tmap
ARG NAERYEO_TMAP_APP_KEY=""
ARG NAERYEO_MOTIS_URL=""
ARG NAERYEO_ODSAY_API_KEY=""
ENV NAERYEO_PROVIDER=${NAERYEO_PROVIDER}
ENV NAERYEO_TMAP_APP_KEY=${NAERYEO_TMAP_APP_KEY}
ENV NAERYEO_MOTIS_URL=${NAERYEO_MOTIS_URL}
ENV NAERYEO_ODSAY_API_KEY=${NAERYEO_ODSAY_API_KEY}

EXPOSE 8080

ENTRYPOINT ["/naeryeo", "mcp", "--http"]
