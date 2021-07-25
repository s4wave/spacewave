const path = require('path');
const child_process = require('child_process');

console.log('Building Go native code...');
console.log('You may need to install Go 1.11 or greater.');
child_process.execSync(
    'go build -trimpath -v -o ../bundle/desktop/bin/runtime.exe ./',
    {
        cwd: path.join(process.cwd(), './runtime'),
        env: {
            ...process.env,
            GO111MODULE: 'on',
        },
        stdio: [0, 1, 2],
    },
);
