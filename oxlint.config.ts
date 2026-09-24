import { defineConfig } from 'oxlint'
import {
  RECOMMENDED_RULES as reactDoctorRules,
  RULES as reactDoctorRuleInfo,
} from 'oxlint-plugin-react-doctor'

// ignorePatterns lists build output, vendored code, and generated sources.
const ignorePatterns = [
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
  'bldr/plugin/host/wazero-quickjs/quickjs/text-encoding.js',
  'bldr/web/bundler/rolldown/run-build.mjs',
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
]

// reactSources are the React application and SDK trees.
const reactSources = [
  'app/**/*.{js,mjs,ts,tsx}',
  'web/**/*.{js,mjs,ts,tsx}',
  'core/**/*.{js,mjs,ts,tsx}',
  'sdk/**/*.{js,mjs,ts,tsx}',
  'plugin/**/*.{js,mjs,ts,tsx}',
  'cmd/**/*.{js,mjs,ts,tsx}',
]

// shadcnRules keep components on the design system's variants and tokens.
const shadcnRules = {
  'shadcn/no-restyle': ['error', { allow: ['layout'] }],
  'shadcn/no-raw-colors': 'error',
  'shadcn/no-arbitrary-values': 'error',
  'shadcn/no-inline-styles': 'error',
  'shadcn/no-unknown-classes': 'error',
  'shadcn/require-static-classes': 'error',
} as const

// serverRenderRules assume every component renders on a server. Only the
// static pages prerender, and their build and hydration tests cover that path.
const serverRenderRules = Object.fromEntries(
  reactDoctorRuleInfo
    .filter(({ rule }) => rule?.requires?.includes('ssr'))
    .map(({ key }) => [key, 'off' as const]),
)

// declinedReactDoctorRules are recommended rules this codebase does not follow.
// The prop identity rules ask for memoized callbacks and literals at every
// call site, and fast refresh does not constrain bldr module exports.
const declinedReactDoctorRules = {
  'react-doctor/jsx-no-new-function-as-prop': 'off',
  'react-doctor/jsx-no-new-object-as-prop': 'off',
  'react-doctor/jsx-no-new-array-as-prop': 'off',
  'react-doctor/jsx-no-jsx-as-prop': 'off',
  'react-doctor/only-export-components': 'off',
} as const

// Lint uses the oxlint correctness category and the React Doctor recommended
// preset, with the few rules each preset leaves out listed explicitly.
export default defineConfig({
  plugins: ['typescript', 'react', 'unicorn'],
  categories: { correctness: 'error' },
  env: { builtin: true },
  ignorePatterns,
  rules: {
    'no-array-constructor': 'error',
    'no-case-declarations': 'error',
    'no-empty': 'error',
    'no-fallthrough': 'error',
    'no-prototype-builtins': 'error',
    'no-redeclare': 'error',
    'no-regex-spaces': 'error',
    'no-unused-vars': [
      'error',
      {
        argsIgnorePattern: '^_',
        varsIgnorePattern: '^_',
        caughtErrors: 'none',
      },
    ],
    'preserve-caught-error': 'error',
    'react/exhaustive-deps': 'warn',
    'react/rules-of-hooks': 'error',

    // These React Compiler validations assume compiled components. The app
    // does not build with the React Compiler, and React Doctor covers render
    // time ref access.
    'react/preserve-manual-memoization': 'off',
    'react/refs': 'off',
    'react/set-state-in-effect': 'off',

    'typescript/ban-ts-comment': 'error',
    'typescript/no-explicit-any': 'warn',
    'typescript/no-namespace': 'error',
    'typescript/no-require-imports': 'error',
    'typescript/no-unnecessary-type-constraint': 'error',
    'typescript/no-unsafe-function-type': 'error',
  },
  overrides: [
    {
      files: reactSources,
      jsPlugins: [
        '@shadcn/lint',
        { name: 'react-doctor', specifier: 'oxlint-plugin-react-doctor' },
      ],
      rules: {
        ...reactDoctorRules,
        ...serverRenderRules,
        ...declinedReactDoctorRules,
        ...shadcnRules,
      },
    },
    {
      // Test probes capture hook results during render, and layout mocks
      // invoke render callbacks the way the real layout does.
      files: ['**/*.test.{ts,tsx}'],
      rules: {
        'react/globals': 'off',
        'react/immutability': 'off',
        'react-doctor/no-prop-callback-in-render': 'off',
      },
    },
    {
      files: ['web/ui/**'],
      rules: {
        'shadcn/no-restyle': 'off',
        'shadcn/no-arbitrary-values': 'off',
        'shadcn/require-static-classes': 'off',
      },
    },
  ],
})
