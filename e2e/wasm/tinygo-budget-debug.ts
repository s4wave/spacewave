type TinyGoBudgetGlobal = typeof globalThis & {
  __BLDR_TINYGO_BROWSER_BUDGET__?: {
    snapshot?: () => unknown
  }
}

type PerformanceWithMemory = Performance & {
  memory?: {
    usedJSHeapSize?: number
    totalJSHeapSize?: number
    jsHeapSizeLimit?: number
  }
}

function jsHeapSnapshot() {
  const memory = (globalThis.performance as PerformanceWithMemory | undefined)
    ?.memory
  if (!memory) {
    return null
  }
  return {
    usedJSHeapSize: memory.usedJSHeapSize ?? 0,
    totalJSHeapSize: memory.totalJSHeapSize ?? 0,
    jsHeapSizeLimit: memory.jsHeapSizeLimit ?? 0,
  }
}

export default function (args: { op: 'snapshot' | 'typeof' }): string {
  const budget = (globalThis as TinyGoBudgetGlobal)
    .__BLDR_TINYGO_BROWSER_BUDGET__
  if (args.op === 'typeof') {
    return typeof budget
  }
  if (!budget?.snapshot) {
    return 'null'
  }
  return JSON.stringify({
    budget: budget.snapshot(),
    jsHeap: jsHeapSnapshot(),
    now: globalThis.performance?.now?.() ?? 0,
  })
}
