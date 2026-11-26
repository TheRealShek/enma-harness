# Enma Harness

Enma Harness is a small Go CLI agent that talks to any OpenAI-compatible chat completions server, runs local file tools behind taint and approval gates, and saves named threads under `~/.enma/threads/`.

`config/config.go` currently defaults to `qwen35-4b-32k` on Ollama (`http://localhost:11434/v1/chat/completions`) and a `32,768`-token context window for local testing. Those defaults are temporary and can be swapped for any OpenAI-compatible model and context size.

## Getting started

Make sure Go and an OpenAI-compatible server are running (check `config/config.go` for the current endpoint and model defaults), then start the CLI:

```bash
go run .
```

At the `You:` prompt, type a prompt or use a slash command:

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
