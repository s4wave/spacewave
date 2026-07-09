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
  const parsed = path.parse(relativePath)
  return parsed.dir ? `${parsed.dir}/${parsed.name}.mjs` : `${parsed.name}.mjs`
}
