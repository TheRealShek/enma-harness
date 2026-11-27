# Enma Harness evals

The `eval` subcommand measures how harness changes (prompts, tool limits, error feedback, compaction, and security gates) affect a fixed model across repeated trials.

## Running evals

```bash
go run . eval -tasks evals/tasks -runs 3
```

Useful flags:

- `-tasks`: task directory (default `evals/tasks`)
- `-runs`: trials per task (default `3`, from `config.DefaultEvalRuns`)
- `-filter`: substring filter for task names
- `-model` / `-url`: model and endpoint overrides (default to `config.DefaultModel` / `config.ChatCompletionsURL`)
- `-diff <runA>,<runB>`: compare two saved runs under `~/.enma/evals/`

## Task structure

Each task lives in `evals/tasks/<name>/` with:

- `workspace/`: starting files copied to a fresh `TempDir` for every trial.
- `task.json`: `prompt`, `check` (argv array, e.g. `["go", "test", "./..."]`), `max_turns`, `timeout_sec`, and `approval_policy` (`"deny_all"` or `"allow_project"`).

The 10 built-in tasks target harness mechanics rather than raw model trivia: `read_file` pagination (`01`), `list_files` pagination (`02`), `edit_file` ambiguity recovery (`03`), `write_file` overwrite gating (`04`), noisy context accumulation (`05`), missing-path recovery (`06`), `untrusted/` taint injection (`07`), outside-root write blocking (`08`), `AlwaysSensitive` gating (`09`), and `AGENTS.md`/`docs/` navigation (`10`).

## Logs and lazy `summary.json`

Each trial writes a normal `events.jsonl` log under `~/.enma/evals/<run-id>/threads/<id>/events.jsonl` and appends one `eval.check` event after running `check`. `summary.json` is written once at the end of a completed run so it never blocks active trials; if missing, `eval.LoadOrBuildSummary` reconstructs it lazily from `events.jsonl`.
