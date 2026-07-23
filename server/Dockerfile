# Multi-stage Dockerfile for MiniShare Cloud Signaling Server
FROM golang:1.23-alpine AS builder

WORKDIR /app
COPY go.mod ./
COPY main.go index.html ./

RUN CGO_ENABLED=0 GOOS=linux go build -o server main.go

FROM alpine:latest
RUN apk --no-cache add ca-certificates

WORKDIR /root/
COPY --from=builder /app/server .

EXPOSE 8080
CMD ["./server", "--port", "8080"]
