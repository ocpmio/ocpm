#!/usr/bin/env node

const { chmodSync, copyFileSync, existsSync, mkdirSync, readdirSync, statSync } = require("node:fs");
const path = require("node:path");

const packageRoot = path.resolve(__dirname, "..");
const distDir = path.join(packageRoot, "dist");
const npmDir = path.join(distDir, "npm");
const binaryPattern = /(darwin|linux|windows)_(amd64|arm64)/;

if (!existsSync(distDir)) {
  console.error("[ocpm] dist/ does not exist. Run GoReleaser first.");
  process.exit(1);
}

function walk(dir) {
  const entries = readdirSync(dir, { withFileTypes: true });
  const files = [];

  for (const entry of entries) {
    const nextPath = path.join(dir, entry.name);
    if (entry.isDirectory()) {
      files.push(...walk(nextPath));
      continue;
    }

    if (entry.isFile()) {
      files.push(nextPath);
    }
  }

  return files;
}

const staged = [];
for (const file of walk(distDir)) {
  const base = path.basename(file);
  if (base !== "ocpm" && base !== "ocpm.exe") {
    continue;
  }

  const match = file.match(binaryPattern);
  if (!match) {
    continue;
  }

  const targetDir = path.join(npmDir, `${match[1]}-${match[2]}`);
  mkdirSync(targetDir, { recursive: true });

  const targetPath = path.join(targetDir, base);
  copyFileSync(file, targetPath);
  if (process.platform !== "win32" || base !== "ocpm.exe") {
    chmodSync(targetPath, 0o755);
  }
  staged.push(targetPath);
}

if (staged.length === 0) {
  console.error("[ocpm] No release binaries were found under dist/.");
  process.exit(1);
}

console.log(`[ocpm] Staged ${staged.length} binaries for npm packaging.`);

