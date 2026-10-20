# Security scanner tooling

This module pins Gosec v2.29.0 with golang.org/x/tools v0.52.0 so its importer understands Go 1.27 export data. It does not change application dependencies. Build from the repository root:

```sh
go -C tools/security build -o ../../gosec github.com/securego/gosec/v2/cmd/gosec
./gosec ./internal/...
```

All scanner rules remain enabled. Two narrowly scoped source annotations document reviewed false positives: owner-only directory mode 0700 and the backend-resolved GitHub CLI executable with constant arguments. Regression tests cover private file permissions and the fixed CLI invocation.
