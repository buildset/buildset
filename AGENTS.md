# AGENTS.md

## Testing

- `make test`. Postgres repository tests start a container via `pkg/pgtest`, needs Docker.
- A new repository behaviour goes in `<service>/repotest`, so both backends are held to it.
- `tests/` needs Playwright browsers and runs both topologies.

## Linting and formatting

`make format` then `make lint`.

## Commit and PR guidelines

Short imperative subject, no trailers. `make` must be clean before committing.
