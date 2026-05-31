export function makeModulePreloadHelperWorkerSafe(code: string): string {
  if (!code.includes('vite/preload-helper.js')) {
    return code
  }
  return code
    .replace(
      /if\s*\(\s*([A-Za-z_$][\w$]*)\s*&&\s*\1\.length\s*>\s*0\s*\)\s*\{/g,
      'if (typeof document !== "undefined" && $1 && $1.length > 0) {',
    )
    .replace(
      /if\(([A-Za-z_$][\w$]*)&&\1\.length>0\)\{/g,
      'if(typeof document<"u"&&$1&&$1.length>0){',
    )
    .replace(
      /window\.dispatchEvent\(([A-Za-z_$][\w$]*)\)/g,
      '(typeof window !== "undefined" && window.dispatchEvent($1))',
    )
    .replace(
      /typeof window !== "undefined" && \(typeof window !== "undefined" && window\.dispatchEvent\(([A-Za-z_$][\w$]*)\)\)/g,
      'typeof window !== "undefined" && window.dispatchEvent($1)',
    )
}

export function createWorkerSafeModulePreloadPlugin() {
  return {
    name: 'bldr-worker-safe-modulepreload',
    renderChunk(code: string) {
      const next = makeModulePreloadHelperWorkerSafe(code)
      if (next === code) return null
      return { code: next, map: null }
    },
    generateBundle(_options: unknown, bundle: Record<string, unknown>) {
      for (const item of Object.values(bundle)) {
        if (
          item &&
          typeof item === 'object' &&
          'type' in item &&
          item.type === 'chunk' &&
          'code' in item &&
          typeof item.code === 'string'
        ) {
          item.code = makeModulePreloadHelperWorkerSafe(item.code)
        }
      }
    },
  }
}
