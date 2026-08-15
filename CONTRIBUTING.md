# Contributor setup

The pre-push workflow verifies both pinned real Runtimes. From a checkout of
this repository, prepare the development cache and install lefthook once:

```sh
learning-loop runtime-setup codex opencode
go tool lefthook install
```

The committed `lefthook.yml` runs exactly
`learning-loop conformance codex opencode`. It resolves `learning-loop` through
`PATH`, so `PATH` must point to the executable under development. If it does
not, lefthook reports the job failure when a push is attempted.

Lefthook does not discard a foreign pre-push hook. When one already exists, it
renames it to `pre-push.old`; `go tool lefthook uninstall` restores that hook.
If a personal hook should also run, compose it explicitly in
`lefthook-local.yml` rather than changing the committed configuration.

To remove the contributor hook:

```sh
go tool lefthook uninstall
```

`git push --no-verify` is the ordinary explicit bypass for the complete
pre-push workflow. The `learning-loop` CLI does not install, recognize,
compose, or remove Git hooks; `connect` and `disconnect` only manage the
selected Runtime's project integration.
