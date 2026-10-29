FROM golang:1.27-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY cmd ./cmd
COPY internal ./internal
RUN CGO_ENABLED=0 go build -trimpath -o /out/auth ./cmd/auth \
    && CGO_ENABLED=0 go build -trimpath -o /out/catalog ./cmd/catalog \
    && CGO_ENABLED=0 go build -trimpath -o /out/orders ./cmd/orders \
    && CGO_ENABLED=0 go build -trimpath -o /out/payment ./cmd/payment \
    && CGO_ENABLED=0 go build -trimpath -o /out/external ./cmd/external \
    && CGO_ENABLED=0 go build -trimpath -o /out/gateway ./cmd/gateway

FROM alpine:3.22
RUN apk add --no-cache ca-certificates wget \
    && adduser -D -u 10001 app
COPY --from=build /out/ /app/
USER app
CMD ["/app/auth"]
 