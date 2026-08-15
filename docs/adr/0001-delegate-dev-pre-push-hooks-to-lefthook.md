# Delegate development pre-push hooks to lefthook

The contributor pre-push workflow (run both Runtime conformance cases before every verified push) needs a Git hook in this repository only. We decided the `learning-loop` CLI will **not** implement any Git hook management: hook installation and removal are delegated to [lefthook](https://lefthook.dev) v2, pinned as a Go tool dependency (`go get -tool github.com/evilmartians/lefthook/v2`), driven by a committed `lefthook.yml` whose pre-push job runs the stable command `learning-loop conformance codex opencode`. Contributor setup is an explicit documented step (`go tool lefthook install`); removal is `go tool lefthook uninstall`.

## Considered Options

- **CLI-owned hooks with byte-exact canonical forms** (the original spec): the CLI writes, recognizes, upgrades, composes, and removes exact hook content. Rejected: the ownership/recognition/composition machinery (exact-content matching, `pre-push.before-learning-loop` preservation, wrapper scripts, rollback, restore-on-remove) is a large implementation and test surface that only exists to let the CLI mutate hook files safely. Delegation deletes the problem rather than solving it, and keeps the shipped binary free of hook-management code and dependencies.
- **CLI orchestrates lefthook with safeguard preflights** (repo-identity check, PATH check, foreign-hook refusal before invoking lefthook). Rejected: the only population that ever runs hook setup is maintainers of this repository, performing an explicit, documented opt-in. Lefthook's foreign-hook behavior (rename to `<hook>.old`, restore on uninstall) is documented and non-destructive. Safeguards would paternalize lefthook's own users to protect against a self-inflicted, recoverable inconvenience.

## Consequences

- The hook logic becomes committed, PR-reviewable repo content (`lefthook.yml`) instead of generated file content; drift is handled by git diffs, not by an upgrade path for older canonical forms.
- `learning-loop runtime-setup` is cache preparation only; `learning-loop pre-push-remove` does not exist.
- Inherited for free from lefthook: correct linked-worktree hook resolution (via `git rev-parse --git-path hooks`), refusal to install when `core.hooksPath` is configured, `git push --no-verify` bypass, and `LEFTHOOK=0` opt-out.
- Lefthook requires Go >= 1.26 as a tool dependency; this repository's `go.mod` already requires it.
- The `lefthook.yml` job invokes `learning-loop` via PATH; if PATH resolution is wrong, the failure surfaces at push time with lefthook's own diagnostics (documented in contributor setup docs).
