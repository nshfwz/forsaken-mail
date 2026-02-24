FROM golang:1.26-alpine AS builder

WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o /bin/forsaken-mail ./cmd/server

FROM alpine:3.22

WORKDIR /app
RUN apk add --no-cache ca-certificates tzdata

COPY --from=builder /bin/forsaken-mail /usr/local/bin/forsaken-mail
COPY --from=builder /app/public /app/public

EXPOSE 25 3000

ENTRYPOINT ["/usr/local/bin/forsaken-mail"]
