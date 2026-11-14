import 'csstype'
import './web/react-css.d.ts'

// CSS
declare module '*.css'
declare module '*.module.css'
declare module '*.module.scss'

// Markdown
declare module 'markdown-to-jsx' {
  import type { FC } from 'react'
  interface MarkdownProps {
    children: string
    options?: Record<string, unknown>
  }
  const Markdown: FC<MarkdownProps>
  export default Markdown
}

// File loader
declare module '*.png' {
  const value: string
  export default value
}
declare module '*.svg' {
  const value: string
  export default value
}

// Augments csstype so React style objects accept the non-standard
// WebkitAppRegion property used for draggable native window chrome.
//
// The type parameters must stay identical to csstype's own
// StandardLonghandProperties declaration, or interface merging is rejected.
declare module 'csstype' {
  interface StandardLonghandProperties<TLength = (string & {}) | 0, TTime = string & {}> {
    WebkitAppRegion?: 'drag' | 'no-drag' | string
  }
}

declare global {
  // BLDR_DEBUG is set by the bldr bundler in debug builds.
  const BLDR_DEBUG: boolean | undefined

  // See: https://github.com/lukewarlow/user-agent-data-types#readme
  // WICG Spec: https://wicg.github.io/ua-client-hints

  interface Navigator extends NavigatorUA {}
  interface WorkerNavigator extends NavigatorUA {}

  // https://wicg.github.io/ua-client-hints/#navigatorua
  interface NavigatorUA {
    readonly userAgentData?: NavigatorUAData
  }

  // https://wicg.github.io/ua-client-hints/#dictdef-navigatoruabrandversion
  interface NavigatorUABrandVersion {
    readonly brand: string
    readonly version: string
  }

  // https://wicg.github.io/ua-client-hints/#dictdef-uadatavalues
  interface UADataValues {
    readonly brands?: NavigatorUABrandVersion[]
    readonly mobile?: boolean
    readonly platform?: string
    readonly architecture?: string
    readonly bitness?: string
    readonly formFactor?: string[]
    readonly model?: string
    readonly platformVersion?: string
    readonly uaFullVersion?: string
    readonly fullVersionList?: NavigatorUABrandVersion[]
    readonly wow64?: boolean
  }

  // https://wicg.github.io/ua-client-hints/#dictdef-ualowentropyjson
  interface UALowEntropyJSON {
    readonly brands: NavigatorUABrandVersion[]
    readonly mobile: boolean
    readonly platform: string
  }

  // https://wicg.github.io/ua-client-hints/#navigatoruadata
  interface NavigatorUAData extends UALowEntropyJSON {
    getHighEntropyValues(hints: string[]): Promise<UADataValues>
    toJSON(): UALowEntropyJSON
  }

  // The WebGPU flag namespaces, which the TypeScript DOM lib leaves out.
  // https://www.w3.org/TR/webgpu/#typedefdef-gpushaderstageflags
  const GPUShaderStage: {
    readonly VERTEX: 0x1
    readonly FRAGMENT: 0x2
    readonly COMPUTE: 0x4
  }

  // https://www.w3.org/TR/webgpu/#typedefdef-gpubufferusageflags
  const GPUBufferUsage: {
    readonly MAP_READ: 0x0001
    readonly MAP_WRITE: 0x0002
    readonly COPY_SRC: 0x0004
    readonly COPY_DST: 0x0008
    readonly INDEX: 0x0010
    readonly VERTEX: 0x0020
    readonly UNIFORM: 0x0040
    readonly STORAGE: 0x0080
    readonly INDIRECT: 0x0100
    readonly QUERY_RESOLVE: 0x0200
  }

  // https://www.w3.org/TR/webgpu/#typedefdef-gputextureusageflags
  const GPUTextureUsage: {
    readonly COPY_SRC: 0x01
    readonly COPY_DST: 0x02
    readonly TEXTURE_BINDING: 0x04
    readonly STORAGE_BINDING: 0x08
    readonly RENDER_ATTACHMENT: 0x10
  }

  // Vite environment variables
  interface ImportMetaEnv {
    readonly DEV: boolean
    readonly VITE_E2E_SERVER_PORT?: string
    // VITE_NOTES_ENGINE names the browser engine in notes screenshot paths.
    readonly VITE_NOTES_ENGINE?: string
  }

  interface ImportMeta {
    readonly env: ImportMetaEnv
    glob<T = unknown>(
      pattern: string,
      options?: { query?: string; eager?: boolean; import?: string },
    ): Record<string, T>
  }
}
