FROM golang:1.23-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o /out/wallet-transfer-service ./cmd/server

FROM alpine:3.20
RUN apk add --no-cache ca-certificates
COPY --from=build /out/wallet-transfer-service /usr/local/bin/wallet-transfer-service
USER nobody
EXPOSE 8080
ENTRYPOINT ["wallet-transfer-service"]
