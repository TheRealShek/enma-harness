# Enma Harness

Enma Harness is a minimal Go CLI agent that streams responses from an OpenAI-compatible model endpoint, runs local file tools behind taint and approval gates, and stores named threads under `~/.enma/threads/`.

It is an early-stage personal harness built around local models (such as `qwen3:4b` on Ollama) and an 8,192-token default context window.

## Getting started

Make sure an OpenAI-compatible server is running at the endpoint in `config/config.go` (default `http://localhost:11434/v1/chat/completions` with `qwen3:4b`), then run:

```bash
go run .
```

At the `You:` prompt, type a message or use a slash command:

- `/threads [page]` lists saved threads for the current directory (10 per page).
- `/resume [name-or-id]` resumes a saved thread, or opens the paginated picker.
- `/rename <name>` sets or updates the current thread's name.
- `/new [name]` starts a fresh thread.

Run the test suite with:

```bash
go test -race ./...
```

## Architecture

See [docs/architecture.md](docs/architecture.md) for how `config`, `tools`, `threads`, streaming, and the CLI loop fit together.
