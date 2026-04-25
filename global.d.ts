// BLDR_DEBUG is set by the bldr bundler in debug builds.
declare const BLDR_DEBUG: boolean | undefined

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

// Declare WebkitAppRegion
declare module 'csstype' {
  interface StandardLonghandProperties {
    WebkitAppRegion?: string
  }
}

// See: https://github.com/lukewarlow/user-agent-data-types#readme
// WICG Spec: https://wicg.github.io/ua-client-hints

declare interface Navigator extends NavigatorUA {}
declare interface WorkerNavigator extends NavigatorUA {}

// https://wicg.github.io/ua-client-hints/#navigatorua
declare interface NavigatorUA {
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
  /** @deprecated in favour of fullVersionList */
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
