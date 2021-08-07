const path = require('path')
const fs = require('fs')
const child_process = require('child_process')

const rootDir = process.cwd()
const scriptsDir = path.join(rootDir, 'scripts')
const goScriptDir = path.join(scriptsDir, 'build-electron-js')
const runtimeDir = path.join(rootDir, 'runtime')

child_process.execSync('go mod vendor', {
  cwd: runtimeDir,
  env: {
    ...process.env,
    GO111MODULE: 'on',
    GOOS: 'js',
  },
  stdio: [0, 1, 2],
})

child_process.execSync('go run -v ./', {
  cwd: goScriptDir,
  env: {
    ...process.env,
    GO111MODULE: 'on',
  },
  stdio: [0, 1, 2],
})
