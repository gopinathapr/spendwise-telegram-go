# syntax=docker/dockerfile:1
FROM golang:1.23-alpine AS builder

WORKDIR /app
COPY . .
RUN go mod tidy && go build -o server .

FROM alpine:latest
# Install CA certificates for HTTPS connections to Google Cloud APIs and add user
RUN apk --no-cache add ca-certificates && \
    adduser -D appuser
WORKDIR /home/appuser
COPY --from=builder /app/server .
USER appuser
EXPOSE 8080
CMD ["./server"]
