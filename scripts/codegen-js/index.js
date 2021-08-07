const path = require('path')
const child_process = require('child_process')

const rootDir = process.cwd()
const scriptsDir = path.join(rootDir, 'scripts')
const goScriptDir = path.join(scriptsDir, 'codegen-js')

child_process.execSync('go run -v ./', {
  cwd: goScriptDir,
  env: {
    ...process.env,
    GO111MODULE: 'on',
  },
  stdio: [0, 1, 2],
})
