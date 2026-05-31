# Automacao chip-MOV

MVP em Go + Gin para adicionar 1 GB preventivo em ICCIDs vinculados a 2 CNPJs, usando a API Tip Brasil/Easy2Use.

Documentacao completa:

```text
docs/manual-completo-chipmov.md
```

## Configuracao

Copie `.env.example` para `.env` e preencha:

```text
APP_ADDR=:8080
GIN_MODE=release
ADMIN_KEY=troque_esta_chave
EASY2USE_BASE_URL=https://mvno.tipbrasil.com.br/api/public
EASY2USE_USER_TOKEN=token_aqui
ALLOWED_CNPJS=00000000000100,11111111111100
DATABASE_URL=postgres://chipmov:chipmov@127.0.0.1:15432/chipmov?sslmode=disable
RECHARGE_INTERVAL_MONTHS=11
RECHARGE_SAFETY_WINDOW_DAYS=10
DEFAULT_RECHARGE_QUANTITY=1
PROVIDER_REQUEST_DELAY_MS=1200
ENABLE_REAL_RECHARGE=false
ENABLE_DEV_ROUTES=false
```

## Rodar

Suba o PostgreSQL local:

```bash
docker compose up -d postgres
```

### PowerShell

```bash
go run ./cmd/api
```

Ou gerar binario:

```bash
go build -o dist/chipmov-api.exe ./cmd/api
```

Para usar uma porta alternativa no PowerShell:

```powershell
$env:APP_ADDR="127.0.0.1:8090"
$env:GIN_MODE="release"
.\dist\chipmov-api.exe
```

### Git Bash

Para usar uma porta alternativa no Git Bash:

```bash
APP_ADDR=127.0.0.1:8090 GIN_MODE=release ./dist/chipmov-api.exe
```

## Painel web

Com o servidor rodando, acesse a interface nova:

```text
http://127.0.0.1:8080
```

Se a porta `8080` estiver ocupada por outro processo, use a porta alternativa configurada em `APP_ADDR`, por exemplo:

```text
http://127.0.0.1:8090
```

Login inicial de desenvolvimento:

```text
Email: admin@chipmov.local
Senha: admin12345
```

O painel legado continua disponivel em:

```text
http://127.0.0.1:8080/legacy
```

O painel permite:

- visualizar resumo dos ICCIDs
- sincronizar assinantes
- sincronizar ultima recarga
- simular rotina
- criar pendencias de aprovacao
- aprovar ou rejeitar pendencias
- consultar operacoes recentes

## Endpoints

Todos os endpoints abaixo, exceto `/health`, exigem:

```text
X-Admin-Key: sua_chave
```

Tambem e aceito:

```text
x-api-key: sua_chave
```

Use o mesmo valor configurado em `ADMIN_KEY` no `.env`.

### Saude

```text
GET /health
```

### Sincronizar assinantes

Busca assinantes na API externa, filtra somente os CNPJs permitidos e salva os ICCIDs.

```text
POST /sync/assinantes
```

### Sincronizar ultima recarga

Consulta a ultima recarga de cada ICCID salvo e calcula `next_recharge_due_at`.

```text
POST /sync/ultima-recarga
```

Por causa do rate limit da API externa, essa rotina espera `PROVIDER_REQUEST_DELAY_MS` entre consultas. O padrao recomendado e `1200`, que fica abaixo de 60 chamadas por minuto.

### Listar ICCIDs

```text
GET /iccids
```

### Adicionar saldo manual

```text
POST /iccids/{iccid}/saldo
```

Body:

```json
{
  "quantity": 1,
  "dry_run": true
}
```

No painel web, use a secao "Adicionar saldo manual" para informar ICCID, quantidade de GB e escolher se quer simular ou executar real.

Quando uma recarga manual real retorna sucesso, o sistema atualiza:

```text
last_recharge_at = data/hora atual
next_recharge_due_at = last_recharge_at + 11 meses - 10 dias
```

Isso impede que a rotina automatica recarregue de novo antes da proxima janela.

Para chamar a API real de recarga, configure no `.env`:

```text
ENABLE_REAL_RECHARGE=true
```

Depois reinicie o servidor e envie:

```json
{
  "quantity": 1,
  "dry_run": false
}
```

Atencao: a API externa pode aplicar uma franquia diferente da quantidade enviada, conforme regra da operadora/plano. Em teste real, foi observado que `quantity: 1` pode resultar em credito maior no provedor.

### Rotina para n8n

Teste seguro:

```text
POST /automation/check-recharges
```

Body:

```json
{
  "dry_run": true
}
```

Criar pendencias para aprovacao manual:

```json
{
  "create_approvals": true
}
```

Execucao real:

```json
{
  "dry_run": false
}
```

### Proxima execucao util

```text
GET /automation/next-run
```

Esse endpoint considera apenas ICCIDs acionaveis:

```text
auto_recharge_enabled = true
contract_status = EM USO
next_recharge_due_at >= hoje
```

Resposta inclui `next_recharge_iccids`, com os ICCIDs e CNPJs que pertencem a proxima data de recarga.

## Respostas da automacao

`POST /automation/check-recharges` retorna, em `results`, o ICCID, CNPJ, nome do assinante e dados da operacao para cada item avaliado ou recarregado.

## Aprovacao manual

Listar pendencias:

```text
GET /recharge-approvals?status=pending
```

Aprovar e executar uma pendencia:

```text
POST /recharge-approvals/{id}/approve
```

Rejeitar uma pendencia:

```text
POST /recharge-approvals/{id}/reject
```

Mesmo com aprovacao manual, a chamada real ao provedor so acontece quando:

```text
ENABLE_REAL_RECHARGE=true
```

estiver configurado no `.env`.

## Teste de vencimento local

Para testar o fluxo de aprovacao sem esperar a data real, habilite rotas dev:

```text
ENABLE_DEV_ROUTES=true
```

Reinicie o servidor e rode:

```text
POST /dev/iccids/{iccid}/force-due
```

Essa rota apenas altera o PostgreSQL local:

```text
next_recharge_due_at = hoje
```

Ela nao chama a API externa e nao faz recarga real.

Se o status local estiver diferente do sistema original, primeiro rode:

```text
POST /sync/assinantes
```

Para teste local, tambem e possivel forcar status no PostgreSQL:

```text
POST /dev/iccids/{iccid}/force-status
```

Body:

```json
{
  "status": "EM USO"
}
```

Essa rota tambem nao chama a API externa.

Depois crie a pendencia:

```json
POST /automation/check-recharges
{
  "create_approvals": true
}
```

E liste:

```text
GET /recharge-approvals?status=pending
```

## Regra preventiva

```text
next_recharge_due_at = ultima_recarga + 11 meses - 10 dias
```

Quando `hoje >= next_recharge_due_at`, a rotina automatica adiciona 1 GB.
