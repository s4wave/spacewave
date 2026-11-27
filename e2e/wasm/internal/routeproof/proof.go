// Package routeproof checks direct-route mount evidence through the browser resource API.
package routeproof

import (
	"testing"

	playwright "github.com/mxschmitt/playwright-go"
)

// AssertStartupMarks requires every mount stage and, for Drive, the visible UnixFS file.
func AssertStartupMarks(t testing.TB, page playwright.Page, drive bool) {
	// Select the marks required by both direct routes.
	t.Helper()
	required := []string{
		"quickstart.session-mount-start",
		"quickstart.session-mount-ready",
		"quickstart.shared-object-mount-start",
		"quickstart.shared-object-mount-ready",
		"quickstart.shared-object-body-mount-start",
		"quickstart.shared-object-body-mount-ready",
		"quickstart.space-resource-created",
		"quickstart.space-world-access-ready",
		"quickstart.space-contents-mount-ready",
	}
	if drive {
		required = append(required, "unixfs.browser-mounted", "unixfs.seeded-file-visible")
	}

	// Require each selected mount stage in the browser's startup evidence.
	assertStartupMarks(t, page, required)
}

// assertStartupMarks checks the labels emitted by the browser startup probe.
func assertStartupMarks(t testing.TB, page playwright.Page, required []string) {
	// Read the startup marks from the page.
	t.Helper()

	// Evaluate the startup marks script and validate its shape.
	raw, err := page.Evaluate(`() => (globalThis.__swStartupMarks ?? []).map((mark) => ({
		label: mark.label ?? mark.name ?? '',
		detail: mark.detail ?? {},
	}))`, nil)
	if err != nil {
		t.Fatalf("read startup marks: %v", err)
	}
	marks, ok := raw.([]any)
	if !ok {
		t.Fatalf("unexpected startup marks %T: %#v", raw, raw)
	}
	labels := make([]string, 0, len(marks))
	seen := make(map[string]bool, len(marks))
	for _, mark := range marks {
		m, ok := mark.(map[string]any)
		if !ok {
			t.Fatalf("unexpected startup mark %T: %#v", mark, mark)
		}
		label := stringField(m, "label")
		labels = append(labels, label)
		seen[label] = true
	}

	// Assert every required startup mark is present.
	for _, label := range required {
		if !seen[label] {
			t.Fatalf("direct route missing startup mark %q; labels=%v", label, labels)
		}
	}
}

