#===============
# Stage 1: Build
#===============

FROM golang:1.22-alpine AS builder

WORKDIR /src

RUN --mount=type=cache,target=/go/pkg/mod/ \
    --mount=type=cache,target=/root/.cache/go-build \
    --mount=type=bind,source=go.sum,target=go.sum \
    --mount=type=bind,source=go.mod,target=go.mod \
    go mod download -x

ENV CGO_ENABLED=0 GOOS=linux 

RUN --mount=type=cache,target=/go/pkg/mod/ \
    --mount=type=cache,target=/root/.cache/go-build \
    --mount=type=bind,target=. \
    go build -o /app/indexer ./cmd

#===============
# Stage 2: Run
#===============

FROM cgr.dev/chainguard/static:latest AS tx-indexer

WORKDIR /var/lib/app
COPY --from=builder /app/indexer /usr/local/bin/indexer
ENTRYPOINT [ "/usr/local/bin/indexer" ]