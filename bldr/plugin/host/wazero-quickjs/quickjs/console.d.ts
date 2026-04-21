/* eslint-disable @typescript-eslint/no-explicit-any */
// Console interface provides console functionality for QuickJS polyfills.
export interface Console {
  assert(condition?: boolean, ...data: any[]): void
  clear(): void
  count(label?: string): void
  countReset(label?: string): void
  debug(...data: any[]): void
  dir(item?: any, options?: any): void
  dirxml(...data: any[]): void
  error(...data: any[]): void
  group(...data: any[]): void
  groupCollapsed(...data: any[]): void
  groupEnd(): void
  info(...data: any[]): void
  log(...data: any[]): void
  table(tabularData?: any, properties?: string[]): void
  time(label?: string): void
  timeEnd(label?: string): void
  timeLog(label?: string, ...data: any[]): void
  trace(...data: any[]): void
  warn(...data: any[]): void
}

// ConsoleOptions defines the configuration for creating a console instance.
export interface ConsoleOptions {
  logger?: (logLevel: string, args: any[], options?: any) => void
  clearConsole?: () => void
  printer?: (
    logLevel: string,
    args: any[],
    options: { indent?: number; isWarn?: boolean },
  ) => void
  formatter?: (...args: any[]) => string
  inspect?: (value: any) => string
}

// createConsole creates a console instance with the given options.
export declare function createConsole(options: ConsoleOptions): Console

// createQuickjsConsole creates a console instance optimized for QuickJS environment.
export declare function createQuickjsConsole(originalConsole: {
  log: (...args: any[]) => void
}): Console

// inspect formats a value for display.
export declare function inspect(value: any): string

// format formats arguments similar to util.format.
export declare function format(...args: any[]): string
