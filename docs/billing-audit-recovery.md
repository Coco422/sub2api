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

Explicit recovery would first need an authorized reconciliation of the original
hold transaction, all other holds for that user, captures/releases and current
balances. An individually reviewed correction would require separate approval,
an immutable correction identity and a transaction proving both ownership and
conservation. This PR supplies no operator action to perform that correction.
