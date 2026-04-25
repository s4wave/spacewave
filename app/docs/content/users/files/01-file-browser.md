---
title: File Browser
section: files
order: 1
summary: Browse, upload, and manage files in your spaces.
---

## Navigating Files

The file browser works like the file manager on your computer. Folders and files are listed with their names, sizes, and types. Click a folder to open it. The path bar at the top shows where you are, and you can click any part of the path to jump back up.

Use the back and forward buttons in the toolbar to retrace your steps. The layout is designed to feel familiar: if you know how to use Finder or File Explorer, you already know how to use this.

## Uploading Files

Click the upload button or drag files directly into the browser window. You can upload single files, multiple files, or entire folders. A progress bar in the bottom bar shows how things are going.

Files are saved to your device's local storage and encrypted automatically. There is no file size limit. Large files are handled efficiently because they are broken into small pieces behind the scenes, so even multi-gigabyte files upload smoothly.

## Folder Structure

Create folders by right-clicking in the browser or using the toolbar. You can nest folders as deep as you want. Move files between folders by dragging them, or use right-click for cut, copy, and paste.

The folder structure syncs across all your linked devices. Create a folder on your laptop and it appears on your phone. Rename or move files on one device and the change shows up everywhere.

## File Actions

Right-click any file or folder to see what you can do with it:

- **Download** saves a copy to your device's downloads folder.
- **Rename** lets you change the name in place.
- **Delete** removes the file from the space.
- **Preview** opens supported file types directly in the browser.

Click any file to preview it inline. Common formats like images, text, and markdown display right in the interface without needing an external application.

## How Storage Works

Spacewave stores files differently from a traditional filesystem. Each file is split into small pieces, and each piece is identified by a fingerprint of its contents. If two files share identical sections, those sections are stored only once.

This has practical benefits for you: syncing a large file that had a small change is fast because only the changed pieces transfer. Backup and storage are efficient because duplicate content is never stored twice.
