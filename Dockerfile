#===============
# Stage 1: Build
#===============

FROM golang:1.21-alpine as builder

COPY . /app

WORKDIR /app

RUN go build -o indexer ./cmd

#===============
# Stage 2: Run
#===============

FROM alpine

COPY --from=builder /app/indexer /usr/local/bin/indexer

ENTRYPOINT [ "/usr/local/bin/indexer" ]