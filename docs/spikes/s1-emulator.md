# Spike S1: local AWS emulator

Date: 2026-10-22. Question: which local emulator can the integration and end-to-end tests
(S12) trust, and which DynamoDB, Streams, Lambda, EventBridge and SQS behaviors does
the library have to cover with unit tests because no emulator reproduces them?

## Decision

Use **LocalStack 2026.09.1** (pinned tag, Hobby plan) for the end-to-end test in S12.
It is the only candidate that runs the whole path, including the Lambda event source
mapping, and it matched DynamoDB Local on every DynamoDB probe.

DynamoDB Local is the fallback for DynamoDB-only tests if the LocalStack license terms
change or if outside contributors need integration tests that run without a secret.
moto is rejected.

## Candidates

| | LocalStack | DynamoDB Local | moto server |
|---|---|---|---|
| Version tested | 2026.09.1 | 3.3.2 | 5.2.4 |
| Image size | 1.66 GB | 780 MB | 567 MB |
| Time to ready | about 10 s | about 1 s | about 1 s |
| Services | all we need | DynamoDB and Streams only | all we need, but Lambda is broken (below) |
| License | proprietary; Hobby is free for non-commercial use and needs an auth token | AWS license, free | Apache 2.0 |
| Runs in CI without a secret | no | yes | yes |

Without a token, LocalStack 2026.09.1 exits at startup. The Hobby plan lists CI testing
as included. Licenses are per user, and telemetry cannot be turned off.

## Results

Probes were throwaway Go tests using AWS SDK for Go v2 (dynamodb 1.72.1, dynamodbstreams
1.46.2, eventbridge 1.55.5, sqs 1.52.6, lambda 1.113.3). They ran against each emulator through
`BaseEndpoint`. They are not committed: the spike's output is this document.

### DynamoDB transactions

| Behavior | LocalStack | DynamoDB Local | moto |
|---|---|---|---|
| Failed condition cancels the whole transaction | yes | yes | yes |
| `CancellationReasons` has one entry per action, `None` or `ConditionalCheckFailed`, and the old item when `ReturnValuesOnConditionCheckFailure=ALL_OLD` | yes | yes | yes |
| Same `ClientRequestToken` and same input: success without re-executing | yes | yes | **no**, re-executes and fails the condition |
| Same token, different input: `IdempotentParameterMismatchException` | yes | yes | **no**, succeeds |
| More than 100 actions: `ValidationException` | yes | yes | yes |
| More than 4 MB per transaction: `ValidationException` | yes | yes | **no**, accepted |
| Item over 400 KB: `ValidationException` | yes | yes | yes |
| `TransactionConflict` under 25 concurrent transactions on one item | not reproduced | not reproduced | not reproduced |

### DynamoDB Streams (`NEW_IMAGE`)

| Behavior | LocalStack | DynamoDB Local | moto |
|---|---|---|---|
| INSERT, MODIFY and REMOVE records, `NewImage` absent on REMOVE | yes | yes | yes |
| Each item of a transaction is its own INSERT record | yes | yes | yes |
| Expired TTL items are deleted | only with `DYNAMODB_REMOVE_EXPIRED_ITEMS=1`, on an hourly sweep or on `DELETE /_aws/dynamodb/expired` | yes, within about 2 s | not within 60 s |
| TTL deletion record | `REMOVE`, no `userIdentity` | **`eventName: "UNKNOWN_TO_SDK_VERSION"`**, no `userIdentity` | none |
| Sequence number format | 21-digit zero-padded string | 21-digit zero-padded string | 22-digit string |

On AWS, a TTL deletion is a `REMOVE` record whose `userIdentity` is
`{"type": "Service", "principalId": "dynamodb.amazonaws.com"}`. No emulator reproduces
that field.

### Lambda event source mapping (LocalStack only)

Setup: Go function on `provided.al2023`, batch size 10, `MaximumRetryAttempts=1`, bisect
on error, `ReportBatchItemFailures`. Three inserts; the handler reports the middle one
as failed.

- After the reported failure, the retry batch started at the failed record and included
  the record after it. That record was processed twice, which matches the documented
  checkpoint behavior.
- When retries were exhausted, the mapping wrote to the on-failure destination and moved
  past the whole range.
- **SQS destination:** metadata only (`requestContext.condition: "RetryAttemptsExhausted"`,
  `DDBStreamBatchInfo` with shard ID, start and end sequence numbers, stream ARN). No
  record data. This matches the AWS documentation.
- **S3 destination:** the same metadata plus the full invocation payload, including the
  item's `NewImage`. This also matches the AWS documentation.

