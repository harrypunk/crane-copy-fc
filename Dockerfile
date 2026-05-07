# Build stage
FROM docker.io/golang:1.26.2-alpine3.22 AS builder

WORKDIR /app
COPY . .

RUN go env -w GO111MODULE=on
RUN go env -w GOPROXY=https://goproxy.cn,direct
RUN go mod download
RUN CGO_ENABLED=0 go build -o bootstrap main.go

# Runtime stage
FROM docker.io/library/alpine:latest

WORKDIR /app
COPY --from=builder /app/bootstrap .

ENTRYPOINT ["./bootstrap"]