// AssertFSHandleOperations checks file, directory, watch, and upload operations through browser resources.
func AssertFSHandleOperations(t testing.TB, page playwright.Page) {
	// Run the FSHandle browser resource operation proof.
	t.Helper()

	// Evaluate the FSHandle operation proof script.
	raw, err := page.Evaluate(`async () => {
		function streamFromText(text) {
			return new ReadableStream({
				start(controller) {
					controller.enqueue(new TextEncoder().encode(text))
					controller.close()
				},
			})
		}
		async function readText(handle, length = 0n) {
			const read = await handle.readAt(0n, length)
			return new TextDecoder().decode(read.data)
		}
		async function waitForWatchEntry(iter, name) {
			const deadline = performance.now() + 15000
			for (;;) {
				const remaining = deadline - performance.now()
				if (remaining <= 0) {
					throw new Error('watchReaddir did not observe ' + name)
				}
				const next = await Promise.race([
					iter.next(),
					new Promise((_, reject) => {
						setTimeout(() => reject(new Error('watchReaddir timed out for ' + name)), Math.min(remaining, 1000))
					}),
				])
				if (next.done) {
					throw new Error('watchReaddir ended before observing ' + name)
				}
				const names = (next.value ?? []).map((entry) => entry.name ?? '')
				if (names.includes(name)) {
					return names
				}
			}
		}
		async function nextWithTimeout(iter, label, timeoutMS = 5000) {
			return await Promise.race([
				iter.next(),
				new Promise((_, reject) => {
					setTimeout(() => reject(new Error(label + ' timed out')), timeoutMS)
				}),
			])
		}
		async function expectWatchClosed(iter) {
			const isAbort = (err) => {
				const text = String(err?.name ?? '') + ' ' + String(err?.message ?? err)
				return /abort/i.test(text)
			}
			try {
					const next = await nextWithTimeout(iter, 'watchReaddir close')
				if (!next.done) {
					throw new Error('watchReaddir yielded after abort')
				}
			} catch (err) {
				if (!isAbort(err)) throw err
			} finally {
				if (typeof iter.return === 'function') {
					try {
						await iter.return()
					} catch (err) {
						if (!isAbort(err)) throw err
					}
				}
			}
		}
		const match = window.location.hash.match(/^#\/u\/([0-9]+)\/so\/([^/]+)/)
		const debug = globalThis.__s4wave_debug
		const root = debug?.root
		const mountSpace = debug?.mountSpace
		const FSHandle = debug?.FSHandle
		const unixfsObjectKey = debug?.UNIXFS_OBJECT_KEY
		const mknodType = debug?.MknodType ?? { FILE: 1, DIR: 2 }
		if (!match || !root || !mountSpace || !FSHandle || !unixfsObjectKey) {
			return { error: 'missing direct Drive route or debug FSHandle context' }
		}
		const sessionIdx = Number(match[1])
		const sharedObjectId = decodeURIComponent(match[2])
		const cleanupStack = []
		const cleanup = (resource) => {
			cleanupStack.push(resource)
			return resource
		}
		const mountedResources = {
			session: null,
			space: null,
			world: null,
			rootHandle: null,
		}
		let watchAbort = null
		let step = 'mount-session'
		try {
			const abort = AbortSignal.timeout(120000)
			const mounted = await root.mountSessionByIdx({ sessionIdx }, abort)
			mountedResources.session = mounted?.session ?? null
			if (!mountedResources.session) return { error: 'mountSessionByIdx returned no session' }
			step = 'mount-space'
			mountedResources.space = await mountSpace({
				session: mountedResources.session,
				spaceResp: {
					sharedObjectRef: {
						providerResourceRef: {
							id: sharedObjectId,
						},
					},
				},
				abortSignal: abort,
				cleanup,
			})
			step = 'access-world'
			mountedResources.world = await mountedResources.space.accessWorldState(true, abort)
			step = 'access-unixfs'
			const access = await mountedResources.world.accessTypedObject(unixfsObjectKey, abort)
			if (!access?.resourceId) return { error: 'accessTypedObject returned no UnixFS resource id' }
			mountedResources.rootHandle = new FSHandle(
				mountedResources.world.getResourceRef().createRef(access.resourceId),
			)

			const rootHandle = mountedResources.rootHandle
			step = 'read-starter-file'
			const starter = await rootHandle.lookup('getting-started.md', abort)
			const starterText = await readText(starter)
			starter.release()
			if (!starterText.includes('Getting Started')) {
				return { error: 'starter file read did not contain expected text' }
			}

			const initialEntries = (await rootHandle.readdirAll(0n, abort)).map((entry) => entry.name ?? '')
			if (!initialEntries.includes('getting-started.md')) {
				return { error: 'root readdir did not include starter file' }
			}

			step = 'watch-create-remove'
			watchAbort = new AbortController()
			const watchIter = rootHandle.watchReaddir(watchAbort.signal)[Symbol.asyncIterator]()
			await nextWithTimeout(watchIter, 'initial watchReaddir snapshot')
			await rootHandle.mknod(['row2-watch-file.txt'], mknodType.FILE, 0o644, true, abort)
			const watchedNames = await waitForWatchEntry(watchIter, 'row2-watch-file.txt')
			watchAbort.abort()
			await expectWatchClosed(watchIter)
			await rootHandle.remove(['row2-watch-file.txt'], abort)

			step = 'file-create-write-truncate'
			await rootHandle.mknod(['row2-created.txt'], mknodType.FILE, 0o644, true, abort)
			let file = await rootHandle.lookup('row2-created.txt', abort)
			await file.writeAt(0n, new TextEncoder().encode('abcdef'), abort)
			if (await readText(file) !== 'abcdef') {
				return { error: 'write/read round trip mismatch' }
			}
			await file.truncate(3n, abort)
			const truncatedText = await readText(file)
			const truncatedSize = await file.getSize(abort)
			file.release()
			if (truncatedText !== 'abc' || truncatedSize !== 3n) {
				return { error: 'truncate mismatch: ' + truncatedText + ' size=' + truncatedSize.toString() }
			}

			step = 'directory-create-read'
			await rootHandle.mkdirAll(['row2-dir', 'nested'], 0o755, abort)
			const nested = await rootHandle.lookupPath('row2-dir/nested', abort)
			const nestedEntries = await nested.handle.readdirAll(0n, abort)
			nested.handle.release()
			if (!Array.isArray(nestedEntries)) {
				return { error: 'nested directory readdir did not return entries' }
			}

			step = 'rename-remove'
			await rootHandle.rename('row2-created.txt', 'row2-renamed.txt', 0, abort)
			file = await rootHandle.lookup('row2-renamed.txt', abort)
			if (await readText(file, 3n) !== 'abc') {
				return { error: 'renamed file read mismatch' }
			}
			file.release()
			await rootHandle.remove(['row2-renamed.txt'], abort)
			const row2Dir = await rootHandle.lookup('row2-dir', abort)
			await row2Dir.remove(['nested'], abort)
			row2Dir.release()
			await rootHandle.remove(['row2-dir'], abort)

			step = 'upload-file'
			const uploadedFileBytes = await rootHandle.uploadFile(
				'row2-upload.txt',
				6n,
				streamFromText('upload'),
				0o644,
				undefined,
				abort,
			)
			const uploadedFile = await rootHandle.lookup('row2-upload.txt', abort)
			const uploadedFileText = await readText(uploadedFile)
			uploadedFile.release()
			if (uploadedFileBytes !== 6n || uploadedFileText !== 'upload') {
				return { error: 'single-file upload mismatch' }
			}

			step = 'read-stream'
			const largeBytes = new Uint8Array(200000)
			for (let i = 0; i < largeBytes.length; i++) {
				largeBytes[i] = (i * 31 + 7) & 0xff
			}
			await rootHandle.uploadFile(
				'row2-large.bin',
				BigInt(largeBytes.length),
				new ReadableStream({
					start(controller) {
						controller.enqueue(largeBytes)
						controller.close()
					},
				}),
				0o644,
				undefined,
				abort,
			)
			const largeFile = await rootHandle.lookup('row2-large.bin', abort)
			const largeRead = await largeFile.readAt(0n, 0n, abort)
			const rangeRead = await largeFile.readAt(1000n, 140000n, abort)
			largeFile.release()
			await rootHandle.remove(['row2-large.bin'], abort)
			function sameBytes(got, want) {
				return got.length === want.length && got.every((b, i) => b === want[i])
			}
			if (!largeRead.eof || !sameBytes(largeRead.data, largeBytes)) {
				return { error: 'multi-frame read mismatch' }
			}
			if (rangeRead.eof || !sameBytes(rangeRead.data, largeBytes.subarray(1000, 141000))) {
				return { error: 'multi-frame range read mismatch' }
			}

			step = 'upload-tree'
			const treeUpload = await rootHandle.uploadTree(
				[
					{ kind: 'directory', path: 'row2-tree', mode: 0o755 },
					{
						kind: 'file',
						path: 'row2-tree/leaf.txt',
						totalSize: 4n,
						stream: streamFromText('leaf'),
						mode: 0o644,
					},
					{ kind: 'directory', path: 'row2-tree/inner', mode: 0o755 },
					{
						kind: 'file',
						path: 'row2-tree/inner/nested.txt',
						totalSize: 6n,
						stream: streamFromText('nested'),
						mode: 0o644,
					},
				],
				undefined,
				abort,
			)
			const leaf = await rootHandle.lookupPath('row2-tree/leaf.txt', abort)
			const nestedUpload = await rootHandle.lookupPath('row2-tree/inner/nested.txt', abort)
			const leafText = await readText(leaf.handle)
			const nestedUploadText = await readText(nestedUpload.handle)
			leaf.handle.release()
			nestedUpload.handle.release()
			if (leafText !== 'leaf' || nestedUploadText !== 'nested') {
				return { error: 'tree upload read mismatch' }
			}

			const finalEntries = (await rootHandle.readdirAll(0n, abort)).map((entry) => entry.name ?? '').sort()
			return {
				starterRead: true,
				initialEntries,
				watchedNames,
				fileWriteRead: true,
				truncate: true,
				directoryRead: true,
				rename: true,
				remove: true,
				uploadFileBytes: uploadedFileBytes.toString(),
				treeUploadBytes: (treeUpload.bytesWritten ?? 0n).toString(),
				treeUploadFiles: (treeUpload.filesWritten ?? 0n).toString(),
				treeUploadDirectories: (treeUpload.directoriesWritten ?? 0n).toString(),
				finalEntries,
			}
		} catch (err) {
			return { error: String(err?.stack ?? err), step }
		} finally {
			watchAbort?.abort?.()
			mountedResources.rootHandle?.release?.()
			mountedResources.world?.release?.()
			while (cleanupStack.length) {
				cleanupStack.pop()?.release?.()
			}
			mountedResources.session?.release?.()
		}
	}`, nil)
	if err != nil {
		t.Fatalf("run FSHandle browser resource operation proof: %v", err)
	}
	result, ok := raw.(map[string]any)
	if !ok {
		t.Fatalf("unexpected FSHandle operation proof %T: %#v", raw, raw)
	}
	if errMsg := stringField(result, "error"); errMsg != "" {
		t.Fatalf("FSHandle operation proof failed at %s: %s", stringField(result, "step"), errMsg)
	}
	for _, key := range []string{
		"starterRead",
		"fileWriteRead",
		"truncate",
		"directoryRead",
		"rename",
		"remove",
	} {
		if !boolField(result, key) {
			t.Fatalf("FSHandle operation proof missing %s: %#v", key, result)
		}
	}
	if stringField(result, "uploadFileBytes") != "6" {
		t.Fatalf("FSHandle uploadFile bytes mismatch: %#v", result)
	}
	if stringField(result, "treeUploadBytes") != "10" {
		t.Fatalf("FSHandle uploadTree bytes mismatch: %#v", result)
	}
}

