"use strict";

const path = require("node:path");

const binaryName = "echo-dust-code";
const goplsBinaryName = "gopls";
const packageName = "@hongchengxianyue/echo-dust-code";
const releaseRepo = "HongChengXianYue/EchoDustAgent";
const releaseBaseURL =
  process.env.ECHODUST_CODE_RELEASE_BASE_URL ||
  `https://github.com/${releaseRepo}/releases/download`;

function mapPlatform(platform) {
  switch (platform) {
    case "darwin":
      return "darwin";
    case "linux":
      return "linux";
    case "win32":
      return "windows";
    default:
      throw new Error(
        `Unsupported platform ${platform}. Supported platforms: darwin, linux, win32.`
      );
  }
}

function mapArch(arch) {
  switch (arch) {
    case "x64":
      return "amd64";
    case "arm64":
      return "arm64";
    default:
      throw new Error(`Unsupported architecture ${arch}. Supported architectures: x64, arm64.`);
  }
}

function detectTarget(platform = process.platform, arch = process.arch) {
  const mappedPlatform = mapPlatform(platform);
  const mappedArch = mapArch(arch);
  return {
    platform: mappedPlatform,
    arch: mappedArch,
    binaryFile: mappedPlatform === "windows" ? `${binaryName}.exe` : binaryName,
    goplsFile: mappedPlatform === "windows" ? `${goplsBinaryName}.exe` : goplsBinaryName,
  };
}

function archiveName(target) {
  return `${binaryName}-${target.platform}-${target.arch}.tar.gz`;
}

function installedBinaryDir(rootDir) {
  return path.join(rootDir, ".echodust-bin");
}

function installedBinaryPath(rootDir, target = detectTarget()) {
  return path.join(installedBinaryDir(rootDir), target.binaryFile);
}

function installedGoplsPath(rootDir, target = detectTarget()) {
  return path.join(installedBinaryDir(rootDir), target.goplsFile);
}

function releaseDownloadURL(version, target) {
  return `${releaseBaseURL.replace(/\/+$/, "")}/v${version}/${archiveName(target)}`;
}

module.exports = {
  archiveName,
  binaryName,
  detectTarget,
  goplsBinaryName,
  installedBinaryDir,
  installedBinaryPath,
  installedGoplsPath,
  packageName,
  releaseDownloadURL,
  releaseRepo,
};
