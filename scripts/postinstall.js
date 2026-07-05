#!/usr/bin/env node

const fs = require("node:fs");
const fsp = require("node:fs/promises");
const https = require("node:https");
const os = require("node:os");
const path = require("node:path");
const { spawn } = require("node:child_process");
const { pipeline } = require("node:stream/promises");

const {
  archiveName,
  binaryName,
  detectTarget,
  installedBinaryDir,
  installedBinaryPath,
  installedGoplsPath,
  releaseDownloadURL,
} = require("../lib/npm-platform");

const rootDir = path.resolve(__dirname, "..");
const packageJSON = require(path.join(rootDir, "package.json"));

function shouldSkipDownload() {
  const skip = String(process.env.ECHODUST_CODE_SKIP_DOWNLOAD || "").toLowerCase();
  if (skip === "1" || skip === "true" || skip === "yes") {
    return "ECHODUST_CODE_SKIP_DOWNLOAD is set";
  }
  if (fs.existsSync(path.join(rootDir, ".git")) && process.env.ECHODUST_CODE_FORCE_DOWNLOAD !== "1") {
    return "git checkout detected";
  }
  return "";
}

function downloadFile(url, destination, redirects = 0) {
  if (redirects > 10) {
    return Promise.reject(new Error(`too many redirects while downloading ${url}`));
  }
  return new Promise((resolve, reject) => {
    const request = https.get(url, (response) => {
      const status = response.statusCode || 0;
      if (status >= 300 && status < 400 && response.headers.location) {
        response.resume();
        downloadFile(new URL(response.headers.location, url).toString(), destination, redirects + 1).then(
          resolve,
          reject
        );
        return;
      }
      if (status < 200 || status >= 300) {
        const chunks = [];
        response.on("data", (chunk) => chunks.push(chunk));
        response.on("end", () => {
          reject(
            new Error(
              `download failed: ${status} ${Buffer.concat(chunks).toString("utf8").trim()}`
            )
          );
        });
        return;
      }
      const file = fs.createWriteStream(destination);
      pipeline(response, file).then(resolve, reject);
    });
    request.on("error", reject);
  });
}

function run(command, args) {
  return new Promise((resolve, reject) => {
    const child = spawn(command, args, { stdio: "inherit" });
    child.on("error", reject);
    child.on("exit", (code) => {
      if (code === 0) {
        resolve();
        return;
      }
      reject(new Error(`${command} exited with code ${code}`));
    });
  });
}

async function main() {
  const skipReason = shouldSkipDownload();
  if (skipReason) {
    console.log(`[${binaryName}] Skipping binary download during postinstall: ${skipReason}.`);
    return;
  }

  const target = detectTarget();
  const installDir = installedBinaryDir(rootDir);
  const binaryPath = installedBinaryPath(rootDir, target);
  const goplsPath = installedGoplsPath(rootDir, target);
  const archive = archiveName(target);
  const downloadURL = releaseDownloadURL(packageJSON.version, target);
  const tempDir = await fsp.mkdtemp(path.join(os.tmpdir(), "echo-dust-code-"));
  const archivePath = path.join(tempDir, archive);

  await fsp.mkdir(installDir, { recursive: true });
  await fsp.rm(binaryPath, { force: true });
  await fsp.rm(goplsPath, { force: true });

  console.log(`[${binaryName}] Downloading ${archive} from ${downloadURL}`);
  await downloadFile(downloadURL, archivePath);
  await run("tar", ["-xzf", archivePath, "-C", installDir]);

  if (!fs.existsSync(binaryPath)) {
    throw new Error(`archive ${archive} did not contain ${path.basename(binaryPath)}`);
  }
  if (!fs.existsSync(goplsPath)) {
    throw new Error(`archive ${archive} did not contain ${path.basename(goplsPath)}`);
  }
  if (target.platform !== "windows") {
    await fsp.chmod(binaryPath, 0o755);
    await fsp.chmod(goplsPath, 0o755);
  }
  await fsp.rm(tempDir, { recursive: true, force: true });
  console.log(
    `[${binaryName}] Installed ${path.basename(binaryPath)} and ${path.basename(goplsPath)} for ${target.platform}/${target.arch}`
  );
}

main().catch((error) => {
  console.error(`[${binaryName}] postinstall failed: ${error.message}`);
  process.exit(1);
});
