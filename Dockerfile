# 1. Use the Go 1.26 image to satisfy Gin v1.12.0
FROM golang:1.26-alpine AS builder

WORKDIR /app

# 2. Allow Go to manage its own toolchain if needed
ENV GOPROXY=https://proxy.golang.org,direct
ENV GOTOOLCHAIN=auto

# 3. Copy only the module files first
COPY go.mod go.sum* ./

# 4. Force the module file to match the environment
RUN go mod edit -go=1.26
RUN go mod download

# 5. Copy source and build
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o todo-app .

# FINAL STAGE
FROM alpine:latest
RUN apk add --no-cache ca-certificates
WORKDIR /root/
COPY --from=builder /app/todo-app .
RUN mkdir data
EXPOSE 8000
CMD ["./todo-app"]