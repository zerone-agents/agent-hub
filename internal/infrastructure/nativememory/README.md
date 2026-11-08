# Native Memory adapter

`New(Config{BaseURL, APIKey, BankID})` binds an application to one dedicated,
private MultiRAG Memory bank. It implements `memory.NativeConversations` for
actual dialogue ingestion and typed retrieval, alongside the older `memory.Index`
quote-ledger interface. No knowledge-base fallback is used.

## Automatic dialogue extraction

`AddConversation` submits the original `user_input` and actual `agent_response`,
without a synthetic quote wrapper or a second extraction model. The configured
bank must enable all four native types: `raw`, `semantic`, `episodic`, and
`procedural`. Native embedding, extraction, storage and ranking remain the
provider's implementation. Typed records preserve their original type, source
message ID, source session, validity fields and active state. Model output is
not represented as a quote-verified Hub ledger fact.

Every Hub tenant/application/user/account-lifecycle hash yields nonempty native
user and agent filters. Each operation gets native session
`hub-life-<scope hash>-<sha256(operationId)>`. This keeps repeated utterances in
different turns distinct, supports cross-runtime-session retrieval within the
same lifecycle, and permits read-only recovery after an uncertain POST. The
native session is an ingestion-operation identity, not the Runtime SDK session.
The actual SDK session and turn are retained by Hub's receipt.

The native POST API has no idempotency key. Hub must persist `submitting` before
calling the adapter, and persist an unknown outcome across restarts.
`FindConversation` only reads: an empty scan never authorizes a second POST.
The adapter also blocks an uncertain dispatch within its own lifetime. A lost
response can be resolved through the operation-specific session and exact
original input/response. Reusing an operation with different text fails closed.

List traverses bounded RAW pages through a final empty page and individually
reads parent and extracted child content. Current Milvus uses per-page totals;
a coherent global total is also accepted. Duplicate IDs, inconsistent totals,
nonterminal partial pages, foreign scope, mismatched parent provenance or the
native endpoint's saturated 512-child limit fail closed. Search always sends
bank/user/agent filters; every returned record and its RAW source are read back.
Inactive or forgotten content is not supplied. Native validity timestamps
remain typed data, not a universal TTL: episodic valid_at / invalid_at describe
an event beginning / ending, so past events and future plans remain recallable.
An inactive source suppresses its derived records even if the native backend
leaves those children active. Native scores are unavailable;
ranking order is preserved without fabricated similarity metrics.

Extraction state is separate from RAW existence. Native task progress can report
`received`, `extracting`, `completed`, or `failed`. If derived records exist but
the task has not completed, the adapter returns `derivedAvailable`. No task and
only RAW means `unknown`. `derivedAvailable` is evidence of extracted records,
not successful queue acknowledgement or task completion. Current local real
acceptance separately confirmed completed task receipts, SQL progress, queue
acknowledgement and independently read message/vector storage; it used synthetic
profiles and does not establish production acceptance.

Native extraction does not establish automatic invalidation of older conflicting
memories. Dialogue corrections remain newer source statements; clients must not
claim the old records were deleted or automatically superseded. No method in
this interface invokes native Forget or DELETE.

## Existing quote-ledger compatibility

The older `Index` interface uses the exact native session
`hub-life-<scope hash>`, so automatic dialogue sessions do not enter that ledger.
Its logical `DocumentID` remains `memory-<scope hash>`; no document is created.
Only canonical JSON `{"schema":"hub-memory-fact-v1","quote":"..."}` with an
empty native agent response is decoded as a quote. Derived messages are never
converted into quote-ledger facts.

Quote writes are confirmed through independent content reads and two stable
bounded snapshots. `Remove` is only logical correction: native `PUT status:false`
plus independent confirmation, with unchanged original content. It cannot back
user deletion, consent withdrawal or account cleanup. `DropDocument` returns
`memory.ErrUnsupported`.

Native Memory stores searchable plaintext. These local checks use synthetic
profiles, real Qwen extraction and real embedding; they do not establish
production, sensitive-data protection or cross-engine acceptance.


## Native management capability audit (2026-11-08)

This audit follows the active Python REST API and actual Web memory consumer,
not API names alone. Bank-wide settings affect every business user sharing that
application bank; Hub does not turn a per-user management screen into a global
configuration editor.

