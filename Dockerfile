# syntax=docker/dockerfile:1
FROM golang:1.22-alpine AS builder

WORKDIR /app
COPY . .
RUN go mod tidy && go build -o server .

FROM alpine:latest
RUN adduser -D appuser
WORKDIR /home/appuser
COPY --from=builder /app/server .
USER appuser
EXPOSE 8080
CMD ["./server"]
