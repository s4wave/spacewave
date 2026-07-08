import path from 'path'

export function buildStableEntryFileName(
  rootDir: string,
  facadeModuleId: string | null | undefined,
  fallbackName: string,
): string {
  if (!facadeModuleId) {
    return `${fallbackName}.mjs`
  }
  const relativePath = path.relative(rootDir, facadeModuleId)
  // Modules outside rootDir (external downstream apps build from a dir
  // outside the code root) would produce ../ traversal patterns, which
  // rolldown rejects for entryFileNames. Use the stable basename instead.
  if (relativePath.startsWith('..') || path.isAbsolute(relativePath)) {
    const parsed = path.parse(facadeModuleId)
    return `${parsed.name}.mjs`
  }
  const parsed = path.parse(relativePath)
  return parsed.dir ? `${parsed.dir}/${parsed.name}.mjs` : `${parsed.name}.mjs`
}
