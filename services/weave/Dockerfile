# syntax=docker/dockerfile:1

FROM node:24-alpine AS web
WORKDIR /web
COPY web/package.json web/package-lock.json ./
RUN npm ci
COPY web/ ./
RUN npm run build

FROM golang:1.27-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY cmd ./cmd
COPY internal ./internal
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/weave ./cmd/weave

FROM alpine:3.22
RUN apk add --no-cache ca-certificates
WORKDIR /app
COPY --from=build /out/weave /app/weave
COPY --from=web /web/dist /app/web/dist

ENV LISTEN_ADDR=:8080 \
    HUBBLE_RELAY_ADDR=hubble-relay.kube-system.svc.cluster.local:80 \
    WATCH_NAMESPACE=weave \
    WINDOW=5m \
    WEB_DIR=/app/web/dist

USER 65532:65532
EXPOSE 8080

HEALTHCHECK --interval=15s --timeout=3s --start-period=5s --retries=3 \
  CMD wget -qO- http://127.0.0.1:8080/health >/dev/null || exit 1

ENTRYPOINT ["/app/weave"]
