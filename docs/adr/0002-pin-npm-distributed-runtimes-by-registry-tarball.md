# Pin npm-distributed Runtimes by registry tarball and recorded integrity

The Runtime cache was built for Runtimes published as single release binaries with published SHA256 sums (Codex, OpenCode). pi is distributed only as the npm package `@earendil-works/pi-coding-agent`, so we decided the cache will pin such Runtimes by their npm registry tarball URL with the registry-published `dist.integrity` (sha512) recorded exactly like a binary checksum: fetched once by `learning-loop runtime-setup`, verified, extracted into the cache, and never downloaded at conformance time. The conformance launch invokes the cached tree via `node`, making a Node.js runtime a documented prerequisite for running pi conformance cases.

## Considered Options

- **Exclude npm-distributed Runtimes from the conformance suite** (ship the pi Adapter and Installer without real-Runtime verification). Rejected: it silently creates a second class of Runtime that the shared-harness and pin-exactly guarantees (issue #1 stories 39–42) do not cover, and the gap would surface as undetected upstream contract drift.
- **Resolve pi from the system or via `npm exec` at conformance time.** Rejected: it downloads or floats at verification time, breaking the pinned, checksummed, pre-populated-cache contract that makes upstream drift explicit.

## Consequences

- The cache abstraction now has two entry shapes — single binary and extracted npm tree — and the checksum kind follows each Runtime's distribution channel (SHA256 sums vs npm sha512 integrity) rather than being uniform.
- The published pi package is not self-contained: its runtime dependencies are pinned by the `npm-shrinkwrap.json` inside the verified tarball, so `learning-loop runtime-setup pi` also runs `npm install --omit=dev` in the extracted tree once. The dependency tree is therefore pinned transitively by the tarball integrity, and conformance still never downloads.
- `Case.Launch` for pi requires `node` on PATH in addition to the cached Runtime, and setup requires `npm`; contributor and CI environments must provide a Node.js runtime (which ships npm).
- Pin migrations for pi mean bumping the npm version and its recorded integrity together, one pin at a time, same as binary Runtimes.
