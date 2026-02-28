#!/usr/bin/env node
"use strict";

// postinstall: download the correct devx binary from GitHub Releases
const https = require("https");
const fs = require("fs");
const path = require("path");
const { execFileSync } = require("child_process");

const REPO = "dever-labs/dever";
const BIN_DIR = path.join(__dirname, "bin");
const VERSION = require("./package.json").version;

function getPlatform() {
  const os = process.platform;
  const arch = process.arch;
  const archMap = { x64: "amd64", arm64: "arm64" };
  const osMap = { linux: "linux", darwin: "darwin", win32: "windows" };

  if (!osMap[os]) throw new Error(`Unsupported OS: ${os}`);
  if (!archMap[arch]) throw new Error(`Unsupported arch: ${arch}`);

  const ext = os === "win32" ? ".exe" : "";
  return { asset: `devx-${osMap[os]}-${archMap[arch]}${ext}`, ext };
}

function download(url, dest) {
  return new Promise((resolve, reject) => {
    const file = fs.createWriteStream(dest);
    const follow = (u) =>
      https.get(u, { headers: { "User-Agent": "devx-npm-installer" } }, (res) => {
        if (res.statusCode === 301 || res.statusCode === 302) return follow(res.headers.location);
        if (res.statusCode !== 200) return reject(new Error(`HTTP ${res.statusCode} for ${u}`));
        res.pipe(file);
        file.on("finish", () => file.close(resolve));
      }).on("error", reject);
    follow(url);
  });
}

async function main() {
  const { asset, ext } = getPlatform();
  const dest = path.join(BIN_DIR, `devx${ext}`);
  const shimDest = path.join(BIN_DIR, "devx");

  if (!fs.existsSync(BIN_DIR)) fs.mkdirSync(BIN_DIR, { recursive: true });

  const ver = VERSION === "0.0.0" ? "latest" : `v${VERSION}`;
  const baseUrl =
    ver === "latest"
      ? `https://github.com/${REPO}/releases/latest/download/${asset}`
      : `https://github.com/${REPO}/releases/download/${ver}/${asset}`;

  console.log(`[devx] Downloading ${asset} (${ver})...`);
  await download(baseUrl, dest);
  fs.chmodSync(dest, 0o755);

  // On non-Windows, create a thin shell shim at bin/devx (no extension)
  if (ext === "") {
    fs.copyFileSync(dest, shimDest);
  } else {
    // On Windows, create a cmd shim
    const cmdShim = `@echo off\n"%~dp0devx.exe" %*\n`;
    fs.writeFileSync(shimDest + ".cmd", cmdShim);
  }
  console.log(`[devx] Installed to ${dest}`);
}

main().catch((e) => {
  // Non-fatal: user can still install manually
  console.warn("[devx] Postinstall failed:", e.message);
  console.warn("[devx] Install manually: https://github.com/dever-labs/dever/releases");
});
