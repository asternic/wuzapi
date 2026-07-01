FROM golang:alpine AS builder
WORKDIR /app
RUN apk add --no-cache gcc musl-dev sqlite-dev
COPY go.mod go.sum ./
COPY whatsmeow/ ./whatsmeow/
ENV GONOSUMDB=github.com/w3nder
ENV GOFLAGS=-mod=mod
RUN go mod download
COPY . .
RUN CGO_ENABLED=1 go build -o wuzapi .

FROM alpine:3.20
RUN apk add --no-cache ca-certificates ffmpeg tzdata sqlite
WORKDIR /app
COPY --from=builder /app/wuzapi .
EXPOSE 8080
ENTRYPOINT ["./wuzapi"]
