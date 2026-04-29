FROM golang:1.22.2-bookworm AS builder

RUN apt-get update && apt-get install -y --no-install-recommends \
    build-essential \
    ca-certificates \
    wget \
    unzip \
 && rm -rf /var/lib/apt/lists/*

WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

RUN wget -q https://github.com/duckdb/duckdb/releases/download/v0.10.2/libduckdb-linux-amd64.zip -O /tmp/duckdb.zip \
 && unzip -q /tmp/duckdb.zip -d /tmp/duckdb \
 && cp /tmp/duckdb/libduckdb.so /usr/local/lib/ \
 && cp /tmp/duckdb/duckdb.h /usr/local/include/ \
 && rm -rf /tmp/duckdb /tmp/duckdb.zip

COPY . .

RUN CGO_ENABLED=1 go build -ldflags="-s -w" -o /out/nat-query-service main.go

FROM debian:bookworm-slim

RUN apt-get update && apt-get install -y --no-install-recommends \
    ca-certificates \
    libgomp1 \
    libstdc++6 \
 && rm -rf /var/lib/apt/lists/*

COPY --from=builder /usr/local/lib/libduckdb.so /usr/local/lib/libduckdb.so
COPY --from=builder /out/nat-query-service /opt/nat-query/nat-query-service

RUN ldconfig \
 && mkdir -p /data/sangfor_fw_log /data/index /opt/nat-query

ENV LOG_DIR=/data/sangfor_fw_log
ENV DB_FILE=/data/index/nat_logs.duckdb
ENV PORT=8080
ENV WORKERS=4
ENV AUTO_SCAN_ENABLED=true
ENV AUTO_SCAN_INTERVAL_SEC=30
ENV GIN_MODE=release

WORKDIR /opt/nat-query

EXPOSE 8080

VOLUME ["/data/sangfor_fw_log", "/data/index"]

CMD ["/opt/nat-query/nat-query-service"]
