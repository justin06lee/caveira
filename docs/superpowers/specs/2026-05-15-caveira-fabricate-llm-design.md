# Caveira — Fabricate (Phase 2: LLM Providers) Design Spec

**Date:** 2026-05-15
**Status:** Approved for planning
**Scope:** Phase 2 — LLM-backed fabrication. Adds five LLM provider engines
(`--groq`, `--claude-code`, `--codex`, `--nvidia`, `--opencode`) as alternatives
to `--flurry`. The LLM designs the full commit plan, delivering realistic commit
messages, layered (file-modifying) commits, and semantic feature grouping through
one mechanism. Builds on the Phase 1 spec
(`2026-05-14-caveira-fabricate-design.md`).

## 1. Purpose

Phase 1 shipped `--flurry`: a templated, purely-additive fabricator that walks the
source HEAD tree, groups files by directory, and emits feat/test/chore commits.
Its output is deterministic but mechanical — every commit only adds whole files,
messages come from a fixed pool, and grouping is naive.

Phase 2 adds an **LLM-backed base engine**. Given the source's final file tree
(history-blind, exactly as flurry), an LLM designs a realistic commit plan:
how to group files into features, how to order commits, how to split large files
across multiple commits (layering), and what each commit message says. Caveira
then **realizes that plan deterministically**, guaranteeing the final tree equals
the source HEAD tree exactly.

This single mechanism delivers three Phase 1 non-goals at once:

- **Sophisticated message synthesis** (Phase 1 §11) — the LLM writes messages.
- **Layered scaffold → impl → test commits** (Phase 1 §11) — the LLM assigns file
  segments across commits; realized commits modify earlier files.
- **Smarter feature grouping** (Phase 1 §11) — the LLM clusters files semantically
  instead of by top-level directory.

## 2. Architecture overview

`--fabricate` runs a **base engine** that produces a linear sequence of synthetic
commits ending at the source HEAD tree. Phase 1's only base engine is `--flurry`.
Phase 2 adds five LLM provider engines as alternative base engines.

`--pigs N` / `--rats N` remain **workflow reshapers** layered on top of whatever
base sequence was produced. They compose with LLM engines unchanged: the LLM
replaces flurry as the source of the base sequence; pigs/rats reshape it into
chaotic single-branch or emergent multi-branch topologies as in Phase 1.

Pipeline flow with an LLM engine:

1. Caveira clones / duplicates the source as today.
2. The LLM orchestrator (`llmgen.go`) walks the source HEAD tree, splits each file
   into segments, builds a prompt, and calls the selected provider.
3. The provider returns a JSON commit plan.
4. The realizer (`realize.go`) validates, clamps, and converts the plan into a
   base `[]SynthCommit` sequence, then verifies the final tree OID equals the
   source HEAD tree OID.
5. The selected workflow (`pigs`, `rats`, or none) reshapes the base sequence into
   the final DAG, exactly as Phase 1.
6. The existing scheduler scales and squashes; the rewriter writes commits, swaps
   folders, and optionally pushes.

## 3. CLI surface

New flags layered on the Phase 1 CLI:

```
caveira ... --fabricate ...
        [--flurry]                       # templated NLP engine (Phase 1)
        [--groq | --claude-code | --codex | --nvidia | --opencode]   # LLM engine
        [--model NAME]                   # override the provider's default model
        [--llm-timeout DURATION]         # per-call timeout, default 120s
        [--pigs N | --rats N]            # workflow reshapers (Phase 1)
        [--pig / --rat identities]       # Phase 1
```

### 3.1 Validation

Extends Phase 1 §3.1:

- A **base engine** is exactly one of
  `{--flurry, --groq, --claude-code, --codex, --nvidia, --opencode}`.
- `--fabricate` with **zero** base engines errors with a message listing all six
  options. This replaces the Phase 1 stub error `no LLM provider configured
  (see Phase 2)`.
- `--fabricate` with **two or more** base engines errors as mutually exclusive.
- `--model` and `--llm-timeout` require an LLM engine; combined with `--flurry`
  (or without `--fabricate`) they error.
- `--pigs N` / `--rats N` remain optional and now compose with an LLM engine
  (e.g. `--fabricate --groq --rats 3`).
- All Phase 1 cross-flag rules (`--pig` requires `--pigs`, etc.) are unchanged.

### 3.2 Help

