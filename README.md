# BuildSet

A proposal for a set of general-purpose, open-source Go services, plus a blogging platform as the reference application that shows how they compose.

Status: four services are implemented (`auth`, `authz`, `content`, and `web`), and they run both ways from the same code. One binary with SQLite, or four containers with Postgres behind a gateway. The remaining general services are still a proposal.

## Running it

```sh
cp .env.example .env   # optional, every setting has a default
make run               # builds bin/blog and starts it
```

Open `http://localhost:8080`. With no accounts yet you are sent to `/setup`, and the account you create there becomes the administrator. Data lands in a SQLite file, `buildset.db` by default.

`make build` writes `bin/blog`. `make format` formats and tidies, `make lint` checks formatting and runs `go vet`, `make test` runs every test.

### Running it as four services

```sh
make compose-up      # four binaries, Postgres, and a gateway
make compose-smoke   # checks the arrangement answers through the one published port
make compose-down
```

Open `http://localhost:8080` again. It is the same site, the same pages, and the same first-run setup, which is the point.

The gateway is the only published port. It routes `/setup`, `/login`, `/logout`, `/register`, and `/password` to `auth` and everything else to `web`, so the browser sees one origin. That is load-bearing: the session cookie is host-only, the post-sign-in redirect check is same-origin, and the cross-origin form protection reads `Sec-Fetch-Site`. All three keep working unchanged because nothing ever leaves the origin. This is also why `auth` serves its pages at top-level paths rather than under a prefix.

Nothing routes to `authz`, to `content`, or to any `/v1` path. Those are reachable only on the compose network, which is the access control for the service APIs.

Postgres runs as one database with a schema and a login role per service, each role's `search_path` holding only its own schema. An unqualified reference to another service's table is an error rather than a silent cross-service read:

```sh
docker compose exec postgres psql -U auth_service -d buildset -c 'select * from content.posts'
# ERROR:  permission denied for schema content
```

The end-to-end tests drive a real browser, and run the same scenarios against both arrangements: the single binary, and the four services behind a proxy carrying the gateway's routing. The scenarios never mention which, because the claim a split makes is that it behaves the same way. `E2E_TOPOLOGY` is `mono`, `split`, or `both`, and defaults to `both`.

They need Playwright's Chromium once:

```sh
go run github.com/playwright-community/playwright-go/cmd/playwright@v0.6000.0 install chromium
```

Without it those tests skip and everything else still runs. `E2E_INSTALL_BROWSER=1 make test` downloads it during the run instead, which is convenient once and a slow surprise every time after, so it is off by default.

That version of the installer still points at a retired download host. Set `PLAYWRIGHT_DOWNLOAD_HOST` to a working one, for example `https://registry.npmmirror.com/-/binary/playwright`.

### What the first cut does

Username and password sign-in, RBAC with `admin`, `author`, and `reader`, posts that move between draft, published, and archived, plain-text bodies rendered to HTML, and server-rendered pages with no JavaScript.

Administrators add and remove accounts and assign roles. Authors write their own posts and hold rights only over the posts they created. Everyone can edit their own name, username, and password.

Each service has a small JSON API, but only for the other services: it is what the split arrangement talks over, it is not published through the gateway, and it is not a public API. There are no comments, no tags, no scheduling, and no pagination yet.

## Motivation

Most applications rebuild the same capabilities: authentication, authorization, file uploads, discussions, reactions, notifications, search, webhooks, background jobs, audit trails. These are not domain logic.

Build each once, as an independent service usable by any application. The blogging platform proves the services compose into something real. It is an example, not the product.

## Architectural principle

Build reusable services around **capabilities**. Keep **application-specific domains** inside the application.

Discussions are a capability: any application has things people talk about. Posts are a domain: only a blogging platform has posts.

Every general service is resource-agnostic. It stores a reference to a resource and knows nothing about what that resource means.

## Resource identity

Services need one way to point at a resource owned by another service:

```text
urn:<service>:<type>:<resource-id>

urn:content:post:01J8XK2P
urn:media:image:01J8XK5Q
urn:auth:user:01J8XK7R
```

Rules:

- The ref is opaque to every service except the one named in `service`.
- No service dereferences a foreign ref. If `discuss` needs to know a post exists, it asks `content`, and only if it must.
- `resource-id` format is the owning service's choice: UUIDv4, UUIDv7, ULID, or something else. It must be opaque, stable, never reused, URL-safe, and contain no colon. UUIDv7 and ULID are time-sortable, which keeps index writes at the right edge and makes cursor pagination work without a second index; they also leak creation time, which UUIDv4 does not.
- A small `ref` package provides parse, format, and validate. Services share that and nothing else.

Services index on the full ref string. They may index on `(service, type)` for filtering, nothing deeper.

