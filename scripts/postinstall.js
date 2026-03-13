#!/usr/bin/env node

const { spawnSync } = require("node:child_process");
const { chmodSync, existsSync, mkdirSync } = require("node:fs");
const path = require("node:path");

const packageRoot = path.resolve(__dirname, "..");
const binaryName = process.platform === "win32" ? "ocpm.exe" : "ocpm";
const targetDir = path.join(packageRoot, "dist", "npm", `${process.platform}-${process.arch}`);
const targetPath = path.join(targetDir, binaryName);

if (existsSync(targetPath)) {
  process.exit(0);
}

mkdirSync(targetDir, { recursive: true });

const build = spawnSync(
  process.env.GO_BINARY || "go",
  ["build", "-trimpath", "-o", targetPath, "./cmd/ocpm"],
  {
    cwd: packageRoot,
    stdio: "inherit",
  },
);

if (build.error || build.status !== 0) {
  console.warn("[ocpm] No prebuilt binary was bundled and local Go compilation was unavailable.");
  console.warn("[ocpm] The launcher is installed, but `ocpm` will fail until a compatible binary is present.");
  process.exit(0);
}

if (process.platform !== "win32") {
  chmodSync(targetPath, 0o755);
}