`--help` documents the new flags. It states explicitly that `--fabricate` with an
LLM engine is **not bit-reproducible** across runs (see §7), unlike `--flurry`.
The README example block gains an LLM-mode example.

## 4. Provider abstraction

### 4.1 Interface

`internal/fabricate/llm/provider.go`:

```go
type Provider interface {
    Name() string
    GeneratePlan(ctx context.Context, prompt string) (rawJSON string, err error)
}
```

A registry maps each flag name (`groq`, `claude-code`, `codex`, `nvidia`,
`opencode`) to a constructor. The pipeline resolves the one selected provider,
builds the prompt, and calls `GeneratePlan`.

### 4.2 HTTP API providers (Groq, NVIDIA)

Both expose an OpenAI-compatible `chat/completions` endpoint, so they share one
`openAICompatProvider` struct parameterized by base URL, API-key env var, and
default model:

| Provider | Base URL                                | API key env      |
|----------|-----------------------------------------|------------------|
| groq     | `https://api.groq.com/openai/v1`        | `GROQ_API_KEY`   |
| nvidia   | `https://integrate.api.nvidia.com/v1`   | `NVIDIA_API_KEY` |

Request details:

- `response_format: {"type": "json_object"}`.
- `temperature: 0.2` (low, for best-effort reproducibility).
- `seed`: forwarded from `--seed` when the user set it (both endpoints accept it).
- `model`: a sensible current instruct model per provider, overridable by
  `--model`.
- Missing API-key env var → hard error naming the variable.

### 4.3 CLI subprocess providers (claude-code, codex, opencode)

These run as installed CLIs in non-interactive print mode, sharing one
`cliProvider` struct parameterized by binary name and an argument template:

| Provider     | Invocation                                    |
|--------------|-----------------------------------------------|
| claude-code  | `claude -p <prompt> --output-format text`     |
| codex        | `codex exec <prompt>`                         |
| opencode     | `opencode run <prompt>`                       |

- The prompt is passed per the CLI's interface (argument or stdin); stdout is
  captured as the raw response.
- `exec.LookPath` checks the binary first; missing binary → hard error naming it.
- `--model` is forwarded when the CLI exposes a model flag.
- These CLIs manage their own auth/config — Caveira does not handle their keys.
- `--llm-timeout` bounds the subprocess via `context.WithTimeout`.

### 4.4 Retry and failure

`GeneratePlan` retries up to **3 attempts total** on transport errors and on
responses that fail JSON parsing or plan validation (§6.1). After exhaustion it
returns a hard error naming the provider and the last failure. There is **no
fallback** to `--flurry` — the user explicitly chose a provider, and silently
downgrading to templated commits would be a surprising quality regression.

### 4.5 Robust JSON extraction

LLM responses often wrap JSON in prose or Markdown code fences. The plan parser
extracts the **first balanced `{…}` object** from the raw response before
unmarshaling, so fenced or explained responses still parse. A response with no
balanced object is a parse failure and triggers a retry.

## 5. The commit plan

### 5.1 Correctness principle

The LLM **never returns file content**. It returns a plan that *references* files
and pre-computed *segment indices*. Caveira owns every byte written, so the final
tree is guaranteed to equal the source HEAD tree (§6).

### 5.2 Segments

Before calling the LLM, Caveira splits each source file into an ordered list of
**segments** — contiguous line ranges. Segments are blank-line-delimited blocks,
coalesced so each is at least a minimum size (constant in `segment.go`). A file's
segments are an **exact partition of its lines**: concatenating all segments in
original order reproduces the file byte-for-byte (including a missing trailing
newline). Each file gets a stable segment count indexed `0..k`.

Edge cases:

- Empty file → one empty segment (index 0).
- One-line file → one segment.
- File with no trailing newline → the final segment carries the exact bytes; the
  partition is still exact.

### 5.3 Prompt input

Caveira sends the LLM:

- The repo's file list, each with its Phase 1 classification (chore/code/test)
  and segment map (e.g. `internal/walk/load.go: 4 segments — [0] lines 1-12,
  [1] 13-40, [2] 41-77, [3] 78-95`).
- File contents, subject to a total byte budget (default ~200 KB, constant in
  `prompt.go`). Files over a per-file cap are truncated with an explicit marker.
  Truncation only degrades the LLM's *judgment* — never correctness, because
  segment indices in the returned plan are validated against the real file.
