'use strict';
const {spawn} = require('child_process');
const electron = require('electron');
const path = require('path');
const split = require('split2');
const filter = require('through2-filter');
const map = require('through2-map');
const pumpify = require('pumpify');

process.env.FORCE_COLOR = '1';
require('./build-runtime-desktop-dev');
const removeJunk = () => {
	const filterStream = filter({wantStrings: true}, chunk => {
		// Example: 2018-08-10 22:48:42.866 Electron[90311:4883863] *** WARNING: Textured window <AtomNSWindow: 0x7fb75f68a770>
		if (/\d+-\d+-\d+ \d+:\d+:\d+\.\d+ Electron(?: Helper)?\[\d+:\d+] /.test(chunk)) {
			return false;
		}

		// Example: [90789:0810/225804.894349:ERROR:CONSOLE(105)] "Uncaught (in promise) Error: Could not instantiate: ProductRegistryImpl.Registry", source: chrome-devtools://devtools/bundled/inspector.js (105)
		if (/\[\d+:\d+\/|\d+\.\d+:ERROR:CONSOLE\(\d+\)\]/.test(chunk)) {
			return false;
		}

		return true;
	});

	return pumpify(split(), filterStream, map(chunk => `${chunk}\n`));
};

const cp = spawn(electron, ['--inspect=5858', '.'], {cwd: path.join(process.cwd(), './bundle/desktop-dev')});
cp.stdout.pipe(removeJunk()).pipe(process.stdout);
cp.stderr.pipe(removeJunk()).pipe(process.stderr);