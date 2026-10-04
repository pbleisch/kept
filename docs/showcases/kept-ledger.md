```
  refs/kept/ledger                    ← the ledger (a git ref, not a working tree directory)
  │
  ├── artifacts/                      ← append-only content-addressed storage for diagnostics
  │   ├── session.transcript/         ← session transcripts w/ retention policy
  │   ├── thread.argument/            ← adversarial pass arguments
  │   ├── thread.brief/               ← repair briefs
  │   └── thread.packet/              ← repair packets
  │
  └── ledger/
      ├── acts.jsonl                  ← record all acts (e.g. ratified, retired, landed, ...)
      │
      ├── disposition/                ← repair outcomes
      │   └── promise/<id>.jsonl      ← {artifact-kind}/<id>.jsonl
      │
      ├── observations/               ← gatherer measurements (content-addressed storage)
      │
      ├── runs/                       ← diagnostics w/ retention policy
      │   └── <id>.json               
      │
      ├── threads/                    ← repair episodes
      │   └── <id>.jsonl              ← packet, receipts, transcript entries
      │
      └── truth/                      ← verdict history (transitions)
          └── promise/<id>.jsonl.     ← {artifact-kind}/<id>.jsonl
```
