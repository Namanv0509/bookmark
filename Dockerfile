
FROM golang:1.24-alpine AS builder

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY main.go ./
RUN CGO_ENABLED=0 GOOS=linux go build -o linkvault main.go

FROM alpine:3.22

WORKDIR /app

COPY --from=builder /app/linkvault ./linkvault
COPY index.html ./index.html

EXPOSE 8080

CMD ["./linkvault"]
