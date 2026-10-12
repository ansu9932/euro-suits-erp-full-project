FROM golang:1.27.1-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -ldflags='-s -w' -o /erp ./cmd/server
FROM alpine:3.23
RUN apk add --no-cache ca-certificates poppler-utils && addgroup -g 10001 erp && adduser -D -H -u 10001 -G erp erp && mkdir -p /data/files && chown -R erp:erp /data
COPY --from=build /erp /usr/local/bin/erp
USER 10001:10001
EXPOSE 8081
ENTRYPOINT ["/usr/local/bin/erp"]
