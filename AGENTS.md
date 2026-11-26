# AGENTS.md

Enma Harness is a minimal Go CLI agent harness that streams model responses, tracks context usage, and runs local tools.

## Repository Rules

- Keep code simple, readable, and consistently named across the repository.
- Add a brief, plain-English comment above major functions and shared helpers explaining what they do.
- Keep all model API integrations OpenAI API compatible.
- Keep all configuration, prompts, and limits centralized in `config/config.go` so every package reads from one source of truth.
- Keep `README.md` and `docs/architecture.md` updated when behavior or structure changes, keeping both concise and free of bloat.
- Keep test files minimal and focused; do not bloat tests.
