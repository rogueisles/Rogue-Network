# Docker build

Build the node from this source on Linux AMD64:

```bash
docker build --platform linux/amd64 -t roguenetwork/rogue-network:soq-rc1-20261025 .
```

The build verifies the pinned Go archive and compiles inside Ubuntu 22.04.
The final image runs as UID/GID 10001. It uses the unchanged public mainnet
configuration: optional SOQ activation is absent. The startup helper checks
the bundled binary/configuration hashes before using persistent storage.
Rebuilding modified code requires updating the binary checksum in that helper;
a mismatched binary deliberately fails startup.

The release bundle also supplies a small prebuilt-payload Docker context,
container qualification evidence and run/publish instructions. Use a named
volume and bind HTTP RPC to localhost. WALLET selects the work beneficiary;
it does not start a miner. Existing production tags must not be overwritten
by this candidate. No SOQ activation is scheduled by building this image.
