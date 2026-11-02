# Disposable OVH guest

Keep this experiment off the default branch. Provision exactly one explicitly
approved hourly VM. Check the live CLI help, regional flavor/image availability,
credit and the full compute plus IPv4 rate before allocation. Reuse the operator's
existing project SSH public key. A stopped guest may still bill: deletion and a
reconciled provider inventory establish the end of the allocation.

## Recovery and setup

Keep ordinary Debian recovery boot and SSH intact. Never format a physical disk,
change the bootloader, install an autostart experiment, or boot the host directly
into this prototype. All experimental paths live under `/srv/spacewave-os/`.

Establish the SSH host public key through an authenticated provider channel before
connecting. The OVH CLI prints final create parameters even without debug, so
**never pass private keys or other secrets in CLI user-data**. Use a non-echoing
authenticated API client if a future provision needs secret user-data. Pin the
verified public key in a dedicated known-hosts file with strict checking enabled.
Keep provider credentials and the operator's login private key out of the guest.

Copy this directory to the new VM, then run:

```sh
sudo sh prepare-guest.sh EXPECTED_INSTANCE_UUID
```

The UUID guard rejects the wrong guest before installation. The script suppresses
package service startup, avoids upgrades of already installed requested tools,
and creates separate `boot/`, `storage/`, `results/` and `src/` directories. It
loads FUSE, loop and Btrfs modules for this session. Re-run `modprobe` after a
restart if needed; it deliberately adds no boot-time module or service setup.

Use the distro kernel in `/boot/vmlinuz-$(uname -r)` and matching modules to build
a separate nested initramfs. The boot worker owns the experimental kernel,
initramfs and QEMU command. Use at most one virtual CPU and 768 MiB guest RAM on
this 2 GiB VM. Prefer `-enable-kvm -cpu host`, serial output, `-display none`,
`-nic none` and no physical disks. Confirm actual KVM startup, not just `/dev/kvm`.

Long-lived runs belong in an explicit tmux session, for example
`spacewave-os-boot`. Record its command, log and image paths. Stop QEMU before
detaching or deleting its images. Preserve SSH outside the nested guest.

## Storage baseline

`../storage/README.md` explains the bounded file-image comparison and recovery
check. Its results are native Linux filesystems over loopback, not UnixFS speed.
Transfer source/build artifacts from a build host; do not build on the Mac.