moto invokes the function synchronously inside `PutItem`. With the mapping configured,
`PutItem` returned HTTP 500 (`ValueError: month must be in 1..12` in moto's invoker),
so moto cannot test the relay path.

LocalStack runs functions in sibling containers through the Docker socket. When it was
removed with `docker rm -f`, it left those containers behind.

### EventBridge `PutEvents`

| Behavior | LocalStack | moto |
|---|---|---|
| Rule with an SQS target delivers the standard envelope (`version`, `id`, `detail-type`, `source`, `account`, `time`, `region`, `resources`, `detail`) | yes | yes, without `account` |
| 11 entries: `ValidationException` | yes | yes |
| Malformed `Detail`: per-entry `MalformedDetail`, other entries succeed, `FailedEntryCount=1` | yes | yes |
| Entry for a bus that does not exist | accepted silently | request fails with `ResourceNotFoundException` |
| One entry of 300 KB | rejected, "Total size of the entries in the request is over the limit" | accepted |
| Five entries of 230 KB (about 1.15 MB in total) | accepted | accepted |

The current API reference says the total entry size of a request must be less than 1 MB
and allows 1 to 10 entries for buses with rules and targets. LocalStack still enforces
the older 256 KB limit, but per entry and not per request. moto enforces neither.

### SQS

Both LocalStack and moto accept a 1 MiB message and reject 1 MiB plus one byte.

## Consequences for the backlog

- **S4 Save():** size and count limits must be enforced by the library and unit-tested
  with the fake client. Emulators only show what AWS would reject.
- **S5 Typed transaction errors:** cancellation reason codes are plain strings (`None`,
  `ConditionalCheckFailed`, and others). `TransactionConflict` and throttling can only be
  tested with a fake client.
- **S6 Record decoding:** process an allowlist of `INSERT` only. Unknown event names, like
  DynamoDB Local's TTL record, are skipped and logged, not sent to the poison path. TTL
  `userIdentity` handling is tested with fixtures taken from AWS documentation.
- **S7 Batching:** limits are 10 entries and less than 1 MB of total entry size per
  request, calculated with the documented entry size formula. End-to-end tests keep each
  event below 256 KB because of LocalStack's older limit.
- **S9 DLQ:** an SQS message is at most 1 MiB, so a DLQ message holding a near-limit
  event plus its reason can be too big. That case must become a batch item failure, not
  a silent drop.
- **S10 Lambda adapter:** sequence numbers are decimal strings of varying length, so the
  "lowest failed sequence number" must be compared numerically, not as strings. The
  reprocessing of records after a reported failure is confirmed and must be documented.
- **S12 End-to-end:** LocalStack with the Docker socket mounted,
  `DYNAMODB_REMOVE_EXPIRED_ITEMS=1` and the expiry endpoint for TTL, a clean shutdown that
  removes Lambda containers, and the token as a repository secret (already in the threat
  model).
- **S14 Guarantees doc:** the relay's own DLQ stays, because it covers failures where the
  function does not fail (`PutEvents` partial failures, oversized events). An S3
  on-failure destination on the event source mapping is a recommended second safety net:
  it keeps full records if the relay itself keeps failing until retries or record age run
  out. That bucket is one more copy of payloads and needs the same encryption and access
  rules as the DLQ. A mapping filter on `eventName: INSERT` also saves the invocations
  for MODIFY and TTL REMOVE records; the relay still filters, so the filter is an
  optimization, not a requirement.

## Not verified

These need real AWS, which is planned under "Later" (real-AWS smoke test):

- `TransactionConflict` and throttling reason codes under real contention.
- `PutEvents` behavior for an entry naming a bus that does not exist.
- The exact TTL `REMOVE` record on AWS (taken from documentation only).
- The 1 MB total entry size limit (taken from the API reference only).

## How to reproduce

1. Pull `amazon/dynamodb-local:3.3.2`, `motoserver/moto:5.2.4` and
   `localstack/localstack:2026.09.1`.
2. Start one emulator. For LocalStack, pass `-e LOCALSTACK_AUTH_TOKEN`, mount
   `/var/run/docker.sock` and set `DYNAMODB_REMOVE_EXPIRED_ITEMS=1`.
3. Point an SDK client at it with `BaseEndpoint` (ports 4566, 8000 and 5000), static
   credentials and region `us-east-1`, then repeat the calls in the tables above.

## Sources

- LocalStack licensing: https://docs.localstack.cloud/aws/licensing/
- LocalStack DynamoDB (TTL): https://docs.localstack.cloud/aws/services/dynamodb/
- DynamoDB Streams and TTL: https://docs.aws.amazon.com/amazondynamodb/latest/developerguide/time-to-live-ttl-streams.html
- DynamoDB Local release history: https://docs.aws.amazon.com/amazondynamodb/latest/developerguide/DynamoDBLocalHistory.html
- Lambda on-failure destinations for DynamoDB: https://docs.aws.amazon.com/lambda/latest/dg/services-dynamodb-errors.html
- EventBridge `PutEvents` API reference: https://docs.aws.amazon.com/eventbridge/latest/APIReference/API_PutEvents.html
