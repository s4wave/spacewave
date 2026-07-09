import 'csstype'
import 'react'

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

// React's CSSProperties uses closed csstype typing with no index signature, so
// inline CSS custom properties (--foo) are rejected. Re-admit only --prefixed
// keys; standard property typos still fail.
declare module 'react' {
  interface CSSProperties {
    [customProperty: `--${string}`]: string | number | undefined
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

  // Vite environment variables
  interface ImportMetaEnv {
    readonly DEV: boolean
    readonly VITE_E2E_SERVER_PORT?: string
  }

  interface ImportMeta {
    readonly env: ImportMetaEnv
    glob<T = unknown>(
      pattern: string,
      options?: { query?: string; eager?: boolean; import?: string },
    ): Record<string, T>
  }
}
