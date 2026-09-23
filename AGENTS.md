# AGENTS.md

## Comments

- Write a comment only for what the code cannot say. Prefer renaming or restructuring over
  explaining.
- A doc comment says what the thing does, from the caller's side, never how it does it. Leave it out
  where the name and the signature already say it.
- A comment inside a body says why. Needing one is a hint the code is not clear enough: name the
  value or extract the step first, and see whether the comment is still worth writing.
- Keep it when the reason sits outside the code: a spec, a driver quirk, another service's
  behaviour, a security property, an ordering that cannot be a transaction. No rename says those.
- One or two lines. Drop a heading sentence that only names the thing below it.
- Keep `TODO` and `FIXME`, with the reason and what unblocks them.
- The same rule holds outside Go: SQL migrations, `compose.yaml`, `deploy/`, the `Makefile`, `README.md`.

## Testing

- `make test`. Postgres repository tests start a container via `pkg/pgtest`, needs Docker.
- A new repository behaviour goes in `<service>/repotest`, so both backends are held to it.
- `tests/` needs Playwright browsers and runs both topologies.

## Linting and formatting

`make format` then `make lint`.

## Commit and PR guidelines

Short imperative subject, no trailers. `make` must be clean before committing.