// AssertSpaceState checks readiness and the content contract of a direct route.
func AssertSpaceState(t testing.TB, page playwright.Page, requireContents bool) {
	// Probe the direct route Space state.
	t.Helper()

	// Evaluate the direct route Space state script.
	raw, err := page.Evaluate(`async () => {
		async function firstStreamValue(stream) {
			for await (const value of stream) {
				return value
			}
			return null
		}
		const match = window.location.hash.match(/^#\/u\/([0-9]+)\/so\/([^/]+)/)
		const debug = globalThis.__s4wave_debug
		const root = debug?.root
		const mountSpace = debug?.mountSpace
		if (!match || !root || !mountSpace) {
			return { error: 'missing direct route or debug root' }
		}
		const sessionIdx = Number(match[1])
		const sharedObjectId = decodeURIComponent(match[2])
		const mountedResources = {
			session: null,
			space: null,
		}
		const cleanupStack = []
		const cleanup = (resource) => {
			cleanupStack.push(resource)
			return resource
		}
		try {
			const abort = AbortSignal.timeout(15000)
			const mounted = await root.mountSessionByIdx({ sessionIdx }, abort)
			mountedResources.session = mounted?.session ?? null
			if (!mountedResources.session) return { error: 'mountSessionByIdx returned no session' }
			mountedResources.space = await mountSpace({
				session: mountedResources.session,
				spaceResp: {
					sharedObjectRef: {
						providerResourceRef: {
							id: sharedObjectId,
						},
					},
				},
				abortSignal: abort,
				cleanup,
			})
			const state = await firstStreamValue(mountedResources.space.watchSpaceState({}, abort))
			const world = cleanup(await mountedResources.space.accessWorldState(true, abort))
			const listing = await world.listObjects({ limit: 100 }, abort)
			return {
				ready: !!state?.ready,
				indexPath: state?.settings?.indexPath ?? '',
				objectKeys: (listing.objects ?? [])
					.map((obj) => obj.objectKey ?? '')
					.filter((key) => !key.startsWith('types/')),
			}
		} catch (err) {
			return { error: String(err?.stack ?? err) }
		} finally {
			while (cleanupStack.length) {
				cleanupStack.pop()?.release?.()
			}
			mountedResources.session?.release?.()
		}
	}`, nil)
	if err != nil {
		t.Fatalf("read direct route Space state: %v", err)
	}
	result, ok := raw.(map[string]any)
	if !ok {
		t.Fatalf("unexpected direct route Space state %T: %#v", raw, raw)
	}
	if errMsg := stringField(result, "error"); errMsg != "" {
		t.Fatalf("direct route Space state probe failed: %s", errMsg)
	}
	if !boolField(result, "ready") {
		t.Fatalf("direct route Space state was not ready: %#v", result)
	}

	// Keep the distinct content contract for the selected route.
	if requireContents {
		objectKeys, ok := result["objectKeys"].([]any)
		if !ok || len(objectKeys) == 0 {
			t.Fatalf("direct route Space state had no world contents: %#v", result)
		}
	} else if indexPath := stringField(result, "indexPath"); indexPath != "" {
		t.Fatalf("direct Space route indexPath=%q want empty: %#v", indexPath, result)
	}
}

// stringField reads a string-valued browser result field.
func stringField(value map[string]any, key string) string {
	result, _ := value[key].(string)
	return result
}

// boolField reads a boolean-valued browser result field.
func boolField(value map[string]any, key string) bool {
	result, _ := value[key].(bool)
	return result
}
