FROM golang:1.26 AS builder
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN go test -c -o /soak-test -run TestSoakNative ./

FROM gcr.io/distroless/static-debian12
COPY --from=builder /soak-test /soak-test
ENTRYPOINT ["/soak-test", "-test.v", "-test.timeout=24h", "-test.run=TestSoakNative"]
