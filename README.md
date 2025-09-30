# Lode

**Eureka — you’ve hit gold!**

Lode is a tiny loader library that makes **querying related data** easy, fast, and explicit.

- **No Magic.** No codegen or schema reflection - define relations in Go.
- **Ergonomic While Avoid N+1.** One query is peformed across all models in the batch, but the the API still feels comfortably per-model.
- **ORM-agnostic core.** Zero deps; you own the SQL/ORM in `Fetch`, keeping things explicit and portable.
- **GORM helpers.** The separate `lodegorm` module adds convenience for GORM.

## Install

```bash
go get github.com/willhf/lode@latest
# Optional GORM adapter:
go get github.com/willhf/lode/lodegorm@latest
```

## Example

See: [example/example.go](./example/example.go)
