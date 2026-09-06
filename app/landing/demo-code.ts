// demoCode contains the fixed examples whose highlighting is generated for the landing pages.
export const demoCode = {
  drive: {
    lang: 'sh',
    code: 'spacewave drive sync ./docs\nspacewave device list --json',
  },
  notes: {
    lang: 'ts',
    code: "export const launchMode = 'local-first'",
  },
  plugin: {
    lang: 'ts',
    code: `export default {
  name: 'release-pulse',
  command: 'release:announce',
  description: 'Turn a changelog entry into a launch checklist and status card.',
}`,
  },
} as const
