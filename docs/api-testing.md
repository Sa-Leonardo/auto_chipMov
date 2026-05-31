# Plano de Testes da API Easy2Use

## Fonte

Documentacao informada:

```text
https://easy2use.com.br/mvno/api-provedores-integradores/
```

Endpoints fornecidos:

```text
GET  /assinantes/listar
GET  /simcard/{numero_simcard}/ultima-recarga
POST /simcard/{numero_simcard}/saldo/adicionar
POST /simcard/{telefone_com_ddd}/saldo/adicionar
```

## Autenticacao

A documentacao mostra `user_token` na query string:

```text
?user_token={token}
```

Exemplo:

```text
https://easy2use.com.br/mvno/api/public/assinantes/listar?user_token={token}
```

Se o header `Authorization: Bearer <token>` tambem funcionar, ele pode ser usado internamente. Ainda assim, para seguir a documentacao, o MVP deve suportar `user_token`.

## Teste 1: listar assinantes

Request:

```text
GET https://easy2use.com.br/mvno/api/public/assinantes/listar?user_token={token}
Content-Type: application/json
```

Validar:

- HTTP 200
- `codigo_status_tip` igual a 0
- Campo `results` presente
- Cada assinante tem `cpf_cnpj`
- Contratos possuem `sim_card`, `linha_contrato`, `status` e possivelmente `plano`

## Teste 2: filtrar os 2 CNPJs

Depois de listar assinantes:

1. Normalizar `cpf_cnpj` deixando apenas numeros.
2. Comparar com os 2 CNPJs permitidos.
3. Guardar apenas contratos desses CNPJs.
4. Ignorar contratos cancelados, salvo se for necessario auditar.

Status recomendado para operar:

```text
EM USO
```

## Teste 3: adicionar saldo por ICCID

Request:

```text
POST https://easy2use.com.br/mvno/api/public/simcard/{numero_simcard}/saldo/adicionar?user_token={token}
Content-Type: application/json
```

Body:

```json
{
  "quantity": 5
}
```

Validar:

- `quantity` inteiro
- `quantity >= 1`
- ICCID pertence a um dos 2 CNPJs permitidos
- Contrato nao esta cancelado

Resposta esperada:

```json
{
  "msg_usuario": "string com a menssagem retornada da Americanet",
  "codigo_status_tip": 0,
  "americanet": {}
}
```

## Teste 4: validar controle de ultima recarga

Endpoint informado:

```text
GET https://mvno.tipbrasil.com.br/api/public/simcard/{numero_simcard}/ultima-recarga?user_token={token}
```

Resposta esperada:

```json
{
  "ultima_recarga": "2025-09-01",
  "codigo_status_tip": "0"
}
```

O MVP deve consultar esse endpoint para preencher `last_recharge_at` e tambem guardar a data no banco para nao depender de chamada externa diaria.

Validar:

- Ao adicionar saldo com sucesso, salvar `last_recharge_at`.
- Ao sincronizar ultima recarga da Easy2Use/Tip Brasil, atualizar `last_recharge_at`.
- Calcular `next_recharge_due_at` como `last_recharge_at + 11 meses - 10 dias`.
- Ao rodar a rotina, nao recarregar ICCID com recarga recente.
- Ao rodar a rotina, recarregar ICCID dentro da janela preventiva.
- Ao falhar uma recarga, nao atualizar `last_recharge_at`.

Request de teste:

```text
GET {{base_url}}/simcard/{{sim_card_teste}}/ultima-recarga?user_token={{user_token}}
```

Observacao: confirmar se `base_url` correto sera `https://easy2use.com.br/mvno/api/public` ou `https://mvno.tipbrasil.com.br/api/public`.

## Teste 5: adicionar saldo por telefone

Usar apenas se o fluxo por ICCID nao for suficiente.

Request:

```text
POST https://easy2use.com.br/mvno/api/public/simcard/{telefone_com_ddd}/saldo/adicionar?user_token={token}
```

Preferencia do sistema:

```text
ICCID primeiro
telefone apenas como alternativa
```

## Variaveis Postman

```text
base_url=https://easy2use.com.br/mvno/api/public
user_token=<TOKEN>
cnpj_1=<CNPJ_1>
cnpj_2=<CNPJ_2>
sim_card_teste=<ICCID_TESTE>
telefone_teste=<DDD_NUMERO_TESTE>
quantity=1
automation_key=<CHAVE_INTERNA>
```

## Teste 6: disparar rotina como n8n

Request:

```text
POST http://localhost:8080/automation/check-recharges
X-Admin-Key: {{automation_key}}
Content-Type: application/json
```

Body opcional:

```json
{
  "dry_run": true
}
```

No modo `dry_run`, o backend deve informar quais ICCIDs seriam recarregados, sem chamar a Easy2Use.

## Teste 7: consultar proxima execucao util

Request:

```text
GET http://localhost:8080/automation/next-run
X-Admin-Key: {{automation_key}}
```

Resposta esperada:

```json
{
  "next_recharge_due_at": "2026-11-21",
  "iccids_due_count": 0,
  "next_iccid_count": 1
}
```

Esse endpoint ajuda o n8n a saber quando havera proxima necessidade real.

## Checklist antes de gerar codigo

- Confirmar os 2 CNPJs autorizados
- Confirmar token valido
- Confirmar se o token e unico para os 2 CNPJs
- Confirmar ICCID de teste
- Confirmar quantidade segura para teste, recomendado 1 GB
- Confirmar se adicionar saldo gera custo real
- Confirmar limite por operadora/plano
- Confirmar se status `EM USO` e o unico operavel
- Confirmar se `codigo_status_tip = 0` sempre significa sucesso
- Prazo maximo sem recarga: 11 meses
- Janela preventiva: 10 dias antes
- Quantidade padrao: 1 GB
- Endpoint de ultima recarga informado: `GET /simcard/{numero_simcard}/ultima-recarga`
