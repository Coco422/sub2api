# Billing audit recovery boundary

New batch image jobs use pricing snapshot version 2. Their complete hold and
actual charge are rounded once to eight USD decimal places with the existing
half-away-from-zero monetary rule. Prices, discounts and account cost rates are
unchanged. The persisted hold is the exact amount moved between spendable and
frozen balance. Version 0/1 jobs with an exact eight-place hold retain their
existing behavior.

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
