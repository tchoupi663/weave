FROM node:24-alpine AS build
WORKDIR /app
COPY package.json package-lock.json ./
RUN npm ci
COPY . .
RUN npm run build

FROM nginx:alpine
RUN apk add --no-cache openssl curl \
    && mkdir -p /etc/nginx/certs \
    && openssl req -x509 -nodes -newkey rsa:2048 -days 3650 \
        -keyout /etc/nginx/certs/frontend.key \
        -out /etc/nginx/certs/frontend.crt \
        -subj "/CN=frontend" \
        -addext "subjectAltName=DNS:frontend,DNS:localhost,IP:127.0.0.1"
COPY nginx.conf /etc/nginx/conf.d/default.conf
COPY --from=build /app/dist /usr/share/nginx/html
EXPOSE 443
HEALTHCHECK --interval=10s --timeout=3s --retries=10 \
    CMD curl -ksf https://localhost/health >/dev/null || exit 1
