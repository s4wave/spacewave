import eslint from '@eslint/js'
import tseslint from 'typescript-eslint'
import reactHooks from 'eslint-plugin-react-hooks'
import reactCompiler from 'eslint-plugin-react-compiler'
import eslintConfigPrettier from 'eslint-config-prettier'

const alphaFiles = [
  'app/**/*.{js,mjs,ts,tsx}',
  'web/**/*.{js,mjs,ts,tsx}',
  'core/**/*.{js,mjs,ts,tsx}',
  'sdk/**/*.{js,mjs,ts,tsx}',
  'plugin/**/*.{js,mjs,ts,tsx}',
  'cmd/**/*.{js,mjs,ts,tsx}',
]

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
      'bldr/e2e/comms/dist/**',
      'bldr/plugin/compiler/js/.test/**',
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
        project: ['./tsconfig.eslint.json'],
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
      'react-compiler/react-compiler': 'off',
    },
  },
  {
    files: alphaFiles,
    rules: {
      ...Object.fromEntries(
        Object.entries(reactHooks.configs.recommended.rules).map(([k]) => [
          k,
          'warn',
        ]),
      ),
      'react-compiler/react-compiler': 'off',
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
  {
    files: ['bldr/**/*.{js,mjs}'],
    extends: [tseslint.configs.disableTypeChecked],
  },
  {
    files: ['bldr/**/*.{js,mjs,ts,tsx}'],
    rules: {
      '@typescript-eslint/no-unsafe-assignment': 'off',
      '@typescript-eslint/no-unsafe-member-access': 'off',
      '@typescript-eslint/no-unsafe-call': 'off',
      '@typescript-eslint/no-unsafe-argument': 'off',
      '@typescript-eslint/no-unsafe-return': 'off',
      '@typescript-eslint/require-await': 'off',
      '@typescript-eslint/no-floating-promises': 'off',
      '@typescript-eslint/no-misused-promises': 'off',
      '@typescript-eslint/restrict-template-expressions': 'off',
      '@typescript-eslint/no-redundant-type-constituents': 'off',
      '@typescript-eslint/only-throw-error': 'off',
      '@typescript-eslint/prefer-promise-reject-errors': 'off',
      'react-hooks/refs': 'off',
      'react-hooks/use-memo': 'off',
      'react-hooks/set-state-in-effect': 'off',
      'react-hooks/static-components': 'off',
      'no-undef': 'off',
    },
  },
  eslintConfigPrettier,
)
