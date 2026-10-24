# Billing audit recovery boundary

New batch image jobs use pricing snapshot version 2. Their complete hold and
actual charge are rounded once to eight USD decimal places with the existing
half-away-from-zero monetary rule. Prices, discounts and account cost rates are
unchanged. The persisted hold is the exact amount moved between spendable and
frozen balance. Version 0/1 jobs with an exact eight-place hold retain their
existing behavior.

Version-2 holds also carry individual ownership and a held/captured/released
receipt, committed with the money operation. Capture and release are mutually
exclusive even when another job has enough money in the user's frozen pool.
Free jobs retain their zero-money behavior. Existing holds are not backfilled.

A legacy hold with more than eight places is ambiguous. Automatic capture and
release fail with `BATCH_IMAGE_MONEY_REVIEW_REQUIRED`; they never infer ownership
from the user's pooled frozen balance. There is no migration, background job or
command that rounds historical holds, refunds them, or charges old usage.

Ordinary usage is staged synchronously as a versioned immutable event before
monetary execution. The money effects, dedup claim, raw quote, canonical usage
and applied receipt commit together. A worker retries only pending events and
idempotent cache invalidations. A lost commit response is checked against the
durable receipt before any replay. Hot and archived legacy claims remain
authoritative; they are not permission to fabricate history or charge again.

There remains an explicit observation boundary: a process can die after the
upstream produces usage but before the complete event has committed to the
journal. A database unavailable at that boundary also prevents durable delivery.
The application cannot reconstruct final usage it never persisted and does not
call the model again or invent a zero-cost success. This change removes the
in-memory queue window and recovers events committed before execution; it does
not promise that this pre-storage window or unknown upstream usage is solved.

Explicit recovery would first need an authorized reconciliation of the original
hold transaction, all other holds for that user, captures/releases and current
balances. An individually reviewed correction would require separate approval,
an immutable correction identity and a transaction proving both ownership and
conservation. This PR supplies no operator action to perform that correction.

New ordinary platform-quota increments commit with the charge and dedup claim,
independently of the legacy flusher flag. `usage_platform_quota_totals` keeps the
new projection separate from the existing baseline. The effective view combines
only matching current windows; old events cannot rewind a newer window. Explicit
administrator resets update both components in one transaction. Quota removal
and recreation use different quota row identities.

The combined read cache has its own namespace. It is invalidated idempotently;
new billing never replays Redis monetary deltas. Legacy dirty entries retain
their original namespace so the old flusher can finish its baseline snapshots
without overwriting or double-counting new deltas. Missing/old-schema legacy
increments return an explicit not-applied error. No historical cache values are
imported, no missing old charges are inferred, and no balance adjustments occur
at migration time. Existing legacy backlog requires separate evidence and must
not be described as repaired by this PR.

The raw customer quote is stored in `usage_billing_quotes.quoted_actual_cost`;
`canonical_actual_cost` records the eight-place canonical amount. Canonical
usage and cache reloads use that same customer amount. `total_cost` and the
account cost multiplier retain their independent meaning. A subscription or
simple-mode key-window charge is not new cash revenue.

Release review must account for the new durable events, projections and owned
holds before any rollback to an old binary that cannot read them. Batch workers
must all understand version-2 ownership before processing new jobs; outstanding
legacy financial writers/caches need a reviewed drain boundary. These are code
compatibility limits, not authorization to perform a cutover or repair data.
