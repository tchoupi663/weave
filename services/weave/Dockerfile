# syntax=docker/dockerfile:1

FROM node:24-alpine AS web
WORKDIR /web
COPY web/package.json web/package-lock.json ./
RUN npm ci
COPY web/ ./
RUN npm run build

FROM golang:1.27-bookworm AS build
RUN apt-get update && apt-get install -y --no-install-recommends build-essential ca-certificates && rm -rf /var/lib/apt/lists/*
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY cmd ./cmd
COPY internal ./internal
RUN CGO_ENABLED=1 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/weave-live ./cmd/live \
 && CGO_ENABLED=1 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/weave-history ./cmd/history \
 && CGO_ENABLED=1 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/weave-inventory ./cmd/inventory \
 && CGO_ENABLED=1 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/weave-api ./cmd/api

FROM debian:bookworm-slim AS runtime
RUN apt-get update && apt-get install -y --no-install-recommends ca-certificates libstdc++6 && rm -rf /var/lib/apt/lists/*
WORKDIR /app
ENV LISTEN_ADDR=:8080
USER 65532:65532
EXPOSE 8080

FROM runtime AS live
COPY --from=build /out/weave-live /app/weave-live
ENTRYPOINT ["/app/weave-live"]

FROM runtime AS history
COPY --from=build /out/weave-history /app/weave-history
ENV DATA_DIR=/data FLUSH_GRACE=15s
ENTRYPOINT ["/app/weave-history"]

FROM runtime AS inventory
COPY --from=build /out/weave-inventory /app/weave-inventory
ENV DATA_DIR=/data
ENTRYPOINT ["/app/weave-inventory"]

FROM runtime AS api
COPY --from=build /out/weave-api /app/weave-api
COPY --from=web /web/dist /app/web/dist
ENV WEB_DIR=/app/web/dist
ENTRYPOINT ["/app/weave-api"]
