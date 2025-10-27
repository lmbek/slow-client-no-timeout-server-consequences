# Simple Docker image for the Go demo server
# Builds the server binary and runs it on port 8080.

FROM golang:1.25-alpine AS build
WORKDIR /app

# Use GOPATH mode since this tiny demo has no go.mod.
ENV GO111MODULE=off

# Only copy what we need to build the server (std lib only).
COPY . ./go-server

# Build a static binary for portability.
RUN CGO_ENABLED=0 go build -o /server ./go-server

# Final runtime image
FROM alpine:3.20
# Non-root user for safety
RUN adduser -D -u 10001 appuser
USER appuser

COPY --from=build /server /server
EXPOSE 8081

# The server listens on :8080 and prints timing/bytes for requests
ENTRYPOINT ["/server"]
