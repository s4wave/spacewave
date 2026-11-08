# Gitea application proof

`TestGiteaQEMUFlowgraph` in `sdk/forge/world` runs an unmodified Gitea 1.25.5
inside the static-init guest. An existing Forge Worker admits a Docker container
that owns QEMU. The graph also contains a separate build Step, triggered through
`DeliverEvents` after a real Git push. The build clones the public fixture
repository, compiles its C program and returns the executable as a Forge output.

The test deliberately makes the first Docker stop unavailable. It checks that
the live VM still holds its capacity debit, restores stop, reconciles through the
Worker and requires a complete cleanup receipt. A second run of the same graph
starts a different VM execution on the original bindings. It checks the exact
issue, Git commit, configuration digest and retained build artifact.

## Inputs and storage

Use a fresh directory on an existing Linux development host with Docker and
KVM. Docker must grant `/dev/kvm` explicitly; ordinary host user access is not
required. The development image is built with:

```sh
docker build --network host -t spacewave-os-qemu - < os/qemu/Dockerfile
docker image inspect spacewave-os-qemu --format '{{.Id}}'
```

Use the returned immutable image ID for the test. The Dockerfile installs tools
from Debian's signed repositories; retain its image and package list to reproduce
that toolchain. It does not install a host service. A separately operated Docker
daemon may use a private socket and a named terminal session.

Prepare an `inputs` directory with these explicit files:

- `init`: the current static `spacewave-init`, built by `make -f os/Makefile build`.
- `gitea`: official `gitea-1.25.5-linux-amd64`, SHA256
  `1c86b689cd44f843299cb12b5b3eb1189d0638d5ffda04eb6b4dd0fe395422fc`.
- `apps.tar`: run `python3 package_apps.py /inputs/gitea /inputs/apps.tar` inside
  the tool image. The packager verifies the Gitea hash and includes Git and its
  loader dependencies from that image.
- `app-initramfs.cpio.gz`: package this same `init` using `os/make-initramfs.sh`.
- `kernel-root/boot/vmlinuz-6.12.116+deb13-amd64`: the kernel from Debian package
  `linux-image-6.12.116+deb13-amd64_6.12.116-1_amd64.deb`, SHA256
  `0cee1131391727e0974245f197cfb22f3d8715ac2428574b290aaf16ff6eda1c`.
- Include that kernel's ordered module dependency chain in the initramfs:
  virtio_blk, crc32c-intel, crc32c_generic, libcrc32c, raid6_pq, xor,
  blake2b_generic, btrfs, failover, net_failover, virtio_net. Use `modprobe
  --show-depends` against the extracted kernel to resolve paths. KVM selects the
  host CPU; TCG selects max, so required CPU instructions are available.
- `run_app.py` and `prepare_fixture.sh` from this directory.

Mount only this private preparation directory into a short-lived tool container
and run `sh /inputs/prepare_fixture.sh /inputs /fixtures/new-fixture`. The fixture
path must be absent. The script creates two 2 GiB sparse Btrfs images without a
host mount. Keep its directory writable by the test user.

The system disk contains the frozen executable image and a writable UnixFS World
for repository files. The second disk contains a separate UnixFS World for Gitea
configuration, generated keys and application files. Each guest reports the real
Volume ID and `os/data` object key. QEMU locks the raw files against another
writer on this host.

The test creates `host-world.s4wave` in the fixture. Its `sql/gitea` object stores
all relational metadata through normal World operations. A caller-owned MySQL
endpoint binds loopback; QEMU reaches it through its user-network gateway. This
endpoint lives across both VM executions. No external MySQL service is used.

## Run

Build the test binary on the build host if the execution host lacks a Go cache:

```sh
go test -p=4 -c -ldflags='-s -w' -o gitea-qemu.test ./sdk/forge/world
SPACEWAVE_QEMU_FIXTURE=/absolute/new-fixture \
SPACEWAVE_DOCKER=/absolute/docker \
SPACEWAVE_QEMU_IMAGE=sha256:the-built-image-id \
DOCKER_HOST=unix:///absolute/private-docker.sock \
./gitea-qemu.test -test.timeout=12m -test.v -test.run '^TestGiteaQEMUFlowgraph$'
```

The fixture and its SQL World survive failure for inspection. The test refuses
to reuse a fixture that already contains its SQL World. Retry with a fresh
fixture, retaining failed state separately. Runtime cleanup joins before the
endpoint and World close. Retain the test log and Docker logs before removing
these explicitly identified test containers.

Gitea runs as UID 1000 and uses its supported database issue indexer. Writable
UnixFS direct I/O does not support Bleve's shared writable mmap. The guest uses
`allow_other` and trusted fixture directory permissions; UnixFS does not yet
provide a multi-user ownership boundary. SSH is disabled; the proof uses HTTP.
Its fixed account is disposable and is never an operator credential.

This establishes clean replacement on one enrolled host. It does not establish
power-loss durability, a transaction across SQL and Git stores, guest Device
enrollment, a fully hosted Spacewave daemon, automatic Gitea webhook ingestion,
or general MySQL/POSIX compatibility.
