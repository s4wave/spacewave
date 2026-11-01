import config from './oxlint.config.ts'

// Parity tools use the repository's rules without the general scripts exclusion.
export default {
  ...config,
  ignorePatterns: config.ignorePatterns?.filter(
    (pattern) => pattern !== 'scripts/**',
  ),
}
