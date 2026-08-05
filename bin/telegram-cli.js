#!/usr/bin/env node
'use strict';

// Shim that spawns the native telegram-cli binary downloaded by
// scripts/install.js (postinstall). The package's own version must match
// the GitHub release tag so install.js fetches the right asset; see
// RELEASING.md for how the release workflow keeps them in sync.

const { spawnSync } = require('child_process');
const fs = require('fs');
const path = require('path');

const isWin = process.platform === 'win32';
const bin = path.join(__dirname, '..', 'vendor', isWin ? 'telegram-cli.exe' : 'telegram-cli');

if (!fs.existsSync(bin)) {
  console.error(
    'telegram-cli: native binary not found at ' + bin +
    '\nRun `npm rebuild @qmahyar/telegram-cli` (or reinstall) to download it. ' +
    'The postinstall step downloads the binary from GitHub Releases; ' +
    'offline/blocked networks can set TELEGRAM_CLI_DIST_BASE_URL to a mirror.'
  );
  process.exit(1);
}

const result = spawnSync(bin, process.argv.slice(2), { stdio: 'inherit' });
if (result.error) {
  console.error('telegram-cli: failed to launch ' + bin + ': ' + result.error.message);
  process.exit(1);
}
process.exit(result.status === null ? 1 : result.status);
