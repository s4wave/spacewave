// Snowpack Configuration File
// See all supported options: https://www.snowpack.dev/reference/configuration

/** @type {import("snowpack").SnowpackUserConfig } */
export default {
  mount: {
    src: '/',
    'target/browser': {url: '/runtime', static: true},
  },
  plugins: [
    /* ... */
    '@snowpack/plugin-react-refresh',
  ],
  packageOptions: {
    source: "local"
  },
  devOptions: {
    /* ... */
  },
  buildOptions: {
    sourcemap: true
  },
  optimize: {
    bundle: true,
    minify: true,
    manifest: true,
    treeshake: true,
    target: "es2018",
  },
}
