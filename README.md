# ocpm

`ocpm` is the Apache-2.0 OpenClaw package manager CLI for installing reusable agent packages into OpenClaw workspaces.

This first production-oriented version is intentionally conservative:

- simple Go CLI with Cobra
- mockable registry abstraction
- explicit workspace targeting
- safe managed ownership for root files
- lockfile-based reproducibility
- no hidden daemons or background state

## Supported Commands

```bash
ocpm init
ocpm add <package>
ocpm install <package>
ocpm create <package>
ocpm remove <package>
ocpm update [<package>]
ocpm list
ocpm doctor
ocpm pack
ocpm publish
ocpm auth login
ocpm auth status
ocpm auth logout
ocpm version
```

`ocpm install` is a friendly alias for `ocpm add`. By default, `add` and `install` only allow `skill` and `overlay` packages. `agent` and `workspace-template` packages must normally be used with `ocpm create`.

## Package Kinds

- `skill`: install into an existing workspace with `ocpm add`
- `overlay`: install into an existing workspace with `ocpm add`
- `workspace-template`: create a fresh workspace with `ocpm create`
- `agent`: create a fresh workspace with `ocpm create`

## Workspace Targeting

`ocpm` resolves the target workspace in this order:

1. `--workspace <path>`
2. current directory, if it already looks like an OpenClaw workspace
3. the default OpenClaw workspace, if `openclaw` is installed and can report one
4. otherwise fail with an actionable error

A directory counts as a workspace when it has at least one strong indicator:

- `AGENTS.md`
- `.ocpm/`
- `skills/` plus one of `SOUL.md`, `IDENTITY.md`, `TOOLS.md`, or `MEMORY.md`

## Ownership Model

`ocpm` does not blindly spray files into the current directory.

For installs into existing workspaces:

- package payloads live under `.ocpm/packages/<package>/<version>/...`
- skills are materialized under `skills/<skill-name>/`
- root files such as `AGENTS.md`, `SOUL.md`, `TOOLS.md`, `IDENTITY.md`, and `MEMORY.md` are updated only through managed sections

Managed sections look like:

```md
<!-- ocpm:begin @acme/browser-skill -->
... package-managed content ...
<!-- ocpm:end @acme/browser-skill -->
```

That makes re-install, update, and uninstall idempotent and keeps user-owned content outside managed regions intact.

For fresh workspaces created with `ocpm create`, direct file generation is allowed.

## Manifest

`ocpm.json` can describe either a workspace or a publishable package.

Workspace example:

```json
{
  "private": true,
  "workspace": true,
  "dependencies": {
    "@acme/browser-skill": ""
  },
  "options": {
    "@acme/browser-skill": {
      "homepage": "https://openclaw.dev"
    }
  }
}
```

Package example:

```json
{
  "name": "@acme/browser-skill",
  "version": "0.1.0",
  "kind": "skill",
  "description": "Browser automation skill for OpenClaw workspaces",
  "license": "Apache-2.0",
  "files": [
    "skills/browser",
    "templates/AGENTS.md"
  ],
  "publishConfig": {
    "access": "public",
    "tag": "latest"
  },
  "ocpm": {
    "engines": {
      "openclaw": "^1.0.0"
    },
    "skills": [
      "skills/browser"
    ],
    "templates": [
      "templates/AGENTS.md"
    ]
  }
}
```

Important workspace fields:

- `private`: whether the workspace manifest is private by default
- `workspace`: whether the manifest describes a workspace
- `dependencies`: top-level desired packages and optional version constraints
- `options`: chosen top-level package option values

Important publish fields:

- `name`: required package name
- `version`: required semver version
- `kind`: required package kind
- `description`, `license`, `repository`, `homepage`, `keywords`: package metadata
- `files`: optional explicit allowlist for publishable content
- `publishConfig`: optional defaults for registry URL, access, and tag
- `ocpm`: package metadata for engines, skills, templates, and payload references

## Lockfile

`ocpm-lock.json` pins the exact resolved state:

```json
{
  "version": 1,
  "workspacePath": "/path/to/workspace",
  "generatedAt": "2026-03-13T10:00:00Z",
  "packages": [
    {
      "name": "@acme/browser-skill",
      "version": "1.0.0",
      "kind": "skill",
      "integrity": "sha256:...",
      "resolvedURL": "mock://registry/acme/browser-skill/1.0.0",
      "options": {
        "homepage": "https://openclaw.dev"
      },
      "dependencies": [],
      "installedFiles": [
        {
          "path": "skills/browser/SKILL.md",
          "integrity": "sha256:..."
        }
      ],
      "managedSections": [
        {
          "file": "AGENTS.md",
          "owner": "@acme/browser-skill"
        }
      ],
      "installedSkills": [
        {
          "name": "browser",
          "path": "skills/browser"
        }
      ]
    }
  ]
}
```

