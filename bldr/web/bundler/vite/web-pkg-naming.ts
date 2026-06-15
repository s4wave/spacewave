export function buildWebPkgImportSpecifier(
  pkgId: string,
  baseName: string,
  rootServedName: string | null,
): string {
  return baseName === rootServedName ||
    (!rootServedName && baseName === 'index')
    ? pkgId
    : `${pkgId}/${baseName}`
}