| Capability | Native REST and Web consumer | Current Hub use / boundary |
| --- | --- | --- |
| Bank identity/configuration | `GET /memories/{id}/config`; Web detail/settings show name, description, models, types, storage, permissions, capacity and extraction settings | Adapter validates the configured private bank and provides `BankInfo` through an explicit allowlist. No credentials, prompts, owner/tenant identity, avatar or arbitrary bank identifier passthrough. |
| Bank creation/settings | `POST /memories`, `PUT /memories/{id}`; Web supports name/avatar/description, permissions, capacity, policy, temperature and prompts | Application provisioning selects the dedicated bank; Hub does not yet expose global editing. Changing type or embedding is rejected by Python when a nonempty bank would be changed; the Web additionally disables chat model edits after messages exist. |
| Capacity | `memory_size` is the configured maximum in bytes (default 5 MiB), returned in bank config | It is not used bytes, message count or vector size. This endpoint has no current-usage measurement. Hub must not invent a usage percentage. |
| Capacity policy | API/config accept `FIFO` / `LRU`; current writer implements FIFO overflow eviction, while another policy reports failure at capacity | Configuration can be reported as configuration only. FIFO may physically evict old records when full, independently of the user-facing Forget feature. LRU is not proven implemented by the enum or selector. |
| Models/temperature | Config returns `llm_id`, `embd_id`, `temperature`; settings select models and temperature | Chat extraction and real embedding are already consumed. Model refs can be numeric strings, not human-readable names. Bank metadata must not resolve or return provider credentials. |
| Prompts | Settings save `system_prompt` / `user_prompt` | Current async extraction caller does not pass these saved fields to `extract_by_llm`; default assembler is used. Do not promise that editing these settings changes the active chat extraction path. Prompt bodies are omitted from Hub's proposed safe metadata. |
| RAW pagination/filtering | `GET /memories/{id}` accepts page/page_size and agent_id; `keywords` is actually an exact native session filter | Hub currently performs a bounded complete scan and returns typed records. Frontend filtering/grouping can improve management, but a text box must not be falsely described as upstream full-text list search. Current Milvus totals are page-local. |
| Parent and derived provenance | Native list returns RAW parents plus `extract` children, source IDs and optional task; Web expands derived content and opens a content drawer | Hub already preserves native types, source IDs and per-message independent content verification; management UI can group by RAW and trace a selected derived record. |
| Raw content/vector diagnostic | `GET /messages/{bank}:{id}/content`; Web drawer separates conversation/raw/vector | Hub already reads content for verification. It deliberately does not expose raw embeddings or full model configuration to the normal management screen. |
| Single-message enable/disable | `PUT /messages/{bank}:{id}` with JSON `{status: boolean}`; Web has a message switch | Adapter now supports scoped `SetMessageStatus` with expected native status and independent readback. It changes one record only, with no parent/child cascade or native compare-and-swap version. |
| Recall diagnostic | `GET /messages/search` accepts query, top_n, similarity_threshold and keywords_similarity_weight, plus native scope fields | Ordinary chat uses real search with strict bank/user/agent filters and fixed threshold 0.2 / keyword weight 0.7. Separate management diagnostics pass a per-request threshold/keyword weight and limit directly to the same verified search; engine-level application differs (see below). The native DTO has no scores, so no score is manufactured. |
| Extraction task status | RAW list attaches the latest task by source digest; Web DTO receives task | Hub already distinguishes received, extracting, derivedAvailable, completed, failed and unknown. There is no dedicated memory-task retry/cancel endpoint in this REST surface. Unknown POSTs remain read-only recovery. |
| Recent messages | `GET /messages` offers limited recent records by bank/agent/session | Hub uses verified bounded list reads instead, allowing parent validation and recent RAW supply without losing lifecycle scope. |
| Forget / bank deletion | Native `DELETE /messages/{bank}:{id}` sets forget_at; `DELETE /memories/{id}` exists and Web exposes both | Explicitly not integrated as Hub business deletion, consent cleanup or physical account erasure. This iteration must not call them or report successful cleanup. |

Important consumer findings: `web/src/pages/memory/MemoryListPage.tsx` currently
labels hard-coded sample values as summary statistics (1280 messages, 45.6 MB,
and 80% active). `MemoryCard` renders `memory_size` next to a message icon. These
are not sources of real capacity usage or message counts and must not be copied
into Hub. Bank usage requires a separately defined trustworthy data source.

