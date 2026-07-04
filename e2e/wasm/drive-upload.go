//go:build !skip_e2e && !js

package wasm

import (
	"encoding/base64"
	"testing"

	playwright "github.com/mxschmitt/playwright-go"
)

// UploadViaPicker uploads files through the hidden UnixFS file picker input.
func UploadViaPicker(t testing.TB, page playwright.Page, files []playwright.InputFile) {
	t.Helper()

	err := page.Locator("[data-testid='unixfs-upload-input']").First().SetInputFiles(files)
	if err != nil {
		t.Fatalf("upload via picker: %v", err)
	}
}

// UploadPathsViaPicker uploads existing local files through the hidden UnixFS
// file picker input. Playwright transports path-backed files without the
// 50 MiB in-memory buffer ceiling used for InputFile.Buffer.
func UploadPathsViaPicker(t testing.TB, page playwright.Page, paths []string) {
	t.Helper()

	err := page.Locator("[data-testid='unixfs-upload-input']").First().SetInputFiles(paths)
	if err != nil {
		t.Fatalf("upload paths via picker: %v", err)
	}
}

// UploadViaDnd uploads files by dispatching a native file drop onto UnixFS.
func UploadViaDnd(t testing.TB, page playwright.Page, files []playwright.InputFile) {
	t.Helper()

	payload := make([]map[string]string, 0, len(files))
	for _, file := range files {
		payload = append(payload, map[string]string{
			"name":     file.Name,
			"mimeType": file.MimeType,
			"data":     base64.StdEncoding.EncodeToString(file.Buffer),
		})
	}

	_, err := page.Evaluate(`async ({ files }) => {
		const target = document.querySelector('[data-testid="unixfs-upload-drop-target"]')
		if (!(target instanceof HTMLElement)) {
			throw new Error('unixfs upload drop target not found')
		}
		const transfer = new DataTransfer()
		for (const file of files) {
			const bytes = Uint8Array.from(atob(file.data), (ch) => ch.charCodeAt(0))
			transfer.items.add(new File([bytes], file.name, { type: file.mimeType || 'application/octet-stream' }))
		}
		for (const type of ['dragenter', 'dragover', 'drop']) {
			target.dispatchEvent(new DragEvent(type, {
				bubbles: true,
				cancelable: true,
				dataTransfer: transfer,
			}))
		}
	}`, map[string]any{"files": payload})
	if err != nil {
		t.Fatalf("upload via dnd: %v", err)
	}
}
