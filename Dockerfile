FROM golang:1.25 AS builder
ENV GO111MODULE=on

WORKDIR /app

COPY go.mod .
COPY go.sum .

RUN go mod download

COPY . .

ARG TARGETOS
ARG TARGETARCH
RUN CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} go build -o b3scale-operator

FROM alpine:3.23 AS certs

RUN apk add ca-certificates && update-ca-certificates

# final stage
FROM scratch
COPY --from=builder /app/b3scale-operator /app/
COPY --from=certs /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
ENTRYPOINT ["/app/b3scale-operator"]