There is no tenant segment. Multi-tenancy is out of scope. It would arrive as a new segment, which is a breaking change: the field count shifts, so parsers detect it, but every stored ref has to be rewritten, including refs held in other services' tables.

`service` sits in the slot RFC 8141 expects to hold a namespace registered with IANA. Unregistered namespaces are common practice, but these are generic words, so the refs are only safe to mix with URNs from other systems once a project segment is added.

## Deployment shapes

Every service runs two ways from the same code.

**Standalone.** Own process, own storage, a transport in front of it, events out.

**Embedded.** Imported as a Go module into one binary. Calls go in-process.

This is what keeps a blog from being overkill. One binary with SQLite and an in-process bus, no external dependency at all, is a valid deployment. So is ten services on Kubernetes with a different database engine behind each. Same code.

### Layering

```text
main.go            dependency injection, wiring, config
   │
http / grpc / cli  transport adapters
   │
service            business logic, the only layer that matters
   │
repository, clients, storage, bus
```

The service layer holds the logic and depends only on interfaces. Everything above and below is an adapter chosen in `main.go`.

### API

The service interface is the contract. Transport layers are optional adapters over it, and a service ships with none by default.

When one is needed, REST is preferred, with an OpenAPI document alongside it. gRPC is available where it fits. Other languages consume whichever a given service exposes.

### Service-to-service calls

A service that needs another declares a Go interface of what it needs, nothing more.

```go
type Content interface {
    GetPost(ctx context.Context, ref string) (*Post, error)
}
```

Two implementations:

- **Direct.** Wraps the other service's own service layer. In-process call.
- **Remote.** HTTP or gRPC client. Owns timeouts, retries, circuit breaking, auth, tracing.

`main.go` picks. The calling service never knows which it got, and never changes when the answer changes.

### Events

The event bus is an interface, like storage. A service publishes and subscribes; it never knows what is behind it.

The single binary uses an in-process implementation. A split deployment uses whatever the developer wires in: NATS, Kafka, Redis Streams, a Postgres outbox, something else. No transport is privileged, and none is required.

Delivery guarantees differ between implementations, so consumers are written to be idempotent and to tolerate redelivery and reordering.

### Storage

Each service owns its tables. Where those tables live is a wiring decision: one database, one database per service, separate schemas, whatever the operator chooses.

Nothing in the code enforces the boundary. A repository implementation handed the same connection as another service can join across it. That is the operator's decision and the operator's consequence. The services do not do it, and the split stays possible for anyone who did not.

Cross-service transactions are not supported. Where a flow writes to two services, it is ordered so that a failure leaves a state an operator can retry rather than an orphan nothing names: grants are purged before the resource they point at, not after. Where that ordering is impossible, because the second call needs a reference only the first can give, the call is idempotent, it is retried once, and the failure says what actually happened instead of reporting a generic error that would invite a duplicate.

Two backends ship, SQLite and Postgres, selected by `DATABASE_DRIVER`. They are held to the same behaviour by a shared conformance suite that both run, so `make test` covers both: the Postgres half starts its own database in a container, which means the tests need Docker running and no other setup. Timestamps are `timestamptz` under Postgres and text under SQLite, which has no such type; the text format's byte order is chronological order, and every Postgres column that orders or uniquifies is `COLLATE "C"` so the database locale cannot change that.

Queries are built with squirrel rather than written as strings. The two backends then differ only in a placeholder format named once per package, the timestamp type, and how each driver reports a constraint violation.

Backends are not equivalent, and the design does not pretend otherwise. A backend may support only part of a service's API, or support it with different quality: SQLite gives simpler search than Postgres full text, which is weaker again than a dedicated engine; cursor pagination, ordering, and aggregate behaviour vary the same way. Each service documents what each of its backends supports, and choosing a backend is choosing that set of features.

## General services

### auth

OAuth2 and OIDC provider. Owns identity: users, service accounts, sessions, anonymous visitors.

Hybrid service. The protocol surface is `/authorize`, `/token`, `/userinfo`, and JWKS. It also renders the pages that cannot be anything but pages: login, register, consent, password reset, MFA challenge. Those handle credentials in the browser, so the consuming application must never render them.

Everything else is plain API. Account management, profile, and session listing are endpoints the application renders itself.

Two credential shapes:

- **JWT** for API clients. Standard claims, signed, verified locally against JWKS. No call to `auth` per request.
- **Opaque session id** for browser sessions and SSR. Verified with `auth`, which owns session state.

Both must be revocable, for logout and for ban. A session id is revoked by deleting it, and takes effect immediately. A JWT cannot be, because nothing checks with `auth` when it is verified. So access tokens are short-lived and revocation happens at refresh, which leaves a window equal to the token lifetime. Where that window is unacceptable, `auth` publishes a revocation event and verifiers keep a local denylist until the token would have expired anyway.

