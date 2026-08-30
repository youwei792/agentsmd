#!/usr/bin/env bash
# Build the npm distribution from a published GitHub release.
#
# Usage: scripts/build-npm.sh <version>   (e.g. 0.2.0)
#
# Downloads the release tarballs, verifies them against the published
# checksums.txt, and assembles the main package + 6 platform packages under
# npm/dist/. Publish each with `npm publish npm/dist/<name>`.
#
# Requires: curl, tar, sha256sum (or shasum).
set -euo pipefail

VERSION="${1:?usage: build-npm.sh <version>}"
REPO="youwei792/agentsmd"
BASE="https://github.com/${REPO}/releases/download/v${VERSION}"
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
DIST="${ROOT}/npm/dist"
rm -rf "$DIST"
mkdir -p "$DIST"

sum() {
  if command -v sha256sum >/dev/null 2>&1; then sha256sum "$1" | awk '{print $1}'
  else shasum -a 256 "$1" | awk '{print $1}'; fi
}

cd "$DIST"
curl -fsSL -o checksums.txt "${BASE}/checksums.txt"

for target in darwin-arm64 darwin-amd64 linux-arm64 linux-amd64 windows-amd64 windows-arm64; do
  tarball="agentsmd_${target/-/_}.tar.gz"
  curl -fsSL -o "$tarball" "${BASE}/${tarball}"
  want="$(grep "  ${tarball}\$" checksums.txt | awk '{print $1}')"
  got="$(sum "$tarball")"
  if [ "$want" != "$got" ]; then
    echo "checksum mismatch for ${tarball}: want ${want}, got ${got}" >&2
    exit 1
  fi
  tar -xzf "$tarball"
  undir="agentsmd_${target/-/_}"
  bin="${undir}/agentsmd"
  ext=""
  case "$target" in windows-*) ext=".exe"; bin="${undir}/agentsmd.exe";; esac

  pkg="agentsmd-${target}"
  mkdir -p "${pkg}/bin"
  cp "$bin" "${pkg}/bin/agentsmd${ext}"
  chmod +x "${pkg}/bin/agentsmd${ext}"
  cat > "${pkg}/package.json" <<JSON
{
  "name": "${pkg}",
  "version": "${VERSION}",
  "description": "agentsmd binary for ${target} (installs via the agentsmd package)",
  "license": "MIT",
  "repository": "https://github.com/${REPO}",
  "os": ["${target%%-*}"],
  "cpu": ["${target##*-}"],
  "files": ["bin/"]
}
JSON
done

mkdir -p "agentsmd/bin"
cat > "agentsmd/bin/agentsmd.js" <<'JS'
#!/usr/bin/env node
// agentsmd npm shim: resolve the platform binary and forward execution.
const { spawnSync } = require("child_process");
const path = require("path");

const plat = { darwin: "darwin", linux: "linux", win32: "windows" }[process.platform];
const arch = { arm64: "arm64", x64: "amd64" }[process.arch];
if (!plat || !arch) {
  console.error(`agentsmd: unsupported platform ${process.platform}/${process.arch}`);
  process.exit(1);
}
const ext = process.platform === "win32" ? ".exe" : "";
const bin = path.join(__dirname, "..", "..", `agentsmd-${plat}-${arch}`, "bin", `agentsmd${ext}`);

let result;
try {
  result = spawnSync(bin, process.argv.slice(2), { stdio: "inherit" });
} catch (e) {
  console.error(`agentsmd: failed to run ${bin}: ${e.message}`);
  console.error("reinstall with: npm install agentsmd --force, or use: go install github.com/youwei792/agentsmd@latest");
  process.exit(1);
}
if (result.error) {
  console.error(`agentsmd: platform binary missing (${result.error.message})`);
  console.error("reinstall with: npm install agentsmd --force, or use: go install github.com/youwei792/agentsmd@latest");
  process.exit(1);
}
process.exit(result.status ?? 0);
JS

cat > "agentsmd/package.json" <<JSON
{
  "name": "agentsmd",
  "version": "${VERSION}",
  "description": "CI for your AI agent's instructions: validate AGENTS.md, bridge CLAUDE.md. Zero-dependency Go binary.",
  "license": "MIT",
  "repository": "https://github.com/${REPO}",
  "homepage": "https://github.com/${REPO}#readme",
  "bin": { "agentsmd": "bin/agentsmd.js" },
  "files": ["bin/"],
  "optionalDependencies": {
    "agentsmd-darwin-arm64": "${VERSION}",
    "agentsmd-darwin-amd64": "${VERSION}",
    "agentsmd-linux-arm64": "${VERSION}",
    "agentsmd-linux-amd64": "${VERSION}",
    "agentsmd-windows-arm64": "${VERSION}",
    "agentsmd-windows-amd64": "${VERSION}"
  },
  "keywords": ["agents-md","claude-code","cursor","codex","ai-agents","ci","lint"]
}
JSON

cat > "agentsmd/README.md" <<MD
# agentsmd (npm distribution)

CI for your AI agent's instructions. See the
[full documentation](https://github.com/youwei792/agentsmd#readme).

\`\`\`bash
npm install -g agentsmd
agentsmd doctor
\`\`\`
MD

echo "assembled:"
ls -d agentsmd agentsmd-* | sed 's/^/  /'
echo "publish each with: npm publish npm/dist/<name>"
