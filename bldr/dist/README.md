# Distribution resources

A native distribution directory contains the executable and `assets.kvfile`.
Distribute and install the directory together. The executable resolves the
volume beside its actual executable path, including when launched through a
symlink or from another working directory. Application bundles should keep the
volume beside their contained executable.

The volume holds the distribution's World and plugin manifests. It stays out
of Go package archives, so editing assets does not make the Go compiler cache
another copy of the payload. Small bootstrap configuration remains embedded.
The existing block reader opens the volume lazily and checks that the expected
root exists. A missing or mismatched volume fails startup.

Web distributions continue to serve their volume separately through the browser
range reader. Both packers include reachable blocks only; unrelated blocks left
in a build store do not become distribution assets.

Set `SOURCE_DATE_EPOCH` to a fixed Unix timestamp for reproducible manifest
construction and distribution copying. Without it, the builder uses the current
time. Reproducibility also requires identical source manifests, configuration,
and toolchain inputs. Existing installed binaries keep their original embedded
resources; this layout applies to newly built native distributions.
