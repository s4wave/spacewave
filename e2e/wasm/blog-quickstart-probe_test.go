//go:build !skip_e2e && !js

package wasm

import (
	"fmt"
	"testing"
)

func TestBlogQuickstartSetupProbe(t *testing.T) {
	sess := testHarness.NewSession(t)
	page := sess.Page()

	WaitForApp(t, page)

	raw, err := page.Evaluate(`async () => {
		const waitForDebugContext = async () => {
			const deadline = Date.now() + 30000
			for (;;) {
				const ctx = globalThis.__s4wave_debug
				if (ctx) {
					return ctx
				}
				if (Date.now() > deadline) {
					throw new Error('debug context not initialized')
				}
				await new Promise((resolve) => requestAnimationFrame(resolve))
			}
		}
		const ctx = await waitForDebugContext()
		const cleanupResources = []
		const cleanup = (resource) => {
			if (
				resource &&
				typeof resource === 'object' &&
				typeof resource[Symbol.dispose] === 'function'
			) {
				cleanupResources.push(resource)
			}
			return resource
		}
		const abortController = new AbortController()
		const started = performance.now()
		try {
			const setup = await Promise.race([
				ctx.createQuickstartSetup(
					ctx.root,
					'blog',
					abortController.signal,
					cleanup,
				),
				new Promise((_, reject) =>
					setTimeout(
						() =>
							reject(
								new Error(
									'timeout waiting for createQuickstartSetup(blog)',
								),
							),
						30000,
					),
				),
			])
			return {
				elapsedMs: Math.round(performance.now() - started),
				sessionIndex: setup?.sessionIndex ?? null,
				spaceID:
					setup?.spaceResp?.sharedObjectRef?.providerResourceRef?.id ?? '',
				hasSession: !!setup?.session,
				hasSpace: !!setup?.space,
				hash: window.location.hash,
			}
		} finally {
			abortController.abort()
			for (let i = cleanupResources.length - 1; i >= 0; i--) {
				try {
					cleanupResources[i][Symbol.dispose]()
				} catch {}
			}
		}
	}`, nil)
	if err != nil {
		t.Fatalf("probe createQuickstartSetup(blog): %v", err)
	}
	t.Logf("blog quickstart setup probe: %#v", raw)

	result, ok := raw.(map[string]any)
	if !ok {
		t.Fatalf("unexpected probe result: %#v", raw)
	}
	sessionIndex, _ := result["sessionIndex"].(float64)
	spaceID, _ := result["spaceID"].(string)

	_, err = page.Evaluate(`({ hash }) => {
		window.location.hash = hash
	}`, map[string]any{
		"hash": fmt.Sprintf("#/u/%d/so/%s/-/blog/site", int(sessionIndex), spaceID),
	})
	if err != nil {
		t.Fatalf("navigate to blog route: %v", err)
	}

	viewRaw, err := page.Evaluate(`async () => {
		const deadline = Date.now() + 10000
		for (;;) {
			const reading = document.querySelector("button[title='Reading mode']")
			const editing = document.querySelector("button[title='Editing mode']")
			const body =
				document.body?.innerText
					?.replace(/\s+/g, ' ')
					.slice(0, 320) ?? ''
			if (reading && editing) {
				return {
					ready: true,
					hash: window.location.hash,
					body,
				}
			}
			if (Date.now() > deadline) {
				return {
					ready: false,
					hash: window.location.hash,
					body,
				}
			}
			await new Promise((resolve) => requestAnimationFrame(resolve))
		}
	}`, nil)
	if err != nil {
		t.Fatalf("probe blog route render: %v", err)
	}
	t.Logf("blog route probe: %#v", viewRaw)
}
