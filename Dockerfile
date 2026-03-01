# Build stage
FROM golang:1.26-alpine AS builder
RUN apk add --no-cache ca-certificates
RUN addgroup -g 1001 probeuser && adduser -u 1001 -G probeuser -D probeuser
WORKDIR /app

# Copy go mod and sum files
COPY go.mod go.sum ./

# Download all dependencies. Dependencies will be cached if the go.mod and go.sum files are not changed
RUN go mod download

# Copy the source from the current directory to the Working Directory inside the container
COPY . .

# Build the Go app

RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-w -s" -o probe cmd/main.go

# Final stage
FROM scratch
COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
COPY --from=builder /etc/passwd /etc/passwd
COPY --from=builder /app/probe /probe
WORKDIR /
USER probeuser
ENTRYPOINT ["/probe"]
