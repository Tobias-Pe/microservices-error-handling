ARG SERVICE_DIR=cart

FROM golang:1.17-alpine as builder

ARG SERVICE_DIR

WORKDIR /app
COPY go.mod ./
COPY go.sum ./
RUN go mod download

COPY . .

RUN go build -o services/$SERVICE_DIR/main services/$SERVICE_DIR/cmd/main.go

FROM alpine

ARG SERVICE_DIR

COPY --from=builder /app/services/$SERVICE_DIR .
COPY config/local.env ./config/

CMD "./main"