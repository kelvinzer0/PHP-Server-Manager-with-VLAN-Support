FROM golang:1.21-alpine AS builder

WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download

COPY *.go ./
RUN CGO_ENABLED=0 GOOS=linux go build -o php-server-manager .

FROM alpine:latest

RUN apk --no-cache add ca-certificates iproute2 sudo bash
WORKDIR /root/

COPY --from=builder /app/php-server-manager .
COPY static/ ./static/

# Install FrankenPHP
RUN wget -O frankenphp https://github.com/dunglas/frankenphp/releases/latest/download/frankenphp-linux-x86_64 && \
    chmod +x frankenphp && \
    mv frankenphp /usr/local/bin/

EXPOSE 80

CMD ["./php-server-manager"]
