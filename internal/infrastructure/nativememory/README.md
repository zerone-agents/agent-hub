# Native Memory adapter

`New(Config{BaseURL, APIKey, BankID})` binds an application to one dedicated,
private memory module bank. The adapter implements `memory.NativeConversations`,
`memory.NativeManagement`, `memory.NativeDiagnostics`, and the quote-ledger
`memory.Index` interface. It does not use a knowledge-base fallback.

Application configuration and business API usage are described in
[Native Memory integration](../../../docs/memory/README.md).

## Configuration and identity

Use a module SDK API token with `SDK_API_TOKEN` authentication. External
`user_id` attribution must remain unchanged; a platform Web session or JWT
cannot replace this credential. Every read checks the configured bank and the
expected user, agent, session, and source identity. Credentials remain on the
server, and redirects cannot forward them to another URL.

Automatic extraction requires a bank with all four types: `raw`, `semantic`,
`episodic`, and `procedural`, with private permissions and configured extraction
and embedding models. Bank validation fails closed when its type, permissions,
or identity do not match the application.

The tenant, application, business user, and account lifecycle determine
nonempty native user and agent filters. The logical scope token is
`memory-<scope hash>`. Each ingestion operation uses native session
`hub-life-<scope hash>-<sha256(operationId)>`; the actual Runtime SDK session and
turn remain in the Hub receipt. This separates repeated utterances in different
turns while allowing retrieval across runtime sessions within one lifecycle.
A native session is an operation identity, not a runtime chat session.

## Conversation ingestion and recovery

`AddConversation` submits the original `user_input` and actual `agent_response`.
It does not wrap a quote or fabricate an assistant response to trigger
extraction. Native storage, embedding, extraction, and ranking are performed by
the configured memory service.

Hub persists an encrypted `submitting` receipt before dispatch. The native
message POST has no idempotency key, so a lost response remains unknown across
restarts. `FindConversation` performs read-only recovery using the
operation-specific session and exact original input and response. An empty read
does not authorize a second POST. Reusing an operation with different text is
rejected, and the adapter also blocks uncertain redispatch within its lifetime.

RAW existence and extraction completion are separate:

| State | Meaning |
| --- | --- |
| `received` | Original content is confirmed; extraction has not completed. |
| `extracting` | The native task reports work in progress. |
| `derivedAvailable` | Derived records are readable, but task completion is not confirmed. |
| `completed` | The task reports completion. |
| `failed` | The task reports failure; the original conversation is not automatically reposted. |
| `unknown` | The operation or extraction state cannot be established from the available records and task. |

A derived record does not override a task's reported failure or prove queue
acknowledgement. Natural-language corrections are newer source statements;
the adapter does not automatically invalidate all older conflicting memories.

## Verified reads and provenance

List reads use bounded RAW pages through a final empty page, then independently
read each parent and derived record's content. The default page size is 100 and
the default maximum is 101 pages, including the final empty page. The adapter
accepts coherent page-local totals or coherent global totals. Duplicate IDs,
inconsistent totals, incomplete pages, foreign scope, mismatched provenance,
or a saturated 512-child page produce an error rather than a partial directory.

Typed records preserve memory type, source message ID, source session,
creation time, validity fields, and native status. Decimal message identities
remain strings, including values beyond JavaScript's safe integer range.
Derived records are never converted into quote-verified Hub ledger facts.

Search sends bank, user, and agent filters. Each hit is independently read, and
each derived hit's RAW parent is checked within the same source session.
Inactive or forgotten content is excluded. An inactive or forgotten RAW source
also suppresses its derived content without rewriting the children.

`valid_at` and `invalid_at` describe native event or information times, not a
universal access TTL. Past experiences and future plans can remain recallable.
Supply eligibility depends on scope, native status, forget markers, and the RAW
parent. Native search does not provide scores in this adapter's DTO; returned
order is preserved without fabricated similarity values.

## Bank metadata and message status

`BankInfo` returns an allowlisted, typed view of the configured private bank:
name, description, memory types, storage type, model references, permissions,
configured capacity, capacity policy, temperature, and UTC creation time.
Optional missing values remain missing, and a real zero temperature is
preserved. It does not return prompts, credentials, owner or tenant identity,
avatar, raw vectors, or unknown fields.

`memory_size` is a configured byte limit, not measured usage, a message count,
or vector size. Capacity and policy metadata do not establish a usage
percentage or guarantee that every storage engine applies an eviction policy
in the same way. A configured capacity policy may evict native content
independently of Hub supply controls. Bank-wide provisioning and settings are outside this adapter's
per-user management operations.

`SetMessageStatus` changes one native boolean switch using an expected observed
status, then independently reads back unchanged content, type, source, session,
and the desired status. A stale expected value fails before writing; a lost
response stays unknown unless readback confirms its effect. This is an
observed-state precondition, not an atomic transaction with other service
clients.

The DTO distinguishes `nativeStatus` from effective Hub supply `status`.
Derived records also expose `parentStatus` and `statusChangeAllowed`:

- Disabling a RAW parent suppresses its derived records in Hub without changing
  each child's native switch.
- Re-enabling a parent can restore children whose own switches remain enabled.
- Disabling a child does not disable its siblings or parent. A child cannot be
  enabled while its parent is disabled.
- Forgotten targets or parents cannot be enabled through this operation.
- A status change neither edits content nor cancels queued extraction; later
  derived records can have their own enabled native switches.

## Recall diagnostics

`SearchDiagnostic` uses the same scoped search and provenance checks as
`SearchMemories`. It accepts a result limit of 1–20, a similarity threshold of
0–1, and a keyword weight of 0–1. Zero is preserved. Invalid ranges, non-finite
numbers, and invalid or overlong queries fail before a service request.

Diagnostics return only verified service hits in their original order. Reading
a RAW parent for validation does not add it to the results. Recent-RAW
supplementation belongs to the domain's normal conversation context, not this
diagnostic operation. Normal search keeps its threshold 0.2 and keyword weight
0.7.

Threshold and hybrid-weight support depend on the memory storage engine. On a
dense-only retrieval path these parameters do not create a hybrid filter; the
adapter does not invent unavailable scores or apply an unsupported additional
filter. A diagnostic call does not write a conversation, trigger extraction,
change bank settings, or alter ordinary chat defaults.

## Quote-ledger interface and deletion boundary

The `Index` interface uses the exact native session `hub-life-<scope hash>`,
keeping automatic dialogue operation sessions separate. Its `DocumentID` is
the logical `memory-<scope hash>` token; no document is created. Only canonical
JSON `{"schema":"hub-memory-fact-v1","quote":"..."}` with an empty native agent
response is decoded as a quote.

Quote writes require independent content reads and two stable bounded
snapshots. `Remove` is a logical correction using `PUT status:false` and
independent confirmation with unchanged content. It does not implement user
deletion, consent withdrawal, or account erasure. `DropDocument` returns
`memory.ErrUnsupported`.

`ForgetMessage` separately calls native message DELETE, which writes `forget_at`
and excludes that record from normal provider retrieval. It verifies scoped
content and provenance before writing and reads the marker back independently.
Confirmed repeated calls are read-only; uncertain DELETE responses are not
blindly replayed. The DTO exposes `forgottenAt` and `parentForgottenAt` so a
forgotten source is distinguishable from a separately forgotten child. It does
not cascade native markers, erase data, or provide undo. No interface calls
bank DELETE. Disabling supply and native forgetting are not physical deletion.
Native searchable content and its
vectors, logs, and backups require their own storage protection; Hub receipt
encryption does not encrypt those stores.