- Instructions: design a realistic commit history that builds this codebase from
  scratch; group files into features; order commits sensibly; optionally split
  large files across commits by assigning segment ranges (layering); write
  conventional-commit messages; return strictly the plan JSON.

### 5.4 Plan JSON schema

The LLM returns:

```json
{
  "commits": [
    { "message": "chore: initialize Go module",
      "type": "chore",
      "changes": [ {"path": "go.mod", "segments": "all"} ] },
    { "message": "feat(walk): scaffold DAG loader types",
      "type": "feat",
      "changes": [ {"path": "internal/walk/load.go", "segments": [0, 1]} ] },
    { "message": "feat(walk): implement DAG traversal",
      "type": "feat",
      "changes": [ {"path": "internal/walk/load.go", "segments": [2, 3]} ] }
  ]
}
```

- `changes[].segments` is either the string `"all"` (whole file in this commit —
  the common case) or an explicit list of segment indices (layering).
- `type` is one of `chore`/`feat`/`test`/`fix`/`refactor`; it feeds the
  difficulty bucket downstream (unrecognized → treated as `feat`).
- `message` is the full commit message line.

## 6. Deterministic realization

`realize.go` converts the validated plan into the base `[]SynthCommit` sequence
and guarantees the final tree.

### 6.1 Validation

Before realizing:

- The plan has at least one commit.
- Every `changes[].path` exists in the source HEAD tree.
- Every explicit segment index is within that file's real `0..k` range.

A plan that fails validation counts as a provider failure → retry (§4.4), then
hard error.

### 6.2 Clamping — the correctness guarantee

- For each source file, Caveira computes the union of segment indices the LLM
  assigned across all commits. Any segment the LLM omitted is appended to that
  file's **last** commit in plan order.
- Any source file the LLM never mentioned is appended in full to a final
  `chore: finalize` reconciliation commit (created only if needed).
- After clamping, every file's assigned segments are provably the complete set
  `0..k`.

### 6.3 Realization

Walking commits in plan order, Caveira maintains cumulative per-file state. At
each commit, for every `(path, segments)` change it:

1. Computes the set of that file's segments assigned at or before this commit.
2. Concatenates those segments in original line order into the file's content at
   this commit.
3. Writes a blob for that content and records `path → blob` on the `SynthCommit`.

A file with a partial segment set yields a partial-content blob — a realistic
work-in-progress intermediate. The last commit touching a file necessarily
includes its final segment (guaranteed by clamping), so that blob is the exact
original content.

### 6.4 Modify support in the data model

Phase 1 `SynthCommit` only *adds* whole files. Phase 2 commits may *modify* a file
added by an earlier commit. The change:

- `SynthCommit` records the files it **sets** (creates or updates), not strictly
  the files it adds.
- `write.go`'s cumulative tree builder already overwrites on path collision, so a
  later commit re-setting a file to a fuller blob just works. This is the only
  write-path change.

### 6.5 Final verification

After realization, Caveira computes the last commit's tree OID and asserts it
equals the source HEAD tree OID. Clamping makes this always hold; the assertion
is a defensive backstop. A mismatch indicates an internal bug and aborts with a
hard error rather than producing an incorrect repo.

### 6.6 Downstream

The realized `[]SynthCommit` is the base sequence — identical in shape to
flurry's output — so `--pigs` / `--rats`, `BuildDurations`, `Schedule`, and the
rewriter all run unchanged. Layered commits that modify files score normally by
edit volume in the difficulty heuristic; this is the first time fabricated
commits are non-additive, which the heuristic handles without changes.

## 7. Composition, determinism and seed

### 7.1 Composition with pigs / rats

The LLM produces the base sequence — exactly flurry's Phase 1 role:

- `--fabricate --groq` → single-author linear history from the LLM plan.
- `--fabricate --groq --pigs 3` → LLM plan, then pigs reshaping (round-robin
  authors, ~15% noise commits, message typos).
- `--fabricate --groq --rats 3` → LLM plan, then rats reshaping (feature
  branches, off-branch forks, conflict-fix scars).

Phase 1 pigs/rats keyed feature boundaries off flurry's by-directory grouping.
With an LLM base, "features" are the LLM's groupings instead: rats derives one
branch per contiguous run of commits sharing the same `feat(<scope>)` scope;
pigs consumes the linear sequence directly. No new reshaping logic — pigs/rats
read structure the LLM plan already expresses.

### 7.2 Determinism

