# Rogue Network

RGN is an EVM-compatible blockchain secured by FORGE Scrypt merged mining.

**FORGE V5 activates at block 1,121,800 — estimated 27 October 2026, 13:35 UTC.**

All RGN nodes, including exchange nodes, must upgrade before block **1,121,800**.

## Run a node with Docker

```sh
docker pull roguenetwork/rogue-network:4.1.0
docker volume create rgn-data
docker run -d --name rgn-node --restart unless-stopped --stop-timeout 120 \
  -p 127.0.0.1:18545:18545 -p 30303:30303/tcp -p 30303:30303/udp \
  -v rgn-data:/var/lib/rgn roguenetwork/rogue-network:4.1.0
```

For a native Linux installation, use the
[node package and installation guide](https://github.com/rogueisles/Rogue-Node).

## Upgrade an existing Docker node

Use the existing data volume and retain any custom container settings. This
example uses the container `rgn-node` and named volume `rgn-data`.

```sh
docker pull roguenetwork/rogue-network:4.1.0
docker stop -t 120 rgn-node
docker rm rgn-node
docker run -d --name rgn-node --restart unless-stopped --stop-timeout 120 \
  -p 127.0.0.1:18545:18545 -p 30303:30303/tcp -p 30303:30303/udp \
  -v rgn-data:/var/lib/rgn roguenetwork/rogue-network:4.1.0
docker exec rgn-node python3 /opt/rgn/docker/health.py
```

Preserve the existing database. Confirm that the node has peers and its height
continues to advance after startup.

## Build from source

The Dockerfile pins the Linux build environment and dependencies.

```sh
git clone --branch v4.1.0 https://github.com/rogueisles/Rogue-Network.git
cd Rogue-Network
docker build -t rgn-node-source:4.1.0 .
```

For a native build, use Go **1.16.15**, GCC 11 and CGO on Ubuntu 22.04:

```sh
export GO111MODULE=off CGO_ENABLED=1 GOMAXPROCS=2
export CGO_CFLAGS='-O2 -g0' CGO_CXXFLAGS='-O2 -g0'
mkdir -p build/bin
bash build/env.sh /path/to/go1.16.15/bin/go build -trimpath \
  -ldflags='-buildid= -X main.gitCommit=rgn400v5' \
  -o build/bin/rgn-node-linux-amd64 ./cmd/egem
```

## Changes in 4.1.0

- Optional SOQ merged-mining work alongside DGB and DOGE.
- Registry support for compatible future coins and treasury rotation.
- A 12.5-second target from block 1,121,800.

Genesis, chain ID 70088, existing balances and rewards per block remain unchanged.
Future coin additions must fit the installed proof profiles; new verification
formats can require a node upgrade.

## Documentation

- [Protocol reference](docs/FORGE-V5-REGISTRY.md)
- [Mainnet configuration](config/RGN-mainnet-v4.1.0-H1121800.json)
- [Pool integration](https://github.com/rogueisles/Mining-Pool)
- [Docker configuration](https://github.com/rogueisles/Rogue-Docker-Node)
- [Atlas](https://rogueisles.com/atlas/) · [Explorer](https://scan.rogueisles.com/)

EGEM/go-ethereum attribution and GPL/LGPL notices are retained in `AUTHORS`,
`COPYING`, `COPYING.LESSER` and the source headers.
