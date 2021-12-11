FROM golang:alpine as builder

WORKDIR /app
COPY src/go.mod ./
COPY src/go.sum ./
RUN go mod download

COPY src/ .

RUN go build -o currency/main currency/cmd/main.go

FROM alpine
COPY --from=builder /app/currency ./currency
CMD ["./currency/main"]