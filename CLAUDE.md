# langler-backend

Go backend for Langler, deployed as AWS Lambda functions. Domain-driven design on a hexagonal (ports and adapters) structure.

## Runtime

<!-- Reviewed 2026-07-18 against Go 1.26 and the Lambda OS-only runtime. -->

Go has no managed Lambda runtime. We target `provided.al2023` (supported to June 2029). Do not use `go1.x`, which is long removed, or `provided.al2`, which is deprecated as of July 2026.

- The compiled binary in the deployment zip must be named `bootstrap`. Nothing else works.
- Build for `GOOS=linux GOARCH=arm64` with the `lambda.norpc` build tag. We run on Graviton.
- The handler configuration value is `bootstrap`, not a function name.

## Commands

- Build: `make build`
- Test: `go test ./...`
- Test one package: `go test ./internal/domain/booking/...`
- Race detector: `go test -race ./...`
- Lint: `golangci-lint run`
- Format: `make fmt`
- Modernize: `go fix ./...`
- Vulnerability check: `govulncheck ./...`

Lint and tests must pass before any commit is proposed.

## Architecture

Four layers. The dependency rule is absolute and one-directional:

```
adapters ──▶ ports ──▶ domain
    │                    ▲
    └──▶ application ────┘
```

- `internal/domain/` — entities, value objects, domain services, domain errors. Pure Go. Imports nothing from this repo except other domain packages, and nothing from outside the standard library except a UUID or decimal type where genuinely needed. No AWS SDK, no HTTP, no SQL, no JSON tags, no logging.
- `internal/ports/` — interfaces only. No implementations, no structs beyond parameter and result types. Split into `ports/inbound/` (use cases the outside world may invoke) and `ports/outbound/` (capabilities the domain requires from the outside world).
- `internal/application/` — use case orchestration. Implements inbound ports, depends on outbound ports, coordinates domain objects and transaction boundaries. Contains no business rules of its own; if a rule belongs to an entity, it goes in the entity.
- `internal/adapters/` — everything touching the outside. `adapters/inbound/` holds Lambda handlers and event decoders; `adapters/outbound/` holds DynamoDB, S3, SQS, HTTP client implementations. Adapters implement ports and know about the domain; nothing in the domain knows an adapter exists.
- `cmd/<function-name>/main.go` — one directory per Lambda function. Wires concrete adapters into application services and calls `lambda.Start`.

Everything is under `internal/` so the compiler enforces that nothing outside this module imports our layers.

**This deviates from the common Go idiom of declaring interfaces in the consuming package. That is intentional.** Do not refactor ports into their consumers, and do not raise it as a suggestion.

If a change seems to require an inward-pointing dependency — the domain needing an adapter, or ports importing an adapter — the design is wrong. Stop and say so rather than adding the import.

## Abstraction and contracts

- The domain and ports define contracts; adapters satisfy them. A port describes **what** the domain needs, never **how** it is achieved. `Save(ctx, booking)` is a port. `ExecuteDynamoTransactWrite(...)` is not.
- Ports state only what a caller invokes. They do not prescribe an implementation's internal structure — no lifecycle hooks, no template methods, no interface methods that exist only for a particular adapter's benefit. An adapter is free to organise its internals however it likes as long as the contract holds.
- Keep interfaces minimal. One to three methods is typical. A large interface is a sign that several unrelated capabilities have been merged; split it.
- Accept interfaces, return concrete types. Constructors return `*Service`, not an interface.
- Dependencies arrive through constructors. No package-level mutable state, no service locators, no `init()` for wiring.

## Encapsulation

Go has no classes; encapsulation is done with packages and unexported identifiers.

- Struct fields are unexported by default. Export a field only when callers legitimately need direct access.
- Construct aggregates through a `New...` function that validates invariants and returns `(T, error)`. A zero-value entity that violates its own rules must not be constructible from outside its package.
- Export the smallest surface that makes the package usable. If a helper exists to serve one call site inside the package, it stays unexported.
- Do not add getters and setters mechanically. Expose behaviour — `booking.Cancel(ctx, reason)` — rather than mutable state.
- Entities enforce their own invariants. Application services must never be the only thing preventing an invalid entity.

## Comments

Write code that does not need explaining. Do not narrate.

- No docstrings and no doc comments, anywhere. Names and signatures carry the contract; if they cannot, rename or restructure until they do.
- No comments restating what the code does, no section banners, no commented-out code, no `// TODO` without a tracked ticket reference.
- A comment is warranted only when the code cannot carry the information: a non-obvious business rule and its source, a workaround for an external bug with a link, a deliberate deviation from the obvious approach, or a concurrency or ordering constraint that is invisible locally.
- When a comment feels necessary, first try renaming or extracting. Usually that removes the need.

## Go conventions

- Errors are values. Return them, wrap with `fmt.Errorf("...: %w", err)`, inspect with `errors.Is` and `errors.As`. Never discard an error with `_` outside a deferred `Close` where failure is genuinely irrelevant.
- Domain errors are sentinel values or typed errors declared in the domain package. Adapters translate infrastructure errors into domain errors at the boundary; a `dynamodb.ConditionalCheckFailedException` must never reach a use case.
- `panic` is for programmer error only, and never inside a handler path. A Lambda that panics returns an opaque failure.
- `context.Context` is the first parameter of every function that performs I/O or may block. Propagate it; never store it in a struct; never pass `nil`.
- Respect the context deadline. Lambda's timeout arrives through the context, and work that ignores it burns the whole invocation before failing.
- Log with `log/slog` in JSON. Structured key-value attributes only, no `fmt.Sprintf` into a message string. Never log request bodies, tokens, or personal data.
- Naming: short receivers, no stuttering (`booking.Service`, not `booking.BookingService`), no `I` prefix or `Impl` suffix on interfaces.
- Prefer the standard library. Justify every new dependency; each one is a supply-chain and cold-start cost.
- Run `go fix ./...` when upgrading Go rather than hand-applying migrations.

## Lambda specifics

- Construct SDK clients, config, and connection pools once at package initialisation in `main`, before `lambda.Start`. Never inside the handler — that pays the cost on every invocation.
- Handlers are thin. Decode the event, call an inbound port, map the result to a response. No business logic in `adapters/inbound/`.
- Assume concurrent invocations and container reuse. Anything cached between invocations must be safe to share and safe to be stale.
- Treat every invocation as potentially a retry. Handlers reached by SQS or EventBridge must be idempotent.
- Cold start is a real cost: keep binaries small, avoid heavyweight reflection-based frameworks, avoid pulling in the whole AWS SDK when one client will do.

## Testing

- Domain and application packages are tested with no infrastructure, no network, and no mocks of our own code beyond port fakes.
- Table-driven tests with subtests. Call `t.Parallel()` where the test allows it.
- Hand-write small fakes for outbound ports in `_test.go` files. Do not add a mock-generation framework.
- Use the standard library and `testing`. Assertion libraries are permitted but not required; do not introduce one into a package that manages without.
- Adapters are tested against the real thing or a local emulator, not against a mock of the SDK. A test that only proves we called the SDK the way we thought proves nothing.
- Test exported behaviour, not unexported helpers. If something unexported feels like it needs direct testing, it probably wants to be its own package.
- Add a failing test before fixing a bug.

## Workflow

- Branch from `main`, conventional commits, never push directly.
- Keep changes within one layer where possible. A change touching all four layers usually means the port was wrong.
- When adding a capability, work outside-in: define the port first, then the application service, then the adapter.
- State in the summary which layers a change touched and why each was necessary.
