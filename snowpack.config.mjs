// Snowpack Configuration File
// See all supported options: https://www.snowpack.dev/reference/configuration

/** @type {import("snowpack").SnowpackUserConfig } */
export default {
  mount: {
    'web': { url: '/' },
    // public: { url: '/', static: true, resolve: false },
    'entrypoint/browser': {url: '/runtime', static: true, resolve: false},
  },
  plugins: [
    /* ... */
    '@snowpack/plugin-react-refresh'
  ],
  packageOptions: {
    source: "local",
    external: ["electron", "fs", "net", "path"],
    knownEntrypoints: [
      './web/index.html',
      './web/sw.js',
      'uint8arrays',
      'uint8arrays/from-string',
      'it-pushable',
      'multiformats/bases/base32',
      'multiformats/bases/base58',
      'multiformats/bases/base64',
    ]
  },
  devOptions: {
    /* ... */
  },
  buildOptions: {
    // sourcemap: true
  },
  optimize: {
    bundle: true,
    minify: true,
    manifest: true,
    treeshake: true,
    target: "es2018",
  },
}
