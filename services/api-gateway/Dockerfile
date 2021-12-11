FROM golang:alpine as builder

ARG SERVICE_DIR=api-gateway

WORKDIR /app
COPY go.mod ./
COPY go.sum ./
RUN go mod download

COPY . .

RUN go build -o main services/$SERVICE_DIR/cmd/main.go

FROM alpine

COPY --from=builder /app/main .
CMD "./main"