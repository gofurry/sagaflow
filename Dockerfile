FROM node:24-alpine AS web
WORKDIR /src/web
RUN corepack enable
COPY web/package.json web/pnpm-lock.yaml ./
RUN pnpm install --frozen-lockfile
COPY web/ ./
RUN pnpm build

FROM golang:1.26.5-alpine AS build
ARG VERSION=v0.1.0
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
COPY --from=web /src/internal/webui/dist ./internal/webui/dist
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w -X main.version=${VERSION}" -o /out/sagaflow ./cmd/sagaflow

FROM alpine:3.22
RUN apk add --no-cache ca-certificates tzdata && adduser -D -H -s /sbin/nologin sagaflow
COPY --from=build /out/sagaflow /usr/local/bin/sagaflow
RUN mkdir -p /data && chown sagaflow:sagaflow /data
USER sagaflow
ENV SAGAFLOW_DATA_DIR=/data SAGAFLOW_HOST=0.0.0.0 SAGAFLOW_PORT=8080
VOLUME ["/data"]
EXPOSE 8080
HEALTHCHECK --interval=10s --timeout=3s --retries=12 CMD wget -qO- http://127.0.0.1:8080/health || exit 1
ENTRYPOINT ["sagaflow"]
CMD ["serve"]