The lockfile persists exact versions, integrity, selected options, installed file ownership, and managed section ownership.

## Packing And Publishing

`ocpm pack` and `ocpm publish` share the same deterministic packaging pipeline.

- `ocpm pack`: validates the current package and writes a local `.tgz`
- `ocpm publish --dry-run`: validates and reports what would be published without writing or uploading
- `ocpm publish --out <file>`: writes the packaged artifact locally instead of uploading
- `ocpm publish --yes`: validates, assembles, and uploads through the registry client abstraction

Human output and `--json` both include:

- package name, version, kind, tag, and access level
- file count
- compressed archive size
- uncompressed content size
- `sha256` and integrity string
- warnings for suspiciously large artifacts

## Include And Exclude Rules

Publish artifacts are small and explicit by default.

Precedence:

1. if `ocpm.json` defines `files`, that allowlist is used
2. otherwise `ocpm` includes only:
   - `ocpm.json`
   - `README`, `README.md`, `README.txt`
   - `LICENSE`, `LICENSE.md`, `LICENSE.txt`
   - `skills/`
   - `templates/`
   - `payload/`
3. hard excludes always win

`.ocpmignore` is supported with a simple glob model. It applies only when `files` is not explicitly allowlisting the same paths.

Hard excludes include:

- `.git/`
- `.ocpm/`
- `memory/`
- `.env` and `.env.*`
- `*.log`, `*.tmp`
- `.DS_Store`
- temp and output folders such as `tmp/`, `temp/`, `dist/`, `coverage/`, `node_modules/`
- unrelated lockfiles such as `ocpm-lock.json`, `package-lock.json`, `pnpm-lock.yaml`, and `yarn.lock`
- obvious key material such as `*.pem`, `*.key`, `*.p12`, and `*.pfx`

If a forbidden path is explicitly listed in `files`, publish fails with a clear error.

## Publish Policy

The first publish policy is intentionally strict:

- warning above 5 MB of uncompressed content
- fail above 10 MB for public packages
- hard fail above 20 MB for all packages
- symlinks are rejected
- empty packages are rejected
- package versions are treated as immutable

The archive is normalized as `tar.gz` with stable file ordering, normalized timestamps, normalized permissions, and a fixed `package/` root.

## Examples

Initialize a manifest:

```bash
ocpm init --workspace-manifest
```

Add a skill into an existing workspace:

```bash
ocpm add @acme/browser-skill --workspace /path/to/workspace
```

Create a new workspace from an agent:

```bash
ocpm create @acme/founder-agent --dir /path/to/new-workspace
```

Remove a package:

```bash
ocpm remove @acme/browser-skill --workspace /path/to/workspace
```

Preview changes without writing:

```bash
ocpm add @acme/browser-skill --workspace /path/to/workspace --dry-run
ocpm update --workspace /path/to/workspace --dry-run
ocpm create @acme/founder-agent --dir /path/to/new-workspace --dry-run
ocpm remove @acme/browser-skill --workspace /path/to/workspace --dry-run
```

Machine-readable inspection:

```bash
ocpm list --workspace /path/to/workspace --json
ocpm doctor --workspace /path/to/workspace --json
```

Pack a publishable package locally:

```bash
ocpm pack --path /path/to/package --out /tmp/browser-skill.tgz
```

Dry-run a publish:

```bash
ocpm publish --path /path/to/package --dry-run --list
```

Publish through the registry abstraction:

```bash
ocpm publish --path /path/to/package --yes
```

Authenticate the CLI with the hosted service:

```bash
ocpm auth login
ocpm auth status
ocpm auth logout
```

## Mock Registry Fixtures

The current implementation ships with an in-memory fixture registry for development and tests. The built-in fixtures are:

- `@acme/browser-skill`
- `@acme/sales-overlay`
- `@acme/founder-agent`
- `@acme/support-template`

This keeps the CLI deterministic while the hosted registry API remains out of scope.

## OpenClaw Integration

`ocpm` can:

- detect whether `openclaw` is on `PATH`
- ask `openclaw` for a default workspace using a small shell adapter
- optionally run `openclaw setup --workspace <path>` during `ocpm create`

It does not edit OpenClaw global config directly.

## Registry And Auth Foundations

The current publish flow uses a mock registry client for tests and local development, but the CLI already has the basic plumbing for future hosted publishing:

- `OCPM_REGISTRY_URL` overrides the default registry URL
- `OCPM_TOKEN` is reserved for future authenticated publish flows
- `OCPM_AUTH_URL` overrides the hosted auth base URL used by `ocpm auth`
- package access can be set with `--access public|private`, `--private`, or `publishConfig.access`

Published versions are treated as immutable. The mock registry rejects duplicate version publishes to match that model.

## Development

```bash
make fmt
make test
make build
./dist/ocpm doctor
```

Or run directly:

```bash
go run ./cmd/ocpm add @acme/browser-skill --workspace /path/to/workspace
```
