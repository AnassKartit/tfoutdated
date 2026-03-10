FROM golang:1.24-alpine AS builder
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -ldflags="-s -w" -o /tfoutdated .
RUN CGO_ENABLED=0 go build -ldflags="-s -w" -o /tfoutdated-mcp ./cmd/tfoutdated-mcp

FROM alpine:3.21
RUN apk add --no-cache ca-certificates
COPY --from=builder /tfoutdated /usr/local/bin/tfoutdated
COPY --from=builder /tfoutdated-mcp /usr/local/bin/tfoutdated-mcp
WORKDIR /data
ENTRYPOINT ["tfoutdated"]