### Status and source semantics

Native `status` is the record's own boolean switch; `forget_at` is a separate
soft-forget marker. The typed DTO preserves `nativeStatus` as this original
switch and `status` as Hub's effective supply state. Derived `parentStatus`
records the RAW source's effective state; `statusChangeAllowed` is false for
forgotten targets or forgotten parents. Native `PUT status` changes neither content nor source ID,
does not cascade to derived records, and does not cancel an already queued
extraction. Derived messages can arrive later with their own native status true.
Hub's supply rule additionally requires an active, non-forgotten RAW parent.
Therefore disabling a RAW source suppresses all of its derived content in Hub
without rewriting every child; enabling that RAW can restore any child whose own
native status remains enabled. Disabling one child should not disable its
siblings or parent. A UI must distinguish a child's own switch from its effective
supply state, rather than silently persisting the parent's state as the child's.

`SetMessageStatus` fixes the configured bank, enforces the complete
business scope and source/session provenance, rejects forgotten records, checks the expected
native switch, issues only the standard PUT, and reads back the same content/type/source/session with
the desired native status. A stale expected switch fails before PUT. A currently disabled child cannot
be enabled while its RAW parent is disabled; independently disabling it remains
allowed. A lost response is unknown until readback confirms its effect. Native
PUT has no atomic version precondition across external Web writers; do not
describe this as a cross-system CAS transaction.

Audit source paths:
`api/apps/restful_apis/memory_api.py`,
`api/apps/services/memory_api_service.py`,
`api/db/joint_services/memory_message_service.py`,
`api/utils/memory_utils.py`, `memory/services/messages.py`,
`web/src/api/memory.ts`, `web/src/pages/memory/MemorySettingsPage.tsx`,
`web/src/pages/memory/MemoryMessagesPage.tsx`,
`web/src/components/memory/MemoryMessageTable.tsx`.


`BankInfo` returns only name, description, normalized memory type strings,
storage type, chat/embedding model references, permissions, configured capacity,
capacity policy, temperature and UTC creation time (from native epoch millis).
Missing optional capacity/temperature stays missing; a real zero temperature is
preserved. It neither returns provider config nor invents measured usage.

Targeted adapter coverage includes bank allowlist/unknown-field secrecy, zero
preservation, rejected bank/permissions/config, decimal IDs above 2^53, native
versus effective child state, parent suppression, independent child control,
forgotten/foreign targets, stale expected state, lost PUT response, false-success
readback, and unexpected content/source mutation. Real status-control acceptance
uses a previously registered synthetic validation scope, restores every switch,
and independently confirms unchanged content, type, provenance and row count.
It does not add messages, trigger an extraction, call Forget/DELETE or restart a
service. Management route authorization and browser interaction have separate
consumer verification.


### Native recall diagnostics

`SearchDiagnostic` implements optional `memory.NativeDiagnostics` using the same
strict search helper as normal `SearchMemories`. It accepts an explicit result
limit from 1 to 20, similarity threshold from 0 to 1, and keyword weight from 0
to 1. Both endpoints are inclusive; valid zero values are forwarded as zero,
not replaced by defaults. NaN, infinities, invalid ranges and invalid/overlong
queries are rejected before any native request.

The diagnostic result contains only the provider's actual returned hits, in its
original order, after full scoped content and RAW parent validation. Reading a
parent for validation does not add that parent to the diagnostic results. The
recent-two-RAW supplementation belongs only to the domain's normal conversation
context and is not part of this diagnostic call. This operation performs real
embedding retrieval but does not write a conversation, invoke extraction, change
bank-wide settings, or alter ordinary chat's defaults (0.2 / 0.7).


Current local engine limitation: `memory/utils/milvus_conn.py` handles the dense
vector expression but does not apply its similarity option, the text expression
or fusion weights. Thus the REST parameters are forwarded correctly, while this
Milvus implementation does not implement their advertised threshold/hybrid
semantics. A real read-only diagnostic probe using the registered synthetic
scope returned the same five IDs for (threshold, keyword weight)=(0,0) and
(1,1); static inspection confirms why. Other engines have distinct consumers.
Do not describe successful parameter transport or a diagnostic result as proof
that every native engine applied the threshold and keyword weighting. The Hub
adapter deliberately does not invent an extra filter without native scores or
change the provider's standard behavior.
