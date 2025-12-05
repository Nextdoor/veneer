FROM golang:1.24.6-alpine AS builder
WORKDIR /app
COPY main.go .
RUN go mod init mock-exporter && \
    CGO_ENABLED=0 go build -o mock-exporter main.go

FROM alpine:latest
COPY --from=builder /app/mock-exporter /mock-exporter
EXPOSE 8080
ENTRYPOINT ["/mock-exporter"]
