```
your-project/
│
├── .kept/                          
│   ├── boxes/                      ← runtime state (never committed)
│   ├── cache/                      ← runtime state (never committed)
│   └── config.yaml                 ← timeouts, retention, confinement
│
├── docs/                           ← the corpus (committed, authored)
│   ├── _links/                     ← manifested links
│   ├── _schema/                    ← per-kind schemas
│   │   ├── decision.yaml
│   │   ├── desire.yaml
│   │   ├── ...
│   │   └── promise.yaml
│   │
│   ├── decision/                   ← each kind has its own directory 
│   │   ├── 0001-standing-and-truth-live-in-the-ledger.md
│   │   ├── 0002-artifacts-are-wiki-entities.md
│   │   └── ...
│   ├── desire/                     
│   ├── .../                     
│   ├── promise/                     
│   └── (your other docs/ content)                   
│
└── (rest of the repo)
```
