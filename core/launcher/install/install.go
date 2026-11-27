// Package launcher_install installs the desktop launcher for the current user.
//
// A downloaded Windows launcher copies itself and its embedded helper to the
// user's Programs folder, adds a Start menu entry and a Settings > Apps entry,
// and needs no administrator rights. Each file is published under its final
// name only when that name is free, so concurrent launches install once
// without a lock, and the launcher is published last so its presence marks a
// complete install.
package launcher_install

// The resource objects give every Windows build importing this package the
// app icon and a manifest selecting Common Controls 6 for its dialogs. A
// release sets SPACEWAVE_VERSION to stamp its version, which the desktop reads
// to tell whether a staged update is newer. The committed objects carry none.
//go:generate go run github.com/tc-hib/go-winres@v0.3.3 simply --arch amd64,arm64 --out rsrc --manifest gui --icon ../../../web/images/spacewave-icon.png --product-name Spacewave --file-description Spacewave --original-filename Spacewave.exe --product-version=$SPACEWAVE_VERSION --file-version=$SPACEWAVE_VERSION
