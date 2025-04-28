// @ts-check

import globals from 'globals'
import eslint from '@eslint/js'
import tseslint from 'typescript-eslint'
import reactHooks from 'eslint-plugin-react-hooks';

export default tseslint.config(
  eslint.configs.recommended,
  tseslint.configs.recommended,
  reactHooks.configs['recommended-latest'],
  {
    ignores: [
      '**/*.gs.ts',
      'dist',
      'vendor',
      'node_modules',
      'bundle',
      'runtime',
      'prototypes',
      'wasm_exec.js',
      '**/determine-cjs-exports.mjs',
      '.bldr',
      '**/*.pb.ts',
    ],
  },
  {
    languageOptions: {
      globals: {
        ...globals.node,
      },
    },
    rules: {
      '@typescript-eslint/explicit-module-boundary-types': 'off',
      '@typescript-eslint/no-non-null-assertion': 'off',
      '@typescript-eslint/no-explicit-any': 'warn',
      '@typescript-eslint/no-empty-object-type': 'off',
      '@typescript-eslint/no-unused-vars': ['warn', { caughtErrors: 'none' }],
    },
  },
)
