# retry

Retries an API call's transient failures.

## Behaviour

- `TransientFailures` runs an attempt, and runs it again only while the attempt reports a failure as transient (`AttemptResult.Transient`). The caller decides which failures are transient
- A permanent failure or a success ends the call at once, also after a transient failure. The last attempt's value and error are returned as is
- `DefaultPolicy`: up to 3 attempts, 2s before the second and 5s before the third, and a 15s deadline on each attempt
- Each attempt runs under its own deadline, derived from the caller's ctx
- The caller's ctx bounds the whole call: once it is done, no further attempt starts and a running wait stops. The error returned then wraps both the last attempt's error and the ctx error, as `<attempt error>, not retried: <ctx error>`
- Each retry logs one line before its wait: `<api> attempt <n> failed, retrying in <wait>: <err>`
- `Policy` holds the waits, the attempt deadline and the wait itself, so a test can record the waits instead of sleeping them

## Doesn't Do

- Doesn't read a server's `Retry-After` or rate-limit headers: the waits are fixed
- Doesn't add jitter
