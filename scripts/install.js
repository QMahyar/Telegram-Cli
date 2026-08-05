'use strict';

// Postinstall downloader: fetches the native telegram-cli binary for this
// platform from GitHub Releases and drops it in vendor/.
//
// Asset naming follows scripts/dist.sh:
//   telegram-cli-<os>-<arch>[.exe]
//
// Resolution rules:
//   - If the package version is "0.0.0-dev" (never published), the latest
//     GitHub release is used so local installs / `npm link` still work.
//   - Otherwise the release tag v<version> is used, so the npm package
//     version must match the GitHub release tag (see RELEASING.md).
//   - TELEGRAM_CLI_DIST_BASE_URL overrides the download base URL for
//     mirrors, staging, or local verification.

const fs = require('fs');
const http = require('http');
const https = require('https');
const os = require('os');
const path = require('path');
const { execFileSync } = require('child_process');

const pkg = require('../package.json');

const REPO = 'QMahyar/Telegram-Cli';
const API = 'https://api.github.com';
const DEFAULT_BASE = `https://github.com/${REPO}/releases/download`;

function die(message) {
  console.error('telegram-cli install: ' + message);
  process.exit(1);
}

function platformAsset() {
  const archMap = {
    x64: 'amd64',
    arm64: 'arm64',
    arm: 'arm64',
  };
  const osMap = {
    linux: 'linux',
    darwin: 'darwin',
    win32: 'windows',
  };
  const osName = osMap[process.platform];
  const arch = archMap[process.arch];
  if (!osName) die('unsupported platform: ' + process.platform);
  if (!arch) die('unsupported architecture: ' + process.arch);
  return osName + '-' + arch;
}

// The default base URL is always https. An explicit TELEGRAM_CLI_DIST_BASE_URL
// override may be http (internal mirrors, local staging); that opt-in is the
// operator's call.
function transport(url) {
  return url.startsWith('http://') ? http : https;
}

function httpsGetJson(url) {
  return new Promise((resolve, reject) => {
    transport(url).get(url, { headers: { 'User-Agent': 'telegram-cli-install' } }, (res) => {
      if (res.statusCode !== 200) {
        res.resume();
        return reject(new Error('GET ' + url + ' -> HTTP ' + res.statusCode));
      }
      let body = '';
      res.setEncoding('utf8');
      res.on('data', (c) => (body += c));
      res.on('end', () => {
        try {
          resolve(JSON.parse(body));
        } catch (e) {
          reject(new Error('invalid JSON from ' + url));
        }
      });
    }).on('error', reject);
  });
}

function download(url, dest) {
  return new Promise((resolve, reject) => {
    const file = fs.createWriteStream(dest);
    transport(url).get(url, (res) => {
      if (res.statusCode !== 200) {
        file.close();
        fs.unlinkSync(dest);
        res.resume();
        return reject(new Error('GET ' + url + ' -> HTTP ' + res.statusCode));
      }
      res.pipe(file);
      file.on('finish', () => file.close(resolve));
    }).on('error', (err) => {
      file.close();
      fs.unlinkSync(dest);
      reject(err);
    });
  });
}

async function latestReleaseTag() {
  const releases = await httpsGetJson(`${API}/repos/${REPO}/releases/latest`);
  if (!releases || !releases.tag_name) die('could not resolve latest release for ' + REPO);
  return releases.tag_name;
}

async function main() {
  const version = pkg.version;
  const base = process.env.TELEGRAM_CLI_DIST_BASE_URL || DEFAULT_BASE;
  const platform = platformAsset();
  const ext = process.platform === 'win32' ? '.exe' : '';
  const asset = `telegram-cli-${platform}${ext}`;

  const tag = version === '0.0.0-dev' ? await latestReleaseTag() : 'v' + version;
  const url = `${base}/${tag}/${asset}`;

  const vendorDir = path.join(__dirname, '..', 'vendor');
  fs.mkdirSync(vendorDir, { recursive: true });
  const dest = path.join(vendorDir, 'telegram-cli' + ext);
  const tmp = dest + '.tmp';

  console.log('telegram-cli: downloading ' + url);
  await download(url, tmp);
  fs.chmodSync(tmp, 0o755);
  fs.renameSync(tmp, dest);

  // Sanity: the binary must at least print its version.
  try {
    const out = execFileSync(dest, ['version'], { encoding: 'utf8' });
    console.log('telegram-cli: installed ' + out.trim());
  } catch (e) {
    die('downloaded binary failed a sanity check: ' + (e && e.message));
  }
}

main().catch((err) => die(err.message));
