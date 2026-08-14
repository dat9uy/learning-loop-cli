# SKILL.md specification and invocation semantics

Primary-source review conducted 2026-08-14. Runtime-specific source claims are pinned to the proposed v0.1 test versions: [Codex CLI 0.147.0](https://github.com/openai/codex/tree/be6e8eac029b183056b7e4402879f15d2c85f61b) and [OpenCode 1.18.18](https://github.com/anomalyco/opencode/tree/31406ccc51b4bd2a4e1e086b2bcaa5f7f804f26d).

## Conclusion

There is an authoritative open [Agent Skills specification](https://agentskills.io/specification), and OpenAI says Codex skills build on it. It standardizes a Skill's directory contents and `SKILL.md` format. It does **not** standardize installation roots, catalog filtering, implicit versus explicit invocation, or a Rule-to-Skill relationship; those are Runtime policies.

`disable-model-invocation` is not a shared-spec field. The canonical strict validator rejects it as an unexpected top-level field. Codex 0.147.0 and OpenCode 1.18.18 do not interpret it in `SKILL.md`; both accept/ignore unknown fields at runtime. Codex's equivalent policy is `agents/openai.yaml` → `policy.allow_implicit_invocation: false`. OpenCode 1.18.18 controls model access through Skill permissions and tool availability instead.

Therefore, the fact that `to-spec` was absent from this conversation's advertised Skill catalog is evidence about this host/session's discovery or filtering, not evidence for portable `disable-model-invocation` semantics. The real-Runtime catalog assertion in the proposed Test Harness is necessary.

## The shared contract

A Skill is a directory containing a file named exactly `SKILL.md`; the shared spec does not mandate where that directory is installed. `.agents/skills/` is a cross-client discovery convention from the implementor guide, not part of the file-format specification ([specification](https://agentskills.io/specification#directory-structure), [implementor guide](https://agentskills.io/client-implementation/adding-skills-support#where-to-scan)).

`SKILL.md` must have YAML frontmatter followed by Markdown. The shared fields are:

| Field | Status | Semantics |
|---|---|---|
| `name` | Required | 1–64 characters; lowercase alphanumeric and hyphens; no leading, trailing, or consecutive hyphens; matches the parent directory. |
| `description` | Required | 1–1024 characters; says both what the Skill does and when it should be used. This is routing metadata. |
| `license` | Optional | License name or reference to a bundled license file. |
| `compatibility` | Optional | Up to 500 characters describing environment requirements. |
| `metadata` | Optional | String-to-string extension map for client-specific properties. |
| `allowed-tools` | Optional, experimental | Space-separated pre-approved tools; support varies by Runtime. |

These constraints come directly from the [frontmatter, name, and description sections](https://agentskills.io/specification#frontmatter). The specification designates `metadata` as the place for client properties not defined by the standard ([metadata field](https://agentskills.io/specification#metadata-field)). The canonical `skills-ref` validator enumerates only those six [allowed top-level fields](https://github.com/agentskills/agentskills/blob/69ef37e9424c0a7ea9dd2293b559e43ec8176379/skills-ref/src/skills_ref/validator.py#L14-L22) and [reports every other one as unexpected](https://github.com/agentskills/agentskills/blob/69ef37e9424c0a7ea9dd2293b559e43ec8176379/skills-ref/src/skills_ref/validator.py#L104-L115). Its tests confirm that an unknown top-level field is a validation error ([validator test](https://github.com/agentskills/agentskills/blob/69ef37e9424c0a7ea9dd2293b559e43ec8176379/skills-ref/tests/test_validator.py#L120-L131)).

The normative filename remains exactly `SKILL.md`. The reference parser tolerates lowercase `skill.md`, but calls that tolerance out explicitly; neither the specification nor the Runtime discovery contracts should be weakened to match it ([reference parser](https://github.com/agentskills/agentskills/blob/69ef37e9424c0a7ea9dd2293b559e43ec8176379/skills-ref/src/skills_ref/parser.py#L12-L27)).

## Runtime behavior

| Concern | Codex CLI 0.147.0 | OpenCode 1.18.18 |
|---|---|---|
| Portable project root | Scans `.agents/skills` from CWD through repository root. | Scans `.agents/skills` from CWD through the Git worktree; also supports native `.opencode/skills` and Claude-compatible locations. |
| Model advertisement | Catalog contains Skill name, description, and source path; Codex tells the model to load `SKILL.md` when named or matched. | The `skill` tool description lists permitted Skills by name and description. |
| `disable-model-invocation` | Ignored in `SKILL.md`. | Ignored as an unknown frontmatter field. |
| Runtime-native filtering | `agents/openai.yaml` → `policy.allow_implicit_invocation: false` hides the Skill from the model-visible prompt while retaining explicit `$skill` invocation. | A `deny` Skill permission hides and rejects the Skill; disabling the `skill` tool omits the catalog entirely. |

Codex's official documentation describes progressive disclosure, explicit `$skill` mentions, implicit description matching, `.agents/skills` discovery, and the `agents/openai.yaml` policy ([OpenAI Docs: Build skills](https://developers.openai.com/codex/skills)). The pinned implementation confirms that `SKILL.md` parsing reads `name`, `description`, and `metadata.short-description`, with no `disable-model-invocation` field ([Codex parser](https://github.com/openai/codex/blob/be6e8eac029b183056b7e4402879f15d2c85f61b/codex-rs/skills/src/parser.rs#L6-L20)). It reads `allow_implicit_invocation` from the sidecar policy ([Codex metadata loader](https://github.com/openai/codex/blob/be6e8eac029b183056b7e4402879f15d2c85f61b/codex-rs/ext/skills/src/loader/metadata.rs#L48-L54)) and marks such Skills hidden from the prompt catalog ([Codex catalog provider](https://github.com/openai/codex/blob/be6e8eac029b183056b7e4402879f15d2c85f61b/codex-rs/ext/skills/src/provider/host.rs#L126-L148)). Its built-in catalog instructions tell the model to use a named or description-matching Skill and read its complete `SKILL.md` ([Codex catalog prompt](https://github.com/openai/codex/blob/be6e8eac029b183056b7e4402879f15d2c85f61b/codex-rs/ext/skills/src/catalog_prompt.rs#L1-L24)).

OpenCode's pinned documentation lists its discovery roots, recognized fields, and the fact that unknown fields are ignored ([OpenCode Skill docs](https://github.com/anomalyco/opencode/blob/31406ccc51b4bd2a4e1e086b2bcaa5f7f804f26d/packages/web/src/content/docs/skills.mdx#L11-L45)). The pinned parser checks only `name` and optional `description`, so `disable-model-invocation` has no behavior ([OpenCode loader](https://github.com/anomalyco/opencode/blob/31406ccc51b4bd2a4e1e086b2bcaa5f7f804f26d/packages/opencode/src/skill/index.ts#L53-L59)). Skills with descriptions are advertised after permission filtering ([OpenCode catalog](https://github.com/anomalyco/opencode/blob/31406ccc51b4bd2a4e1e086b2bcaa5f7f804f26d/packages/opencode/src/skill/index.ts#L310-L345)), and the model is told to call the `skill` tool for a listed match ([OpenCode Skill tool](https://github.com/anomalyco/opencode/blob/31406ccc51b4bd2a4e1e086b2bcaa5f7f804f26d/packages/opencode/src/tool/skill.txt#L1-L5)).

## Can a Rule direct an agent to load a Skill?

Yes, as an agentic instruction. The Agent Skills implementor guide explicitly recommends behavioral instructions that tell the model to load `SKILL.md` or call a Skill tool when a task matches ([behavioral instructions](https://agentskills.io/client-implementation/adding-skills-support#behavioral-instructions)). A minimal Rule can use the same mechanism: “when this context applies, load and follow the `<name>` Skill.”

That directive is not Runtime-level explicit invocation. It works only if the Runtime advertises the Skill, the selected agent may access it, the loading mechanism is available, and the model follows the Rule. A Skill hidden from model-driven discovery cannot be made portably discoverable merely by placing its name in an injected Rule. Deterministic activation would require a Runtime-specific adapter to invoke/inject the Skill through the host's explicit-activation API; that is a different v0.1 contract. The shared guide itself separates model-driven activation from harness-intercepted user-explicit activation ([activation mechanisms](https://agentskills.io/client-implementation/adding-skills-support#activate-skills)).

## Honest validation boundary for `learning-loop-cli`

The shared renderer can deterministically validate:

1. the same-named directory and exact `SKILL.md` exist beneath the chosen project Skill root;
2. YAML/frontmatter parses and satisfies strict Agent Skills fields, types, lengths, naming, and name-directory equality;
3. a Rule's Skill name resolves without ambiguity; and
4. the Rule contains the intended requirement to load and follow that Skill.

Runtime-specific validation or the real-Runtime Test Harness must establish:

1. the selected CWD/project causes the Runtime to discover the Skill;
2. native policy and permissions do not hide it from model-driven discovery;
3. the first request advertises the same Skill name and description; and
4. the Rule Instruction is delivered before the first model request without injecting the Skill body.

No static or cross-runtime validator can prove that the model later loads or follows the Skill. Catalog presence proves availability, not compliance. That is exactly the boundary between deterministic Instruction delivery and agentic Skill use, so the Rule remains at I2.

For strict shared conformance, `disable-model-invocation` should be removed from top-level `SKILL.md` frontmatter. Runtime-specific invocation policy belongs in a Runtime-native sidecar/configuration or, where a Runtime defines one, a namespaced `metadata` key. A generic Rule-to-Skill validator must not infer eligibility from this nonstandard field.