- **Structural RNG stays fully seeded.** Pig author round-robin, rat topology,
  noise placement, typos, and duration draws remain bit-reproducible given
  `--seed`, exactly as Phase 1.
- **The LLM call is best-effort reproducible, not guaranteed.** Groq/NVIDIA
  receive `temperature: 0.2` and the forwarded `seed`; claude-code/codex/opencode
  subprocesses have no seed control.
- **Net:** `--fabricate --flurry` stays fully deterministic. `--fabricate
  --<llm>` is *not* guaranteed bit-reproducible across runs. This is documented
  in `--help` and the README. Given one fixed base plan, all downstream
  reshaping and scheduling remain deterministic.

### 7.3 Dry-run

`--dry-run` with an LLM engine still calls the provider — the real plan is
required to print the schedule — then prints the schedule and writes nothing,
the same contract as Phase 1.

## 8. Project structure

New files extending `internal/fabricate/`:

```
internal/fabricate/
├── llm/
│   ├── provider.go        # Provider interface + flag→constructor registry
│   ├── provider_test.go
│   ├── openai_compat.go   # Groq + NVIDIA (shared OpenAI-compatible HTTP client)
│   ├── openai_compat_test.go
│   ├── cli_provider.go    # claude-code, codex, opencode (shared subprocess runner)
│   ├── cli_provider_test.go
│   ├── prompt.go          # builds the prompt from file tree + segment maps + budget
│   ├── prompt_test.go
│   ├── plan.go            # plan JSON schema, balanced-object extraction, parsing
│   └── plan_test.go
├── segment.go             # file → ordered line-range segment partition
├── segment_test.go
├── realize.go             # plan + segments → []SynthCommit; clamp, realize, verify
├── realize_test.go
├── llmgen.go              # orchestrator: walk → prompt → provider → parse → realize
└── llmgen_test.go
```

Wire-in points:

- `internal/cli/cli.go` — new flags (`--groq`, `--claude-code`, `--codex`,
  `--nvidia`, `--opencode`, `--model`, `--llm-timeout`) + validation.
- `internal/input/config.go` — new fields `Provider` (string), `Model` (string),
  `LLMTimeout` (`time.Duration`); `Validate` gains the §3.1 base-engine rules.
- `internal/cli/pipeline.go` — select the base engine: flurry vs LLM. When an LLM
  engine is selected, call `fabricate.GenerateLLM(...)` instead of the flurry
  path; the result feeds the same pigs/rats/scheduler downstream.
- `internal/fabricate/types.go` — `SynthCommit` gains "files set" (create-or-update)
  semantics.

## 9. Testing

- **Segment** (`segment_test.go`): the partition is exact — concatenation equals
  the original — for a normal file, a file with no trailing newline, an empty
  file, and a one-line file.
- **Realization** (`realize_test.go`): clamping fills segments the plan omitted;
  an unmentioned file lands in the reconciliation commit; the final tree OID
  equals source HEAD for hand-written plans, including plans with layered
  (multi-commit) files.
- **Plan parsing** (`plan_test.go`): plain JSON, fenced ```` ```json ```` blocks,
  JSON preceded by prose, and malformed JSON (→ retry path) all behave correctly.
- **Providers**: HTTP providers (`openai_compat_test.go`) are tested against an
  `httptest` stub server — no real API calls, no keys required in CI; CLI
  providers (`cli_provider_test.go`) are tested with a fake binary script placed
  on `PATH`. Missing API key and missing binary both produce hard errors.
- **Orchestrator** (`llmgen_test.go`): with a stub `Provider` returning a canned
  plan, the realized base sequence matches expectations.
- **Integration** (`internal/cli/`): a stub provider returning a canned plan,
  asserting the rewritten repo's final tree matches source HEAD, and that
  `--pigs` / `--rats` correctly reshape the LLM base sequence.
- **Property test:** for any valid plan over a fixture repo, the realized final
  tree equals the source HEAD tree.

## 10. Non-goals (Phase 2)

- Streaming or token-level provider output — responses are read whole.
- Provider auto-selection or fallback chains — the user picks exactly one.
- Caching LLM plans across runs.
- LLM-authored *real* merge-conflict diffs — rats conflict-fix scars stay
  cosmetic, as in Phase 1.
- Non-OpenAI-compatible HTTP APIs beyond the five named providers.
- Configurable segment-size and prompt byte budget via the CLI — these are
  constants in code.
- Non-Git VCS support.
