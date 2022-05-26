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
    knownEntrypoints: ['./web/index.html', './web/sw.js']
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
