# Seguranca e Regras de Bloqueio

## Regra mais importante

O sistema so pode adicionar GB em ICCIDs vinculados aos 2 CNPJs autorizados.

Mesmo que alguem informe um ICCID valido da plataforma, o backend deve bloquear se o CNPJ nao estiver na lista permitida.

## Token

O token apareceu em print durante testes. Tratar como exposto.

Antes de producao:

- Gerar novo token
- Remover token antigo
- Salvar token apenas no `.env`
- Nao commitar `.env`
- Nao imprimir token em logs

## Arquivo .env esperado

```text
EASY2USE_BASE_URL=https://easy2use.com.br/mvno/api/public
EASY2USE_USER_TOKEN=token_aqui
ALLOWED_CNPJS=00000000000100,11111111111100
DATABASE_URL=postgres://chipmov:chipmov@127.0.0.1:15432/chipmov?sslmode=disable
ADMIN_KEY=chave_para_postman_e_n8n
RECHARGE_INTERVAL_MONTHS=11
RECHARGE_SAFETY_WINDOW_DAYS=10
DEFAULT_RECHARGE_QUANTITY=1
```

Observacao: o endpoint de ultima recarga foi informado usando `https://mvno.tipbrasil.com.br/api/public`. Confirmar a base final e ajustar `EASY2USE_BASE_URL`.

## Validacoes obrigatorias

Antes de chamar `saldo/adicionar`:

```text
ICCID informado
quantity inteiro
quantity >= 1
ICCID encontrado na sincronizacao
CNPJ do ICCID esta em ALLOWED_CNPJS
contrato esta ativo ou EM USO
```

## Validacoes da rotina automatica

Antes de adicionar saldo automaticamente:

```text
auto_recharge_enabled esta ativo
next_recharge_due_at esta vencido ou dentro da janela de recarga
nao existe operacao success recente para o mesmo ICCID
nao existe operacao pending para o mesmo ICCID
```

O sistema deve ter modo `dry_run` para testar a rotina sem adicionar saldo real.

## Regra preventiva

Padrao aprovado:

```text
Prazo maximo sem recarga: 11 meses
Recarregar: 10 dias antes
Quantidade: 1 GB
```

O backend deve calcular e persistir:

```text
next_recharge_due_at = last_recharge_at + 11 meses - 10 dias
```

## Logs

Registrar:

- ICCID
- CNPJ
- quantidade adicionada
- horario
- status interno
- HTTP status da Easy2Use
- `codigo_status_tip`
- `msg_usuario`

Nao registrar:

- token completo
- headers sensiveis
- URLs completas contendo `user_token`

## Bloqueios

O sistema deve retornar erro e nao chamar a Easy2Use quando:

- ICCID nao existe na base local
- ICCID pertence a CNPJ nao autorizado
- CNPJ esta vazio
- quantidade e menor que 1
- quantidade tem decimal
- contrato esta cancelado

## Operacao segura no MVP

No primeiro MVP, evitar endpoint publico aberto.

Opcoes simples:

- Rodar localmente e usar Postman
- Proteger com uma chave interna simples via header
- Liberar apenas em rede local/VPN

Exemplo:

```text
X-Admin-Key: chave_interna
```

O n8n deve usar essa chave no header ao chamar:

```text
POST /automation/check-recharges
```
