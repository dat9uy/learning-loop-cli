# Learning Loop

The learning loop preserves lessons from agent work and moves reusable guidance from agent-dependent discovery toward deterministic delivery and application.

## Language

**Internalization Level**:
The degree to which guidance has been internalized: **I1 Discoverable** requires agent retrieval, **I2 Delivered** guarantees timely presentation while retaining agent judgment, and **I3 Deterministic** applies the guidance without relying on agent judgment. The responsible mechanism may belong to the learning-loop harness, a runtime adapter, or the user application.
_Avoid_: State 1/2/3, L1/L2/L3

**Effective Internalization Level**:
The Internalization Level currently demonstrated for a Rule in a repository. Effective I3 requires a deterministic Verifier result of `pass`; `fail` or `unknown` leaves the Rule effectively I2 and eligible for delivery.
_Avoid_: Declared level, configured level

**Deterministic Mechanism**:
An implementation that applies a Rule without agent judgment. It lives with the behavior it governs rather than necessarily inside the learning-loop harness.
_Avoid_: Gate, enforcement

**Verifier**:
A repository-scoped deterministic check whose owner asserts that it represents a Rule and which reports `pass`, `fail`, or `unknown` with evidence. The learning-loop can execute and interpret the check but cannot prove that the owner's semantic assertion is correct.
_Avoid_: Mechanism check, gate

**Rule**:
Operator-approved guidance that states what should remain true and why. An Instruction may be derived from a Rule.
_Avoid_: Instruction, policy

**Finding**:
Evidence about something learned during agent work that may justify new guidance. A Finding becomes a Rule only through explicit human promotion.
_Avoid_: Rule, automatic rule

**Abstraction Level**:
The stability level at which knowledge is stated: **L1 Concept** names enduring domain ideas, **L2 Contract** defines behavior independent of current wiring, and **L3 Implementation** describes replaceable technical details.
_Avoid_: Internalization level, I1/I2/I3

**Instruction**:
The minimal delivery view of a Rule that may be presented to an agent when the Rule's declared context applies. It is derived deterministically.
_Avoid_: Prompt, hint, policy

**Instruction Registry**:
The derived catalog of Rules that are eligible for contextual delivery as Instructions.
_Avoid_: Registry, record registry, meta-surface

**Record Store**:
The append-only memory of learning-loop records. Each Rule identity has an ordered sequence of immutable Rule Revisions, of which the latest is current.
_Avoid_: Registry, ledger

**Rule Revision**:
An immutable version of a Rule within the Record Store. A later revision replaces an earlier revision for delivery without rewriting its history.
_Avoid_: Rule version file, mutable Rule

**Runtime**:
A supported agent program to which the learning loop delivers Instructions through a native integration boundary.
_Avoid_: Agent, host

**Adapter**:
The Runtime-specific boundary that translates between learning-loop behavior and one Runtime's native interfaces, including authorized changes to its Runtime Configuration.
_Avoid_: Universal runtime interface, runtime agent

**Runtime Configuration**:
The native settings and integration artifacts through which a Runtime is connected to the learning loop. Authority to change it is granted for an explicit scope; ordinary startup and discovery are read-only, while persistent user-owned configuration remains protected.
_Avoid_: Runtime State, learning-loop configuration

**Installer**:
An explicit operation authorized to connect one selected Runtime to one selected project while preserving Runtime Configuration it does not own.
_Avoid_: Initialization, discovery

**Test Harness**:
An automated verifier that prepares a disposable project environment, invokes the Installer, and launches a real Runtime with isolated Runtime Configuration. It owns test-only configuration but does not perform the Installer's changes.
_Avoid_: Installer, live environment
