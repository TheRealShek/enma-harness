# Enma Harness evals

The `eval` subcommand measures how harness changes (prompts, tool limits, error feedback, compaction, and security gates) affect a fixed model across repeated trials.

Eval tasks and run results live in the sibling repository [enma-evals](https://github.com/TheRealShek/enma-evals) (`../enma-evals/v1/tasks` and `../enma-evals/v1/results` locally) rather than inside `enma-harness` or `~/.enma/`.

## Running evals

```bash
go run . eval
```

Useful flags:

- `-tasks`: task directory (default `../enma-evals/v1/tasks`, from `config.DefaultEvalTasksDir`)
- `-results`: results directory (default `../enma-evals/v1/results`, from `config.DefaultEvalResultsDir`)
- `-runs`: trials per task (default `3`, from `config.DefaultEvalRuns`)
- `-filter`: substring filter for task names
- `-model` / `-url`: model and endpoint overrides (default to `config.DefaultModel` / `config.ChatCompletionsURL`)
- `-diff <runA>,<runB>`: compare two saved runs under `-results`

## Logs and lazy `summary.csv`

Each run creates a sortable timestamped folder `<results>/YYYY-MM-DD_HH-MM-SS-<hex>/` containing:

- `threads/<id>/events.jsonl`: standard thread event log plus one `eval.check` event after running `check`.
- `summary.csv`: per-trial results written once at the end of a completed run so it never blocks active trials; if missing, `eval.LoadOrBuildSummary` reconstructs it lazily from `events.jsonl`.
