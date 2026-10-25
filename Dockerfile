FROM ubuntu:22.04@sha256:0a9887931b0e38af2e3487ed39e29436b0535c4f3f52e57745baaf79d1898956 AS builder
RUN apt-get update && apt-get install -y --no-install-recommends gcc g++ libc6-dev ca-certificates curl && rm -rf /var/lib/apt/lists/*
ARG GO_ARCHIVE_URL=https://go.dev/dl/go1.16.15.linux-amd64.tar.gz
RUN curl -fsSL "$GO_ARCHIVE_URL" -o /tmp/go.tar.gz && echo '77c782a633186d78c384f972fb113a43c24be0234c42fef22c2d8c4c4c8e7475  /tmp/go.tar.gz' | sha256sum -c - && tar -xzf /tmp/go.tar.gz -C / && rm /tmp/go.tar.gz
ENV PATH=/go/bin:/usr/bin:/bin GOROOT=/go GO111MODULE=off CGO_ENABLED=1 GOMAXPROCS=2 CGO_CFLAGS="-O2 -g0" CGO_CXXFLAGS="-O2 -g0"
WORKDIR /node
COPY . /node/
RUN mkdir /release && bash build/env.sh /go/bin/go build -trimpath -ldflags='-buildid= -X main.gitCommit=rgn400v5' -o /release/egem ./cmd/egem && cp config/RGN-mainnet-v4.1.0-H1121800.json /release/RGN-mainnet-v4.1.0-H1121800.json && cp docker/rgn-node.toml /release/ && cd /release && sha256sum egem RGN-mainnet-v4.1.0-H1121800.json rgn-node.toml > PACKAGE-SHA256SUMS

FROM ubuntu:22.04@sha256:0a9887931b0e38af2e3487ed39e29436b0535c4f3f52e57745baaf79d1898956
LABEL org.opencontainers.image.title="Rogue Network node"  org.opencontainers.image.version="4.1.0"  org.opencontainers.image.revision="4bd6206e1bf07290bc028285858dff87d8c8488f51dc423e9f6d7ef05adb435d"  org.opencontainers.image.source="https://github.com/rogueisles/Rogue-Network"  org.opencontainers.image.description="FORGE V5 activation at RGN block 1121800; optional SOQ and bounded registry"
RUN apt-get update && apt-get install -y --no-install-recommends python3 ca-certificates  && rm -rf /var/lib/apt/lists/*  && groupadd --gid 10001 rgn  && useradd --uid 10001 --gid 10001 --home-dir /var/lib/rgn --no-create-home --shell /usr/sbin/nologin rgn  && mkdir -p /var/lib/rgn && chown 10001:10001 /var/lib/rgn
COPY --from=builder /release/ /opt/rgn/
COPY docker/entrypoint.py docker/health.py /opt/rgn/docker/
ENV RGN_DATADIR=/var/lib/rgn RGN_CACHE_MB=512 RGN_MAX_PEERS=25 RGN_P2P_PORT=30303 RGN_RPC_PORT=18545
WORKDIR /opt/rgn
VOLUME ["/var/lib/rgn"]
EXPOSE 18545/tcp 30303/tcp 30303/udp
USER 10001:10001
STOPSIGNAL SIGINT
HEALTHCHECK --interval=30s --timeout=15s --start-period=180s --retries=3 CMD ["python3", "/opt/rgn/docker/health.py", "--quiet"]
ENTRYPOINT ["python3", "/opt/rgn/docker/entrypoint.py"]
