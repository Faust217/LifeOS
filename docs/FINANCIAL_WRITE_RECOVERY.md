# Financial Write Recovery and Capacity Operations

Accepted [ADR-019](adr/ADR-019-owner-scoped-financial-write-idempotency.md) governs Issue #34. This is an online bookkeeping retry mechanism, not offline synchronization, transaction editing or a deletion inventory.

## Client and API contract

Account creation, income/expense creation, transfer, opening-balance confirmation, Void and Soft Delete require one cryptographically random lowercase UUIDv4 `Idempotency-Key`. The session determines the owner. All six service entry points also require the explicit key. No unprotected fallback is available.

The original successful DTO/status has `Idempotency-Replayed: false`. A matching committed retry returns only `200 {"outcome":"already_applied"}` with `Idempotency-Replayed: true`; refresh current records, because a record may since have been Voided, Deleted or changed. Responses are private/no-store. A different normalized command under a committed key gets `409 idempotency_key_conflict`. Do not rotate the key automatically.

The PostgreSQL marker and financial changes commit in one READ COMMITTED transaction. Owner transaction locks serialize covered writes. Lookup happens again after waiting, before current target/revision checks. Thus an already-successful opening confirmation is acknowledged even if its revision subsequently advances. Rollbacks retain neither marker nor partial effects. No original request, result ID or DTO is stored in the marker table.

## Uncertain Save recovery

- Before sending, `PendingFinancialWrites.prepare` freezes the exact operation, target, path, body and key in one bounded per-tab sessionStorage record. A local SHA-256 checksum detects accidental record corruption before retry. It is not authorization or protection against a compromised same-origin script.
- Normal successful Save needs no additional confirmation. A valid first success or replay clears only that exact record. An older asynchronous completion cannot clear another owner's newer record.
- Network loss, timeout, malformed success, 5xx or auth failure keeps recovery. Close the form if needed and use **Retry Save** with fresh session/CSRF credentials. Do not assemble a retry from edited fields, a new current time, a new preview revision or a fresh key.
- New financial Saves cannot overwrite a pending record. Conflicts need review. Clearing recovery requires explicit acknowledgement that activity was checked and clearing loses the retry key; clearing cannot undo a server commit or prove failure.
- A definitive rejection of the initial attempt restores the editable draft. Rejected retries remain unresolved because an earlier attempt may have committed. Storage unavailable/full blocks preparation before HTTP; unreadable, invalid or checksum-mismatched recovery blocks replay. Never say a lost key can be recovered.
- Same-tab reload can recover the stored record after same-owner authentication. Session expiry hides it until reauthentication. Different-owner sign-in retains the prior owner's unresolved record but hides its contents and blocks new financial writes; the user must return to the original owner or explicitly acknowledge losing the key before clearing it. Explicit sign-out warns about unresolved recovery and clears it after confirmation; closing the tab or losing storage does not guarantee recoverability.
- The browser warns before leaving with unresolved recovery where supported. Browsers may handle that warning differently. No background retry, cross-device queue or offline capture is implemented. Private submitted draft text is unencrypted session storage and is cleared promptly on resolution/sign-out.

## Detect approaching capacity without exposing private data

The initial reviewed constant is `FinancialWriteKeyLimit = 100_000` in `internal/finance/idempotency.go`. Successful keys do not expire or get evicted. The owner-leading primary key supports bounded per-owner counting; the owner lock protects check-plus-insert against races.

At **90,000** committed keys, and on subsequent successful writes above that threshold, the application emits a structured warning with only:

`event=finance_idempotency_capacity band=approaching limit=100000`

At the hard limit, a new key emits `band=full` and receives `409 idempotency_capacity_exceeded`. Matching retained-key replays are looked up before quota checks and still succeed. Warnings use the Go default structured logger (normally stderr); retain/access that stream using the existing approved operation environment. They contain no owner IDs, keys, account/event IDs, transaction text, amounts or session secrets. No dashboard, scheduler or separate telemetry service is added. Warnings are best effort, not a durable alert queue.

