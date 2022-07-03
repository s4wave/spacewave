const path = require('path')
const fs = require('fs')
const child_process = require('child_process')

const rootDir = process.cwd()
const scriptsDir = path.join(rootDir, 'scripts')
const goScriptDir = path.join(scriptsDir, 'build-browser')
const runtimeDir = path.join(rootDir, 'runtime')
const vendorDir = path.join(rootDir, 'vendor')

// go mod vendor
if (!fs.existsSync(vendorDir)) {
  child_process.execSync('go mod vendor', {
    cwd: runtimeDir,
    env: {
      ...process.env,
      GO111MODULE: 'on',
      GOOS: 'js',
      GOARCH: 'wasm',
    },
    stdio: [0, 1, 2],
  })
}

child_process.execSync('go run -v ./', {
  cwd: goScriptDir,
  env: {
    ...process.env,
    GO111MODULE: 'on',
    GOOS: '',
    GOARCH: '',
  },
  stdio: [0, 1, 2],
})
