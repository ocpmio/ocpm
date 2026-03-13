#!/usr/bin/env node

const { spawnSync } = require("node:child_process");
const { existsSync } = require("node:fs");
const path = require("node:path");

const packageRoot = path.resolve(__dirname, "..");
const binaryName = process.platform === "win32" ? "ocpm.exe" : "ocpm";
const platformKey = `${process.platform}-${process.arch}`;

const candidates = [
  process.env.OCPM_BINARY_PATH,
  path.join(packageRoot, "dist", "npm", platformKey, binaryName),
  path.join(packageRoot, "dist", binaryName),
].filter(Boolean);

const binaryPath = candidates.find((candidate) => existsSync(candidate));

if (!binaryPath) {
  console.error("[ocpm] No compatible native binary was found.");
  console.error("[ocpm] Expected one of:");
  for (const candidate of candidates) {
    console.error(`  - ${candidate}`);
  }
  console.error("[ocpm] Build the Go CLI first or publish the npm package with staged release binaries.");
  process.exit(1);
}

const result = spawnSync(binaryPath, process.argv.slice(2), {
  stdio: "inherit",
  env: process.env,
});

if (result.error) {
  console.error(`[ocpm] Failed to launch ${binaryPath}: ${result.error.message}`);
  process.exit(1);
}

process.exit(result.status ?? 1);

