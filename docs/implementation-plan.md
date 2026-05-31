# Plano de Implementacao Simplificado

## Fase 1: validar API no Postman

Objetivo: confirmar que os endpoints funcionam com dados reais.

Testes:

```text
GET /assinantes/listar
POST /simcard/{iccid}/saldo/adicionar
```

Usar quantidade pequena:

```json
{
  "quantity": 1
}
```

Resultado esperado:

- Listagem retorna os assinantes
- Conseguimos filtrar os 2 CNPJs
- Conseguimos identificar ICCIDs nos contratos
- Endpoint de adicionar saldo retorna HTTP 200
- `codigo_status_tip` indica sucesso

## Fase 2: criar backend Go minimo

Entregas:

- Projeto Go
- Configuracao `.env`
- Endpoint `GET /health`
- Cliente Easy2Use
- Endpoint para sincronizar assinantes
- Endpoint para sincronizar data de ultima recarga
- Endpoint para adicionar saldo em ICCID
- Endpoint para rotina automatica chamada pelo n8n

Endpoints internos:

```text
GET  /health
POST /sync/assinantes
POST /sync/ultima-recarga
GET  /iccids
POST /iccids/{iccid}/saldo
POST /automation/check-recharges
GET  /automation/next-run
GET  /operacoes
```

## Fase 3: persistencia simples

Entregas:

- PostgreSQL
- Tabela de CNPJs permitidos
- Tabela de ICCIDs sincronizados
- Tabela de historico de operacoes
- Campo de ultima recarga por ICCID
- Campo de proxima recarga preventiva por ICCID
- Tabela de execucoes da automacao

## Fase 4: validacoes de seguranca

Entregas:

- Bloquear ICCID fora dos 2 CNPJs
- Bloquear quantidade invalida
- Bloquear contrato cancelado
- Mascarar token em logs
- Registrar todas as tentativas
- Modo `dry_run` para rotina automatica

## Fase 5: operacao manual controlada

No inicio, operar com Postman ou interface simples.

Fluxo:

1. Rodar sync de assinantes.
2. Consultar ICCIDs encontrados.
3. Escolher ICCID.
4. Enviar quantidade de GB.
5. Conferir historico.

## Fase 6: rotina n8n economica

Objetivo: executar verificacao preventiva em intervalo de tempo.

Fluxo:

1. n8n dispara `POST /automation/check-recharges`.
2. Backend consulta PostgreSQL por ICCIDs com `next_recharge_due_at <= hoje`.
3. Backend adiciona 1 GB somente nos ICCIDs elegiveis.
4. Backend registra resumo da execucao.

Primeiro teste:

```json
{
  "dry_run": true
}
```

Depois de validado:

```json
{
  "dry_run": false
}
```

Frequencia recomendada:

```text
POST /automation/check-recharges: diario
POST /sync/assinantes: semanal
POST /sync/ultima-recarga: semanal ou mensal
```

O custo diario sera baixo porque a checagem principal usa PostgreSQL e so chama a Easy2Use quando houver ICCID elegivel.

## Fase 7: automacao em lote, se necessario

Somente depois do MVP validado.

Possibilidades:

- Upload CSV com ICCID e quantidade
- Job agendado
- Retry automatico
- Fila com Redis/Asynq
- Painel web

## Decisoes pendentes

- Quais sao os 2 CNPJs autorizados?
- O token atual atende os 2 CNPJs?
- Qual ICCID sera usado no primeiro teste real?
- Adicionar saldo gera custo real imediato?
- Devemos operar somente ICCIDs com status `EM USO`?
- O limite por operadora pode ser identificado pelo campo `plano`?
- Confirmar se a base URL final sera `easy2use.com.br` ou `mvno.tipbrasil.com.br`
- Confirmar se `codigo_status_tip = "0"` no endpoint de ultima recarga sempre significa sucesso
- Confirmar comportamento quando o ICCID nunca recebeu recarga
