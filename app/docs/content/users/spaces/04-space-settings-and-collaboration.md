---
title: Space Settings and Collaboration
section: spaces
order: 4
summary: Configure a space, share it across your devices, and control who can access it.
---

## Overview

The space settings panel lets you rename a space, configure how it opens, see which plugins are installed, and reach management tools like the object browser and data export. Open it by clicking a space in the bottom bar and selecting the settings view.

## Prerequisites

- A space must be open. Settings are per-space, so each space has its own configuration.

## Steps

### Renaming a space

The settings panel shows the space name at the top. Edit it to rename the space. The new name syncs to all your linked devices automatically. Renaming does not touch the data inside the space.

### Configuring the index path

The Index Path selector determines which object opens by default when someone navigates to the space. Pick any object from the dropdown, or leave it empty for no default. Spaces with an index path set jump straight to the chosen object, useful for spaces that should always open to a particular canvas, repository, or file browser view.

### Viewing installed plugins

The panel lists the plugins installed in the space. Plugins determine which object types the space supports. You can manage which plugins are active from this section.

### Managing space data

The panel links to data management tools: the [object browser](/docs/users/spaces/object-browser), space details (including the underlying shared object identifier), and data export options.

## Sharing and Collaboration

Spaces are shared across *your* devices by default, and with other people through organizations. There are no per-space permission settings to configure.

### Across your own devices

Every space is automatically available on all of your linked devices. Create a space on your laptop, open it from your phone, edit it on your tablet. Changes sync in the background with no upload step and no sync button. If you make edits on two devices while offline, both sets of changes are kept when the devices reconnect.

To add a new device, open device linking from account settings and follow the steps in [Link Devices](/docs/users/devices/link-devices).

### With other people

To give another person access to a specific space, transfer it to an [organization](/docs/users/organizations/organizations). Every member of the organization has access to spaces the org owns. To move an existing personal space to an org, see [Transfer Space Ownership](/docs/users/organizations/transfer-space-ownership).

### Who can actually read your data

Access is controlled by encryption keys stored on your linked devices and, for shared spaces, on the devices of your org members. Only devices with the right keys can read the data. If you use cloud backup, the cloud server only ever sees encrypted blocks it cannot decrypt.

## Verify

Changes to settings like rename or index path take effect immediately and sync to all linked devices. Open the space in a fresh tab to confirm.

## Troubleshooting

- **Index path has no effect.** The selected object may have been deleted. If so, the space opens to its default view.
- **Cannot see settings.** Make sure you are clicking the space in the bottom bar, not just opening space content.
- **Space not showing up on another device.** Confirm both devices are linked under the same account and the new device has finished initial sync.

## Next Steps

- [Browse objects in a space](/docs/users/spaces/object-browser)
- [Link another device](/docs/users/devices/link-devices)
- [Transfer space ownership](/docs/users/organizations/transfer-space-ownership)
