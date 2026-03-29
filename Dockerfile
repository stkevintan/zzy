FROM golang:1.25-bookworm AS builder

WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o /zzy .

FROM debian:bookworm-slim

RUN apt-get update && \
    apt-get install -y --no-install-recommends \
        ca-certificates \
        libreoffice-writer \
        fonts-wqy-zenhei \
    && rm -rf /var/lib/apt/lists/*

COPY --from=builder /zzy /usr/local/bin/zzy

WORKDIR /app
CMD ["zzy"]
