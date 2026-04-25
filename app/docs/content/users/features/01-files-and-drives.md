---
title: Files and Drives
section: features
order: 1
summary: File browser, drag-drop upload, and sync behavior.
---

## Overview

A Drive space gives you a file browser for storing, organizing, and syncing files across your devices. It works like the file manager on your computer: folders, files, drag-and-drop uploads, and right-click menus.

## Prerequisites

- Create a Drive space from the dashboard quickstart, or add the file browser plugin to an existing space.

## Steps

### Navigating files

The file browser shows your files and folders in a list. Click a folder to open it. The path bar at the top shows your current location, and each segment is clickable to jump back up the tree. Use the toolbar's back and forward buttons to retrace your steps.

### Uploading files

Click the upload button in the toolbar, or drag files directly into the browser window. You can upload individual files, multiple files at once, or entire folders. A progress indicator appears in the bottom bar while uploads are in progress, showing the file count and overall progress.

Files are encrypted and saved to your device's local storage automatically. There is no file size limit. Large files are split into small pieces internally, so multi-gigabyte uploads proceed smoothly.

### Managing files

Right-click any file or folder to see available actions:

- **Open** navigates into a folder or opens a file in the inline viewer.
- **Download** saves a copy to your device's downloads folder.
- **Rename** lets you change the name in place.
- **Delete** removes the file from the space.

Right-clicking the empty background gives you options to create a new folder or upload files.

### Creating folders

Right-click the browser background and select **New folder**, or use the toolbar. You can nest folders as deeply as you want. The folder structure syncs to all linked devices.

### Viewing files inline

Click any file to preview it. Common formats like images, text, markdown, and code display directly in the interface without an external application. For unsupported formats, you can download the file instead.

## Verify

After uploading, the new files appear in the file list. Navigate to a different folder and back to confirm they persist. On a linked device, the files appear automatically once sync completes.

## Troubleshooting

- **Upload seems stuck.** Check the progress indicator in the bottom bar. Very large files take time to split and encrypt. The upload continues in the background.
- **Files not appearing on another device.** Both devices must be online. Sync happens continuously when devices are connected. If one was offline, it catches up on reconnect.

## Next Steps

- [Learn about the object browser](/docs/users/spaces/object-browser)
- [Understanding spaces](/docs/users/spaces/understanding-spaces)
