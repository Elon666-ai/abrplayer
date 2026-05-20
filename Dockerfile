# =======================================================
# Stage 1: Build binary
# =======================================================
FROM golang:1.24-alpine AS builder
WORKDIR /src/backend
RUN apk add --no-cache tzdata git

COPY backend/go.mod backend/go.sum* ./
RUN go mod download

COPY backend/ ./
RUN go build -mod=mod \
    -ldflags="-s -w -X 'main.BuildTime=$(date -u +'%Y-%m-%dT%H:%M:%SZ')'" \
    -o /out/abrplayer-backend .

# =======================================================
# Stage 2: Runtime image (with ffmpeg+ffprobe)
# =======================================================
FROM jrottenberg/ffmpeg:6.1-alpine

WORKDIR /app
USER root

# 安装时区支持并设置为北京时间
RUN apk add --no-cache tzdata wget && \
    cp /usr/share/zoneinfo/Asia/Shanghai /etc/localtime && \
    echo "Asia/Shanghai" > /etc/timezone

ENV TZ=Asia/Shanghai \
    ABRPLAYER_ROOT=/opt/abrplayer

# 拷贝可执行文件、配置、静态资源
COPY --from=builder /out/abrplayer-backend /app/abrplayer-backend
COPY backend/conf /app/conf
COPY abrplayer /opt/abrplayer
COPY admin-web /opt/admin-web

RUN mkdir -p /app/data /app/logs && \
    ln -sf /opt/admin-web /app/admin-web && \
    date && ffprobe -version

EXPOSE 8088

HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 \
  CMD wget -q -O- "http://127.0.0.1:8088/healthz" >/dev/null || exit 1

ENTRYPOINT ["/app/abrplayer-backend"]
