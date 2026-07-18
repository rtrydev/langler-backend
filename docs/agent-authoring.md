# Agent authoring

Browser token management uses Cognito-authenticated `POST /agent-tokens`,
`GET /agent-tokens`, and `DELETE /agent-tokens/{id}` routes. Creation accepts a
label, one or both `read-reference` and `import-lessons` scopes, and an RFC 3339
expiry. The response includes the `lang_sk_` secret once. Lists expose only its
last four characters.

Secrets contain 256 random bits. The API stores only their SHA-256 hashes in the
owner record and direct lookup record. Revocation updates both records in one
DynamoDB transaction. The uncached machine Lambda authorizer performs a
consistent hash lookup, checks expiry, revocation, and route scope, consumes a
token-specific minute counter, and updates `lastUsed` before allowing the call.
Rate counters use the `expiresAtUnix` DynamoDB TTL attribute.

The machine HTTP API is separate from the Cognito API and exposes only:

- `GET /reference/vocab`, `GET /reference/grammar`, and
  `GET /reference/scripts` with `read-reference`.
- `POST /lessons/import` with `import-lessons`.

The authorizer passes `owner` and `tokenId` in Lambda context. Imports are stored
under that owner, so they appear in the same library as browser-authored lessons.
The app's downloadable `SKILL.md` and OpenAPI 3.1 contract teach the reference →
compose → import workflow. The optional dependency-free MCP server wraps the
same four endpoints and performs no model invocation.