A separately authorized operator may check aggregate headroom in the **explicitly selected and approved database** using a read-only transaction. This is documentation, not authorization for an agent to inspect the personal database:

```sql
BEGIN READ ONLY;
SELECT COALESCE(max(request_count), 0) AS largest_owner_key_count,
       count(*) FILTER (WHERE request_count >= 90000) AS owners_approaching_limit,
       count(*) FILTER (WHERE request_count >= 100000) AS owners_at_limit
FROM (
    SELECT count(*) AS request_count
    FROM finance_write_requests
    GROUP BY owner_id
) AS counts;
COMMIT;
```

The result reveals aggregate capacity, not owner identities or financial contents. Do not add these internal counts to ordinary user exports or a public endpoint. Check existing database storage capacity through the approved operational process as well.

## Raise the limit safely before blocking writes

1. Respond to the approaching warning with the remaining 10% headroom. Confirm the deployment/database being operated on, its tested backup/restore status, disk capacity and expected growth. Personal-data access and deployment require their own approval.
2. Propose and review an explicit increase of `FinancialWriteKeyLimit` (for example 200,000); retain every marker and the existing UUID/fingerprint contract. The warning threshold is calculated as 90% of the selected limit. Do not lower the limit, prune rows or add an expiry to recover capacity.
3. Run boundary/replay/concurrency tests and a disposable near-new-limit rehearsal, including lookup/count latency within the 10-second deadline. The initial acceptance tests intentionally exercise the actual 90,000/100,000 thresholds; update their expected operational limit only in the reviewed increase.
4. After separate delivery/deployment authorization, activate the same reviewed limit on every writer. Drain in-flight requests normally before restarting; retained keys make uncertain requests safe to retry. Older processes with the lower limit can still reject new writes, so verify all writers before claiming the increase is active.
5. Recheck aggregate headroom and normal/replayed writes with synthetic data. No DDL, ledger recalculation, marker rewrite, new infrastructure or import is necessary. If performance requires another design, review it explicitly rather than silently evicting old keys.

If capacity is already full, existing-key retries continue to work while the operator follows this procedure. A new write rejected at capacity did not register that new key. Do not promise an automatic retry time or tell users to discard an unresolved earlier Save.

## Bounds, upgrades and restores

- Eight guarded financial handlers per service/process, nonblocking admission; direct application clients share the same gate. Shared authentication guards run before handler admission. The whole HTTP request has a 10-second deadline, including pool/auth waits and final revalidation.
- Transaction-local lock/statement/idle-in-transaction limits are 2/8/10 seconds. No global PostgreSQL or machine settings change. Transactions/connections are released before final session revalidation; revocation after commit can deny the response, so keep the pending key.
- `503 financial_write_busy` has `Retry-After: 1`; `financial_write_outcome_unknown` means commit might have succeeded. Both require the original key/body. Unsupported fingerprint versions fail closed. The body cap is 16 KiB and serialized browser recovery cap is 32 KiB.
- v9 adds only `finance_write_requests`. Published v1–v8 and export v1/v2 fixtures remain unchanged. Populated v9 down refuses to discard keys; use reviewed forward correction. Existing v8 writes are not retroactively assigned keys.
- Client/server activation is coordinated: old clients missing keys fail clearly; settle unkeyed uncertain Saves before switching. An old binary writing against populated v9 is unsupported.
- Database backups must include marker and ledger state together. An older restored backup can lack newer commands/keys. Reconcile stale clients and pending retries before reopening writes; never promise exactly-once effects across such a restore. The Personal Data Ready backup/restore gate remains open.

See the [English learning journal](../learning/journal/2026-10-26-financial-write-idempotency.md) for actual function flow and verification evidence. No real financial data or personal database is needed for the Issue #34 tests.
