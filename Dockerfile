FROM golang:1.25-alpine3.22 AS builder

WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o /zzy .

FROM alpine:3.22

RUN apk add --no-cache \
	ca-certificates \
	font-noto-cjk \
	libreoffice \
	pandoc

COPY --from=builder /zzy /usr/local/bin/zzy

WORKDIR /app
CMD ["zzy"]
