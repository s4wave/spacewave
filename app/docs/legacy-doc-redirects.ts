const legacyDocRedirects = new Map<string, string>([
  ['/docs/users/cli/install', '/docs/users/cli/command-line-basics'],
  [
    '/docs/developers/cli/installation-and-commands',
    '/docs/developers/cli/cli-reference',
  ],
  ['/docs/users/devices/move-to-cloud', '/docs/users/accounts/move-to-cloud'],
  [
    '/docs/self-hosters/ownership/teams-and-space-ownership',
    '/docs/users/spaces/share-spaces-and-organizations',
  ],
  [
    '/docs/developers/start/sync-library',
    '/docs/developers/sync/build-a-live-application',
  ],
  [
    '/docs/developers/objects/quickstarts-and-app-surfaces',
    '/docs/developers/contributing/add-a-quickstart',
  ],
  [
    '/docs/developers/platform/prerender-and-public-docs',
    '/docs/developers/contributing/public-docs-and-markdown',
  ],
  [
    '/docs/developers/platform/space-native-docs-boundaries',
    '/docs/developers/contributing/public-docs-and-markdown',
  ],
])

export function getLegacyDocRedirect(url: string): string | undefined {
  return legacyDocRedirects.get(url)
}
