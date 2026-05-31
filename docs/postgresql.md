# PostgreSQL

## Configuracao local

O projeto inclui um `docker-compose.yml` com PostgreSQL 16.

```bash
docker compose up -d postgres
```

Use a seguinte variavel no `.env`:

```text
DATABASE_URL=postgres://chipmov:chipmov@127.0.0.1:15432/chipmov?sslmode=disable
```

## Migrations

Neste momento, as tabelas sao criadas automaticamente na inicializacao da API por `storage.Migrate`.

Em uma etapa futura, para ambiente corporativo, a recomendacao e mover esse schema para migrations versionadas, por exemplo:

```text
internal/database/migrations/
  001_initial_schema.sql
  002_auth_tables.sql
  003_audit_logs.sql
```

## Tabelas atuais

- `allowed_cnpjs`
- `iccids`
- `gb_operations`
- `automation_runs`
- `last_recharge_syncs`
- `recharge_approvals`

## Observacao operacional

Se existirem dados antigos em SQLite, eles precisam ser exportados antes da troca definitiva e importados no PostgreSQL com um script dedicado.
