import eslint from '@eslint/js'
import tseslint from 'typescript-eslint'
import reactHooks from 'eslint-plugin-react-hooks'
import reactCompiler from 'eslint-plugin-react-compiler'
import eslintConfigPrettier from 'eslint-config-prettier'

const alphaFiles = ['app/**/*.{js,mjs,ts,tsx}', 'web/**/*.{js,mjs,ts,tsx}', 'core/**/*.{js,mjs,ts,tsx}', 'sdk/**/*.{js,mjs,ts,tsx}', 'plugin/**/*.{js,mjs,ts,tsx}', 'cmd/**/*.{js,mjs,ts,tsx}']

export default tseslint.config(
  {
    ignores: [
      'node_modules/**',
      'dist/**',
      'net/dist/**',
      '.bldr/**',
      '.bldr-dist/**',
      'bldr/.bldr/**',
      'bldr/.bldr-dist/**',
      'bldr/dist/**',
      'bldr/prototypes/**',
      'db/prototypes/**',
      'coverage/**',
      'bundle/**',
      'runtime/**',
      'vendor/**',
      'vite-check/**',
      'scripts/**',
      'wasm_exec.js',
      'hydra/**',
      '**/.bldr/**',
      '**/.tools/**',
      '**/.bldr-dist/**',
      'app/prerender/dist/**',
      'app/prerender/ssr-dist/**',
      'e2e/wasm/memlab/**',
      'prototypes/**',
      '**/*.pb.ts',
      '**/*.pb.js',
      '**/*.esm.js',
      '.tmp/**',
      'eslint.config.mjs',
    ],
  },
  eslint.configs.recommended,
  ...tseslint.configs.recommendedTypeChecked,
  {
    languageOptions: {
      parserOptions: {
        project: ['./tsconfig.json'],
        tsconfigRootDir: import.meta.dirname,
      },
    },
  },
  {
    plugins: {
      'react-hooks': reactHooks,
      'react-compiler': reactCompiler,
    },
    rules: {
      ...reactHooks.configs.recommended.rules,
      'react-compiler/react-compiler': 'error',
    },
  },
  {
    files: alphaFiles,
    rules: {
      ...Object.fromEntries(
        Object.entries(reactHooks.configs.recommended.rules).map(([k]) => [k, 'warn']),
      ),
      'react-compiler/react-compiler': 'warn',
      '@typescript-eslint/no-unused-vars': [
        'warn',
        {
          argsIgnorePattern: '^_',
          varsIgnorePattern: '^_',
          caughtErrors: 'none',
        },
      ],
      '@typescript-eslint/no-unnecessary-type-assertion': 'warn',
    },
  },
  {
    rules: {
      '@typescript-eslint/explicit-module-boundary-types': 'off',
      '@typescript-eslint/no-non-null-assertion': 'off',
      '@typescript-eslint/no-empty-object-type': 'off',
      '@typescript-eslint/unbound-method': 'off',
      '@typescript-eslint/no-explicit-any': 'warn',
      '@typescript-eslint/no-unused-vars': [
        'error',
        {
          argsIgnorePattern: '^_',
          varsIgnorePattern: '^_',
          caughtErrors: 'none',
        },
      ],
    },
  },
  eslintConfigPrettier,
)
