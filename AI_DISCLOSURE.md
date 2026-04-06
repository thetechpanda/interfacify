# AI Disclosure

## Disclosure

This repository has been built and maintained with material assistance from an AI coding model. Code, tests, documentation, fixture updates, and implementation plans may have been proposed or authored in whole or in part by an LLM and then reviewed, edited, and executed through a developer-assisted workflow.

AI-generated output should be treated as accelerative assistance, not as an authoritative source of truth. Human review remains responsible for correctness, maintainability, security, and release quality.

## Model Information

- **AI provider:** OpenAI
- **Usage mode:** API-driven coding assistant / agent workflow
- **Model used for this AI-assisted work:** `openai-codex:gpt-5.4` with **xhigh reasoning**
- **Model provenance note:** Historical AI-assisted changes may not all have an exact model identifier recorded. Future AI-authored changes should record the exact model name/version in the PR description, commit message body, or release notes when it is known.

## How Development Was Done

Development in this repository is expected to be iterative and evidence-driven:

1. Read the relevant code and test fixtures before changing anything.
2. Make the smallest targeted implementation change that solves the problem.
3. Add or update tests and fixtures for the changed behavior.
4. Format code and rerun the affected test suite.
5. Verify generated output against golden files.
6. Update documentation when user-visible behavior changes.

AI assistance is expected to follow the same workflow. Large speculative refactors, broad rewrites, and unverified output are discouraged.

## Context

This section contains project-specific guidance intended to help future LLMs and human contributors work on the repository safely and consistently.

### Requirements

- The project is written in **Go** and currently targets **Go 1.26.1**.
- `interfacify` generates interfaces from fully-qualified Go type names.
- Package lookup is driven by the `-paths` flag and may span local modules or workspaces.
- Generated code must be deterministic, formatted, and valid Go source.
- Exported doc comments should be preserved when they can be resolved from source.
- With `-deep=true`, exported methods promoted through embedded **local** structs or interfaces are included.
- When the output package differs from the source package, exported source-package types in signatures must be qualified with the source package import.
- Unexported local source-package types must **not** be emitted into a different package; generation should fail instead.
- **Minimum total statement coverage is a hard requirement: it must remain above 80%.** If coverage is not above 80%, the work is not complete and more tests must be added.

### Guardrails

- Keep CLI wiring and flag parsing in `main.go`.
- Keep core implementation logic under `pkg/`.
- Preserve deterministic generation behavior and stable formatting.
- Do not silently change fixture expectations. If behavior changes intentionally, update the corresponding golden files deliberately and review the full diff.
- Preserve existing semantics around:
  - package/module resolution
  - embedded method promotion
  - import alias conflict detection
  - documentation rendering
  - cross-package type qualification
- Prefer small, targeted changes over broad refactors.
- Do not claim work is complete until formatting, tests, and coverage checks have passed.

### Expected Development Workflow

1. Inspect the relevant production files and existing tests.
2. Implement the smallest viable change.
3. Add or update regression coverage.
4. Format the code with one of:
   - `go fmt ./...`
   - `make format`
5. Run tests with one of:
   - `go test ./...`
   - `make test`
6. Verify total statement coverage is **>80%**.
7. Update `README.md` when CLI behavior, documented examples, or constraints change.

### Where Things Go

- `main.go`: CLI entry point, defaults, usage text, and flag parsing.
- `main_test.go`: CLI-facing tests.
- `pkg/interfacify.go`: shared configuration, path validation, and write-to-disk orchestration.
- `pkg/loader.go`: module/workspace resolution, AST loading, type discovery, and method collection.
- `pkg/generator.go`: top-level generation orchestration and output assembly.
- `pkg/render.go`: method signature rendering, qualification behavior, and type-expression handling.
- `pkg/generator_test.go`: fixture-driven regression tests for generator behavior.
- `pkg/test_data/`: self-contained fixture modules used by tests.

### Golden File and Fixture Conventions

Fixture directories live under `pkg/test_data/_<scenario>/` and are expected to be self-contained. A typical fixture contains:

- a `go.mod`
- one or more package directories such as `service/`, `nested/`, or `examples/`
- one or more golden files such as:
  - `expected.golden`
  - `expected_deep.golden`
  - `expected_shallow.golden`

Rules:

- Use `expected.golden` for the primary/default expected output.
- Use variant golden files only when a single fixture intentionally covers multiple modes.
- Keep fixtures minimal and scenario-focused.
- For behavior changes, update the matching golden file rather than introducing ad hoc inline expectations.
- For error-path tests, it is acceptable to assert an error substring instead of using a golden output file.
- The helper `fixturePath(...)` in `pkg/generator_test.go` is the canonical way tests locate fixture assets.

### Quality Bar

- Tests must pass locally.
- Generated output must match the intended golden files exactly.
- Documentation must reflect the implemented behavior.
- Coverage must remain **above 80%**.
- Human review is strongly recommended for any AI-authored or AI-assisted change.

This document is intentionally normative: it describes how future changes to this repository are expected to be performed.