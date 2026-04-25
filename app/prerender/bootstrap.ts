// buildBootstrapScript returns the stable boot asset script tag. The stable
// boot layer resolves the current browser release manifest and then loads the
// hashed entrypoint for that release.
export function buildBootstrapScript(): string {
  return '<script type="module" src="/boot.mjs"></script>'
}
