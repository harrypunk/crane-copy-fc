# Build stage
FROM docker.io/golang:1.26.2-alpine3.22 AS builder

ENV GO111MODULE=on
ENV GOPROXY=https://goproxy.cn,direct

WORKDIR /app

# Copy module files first to cache dependencies.
COPY go.mod go.sum ./
RUN go mod download

# Copy source and build.
COPY . .
RUN CGO_ENABLED=0 go build -o bootstrap main.go

# Runtime stage
FROM docker.io/library/alpine:latest

WORKDIR /app
COPY --from=builder /app/bootstrap .

ENTRYPOINT ["./bootstrap"]
