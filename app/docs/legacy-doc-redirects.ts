const legacyDocRedirects = new Map<string, string>([
  ['/docs/users/cli/install', '/docs/users/cli/command-line-basics'],
  [
    '/docs/developers/cli/installation-and-commands',
    '/docs/developers/cli/cli-reference',
  ],
])

export function getLegacyDocRedirect(url: string): string | undefined {
  return legacyDocRedirects.get(url)
}
