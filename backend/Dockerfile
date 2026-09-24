ARG GO_IMAGE=golang:1.26.8-alpine
ARG ALPINE_IMAGE=alpine:3.22

FROM ${GO_IMAGE} AS build
WORKDIR /src
RUN apk add --no-cache ca-certificates tzdata
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/api ./cmd/api && \
    CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/worker ./cmd/worker && \
    CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/ranking-rebuild ./cmd/ranking-rebuild

FROM ${ALPINE_IMAGE} AS api-runtime
RUN apk add --no-cache ca-certificates tzdata && addgroup -S app && adduser -S app -G app
WORKDIR /app
COPY --from=build /out/api /app/api
USER app
EXPOSE 8081
ENTRYPOINT ["/app/api"]

FROM ${ALPINE_IMAGE} AS worker-runtime
RUN apk add --no-cache ca-certificates tzdata ffmpeg && addgroup -S app && adduser -S app -G app && mkdir -p /tmp/video-share && chown app:app /tmp/video-share
WORKDIR /app
COPY --from=build /out/worker /app/worker
USER app
ENTRYPOINT ["/app/worker"]

FROM ${ALPINE_IMAGE} AS ranking-runtime
RUN apk add --no-cache ca-certificates tzdata && addgroup -S app && adduser -S app -G app
WORKDIR /app
COPY --from=build /out/ranking-rebuild /app/ranking-rebuild
USER app
ENTRYPOINT ["/app/ranking-rebuild"]
