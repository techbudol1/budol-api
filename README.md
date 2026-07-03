# BudolPH API

Go/Fiber API for BudolPH.

```bash
cp .env.server.example .env.server
bun install
go test ./...
go run ./cmd/api
```

The API requires Memgraph and communicates with GMR Engine through
`GMR_ENGINE_API_BASE`.
