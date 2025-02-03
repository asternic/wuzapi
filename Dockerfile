FROM golang:1.23-alpine AS builder

RUN apk add --no-cache gcc musl-dev

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .

ENV CGO_ENABLED=1
RUN go build -o wuzapi .

FROM alpine:latest

RUN apk add --no-cache ca-certificates netcat-openbsd postgresql-client

WORKDIR /app

COPY --from=builder /app/wuzapi /app/
COPY static/ /app/static/

COPY migrations/ /app/migrations/

VOLUME [ "/app/dbdata", "/app/files" ]

ENV WUZAPI_ADMIN_TOKEN="SetToRandomAndSecureTokenForAdminTasks"

COPY entrypoint.sh /entrypoint.sh
RUN chmod +x /entrypoint.sh

ENTRYPOINT ["/entrypoint.sh"] 
