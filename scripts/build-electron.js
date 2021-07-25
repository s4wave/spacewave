const path = require('path');
const child_process = require('child_process');
const rimraf = require('rimraf');
const snowpack = require('snowpack');

process.env.NODE_ENV = 'production';
process.env.IS_ELECTRON_BUILD = 'true';

console.log('Building Go native code...');
console.log('You may need to install Go 1.11 or greater.');
child_process.execSync(
    'go build -trimpath -ldflags="-s -w" -v -o ../bundle/desktop/bin/runtime.exe ./',
    {
        env: {
            ...process.env,
            GO111MODULE: 'on',
        },
        cwd: path.join(process.cwd(), './runtime'),
        stdio: [0, 1, 2],
    },
);

console.log('Starting main snowpack build...');
rimraf.sync(path.join(process.cwd(), './bundle/desktop/app'));
snowpack.loadConfiguration({
  mode: 'production',
  buildOptions: {
    out: './bundle/desktop/app',
  },
}, './snowpack.config.mjs').then(config => {
  snowpack.build({ config });
})

