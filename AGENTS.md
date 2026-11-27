# AGENTS.md

Enma Harness is a minimal Go CLI agent harness that streams model responses, tracks context usage, and runs local tools.

## Repository Rules

- Keep code simple, readable, and consistently named across the repository.
- Keep functions modular and focused; split multi-step flows into small, readable helpers instead of one monolithic function.
- Add a brief, plain-English comment above major functions and shared helpers, and one-line phase comments inside long multi-step functions; avoid line-by-line narration.
- Keep all model API integrations OpenAI API compatible.
- Keep all configuration, prompts, and limits centralized in `config/config.go` so every package reads from one source of truth.
- Keep `README.md`, `docs/architecture.md`, and `docs/evals.md` updated when behavior, architecture, or eval tasks change, keeping them concise and free of bloat.
- Keep test files minimal and focused; do not bloat tests.

## Repository Structure

Place new code in the package that owns its responsibility instead of adding files to the root:

```text
main.go     # CLI entrypoint (dispatches interactive mode vs eval)
config/     # Centralized URLs, prompts, limits, and security path lists
llm/        # OpenAI-compatible HTTP/SSE client and wire types
threads/    # Thread metadata and append-only events.jsonl persistence
tools/      # Local file tools and symlink/taint/approval gates
harness/    # Headless turn loop (Runner), event folding, and context metrics
eval/       # Eval task loader, isolated trial runner, and diff/summary reports
cli/        # Interactive terminal loop, slash commands, display, and eval CLI
```
