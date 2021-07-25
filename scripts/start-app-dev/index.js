const path = require('path');
const fs = require('fs');
const child_process = require('child_process')

const rootDir = process.cwd();
const devDir = path.join(rootDir, 'runtime/cmd/sandbox-dev')

child_process
    .execSync(
        'go run -trimpath -v ./',
        {
            cwd: devDir,
            env: {
                ...process.env,
                GO111MODULE: 'on',
            },
            stdio: [0, 1, 2],
        },
    ) ;
