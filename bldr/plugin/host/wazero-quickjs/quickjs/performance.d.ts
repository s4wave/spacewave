// PerformanceEntry represents a performance measurement entry.
export interface PerformanceEntry {
  readonly name: string
  readonly entryType: string
  readonly startTime: number
  readonly duration: number
}

// Performance interface provides performance timing functionality for QuickJS polyfills.
export interface Performance {
  now(): number
  mark(name: string): PerformanceEntry
  measure(name: string, startMark?: string, endMark?: string): PerformanceEntry
  getEntriesByType(type: string): PerformanceEntry[]
  getEntriesByName(name: string): PerformanceEntry[]
  clearMarks(name?: string): void
  clearMeasures(name?: string): void
}

// createQuickjsPerformance creates a performance instance optimized for QuickJS environment.
export declare function createQuickjsPerformance(originalPerformance: {
  now(): number
}): Performance

// mark creates a performance mark entry.
export declare function mark(name: string): PerformanceEntry

// measure creates a performance measure entry.
export declare function measure(
  name: string,
  startMark?: string,
  endMark?: string,
): PerformanceEntry

// getEntriesByType returns performance entries filtered by type.
export declare function getEntriesByType(type: string): PerformanceEntry[]

// getEntriesByName returns performance entries filtered by name.
export declare function getEntriesByName(name: string): PerformanceEntry[]

// clearMarks clears performance mark entries.
export declare function clearMarks(name?: string): void

// clearMeasures clears performance measure entries.
export declare function clearMeasures(name?: string): void
