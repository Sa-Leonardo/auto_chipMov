# Modelo de Dados Simplificado

## Objetivo

Guardar apenas o necessario para validar ICCIDs dos 2 CNPJs permitidos, controlar a data da ultima recarga e auditar as adicoes de saldo.

## Tabelas

### allowed_cnpjs

Lista dos CNPJs autorizados.

```text
id integer primary key
cnpj text not null unique
name text
active boolean not null default true
created_at datetime not null
```

### iccids

ICCIDs/SIM Cards encontrados na listagem de assinantes.

```text
id integer primary key
cnpj text not null
subscriber_name text
sim_card text not null unique
phone_number text
contract_number text
contract_status text
plan_name text
last_recharge_at datetime
next_recharge_due_at datetime
default_quantity integer not null default 1
recharge_interval_months integer not null default 11
safety_window_days integer not null default 10
auto_recharge_enabled boolean not null default true
last_sync_at datetime not null
created_at datetime not null
updated_at datetime not null
```

Campos vindos da documentacao:

- `cpf_cnpj`
- `nome`
- `contratos[].sim_card`
- `contratos[].linha_contrato`
- `contratos[].numero_contrato`
- `contratos[].status`
- `contratos[].plano`

### gb_operations

Historico de cada tentativa de adicionar saldo, manual ou automatica.

```text
id integer primary key
sim_card text not null
cnpj text
quantity integer not null
status text not null
trigger_type text not null
easy2use_status_code integer
easy2use_user_message text
request_payload text
response_payload text
error_message text
created_at datetime not null
finished_at datetime
```

Valores de `trigger_type`:

```text
manual
automation
```

Status internos:

```text
pending
success
failed
blocked
```

Uso dos status:

- `pending`: operacao criada e ainda nao finalizada
- `success`: Easy2Use retornou HTTP 200 e resposta aceita
- `failed`: erro tecnico ou erro retornado pela Easy2Use
- `blocked`: ICCID nao pertence aos CNPJs permitidos ou quantidade invalida

### automation_runs

Historico de cada execucao disparada pelo n8n.

```text
id integer primary key
started_at datetime not null
finished_at datetime
status text not null
checked_count integer not null default 0
recharged_count integer not null default 0
skipped_count integer not null default 0
failed_count integer not null default 0
summary text
```

Status:

```text
running
success
failed
partial
```

### last_recharge_syncs

Historico das sincronizacoes de ultima recarga da Easy2Use/Tip Brasil.

```text
id integer primary key
started_at datetime not null
finished_at datetime
status text not null
items_found integer not null default 0
items_updated integer not null default 0
error_message text
```

## Regra principal

Antes de adicionar saldo, o sistema deve confirmar:

```text
sim_card existe na tabela iccids
cnpj do sim_card esta em allowed_cnpjs
quantity e inteiro
quantity >= 1
quantity respeita limite da operadora/plano quando identificavel
quando automatico, next_recharge_due_at indica necessidade de recarga
```

## Regra de datas

Padrao inicial:

```text
recharge_interval_months = 11
safety_window_days = 10
default_quantity = 1
```

Calculo:

```text
next_recharge_due_at = last_recharge_at + 11 meses - 10 dias
```

Depois de uma recarga com sucesso:

```text
last_recharge_at = data/hora da recarga bem-sucedida
next_recharge_due_at = last_recharge_at + 11 meses - 10 dias
```

## Banco de dados

O sistema usa PostgreSQL para suportar uma evolucao corporativa com multiplos usuarios, auditoria, relatorios e operacoes concorrentes.

```text
PostgreSQL
```
