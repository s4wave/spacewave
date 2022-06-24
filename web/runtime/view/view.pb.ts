/* eslint-disable */
export const protobufPackage = 'web.runtime.view'

/** WebViewRenderer is exposed by the WebView renderer. */
export interface WebViewRenderer {}

export class WebViewRendererClientImpl implements WebViewRenderer {
  private readonly rpc: Rpc
  constructor(rpc: Rpc) {
    this.rpc = rpc
  }
}

/** WebViewRenderer is exposed by the WebView renderer. */
export type WebViewRendererDefinition = typeof WebViewRendererDefinition
export const WebViewRendererDefinition = {
  name: 'WebViewRenderer',
  fullName: 'web.runtime.view.WebViewRenderer',
  methods: {},
} as const

/** WebViewHost is exposed by the Go Runtime for the WebView to call. */
export interface WebViewHost {}

export class WebViewHostClientImpl implements WebViewHost {
  private readonly rpc: Rpc
  constructor(rpc: Rpc) {
    this.rpc = rpc
  }
}

/** WebViewHost is exposed by the Go Runtime for the WebView to call. */
export type WebViewHostDefinition = typeof WebViewHostDefinition
export const WebViewHostDefinition = {
  name: 'WebViewHost',
  fullName: 'web.runtime.view.WebViewHost',
  methods: {},
} as const

interface Rpc {
  request(
    service: string,
    method: string,
    data: Uint8Array
  ): Promise<Uint8Array>
}