### authz

Answers one question: can subject X perform action Y on resource Z?

- RBAC (roles and permissions)
- ACL (direct subject-to-resource grants)
- Groups of subjects
- Fine-grained policies for what neither covers

A subject is an opaque ref. `authz` never resolves it and stores nothing about it. `auth` decides who you are; `authz` decides what that subject may do.

Organizations are out of scope. Whether they land here or in `auth` is deferred until something needs them.

### media

Upload and download over an object storage abstraction, so the backend (S3, GCS, local disk) is a deployment choice.

- Image resizing, compression, format conversion
- Metadata extraction
- Malware scanning on upload
- Signed URLs

### discuss

Threaded discussion attached to any resource. The largest of the general services.

- Replies and arbitrary nesting, with a depth limit
- Edit history, soft delete, tombstones that keep a thread readable
- Moderation: queue, flags, spam signals, per-thread locks
- Mentions, which emit events for `notify`
- Sort and pagination over trees, not just lists

### react

Reactions attached to any resource: likes, dislikes, emoji, bookmarks, ratings. The set of allowed reactions is configuration. Provides aggregate counts and per-subject state.

Deliberately small. It is a counter with rules. Keeping it separate from `discuss` keeps it fast and cacheable.

### notify

Delivery across email, push, SMS, and in-app.

- Templates with per-channel rendering
- Per-user channel preferences and opt-outs
- Retries, rate limiting, delivery status

### search

Generic indexing and query API. Search knows no domain shape.

A service gets its documents into an index either way: it pushes them, or `search` subscribes to its events and builds the index itself. Push is explicit and immediate. Subscribing keeps the indexed service unaware that `search` exists. Both are supported, per service.

### hooks

Outbound event subscriptions for third parties. Subscription management, delivery with backoff, request signing, delivery history and replay.

### jobs

Async execution for the other services. Retries with backoff, delayed and scheduled jobs, dead-letter queues.

### audit

Immutable append-only record of important actions. Generic actor, action, resource, context. Queryable on any dimension.

## Blogging-specific services

### content

Owns the blogging domain: posts, drafts, revisions, tags, categories, publishing and scheduling. It does not own the website UI.

### web

Owns the public website and all server-side rendering. Holds no domain data. Composes `content` and the general services into pages.

## Example request

```text
Browser
   │ GET /p/post-1
   ▼
web
   ├── content
   ├── discuss
   ├── react
   └── media
   │
   ▼
SSR HTML
   │
   ▼
Browser
```

Prefer events over synchronous fan-out. A page render should not call five services on every hit. `web` keeps a read model updated by events and calls directly only where freshness matters.

## OAuth flow

`auth` owns its own browser UI. `web` never sees a password.

```text
Browser
   │
   ▼
web
   │ redirect
   ▼
auth
   ├── /authorize
   ├── /login       ← SSR
   ├── /consent     ← SSR
   ├── /token       ← protocol/API
   └── /userinfo
   │
   ▼
web callback
```

## Replaceability

A service here can be swapped for an existing product. `auth` for Keycloak, Ory Hydra, Zitadel, or Auth0; `authz` for OpenFGA, Ory Keto, SpiceDB, or Cedar. The consumer keeps its interface and gets a different implementation behind it.

For `auth`, most of it needs no adapter at all: consumers speak OIDC, so they point at another issuer and stop caring. Only the parts OIDC does not define need one, which is service accounts, session listing, and account management.

Two rules keep this possible, and both cost nothing now:

- A dependency interface describes the capability, never our endpoints or our tables.
- `authz` gets no policy language of its own. Nothing external would be able to express it.

Two things do not survive the swap. An external service is always a network call, so it cannot be embedded in the single binary. And external identity providers are unreliable event sources, so cleanup that depends on hearing `user.deleted` must be reconcilable, not event-only.

## Service contract

- Each service owns its tables and reads no one else's, whatever database they sit in.
- Business logic lives in the service layer and depends only on interfaces.
- Transport, storage, and clients are adapters wired in `main.go`.
- A dependency on another service is a narrow Go interface, with a direct and a remote implementation.
- Foreign resources are refs, never dereferenced.
- Authorization goes to `authz`. Never reimplemented locally.
- Health, readiness, metrics, structured logs on every service.
- Standard library first. A dependency needs a reason.

## Repository layout

One folder per service in this repository, one Go module for all of them while the contracts are unstable. A service moves to its own module and its own repository once its API stops changing. That move changes its import path, which breaks anyone importing it, so it happens before the first tagged release.

## License

Apache-2.0. Free for commercial and closed-source use, with an explicit patent grant.
