# Ponytail architecture study

Primary-source study of [`dietrichgebert/ponytail`](https://github.com/dietrichgebert/ponytail) at commit [`2ed6c52`](https://github.com/dietrichgebert/ponytail/commit/2ed6c52c9d7e5e56942508591085fd45dea277d3), inspected locally at `/home/datguy/ponytail` on 2026-08-14.

## Executive conclusion

Ponytail supports many agent runtimes without implementing a universal agent runtime. Its stable center is much smaller:

1. canonical instruction content in `skills/` (plus a compact `AGENTS.md` fallback),
2. shared mode/configuration and instruction-rendering functions,
3. host-native adapters that deliver the rendered text through whatever surface a host actually provides.

The host-native part is the important lesson. Claude/Codex/Copilot/Qoder can share command-hook scripts because their protocols are similar enough. OpenCode uses its JavaScript plugin callbacks instead. Pi uses its extension API. Hermes uses a Python plugin. Gemini and several other hosts simply load an instruction file or skills. Grok deliberately has no lifecycle hook because its hook output cannot inject instructions. Ponytail shares the *meaning* of “render these instructions for this mode,” but it does not pretend that all hosts have the same events or capabilities ([`docs/agent-portability.md:L9-L39`](https://github.com/dietrichgebert/ponytail/blob/2ed6c52c9d7e5e56942508591085fd45dea277d3/docs/agent-portability.md#L9-L39)).

For `learning-loop-cli`, the reusable architecture is therefore a Go semantic core plus explicit Codex and OpenCode adapters. The OpenCode adapter will still need to be a very small JavaScript module, because OpenCode's injection point is a JavaScript plugin callback; it can call the Go CLI for selection/rendering. The Go core should not absorb OpenCode event names, payloads, installation details, or state conventions.

## 1. Overall architecture and package boundaries

### 1.1 Canonical behavior/content

The full Ponytail behavior is a skill, `skills/ponytail/SKILL.md`. It contains activation metadata, the rules, and mode-specific variants ([`skills/ponytail/SKILL.md:L1-L18`](https://github.com/dietrichgebert/ponytail/blob/2ed6c52c9d7e5e56942508591085fd45dea277d3/skills/ponytail/SKILL.md#L1-L18), [`skills/ponytail/SKILL.md:L77-L88`](https://github.com/dietrichgebert/ponytail/blob/2ed6c52c9d7e5e56942508591085fd45dea277d3/skills/ponytail/SKILL.md#L77-L88)). Other operations are separate skills under `skills/`, not methods on a runtime abstraction ([`docs/agent-portability.md:L41-L49`](https://github.com/dietrichgebert/ponytail/blob/2ed6c52c9d7e5e56942508591085fd45dea277d3/docs/agent-portability.md#L41-L49)).

`AGENTS.md` is a compact, instruction-only representation for hosts that natively read repository instructions ([`AGENTS.md:L1-L30`](https://github.com/dietrichgebert/ponytail/blob/2ed6c52c9d7e5e56942508591085fd45dea277d3/AGENTS.md#L1-L30)). Copies for Cursor, Windsurf, Cline, Copilot, Kiro, and Qoder are checked against it, while only invariant phrases are checked between `AGENTS.md` and the longer skill ([`scripts/check-rule-copies.js:L15-L43`](https://github.com/dietrichgebert/ponytail/blob/2ed6c52c9d7e5e56942508591085fd45dea277d3/scripts/check-rule-copies.js#L15-L43), [`scripts/check-rule-copies.js:L44-L69`](https://github.com/dietrichgebert/ponytail/blob/2ed6c52c9d7e5e56942508591085fd45dea277d3/scripts/check-rule-copies.js#L44-L69)). This means Ponytail has a practical canonical source for dynamic injection, but not one generated canonical artifact for every distribution format.

### 1.2 Shared JavaScript semantic helpers

Three files form the reusable center of the JavaScript adapters:

- `hooks/ponytail-config.js` defines valid modes, resolves defaults from environment → config file → built-in default, and owns the optional global config path ([`hooks/ponytail-config.js:L4-L18`](https://github.com/dietrichgebert/ponytail/blob/2ed6c52c9d7e5e56942508591085fd45dea277d3/hooks/ponytail-config.js#L4-L18), [`hooks/ponytail-config.js:L54-L100`](https://github.com/dietrichgebert/ponytail/blob/2ed6c52c9d7e5e56942508591085fd45dea277d3/hooks/ponytail-config.js#L54-L100)).
- `hooks/ponytail-instructions.js` removes skill frontmatter, filters mode-specific rows/examples, and renders the final injected text from the canonical skill ([`hooks/ponytail-instructions.js:L8-L40`](https://github.com/dietrichgebert/ponytail/blob/2ed6c52c9d7e5e56942508591085fd45dea277d3/hooks/ponytail-instructions.js#L8-L40), [`hooks/ponytail-instructions.js:L77-L91`](https://github.com/dietrichgebert/ponytail/blob/2ed6c52c9d7e5e56942508591085fd45dea277d3/hooks/ponytail-instructions.js#L77-L91)).
- `hooks/ponytail-runtime.js` is a compatibility bridge for the subset of hosts using command lifecycle hooks. It chooses state storage and serializes each host's required output shape ([`hooks/ponytail-runtime.js:L19-L31`](https://github.com/dietrichgebert/ponytail/blob/2ed6c52c9d7e5e56942508591085fd45dea277d3/hooks/ponytail-runtime.js#L19-L31), [`hooks/ponytail-runtime.js:L51-L90`](https://github.com/dietrichgebert/ponytail/blob/2ed6c52c9d7e5e56942508591085fd45dea277d3/hooks/ponytail-runtime.js#L51-L90)).

The first two are genuinely semantic. The third is adapter infrastructure, despite living in a shared file.

### 1.3 Lifecycle hook executables

The command-hook family consists of three small Node processes:

- `ponytail-activate.js`: choose the default, write active-mode state, build instructions, and emit session-start context ([`hooks/ponytail-activate.js:L24-L42`](https://github.com/dietrichgebert/ponytail/blob/2ed6c52c9d7e5e56942508591085fd45dea277d3/hooks/ponytail-activate.js#L24-L42), [`hooks/ponytail-activate.js:L92-L96`](https://github.com/dietrichgebert/ponytail/blob/2ed6c52c9d7e5e56942508591085fd45dea277d3/hooks/ponytail-activate.js#L92-L96)).
- `ponytail-mode-tracker.js`: consume a prompt event from stdin, recognize mode commands, update state, and—in Qoder—also inject every turn because Qoder lacks `SessionStart` ([`hooks/ponytail-mode-tracker.js:L12-L23`](https://github.com/dietrichgebert/ponytail/blob/2ed6c52c9d7e5e56942508591085fd45dea277d3/hooks/ponytail-mode-tracker.js#L12-L23), [`hooks/ponytail-mode-tracker.js:L91-L112`](https://github.com/dietrichgebert/ponytail/blob/2ed6c52c9d7e5e56942508591085fd45dea277d3/hooks/ponytail-mode-tracker.js#L91-L112)).
- `ponytail-subagent.js`: read active state and inject the same rendered instructions into subagents, optionally filtered by agent type ([`hooks/ponytail-subagent.js:L13-L29`](https://github.com/dietrichgebert/ponytail/blob/2ed6c52c9d7e5e56942508591085fd45dea277d3/hooks/ponytail-subagent.js#L13-L29), [`hooks/ponytail-subagent.js:L31-L70`](https://github.com/dietrichgebert/ponytail/blob/2ed6c52c9d7e5e56942508591085fd45dea277d3/hooks/ponytail-subagent.js#L31-L70)).

These hooks are explicitly best-effort. Exceptions are swallowed, and stdin-reading processes self-terminate after one second to avoid freezing a host session ([`hooks/ponytail-mode-tracker.js:L114-L130`](https://github.com/dietrichgebert/ponytail/blob/2ed6c52c9d7e5e56942508591085fd45dea277d3/hooks/ponytail-mode-tracker.js#L114-L130), [`hooks/ponytail-subagent.js:L73-L77`](https://github.com/dietrichgebert/ponytail/blob/2ed6c52c9d7e5e56942508591085fd45dea277d3/hooks/ponytail-subagent.js#L73-L77)).

### 1.4 Native adapters

Where a host has a richer or incompatible plugin API, Ponytail writes a native adapter rather than forcing it through the command-hook bridge:

- OpenCode: an ESM server plugin under `.opencode/plugins/`.
- Pi: an ESM extension under `pi-extension/`.
- Hermes: a Python plugin at repository root.
- MCP: a private, optional stdio server that exposes instructions as a prompt and read-only tool, explicitly *not* replacing always-on adapters ([`ponytail-mcp/index.js:L1-L16`](https://github.com/dietrichgebert/ponytail/blob/2ed6c52c9d7e5e56942508591085fd45dea277d3/ponytail-mcp/index.js#L1-L16), [`ponytail-mcp/index.js:L23-L49`](https://github.com/dietrichgebert/ponytail/blob/2ed6c52c9d7e5e56942508591085fd45dea277d3/ponytail-mcp/index.js#L23-L49)).

Pi directly reuses the shared config and instruction functions, but owns Pi-specific session entries, commands, status UI, and `before_agent_start` prompt mutation ([`pi-extension/index.js:L1-L20`](https://github.com/dietrichgebert/ponytail/blob/2ed6c52c9d7e5e56942508591085fd45dea277d3/pi-extension/index.js#L1-L20), [`pi-extension/index.js:L183-L210`](https://github.com/dietrichgebert/ponytail/blob/2ed6c52c9d7e5e56942508591085fd45dea277d3/pi-extension/index.js#L183-L210)). Hermes cannot reuse the JavaScript functions, so it repeats mode normalization and skill filtering in Python before registering native hooks and commands ([`__init__.py:L30-L87`](https://github.com/dietrichgebert/ponytail/blob/2ed6c52c9d7e5e56942508591085fd45dea277d3/__init__.py#L30-L87), [`__init__.py:L195-L217`](https://github.com/dietrichgebert/ponytail/blob/2ed6c52c9d7e5e56942508591085fd45dea277d3/__init__.py#L195-L217)).

### 1.5 Packaging and verification

The repository is also a multi-ecosystem distribution bundle: marketplace metadata, host manifests, npm metadata, command templates, rule copies, and generated OpenClaw skills. The npm package exports the OpenCode plugin as its entry point and separately declares the Pi extension and skills ([`package.json:L19-L42`](https://github.com/dietrichgebert/ponytail/blob/2ed6c52c9d7e5e56942508591085fd45dea277d3/package.json#L19-L42)).

This breadth creates release bookkeeping: a script enumerates eight version-bearing files across host ecosystems and fails if they drift ([`scripts/check-versions.js:L1-L10`](https://github.com/dietrichgebert/ponytail/blob/2ed6c52c9d7e5e56942508591085fd45dea277d3/scripts/check-versions.js#L1-L10), [`scripts/check-versions.js:L20-L30`](https://github.com/dietrichgebert/ponytail/blob/2ed6c52c9d7e5e56942508591085fd45dea277d3/scripts/check-versions.js#L20-L30)). Adapter tests are structural contract tests: they invoke hooks with representative payloads and assert exact state/output behavior rather than launching every host ([`tests/hooks.test.js:L21-L27`](https://github.com/dietrichgebert/ponytail/blob/2ed6c52c9d7e5e56942508591085fd45dea277d3/tests/hooks.test.js#L21-L27), [`tests/opencode-plugin.test.js:L1-L35`](https://github.com/dietrichgebert/ponytail/blob/2ed6c52c9d7e5e56942508591085fd45dea277d3/tests/opencode-plugin.test.js#L1-L35)).

## 2. Exact hook/plugin mechanism

### 2.1 Codex (and the Claude-compatible hook family)

The Codex manifest declares `skills/` and points to `hooks/claude-codex-hooks.json` ([`.codex-plugin/plugin.json:L13-L21`](https://github.com/dietrichgebert/ponytail/blob/2ed6c52c9d7e5e56942508591085fd45dea277d3/.codex-plugin/plugin.json#L13-L21)). That hook map launches Node commands for:

- `SessionStart` (`startup|resume|clear|compact`) → `ponytail-activate.js`,
- `SubagentStart` → `ponytail-subagent.js`,
- `UserPromptSubmit` → `ponytail-mode-tracker.js`.

Each command has a five-second host timeout ([`hooks/claude-codex-hooks.json:L1-L40`](https://github.com/dietrichgebert/ponytail/blob/2ed6c52c9d7e5e56942508591085fd45dea277d3/hooks/claude-codex-hooks.json#L1-L40)).

At process startup, `ponytail-runtime.js` identifies Codex by `PLUGIN_DATA` (unless Copilot was detected first), stores the current mode at `$PLUGIN_DATA/.ponytail-active`, and writes Codex's exact response schema: a visible `systemMessage` plus `hookSpecificOutput.{hookEventName,additionalContext}` ([`hooks/ponytail-runtime.js:L19-L31`](https://github.com/dietrichgebert/ponytail/blob/2ed6c52c9d7e5e56942508591085fd45dea277d3/hooks/ponytail-runtime.js#L19-L31), [`hooks/ponytail-runtime.js:L58-L67`](https://github.com/dietrichgebert/ponytail/blob/2ed6c52c9d7e5e56942508591085fd45dea277d3/hooks/ponytail-runtime.js#L58-L67)). Tests assert this exact schema and state location ([`tests/hooks.test.js:L49-L68`](https://github.com/dietrichgebert/ponytail/blob/2ed6c52c9d7e5e56942508591085fd45dea277d3/tests/hooks.test.js#L49-L68)).

Claude uses the same executables but different output: raw stdout for `SessionStart`, structured JSON for `SubagentStart`. Copilot uses a separate event-name manifest and only consumes `additionalContext` at session start. Qoder uses `UserPromptSubmit` as activation plus per-turn injection and maps `PreToolUse(task|Task)` to the subagent script ([`hooks/ponytail-runtime.js:L51-L89`](https://github.com/dietrichgebert/ponytail/blob/2ed6c52c9d7e5e56942508591085fd45dea277d3/hooks/ponytail-runtime.js#L51-L89), [`hooks/qoder-hooks.json:L3-L25`](https://github.com/dietrichgebert/ponytail/blob/2ed6c52c9d7e5e56942508591085fd45dea277d3/hooks/qoder-hooks.json#L3-L25)).

### 2.2 OpenCode

OpenCode loads `.opencode/plugins/ponytail.mjs` as a JavaScript server plugin. The plugin returns three host callbacks:

1. `config(config)`: parse the bundled Markdown command files into `config.command`, then append the shared `skills/` path to `config.skills.paths` ([`.opencode/plugins/ponytail.mjs:L46-L71`](https://github.com/dietrichgebert/ponytail/blob/2ed6c52c9d7e5e56942508591085fd45dea277d3/.opencode/plugins/ponytail.mjs#L46-L71)).
2. `experimental.chat.system.transform`: on every turn, read the active mode, render the shared instructions, and append them to the last system entry (or create one if empty) ([`.opencode/plugins/ponytail.mjs:L73-L83`](https://github.com/dietrichgebert/ponytail/blob/2ed6c52c9d7e5e56942508591085fd45dea277d3/.opencode/plugins/ponytail.mjs#L73-L83)).
3. `command.execute.before`: when `/ponytail` runs, validate and persist its argument for subsequent transforms ([`.opencode/plugins/ponytail.mjs:L85-L97`](https://github.com/dietrichgebert/ponytail/blob/2ed6c52c9d7e5e56942508591085fd45dea277d3/.opencode/plugins/ponytail.mjs#L85-L97)).

OpenCode mode state is adapter-owned at `$XDG_CONFIG_HOME/opencode/.ponytail-active` (falling back to `~/.config/opencode/.ponytail-active`), not stored through `ponytail-runtime.js` ([`.opencode/plugins/ponytail.mjs:L26-L44`](https://github.com/dietrichgebert/ponytail/blob/2ed6c52c9d7e5e56942508591085fd45dea277d3/.opencode/plugins/ponytail.mjs#L26-L44)). The plugin still calls the shared instruction builder and default-mode resolver. Tests exercise the actual callback shapes, mode persistence, “off,” and mutation of an existing system message ([`tests/opencode-plugin.test.js:L38-L78`](https://github.com/dietrichgebert/ponytail/blob/2ed6c52c9d7e5e56942508591085fd45dea277d3/tests/opencode-plugin.test.js#L38-L78)).

This is the strongest direct precedent for `learning-loop-cli`: an OpenCode adapter is necessarily a native JavaScript callback surface, even if all instruction matching and rendering live in a Go executable.

## 3. How Ponytail supports many runtimes without over-unifying them

Ponytail uses capability tiers rather than one lowest-common-denominator interface:

| Host capability | Ponytail delivery |
|---|---|
| Command lifecycle hooks with compatible event concepts | Shared Node executables plus host-specific manifest/output encoding |
| Native prompt-transform/plugin API | Native OpenCode, Pi, or Hermes adapter |
| Skills and commands | Point the host at existing `skills/` and command assets |
| Always-on repository instructions only | `AGENTS.md` or a host-specific rule copy |
| Prompt/tool retrieval only | Optional MCP prompt/tool |
| Hook exists but cannot inject instructions | Do not use the hook; use skills/instructions instead |

The design rule is explicit: keep adapters thin, reuse `skills/` and `hooks/` when supported, and align instruction-only copies with `AGENTS.md` ([`docs/agent-portability.md:L35-L39`](https://github.com/dietrichgebert/ponytail/blob/2ed6c52c9d7e5e56942508591085fd45dea277d3/docs/agent-portability.md#L35-L39)). Gemini demonstrates restraint: its manifest only points `contextFileName` at `AGENTS.md`; tests ensure no `hooks/hooks.json` exists because Gemini would auto-load Claude/Codex event names it cannot understand ([`gemini-extension.json:L1-L6`](https://github.com/dietrichgebert/ponytail/blob/2ed6c52c9d7e5e56942508591085fd45dea277d3/gemini-extension.json#L1-L6), [`tests/gemini-extension.test.js:L25-L31`](https://github.com/dietrichgebert/ponytail/blob/2ed6c52c9d7e5e56942508591085fd45dea277d3/tests/gemini-extension.test.js#L25-L31), [`tests/gemini-extension.test.js:L85-L90`](https://github.com/dietrichgebert/ponytail/blob/2ed6c52c9d7e5e56942508591085fd45dea277d3/tests/gemini-extension.test.js#L85-L90)). Grok similarly uses native skill activation because its lifecycle hook output cannot inject instructions ([`README.md:L249-L264`](https://github.com/dietrichgebert/ponytail/blob/2ed6c52c9d7e5e56942508591085fd45dea277d3/README.md#L249-L264)).

History shows adapters being added independently rather than through a growing universal interface: [Codex support](https://github.com/dietrichgebert/ponytail/commit/c16f967d37866a242940553b1a3a55b262b854c5), the [thin OpenCode adapter](https://github.com/dietrichgebert/ponytail/commit/46c5c28b353ac6738ab4942aeaf3caa06894b80f), a [manifest-only Gemini adapter](https://github.com/dietrichgebert/ponytail/commit/e01aa900f7e12e8a4660fb5d757ad016baeffed9), a [native Hermes plugin](https://github.com/dietrichgebert/ponytail/commit/4198fc30ac6d5090b9c16dba9bac572fab59075e), and [Qoder-specific event handling](https://github.com/dietrichgebert/ponytail/commit/83493b97bc529e36444ab7f1730569de74a5acfb). Later fixes are host-specific too: [Codex output schema correction](https://github.com/dietrichgebert/ponytail/commit/3465b1a3ca4bdfaac0d9f67c7c969d0d0990f259) and [OpenCode/Qwen system-message compatibility](https://github.com/dietrichgebert/ponytail/commit/055a1453d31a7e15d81019fbba2f17efd089c627).

There is still a limited unification seam: `ponytail-runtime.js` detects several compatible hosts by environment variables and branches over their output schema. It succeeds because those hosts already share process-hook semantics. It is not evidence that native OpenCode/Pi/Hermes APIs should be folded into the same interface.

## 4. Runtime discovery, installation, and configuration generation

### 4.1 Runtime discovery

Ponytail does **not** scan the machine for agent runtimes. Installation is initiated through each host, and the host discovers Ponytail through its own package/manifest convention. The only “runtime detection” occurs after a shared hook process has already been launched: environment variables distinguish Copilot, Codex, Qoder, or native Claude so the process can choose state paths and output encoding ([`hooks/ponytail-runtime.js:L8-L31`](https://github.com/dietrichgebert/ponytail/blob/2ed6c52c9d7e5e56942508591085fd45dea277d3/hooks/ponytail-runtime.js#L8-L31)).

This inversion is important: Ponytail does not own runtime discovery; runtimes own plugin discovery.

### 4.2 Installation

Installation is host-native and documented separately:

- Codex/Claude/Copilot use marketplace/plugin commands ([`README.md:L114-L156`](https://github.com/dietrichgebert/ponytail/blob/2ed6c52c9d7e5e56942508591085fd45dea277d3/README.md#L114-L156)).
- OpenCode loads the npm package from `opencode.json`, or a checkout-relative `.mjs` path ([`README.md:L164-L180`](https://github.com/dietrichgebert/ponytail/blob/2ed6c52c9d7e5e56942508591085fd45dea277d3/README.md#L164-L180), [`opencode.json:L1-L4`](https://github.com/dietrichgebert/ponytail/blob/2ed6c52c9d7e5e56942508591085fd45dea277d3/opencode.json#L1-L4)).
- Pi installs the Git repository as a package; package metadata points to its extension and shared skills ([`README.md:L158-L162`](https://github.com/dietrichgebert/ponytail/blob/2ed6c52c9d7e5e56942508591085fd45dea277d3/README.md#L158-L162), [`package.json:L39-L42`](https://github.com/dietrichgebert/ponytail/blob/2ed6c52c9d7e5e56942508591085fd45dea277d3/package.json#L39-L42)).
- Instruction-tier hosts copy or natively read repository rule files ([`README.md:L274-L286`](https://github.com/dietrichgebert/ponytail/blob/2ed6c52c9d7e5e56942508591085fd45dea277d3/README.md#L274-L286)).

There is no central installer that modifies every host. Host uninstall removes plugin files; Ponytail's own script only removes the external state it owns ([`scripts/uninstall.js:L1-L10`](https://github.com/dietrichgebert/ponytail/blob/2ed6c52c9d7e5e56942508591085fd45dea277d3/scripts/uninstall.js#L1-L10), [`scripts/uninstall.js:L23-L30`](https://github.com/dietrichgebert/ponytail/blob/2ed6c52c9d7e5e56942508591085fd45dea277d3/scripts/uninstall.js#L23-L30)).

### 4.3 Configuration generation

Ponytail has no general configuration generator. It uses three narrower mechanisms:

1. **Static manifests** point hosts at hooks, skills, commands, or `AGENTS.md`.
2. **Native config mutation at plugin load** is used only where the API provides it—OpenCode's `config` callback registers command definitions and the skills path in memory ([`.opencode/plugins/ponytail.mjs:L53-L71`](https://github.com/dietrichgebert/ponytail/blob/2ed6c52c9d7e5e56942508591085fd45dea277d3/.opencode/plugins/ponytail.mjs#L53-L71)).
3. **Manual templates** are used where automatic installation is unavailable—Qoder's hook file explicitly tells the user to copy the hook object and replace `PONYTAIL_DIR` ([`hooks/qoder-hooks.json:L1-L10`](https://github.com/dietrichgebert/ponytail/blob/2ed6c52c9d7e5e56942508591085fd45dea277d3/hooks/qoder-hooks.json#L1-L10)).

The shared user configuration is intentionally optional: mode defaults to `full`; environment or a small config file can override it ([`hooks/ponytail-config.js:L76-L100`](https://github.com/dietrichgebert/ponytail/blob/2ed6c52c9d7e5e56942508591085fd45dea277d3/hooks/ponytail-config.js#L76-L100), [`README.md:L329-L335`](https://github.com/dietrichgebert/ponytail/blob/2ed6c52c9d7e5e56942508591085fd45dea277d3/README.md#L329-L335)).

## 5. What `learning-loop-cli` should reuse

### Reuse these decisions

1. **Share semantics, not runtime APIs.** Put Instruction Registry lookup, deterministic applicability matching, ordering, and rendering in Go. Give each adapter the narrow job of translating one native event into core input and translating selected instructions into one native output.

2. **Use explicit capability tiers.** For v0.1, Codex has executable lifecycle hooks and OpenCode has a system-prompt transform. Model those as two adapters, not two implementations of a speculative “universal agent” interface. If a future runtime only supports skills or an instruction file, support that honestly at a lower delivery level.

3. **Keep the runtime adapter thin.** Ponytail's OpenCode plugin is a good shape: native callbacks, tiny local state, and calls into a shared builder ([`.opencode/plugins/ponytail.mjs:L20-L24`](https://github.com/dietrichgebert/ponytail/blob/2ed6c52c9d7e5e56942508591085fd45dea277d3/.opencode/plugins/ponytail.mjs#L20-L24), [`.opencode/plugins/ponytail.mjs:L73-L97`](https://github.com/dietrichgebert/ponytail/blob/2ed6c52c9d7e5e56942508591085fd45dea277d3/.opencode/plugins/ponytail.mjs#L73-L97)). In the successor, the JS callback should invoke the Go core/CLI instead of reimplementing selection.

4. **Make hooks non-blocking and failure-contained.** Instruction delivery should not prevent the runtime from accepting the user's task. Bound execution time, produce no output on recoverable failure, and record adapter health separately if needed. Ponytail's timeout and silent-failure behavior is a strong operational precedent.

5. **Contract-test exact host payloads.** Keep fixtures for Codex stdin/stdout JSON and OpenCode callback inputs/outputs. Ponytail's tests caught real incompatibilities in strict Codex schemas and model handling without requiring a full host process.

6. **Let the host own installation discovery.** A `learning-loop install codex|opencode` convenience command may eventually generate or print the host-native setup, but the architecture should not begin with machine-wide runtime scanning. For v0.1, explicit adapter choice is more deterministic.

7. **Keep configuration tied to readers.** Ponytail's small config resolver follows the desired successor rule: each persisted field has implemented behavior. `learning-loop-cli` should go further and avoid serializing defaults until a user actually overrides one.

### Suggested v0.1 seam

```text
Codex event JSON ──> codex adapter ─┐
                                   ├─> Go Select(context) ─> ordered Instructions ─> Render
OpenCode callback ─> tiny JS shim ─┘

Codex adapter  <── renders Codex hook JSON
OpenCode shim  <── appends rendered text to OpenCode's native system entry
```

The common contract should describe only facts the Instruction Registry can deterministically use—runtime identity, lifecycle moment, repository/workspace identity, and available event metadata. It should not invent normalized tool names, permissions, session models, or workflow states before both adapters have an actual deterministic consumer for them.

## 6. What should not be copied

1. **Do not inject everything on every turn.** Ponytail is one always-on persona, so per-turn injection is appropriate for it. The successor has an Instruction Registry specifically to select relevant instructions. Copy the delivery seam, not Ponytail's unconditional policy.

2. **Do not copy the multi-runtime compatibility bridge prematurely.** `ponytail-runtime.js` branches on environment variables and output schemas because four similar hosts share its executables. With only Codex and OpenCode—and fundamentally different plugin APIs—two explicit adapters are clearer than an `if runtime == ...` serializer.

3. **Do not reimplement core semantics in adapters.** Hermes repeats the JavaScript mode/filtering logic in Python ([`__init__.py:L30-L87`](https://github.com/dietrichgebert/ponytail/blob/2ed6c52c9d7e5e56942508591085fd45dea277d3/__init__.py#L30-L87)). A Go executable gives `learning-loop-cli` a language-neutral process boundary; adapters should call it rather than duplicate registry selection or rendering.

4. **Do not hard-code a second full instruction fallback.** Ponytail carries a large fallback ruleset alongside the skill source ([`hooks/ponytail-instructions.js:L43-L74`](https://github.com/dietrichgebert/ponytail/blob/2ed6c52c9d7e5e56942508591085fd45dea277d3/hooks/ponytail-instructions.js#L43-L74)). For the successor, failure to read a versioned Instruction Registry should yield a clear health record/no injection, not a stale shadow registry embedded in adapter code.

5. **Do not copy rule-file proliferation as the starting architecture.** Ponytail's broad portability requires repeated host files plus drift checks. The successor's declared v0.1 runtimes are Codex and OpenCode; add formats only when a supported runtime requires them.

6. **Do not copy global mode state without deciding its scope.** OpenCode stores one active flag under global OpenCode config ([`.opencode/plugins/ponytail.mjs:L26-L44`](https://github.com/dietrichgebert/ponytail/blob/2ed6c52c9d7e5e56942508591085fd45dea277d3/.opencode/plugins/ponytail.mjs#L26-L44)). Learning-loop applicability is likely repository- and event-dependent, so any Runtime State should be keyed by the deterministic scope that consumes it, not by whichever config directory is convenient.

7. **Do not copy adjacent product features into the delivery core.** Ponytail includes intensity modes, statusline setup, command aliases, benchmarks, debt harvesting, and cleanup tooling. Those may be valid for Ponytail, but they are not evidence that an instruction-delivery core needs UI, enforcement, workflow, changelog, or benchmark subsystems.

8. **Do not parse arbitrary user prose to infer system state unless that is the runtime's actual command surface.** Ponytail's prompt tracker carefully narrows deactivation phrases after false positives ([`hooks/ponytail-config.js:L36-L43`](https://github.com/dietrichgebert/ponytail/blob/2ed6c52c9d7e5e56942508591085fd45dea277d3/hooks/ponytail-config.js#L36-L43)). The successor should prefer explicit CLI/config changes and deterministic event metadata.

## Verification notes

The adapter-focused tests were run locally and passed: 43 shared/host adapter and packaging tests, 23 Pi extension tests, and 3 MCP instruction tests. The full `npm test` run passed 83 of 84 root tests; the only failure was the unrelated CSV correctness benchmark (`tests/correctness.test.js`, “csv: correct pandas one-liner passes”), not an adapter or hook test. No Ponytail files were modified.

