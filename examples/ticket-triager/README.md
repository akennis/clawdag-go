# Example 1 — Customer Support Ticket Triager

Reads a ticket file, classifies it (billing / bug / feature / other), runs a
category-specific AI extraction lane, and emits a single structured JSON brief.

## Usage

```bash
export CLAUDE_API_KEY=<your key>

go run ./examples/01-ticket-triager --ticket examples/01-ticket-triager/testdata/tickets/billing.txt
go run ./examples/01-ticket-triager --ticket examples/01-ticket-triager/testdata/tickets/bug.txt
go run ./examples/01-ticket-triager --ticket examples/01-ticket-triager/testdata/tickets/feature.txt
go run ./examples/01-ticket-triager --ticket examples/01-ticket-triager/testdata/tickets/other.txt
```

## Expected output

Each invocation prints a JSON document on stdout (debug logs go to stderr) of the form:

```json
{
  "category": "billing",
  "details": { "...": "lane-specific" },
  "refund_amount_usd": 49,
  "ai_nodes": [
    "ModeSelectOp",
    "AIExtractMapOp(billing.extract)",
    "AIParseNumberOp(billing.refund)"
  ]
}
```

The `ai_nodes` field lists every AI op whose output wire actually fired —
so the bug, feature, and other lanes show different sets.

## Per-cost-center credentials

Every AI vertex declares a `credential_ref` param (`triage`, `billing`, `bug`,
`feature`, `other`). The bundled `costCenterFactory` resolves each ref by
reading `CLAUDE_API_KEY_<COSTCENTER>` from the environment, falling back to
`CLAUDE_API_KEY` when the per-center var is unset, so the example runs with a
single shared key.

The env-var lookup is purely for demo convenience. A production factory would
resolve `credential_ref` against a real credential store — AWS Secrets
Manager, GCP Secret Manager, HashiCorp Vault, Azure Key Vault, a
KMS-decrypted blob, etc. — and return the appropriate client. The factory
interface is the same; only the lookup changes.
