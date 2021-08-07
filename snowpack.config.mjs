// Snowpack Configuration File
// See all supported options: https://www.snowpack.dev/reference/configuration

/** @type {import("snowpack").SnowpackUserConfig } */
export default {
  mount: {
    src: '/',
    'src/sandbox': {url: '/', static: true, resolve: false},
    public: { url: '/', static: true, resolve: false },
    'target/browser/worker': {url: '/runtime', static: true, resolve: false},
  },
  plugins: [
    /* ... */
    '@snowpack/plugin-react-refresh'
  ],
  packageOptions: {
    source: "local",
    external: ["electron", "fs", "net", "path"]
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
