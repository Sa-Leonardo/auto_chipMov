# Stack do Projeto

## Escopo real

O sistema deve adicionar saldo de dados em GB para ICCIDs/SIM Cards vinculados a 2 CNPJs conhecidos, de forma preventiva, verificando a data da ultima recarga para evitar que o numero seja perdido por falta de recarga dentro do prazo necessario.

Fora do escopo inicial:

- Gerar valor financeiro
- Criar cobranca
- Controlar pagamento
- Criar marketplace de recargas
- Multiempresa dinamico
- Painel complexo
- Regras complexas de billing

## Stack recomendada

Para fazer da forma mais simples possivel:

```text
Backend: Go
HTTP router: Gin
Banco: PostgreSQL
Jobs: processamento direto no inicio
Agendamento: n8n chamando endpoint do backend
Config: arquivo .env
Testes: go test + Postman
Deploy: binario Go ou Docker simples
```

## Por que essa stack

### Go

Go e uma boa escolha porque gera um binario simples, tem HTTP nativo forte, e facilita criar uma automacao pequena sem muita dependencia.

### Gin

Gin e simples, popular no ecossistema Go e agiliza a criacao dos endpoints REST do MVP. Para este projeto, ele atende bem porque teremos poucos endpoints, validacoes diretas e respostas JSON.

### PostgreSQL

PostgreSQL foi adotado para preparar o chip-MOV para uso corporativo com:

- multiplos usuarios simultaneos
- auditoria de acoes sensiveis
- relatorios com filtros por status, CNPJ, data e operadora
- indices para consultas de proxima recarga
- evolucao futura para filas, workers e processamento concorrente

### Sem fila no inicio

Como o objetivo e simples, o primeiro MVP pode processar uma rotina por vez diretamente na API, acionada pelo n8n em intervalo configurado.

Adicionar fila depois se houver:

- Muitas adicoes em lote
- Erros por rate limit
- Necessidade de retry automatico
- Processamento agendado

## Papel do n8n

O n8n deve ser responsavel apenas por disparar rotinas do backend. A decisao de quais ICCIDs precisam de recarga deve ficar no backend Go.

Forma mais economica recomendada:

```text
1. Backend calcula e salva next_recharge_due_at para cada ICCID.
2. n8n chama uma rotina leve diariamente.
3. Backend consulta apenas o banco local por ICCIDs vencidos ou perto do vencimento.
4. Easy2Use so e chamada quando realmente houver ICCID para recarregar ou quando houver sincronizacao planejada.
```

Chamada diaria leve:

```text
Cron n8n diario -> POST /automation/check-recharges
```

Alternativa ainda mais economica:

```text
n8n chama GET /automation/next-run
n8n agenda/wait ate a proxima data necessaria
n8n chama POST /automation/check-recharges nessa data
```

Mesmo nessa alternativa, manter uma checagem diaria simples e mais resiliente contra falhas, servidor desligado ou mudancas manuais.

## Stack MVP

```text
Go + Gin
PostgreSQL
Postman
.env
n8n
go test
```

## Stack futura, se crescer

```text
Go + Gin
PostgreSQL
Redis + Asynq
React + Vite
Docker Compose
```
