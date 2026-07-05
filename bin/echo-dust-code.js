#!/usr/bin/env node

const fs = require("node:fs");
const path = require("node:path");
const { spawn } = require("node:child_process");

const {
  binaryName,
  detectTarget,
  installedBinaryPath,
  packageName,
} = require("../lib/npm-platform");

const rootDir = path.resolve(__dirname, "..");

let target;
try {
  target = detectTarget();
} catch (error) {
  console.error(`[${binaryName}] ${error.message}`);
  process.exit(1);
}

const binaryPath = installedBinaryPath(rootDir, target);
if (!fs.existsSync(binaryPath)) {
  console.error(
    `[${binaryName}] Missing packaged binary at ${binaryPath}.\n` +
      `Reinstall with: npm install -g ${packageName}@latest`
  );
  process.exit(1);
}

const child = spawn(binaryPath, process.argv.slice(2), {
  stdio: "inherit",
});

child.on("error", (error) => {
  console.error(`[${binaryName}] Failed to start binary: ${error.message}`);
  process.exit(1);
});

child.on("exit", (code, signal) => {
  if (signal) {
    process.kill(process.pid, signal);
    return;
  }
  process.exit(code ?? 1);
});
