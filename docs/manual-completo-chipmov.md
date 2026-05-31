# Manual Completo do chip-MOV

## 1. Visao geral

O chip-MOV e um sistema interno para controle de recargas preventivas de ICCIDs/SIM Cards vinculados a CNPJs autorizados.

O objetivo principal e evitar perda, bloqueio ou cancelamento de linhas por falta de recarga dentro do prazo operacional definido.

O sistema faz quatro coisas centrais:

1. Sincroniza assinantes e contratos da API externa.
2. Armazena ICCIDs permitidos em PostgreSQL.
3. Calcula a proxima data preventiva de recarga.
4. Permite simulacao, aprovacao e execucao de recargas.

## 2. Stack atual

Backend:

```text
Go
Gin
JWT
bcrypt
PostgreSQL
WebSocket basico
```

Frontend:

```text
React
TypeScript
Vite
lucide-react
Recharts
CSS responsivo em tema dark
```

Banco:

```text
PostgreSQL
```

Execucao local:

```text
Docker Compose para PostgreSQL
Binario Go ou go run para API
```

## 3. Arquitetura geral

Fluxo simplificado:

```text
Operador -> Frontend React -> API Go -> PostgreSQL
                              |
                              -> API Easy2Use/Tip Brasil
```

Fluxo em producao:

```text
Usuario
  -> HTTPS / Dominio
  -> Reverse proxy
  -> API Go chip-MOV
  -> PostgreSQL
  -> API externa Easy2Use/Tip Brasil
```

## 4. Estrutura de pastas

```text
cmd/
  api/
    main.go

internal/
  app/
    server.go
  auth/
    auth.go
  config/
    config.go
  domain/
    models.go
  easy2use/
    client.go
  storage/
    storage.go

webapp/
  src/
    App.tsx
    index.css
  dist/

web/
  index.html
  assets/

docs/
```

Observacao:

- `webapp/` contem a interface nova em React.
- `web/` contem o painel legado.
- A API Go serve a interface nova na raiz `/`.
- O painel legado fica em `/legacy`.

## 5. Funcionamento do sistema

### 5.1 Sincronizacao de assinantes

Endpoint:

```http
POST /sync/assinantes
```

O sistema consulta a API externa de assinantes, percorre os contratos retornados e salva apenas os ICCIDs vinculados aos CNPJs autorizados em `ALLOWED_CNPJS`.

Ele salva:

- ICCID/SIM Card
- CNPJ
- nome do assinante
- telefone
- numero do contrato
- status do contrato
- plano
- configuracoes padrao de recarga
- data da ultima sincronizacao

Essa etapa nao calcula proxima recarga, porque ainda nao sabe a ultima recarga de cada ICCID.

### 5.2 Sincronizacao de ultima recarga

Endpoint:

```http
POST /sync/ultima-recarga
```

Para cada ICCID salvo, o backend consulta a API externa de ultima recarga.

Quando a API retorna uma data valida, o sistema salva:

```text
last_recharge_at
next_recharge_due_at
```

### 5.3 Calculo da proxima recarga

Formula:

```text
next_recharge_due_at = last_recharge_at + RECHARGE_INTERVAL_MONTHS - RECHARGE_SAFETY_WINDOW_DAYS
```

Valores padrao:

```text
RECHARGE_INTERVAL_MONTHS=11
RECHARGE_SAFETY_WINDOW_DAYS=10
```

Exemplo:

```text
Ultima recarga: 2026-01-01
+ 11 meses:     2026-12-01
- 10 dias:      2026-11-21
```

Resultado:

```text
Proxima recarga preventiva: 2026-11-21
```

### 5.4 Elegibilidade para recarga automatica

Um ICCID entra na rotina quando:

```text
auto_recharge_enabled = true
contract_status = EM USO
next_recharge_due_at <= hoje
```

Contratos cancelados, portados ou nao permitidos nao devem receber recarga automatica.

## 6. Modulos da interface

### 6.1 Dashboard

Mostra:

- total de ICCIDs
- ICCIDs em uso
- bloqueados
- cancelados
- proximas recargas
- grafico por status
- ultimas operacoes
- alertas importantes
- status do WebSocket

Tambem possui acao para sincronizar assinantes.

### 6.2 ICCIDs

Mostra tabela de ICCIDs sincronizados.

Campos principais:

- ICCID
- CNPJ
- assinante
- status
- ultima recarga
- proxima recarga

Recursos:

- busca
- filtro por status
- exportacao CSV

### 6.3 Recarga Manual

Permite informar:

- ICCID
- quantidade em GB
- modo simulacao ou real

Modo seguro:

```text
dry_run = true
```

Esse modo nao chama a API real de recarga.

### 6.4 Aprovacoes

Lista pendencias criadas pela rotina preventiva.

Acoes:

- aprovar
- rejeitar

Quando a aprovacao e executada com recarga real habilitada, o backend chama a API externa e registra a operacao.

### 6.5 Operacoes

Mostra historico tecnico das tentativas de recarga.

Status comuns:

```text
pending
success
failed
blocked
dry_run
```

### 6.6 Relatorios

Mostra dados consolidados de operacoes.

Permite exportacao CSV.

Pode ser expandido futuramente para:

- PDF
- Excel
- relatorio por CNPJ
- relatorio por operadora
- relatorio por periodo
- relatorio por falha

### 6.7 Documentacao interna

Modulo com explicacoes operacionais para novos usuarios.

### 6.8 Configuracoes

Mostra:

- usuario atual
- perfil
- usuarios cadastrados
- sessoes basicas

Pode ser expandido para configuracoes operacionais do sistema.

## 7. Autenticacao e seguranca

### 7.1 Login

Endpoint:

```http
POST /api/auth/login
```

Body:

```json
{
  "email": "admin@chipmov.local",
  "password": "admin12345"
}
```

Retorna:

```json
{
  "access_token": "...",
  "refresh_token": "...",
  "expires_at": "...",
  "user": {
    "email": "admin@chipmov.local",
    "role": "admin"
  }
}
```

### 7.2 JWT

O `access_token` deve ser enviado no header:

```http
Authorization: Bearer TOKEN
```

### 7.3 Refresh token

Endpoint:

```http
POST /api/auth/refresh
```

Body:

```json
{
  "refresh_token": "TOKEN"
}
```

O refresh token e armazenado no banco como hash.

### 7.4 Logout

Endpoint:

```http
POST /api/auth/logout
```

Revoga o refresh token informado.

### 7.5 Perfis

Admin:

- acesso total
- gerencia usuarios
- aprova operacoes
- consulta relatorios
- executa recargas

Supervisor:

- aprova recargas
- consulta relatorios
- gerencia operadores

Operador:

- consulta ICCIDs
- executa recarga manual
- consulta operacoes

Visualizacao:

- somente leitura

### 7.6 Compatibilidade com chave interna

Alguns endpoints ainda aceitam:

```http
x-api-key: ADMIN_KEY
```

Uso recomendado:

- n8n
- cron
- automacoes internas

Para usuarios humanos, usar login JWT.

## 8. Variaveis ambiente

Exemplo:

```env
APP_ADDR=:8080
GIN_MODE=release

DATABASE_URL=postgres://chipmov:SENHA_FORTE@postgres:5432/chipmov?sslmode=disable

JWT_SECRET=secret_grande_e_aleatorio
BOOTSTRAP_ADMIN_EMAIL=admin@suaempresa.com
BOOTSTRAP_ADMIN_PASSWORD=senha_forte
BOOTSTRAP_ADMIN_NAME=Administrador

ADMIN_KEY=chave_forte_para_automacao

EASY2USE_BASE_URL=https://mvno.tipbrasil.com.br/api/public
EASY2USE_USER_TOKEN=token_real
ALLOWED_CNPJS=58420964000179,15070244000118

RECHARGE_INTERVAL_MONTHS=11
RECHARGE_SAFETY_WINDOW_DAYS=10
DEFAULT_RECHARGE_QUANTITY=1
PROVIDER_REQUEST_DELAY_MS=1200

ENABLE_DEV_ROUTES=false
ENABLE_REAL_RECHARGE=false
```

Importante:

- nunca commitar `.env`
- trocar `JWT_SECRET`
- trocar `BOOTSTRAP_ADMIN_PASSWORD`
- trocar `ADMIN_KEY`
- manter `ENABLE_REAL_RECHARGE=false` ate finalizar os testes

## 9. Banco de dados

### 9.1 Tabelas principais

```text
allowed_cnpjs
iccids
gb_operations
automation_runs
last_recharge_syncs
recharge_approvals
users
refresh_tokens
audit_logs
```

### 9.2 allowed_cnpjs

Guarda CNPJs autorizados.

### 9.3 iccids

Guarda ICCIDs sincronizados e calculos de recarga.

Campos importantes:

```text
sim_card
cnpj
contract_status
last_recharge_at
next_recharge_due_at
auto_recharge_enabled
```

### 9.4 gb_operations

Historico de recargas manuais, automaticas e aprovadas.

### 9.5 recharge_approvals

Pendencias de aprovacao.

### 9.6 users

Usuarios do sistema.

### 9.7 refresh_tokens

Tokens de sessao persistentes e revogaveis.

### 9.8 audit_logs

Auditoria de login, usuarios e acoes sensiveis.

## 10. API principal

### Saude

```http
GET /health
```

### Autenticacao

```http
POST /api/auth/login
POST /api/auth/refresh
POST /api/auth/logout
GET  /api/me
```

### Dashboard

```http
GET /api/dashboard/summary
```

### ICCIDs

```http
GET /iccids
GET /iccids/summary
```

### Sincronizacao

```http
POST /sync/assinantes
POST /sync/ultima-recarga
```

### Recarga manual

```http
POST /iccids/{iccid}/saldo
```

Body:

```json
{
  "quantity": 1,
  "dry_run": true
}
```

### Automacao

```http
POST /automation/check-recharges
GET  /automation/next-run
```

Criar pendencias:

```json
{
  "create_approvals": true
}
```

Simular:

```json
{
  "dry_run": true
}
```

### Aprovacoes

```http
GET  /recharge-approvals?status=pending
POST /recharge-approvals/{id}/approve
POST /recharge-approvals/{id}/reject
```

### Operacoes

```http
GET /operacoes
```

### Usuarios

```http
GET  /api/users
POST /api/users
PUT  /api/users/{id}
```

### Auditoria

```http
GET /api/audit-logs
```

## 11. Como rodar localmente

### 11.1 Subir PostgreSQL

```bash
docker compose up -d postgres
```

Confirmar:

```bash
docker compose ps
```

### 11.2 Rodar com Git Bash

```bash
APP_ADDR=127.0.0.1:8090 GIN_MODE=release ./dist/chipmov-api-current.exe
```

### 11.3 Rodar com PowerShell

```powershell
$env:APP_ADDR="127.0.0.1:8090"
$env:GIN_MODE="release"
.\dist\chipmov-api-current.exe
```

### 11.4 Acessar

```text
http://127.0.0.1:8090
```

### 11.5 Login inicial

```text
Email: admin@chipmov.local
Senha: admin12345
```

## 12. Fluxo operacional recomendado

Primeiro acesso:

1. Subir banco.
2. Subir API.
3. Fazer login.
4. Sincronizar assinantes.
5. Conferir total de ICCIDs.
6. Sincronizar ultima recarga.
7. Conferir proximas recargas.
8. Rodar simulacao de rotina.
9. Criar pendencias.
10. Aprovar somente depois de conferir.

## 13. Como colocar em producao

### 13.1 Infraestrutura recomendada

```text
VPS Linux Ubuntu
Docker Engine
Docker Compose
PostgreSQL com volume persistente
Caddy ou Nginx para HTTPS
Backup automatico
```

### 13.2 Preparar servidor

```bash
sudo apt update
sudo apt install docker.io docker-compose-plugin
sudo systemctl enable docker
sudo systemctl start docker
```

### 13.3 Copiar projeto

```bash
git clone SEU_REPOSITORIO chipmov
cd chipmov
```

### 13.4 Build

Frontend:

```bash
cd webapp
npm install
npm run build
cd ..
```

Backend:

```bash
go build -p 1 -o dist/chipmov-api ./cmd/api
```

### 13.5 Rodar API

```bash
APP_ADDR=0.0.0.0:8080 GIN_MODE=release ./dist/chipmov-api
```

### 13.6 Usar systemd

Arquivo:

```text
/etc/systemd/system/chipmov.service
```

Exemplo:

```ini
[Unit]
Description=chip-MOV API
After=network.target

[Service]
WorkingDirectory=/opt/chipmov
EnvironmentFile=/opt/chipmov/.env
ExecStart=/opt/chipmov/dist/chipmov-api
Restart=always
RestartSec=5

[Install]
WantedBy=multi-user.target
```

Ativar:

```bash
sudo systemctl daemon-reload
sudo systemctl enable chipmov
sudo systemctl start chipmov
sudo systemctl status chipmov
```

### 13.7 Reverse proxy com Caddy

Arquivo:

```text
/etc/caddy/Caddyfile
```

Exemplo:

```text
chipmov.suaempresa.com {
  reverse_proxy 127.0.0.1:8080
}
```

Caddy emite HTTPS automaticamente.

### 13.8 Reverse proxy com Nginx

Exemplo:

```nginx
server {
    listen 80;
    server_name chipmov.suaempresa.com;

    location / {
        proxy_pass http://127.0.0.1:8080;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
    }
}
```

Depois usar Certbot para HTTPS.

## 14. Ativacao segura da recarga real

Antes de producao:

```env
ENABLE_REAL_RECHARGE=false
```

Validar:

1. Login.
2. Sincronizacao de assinantes.
3. Sincronizacao de ultima recarga.
4. Dashboard.
5. Simulacao manual.
6. Criacao de pendencias.
7. Aprovar fluxo em ambiente controlado.

Somente depois:

```env
ENABLE_REAL_RECHARGE=true
```

Reiniciar API:

```bash
sudo systemctl restart chipmov
```

## 15. Automacao em producao

### 15.1 Modo recomendado

Criar pendencias, nao recarregar direto:

```http
POST /automation/check-recharges
```

Body:

```json
{
  "create_approvals": true
}
```

Header:

```http
x-api-key: ADMIN_KEY
```

### 15.2 Rotina diaria

```text
Todo dia 08:00 -> criar pendencias
Supervisor/Admin -> revisar e aprovar
```

### 15.3 Rotinas de sincronizacao

Sugestao:

```text
Diario: checar recargas preventivas
Semanal: sincronizar assinantes
Semanal ou mensal: sincronizar ultima recarga
```

## 16. Backup

### 16.1 Backup manual

```bash
docker compose exec postgres pg_dump -U chipmov chipmov > backup-chipmov.sql
```

### 16.2 Backup com data

```bash
docker compose exec postgres pg_dump -U chipmov chipmov > backup-$(date +%F).sql
```

### 16.3 Restaurar backup

```bash
cat backup-chipmov.sql | docker compose exec -T postgres psql -U chipmov -d chipmov
```

## 17. Monitoramento

Verificar saude:

```http
GET /health
```

Logs do servico:

```bash
sudo journalctl -u chipmov -f
```

Docker:

```bash
docker compose ps
docker compose logs -f postgres
```

## 18. Problemas comuns

### 18.1 Dashboard com zero ICCIDs

Causa:

```text
Banco vazio ou assinantes ainda nao sincronizados.
```

Solucao:

```text
Clicar em Sincronizar assinantes.
```

Ou:

```bash
POST /sync/assinantes
```

### 18.2 Proxima recarga aparece vazia

Causa:

```text
Ainda nao foi feita a sincronizacao da ultima recarga.
```

Solucao:

```bash
POST /sync/ultima-recarga
```

### 18.3 WebSocket offline

Possiveis causas:

- origem nao permitida
- API em porta diferente
- proxy sem suporte a upgrade

No Nginx, configurar:

```nginx
proxy_set_header Upgrade $http_upgrade;
proxy_set_header Connection "upgrade";
```

### 18.4 Login nao funciona

Verificar:

- `BOOTSTRAP_ADMIN_EMAIL`
- `BOOTSTRAP_ADMIN_PASSWORD`
- `JWT_SECRET`
- tabela `users`

### 18.5 Porta 8080 ocupada

Usar porta alternativa:

```bash
APP_ADDR=127.0.0.1:8090 ./dist/chipmov-api-current.exe
```

## 19. Boas praticas

- usar HTTPS em producao
- trocar senha padrao
- usar senha forte no PostgreSQL
- nunca commitar `.env`
- manter `ENABLE_DEV_ROUTES=false`
- usar `ENABLE_REAL_RECHARGE=false` durante validacao
- revisar pendencias antes de aprovar
- manter backups diarios
- auditar usuarios
- remover usuarios inativos
- revisar logs de falha

## 20. Melhorias futuras

- migrations versionadas
- workers/fila com Redis
- exports Excel/PDF reais
- dashboard em tempo real com eventos reais
- tela de detalhes do ICCID
- historico completo por ICCID
- permissao mais granular por recurso
- backup automatico configuravel pelo painel
- monitoramento Prometheus/Grafana
- alertas por email ou WhatsApp
- testes automatizados de frontend
- modo multiempresa
- trilha de auditoria mais detalhada

## 21. Checklist de producao

Antes de ativar:

```text
[ ] PostgreSQL com volume persistente
[ ] Backup configurado
[ ] HTTPS ativo
[ ] JWT_SECRET trocado
[ ] ADMIN_KEY trocada
[ ] BOOTSTRAP_ADMIN_PASSWORD trocada
[ ] ENABLE_DEV_ROUTES=false
[ ] ENABLE_REAL_RECHARGE=false
[ ] Login testado
[ ] Sincronizacao de assinantes testada
[ ] Sincronizacao de ultima recarga testada
[ ] Dry-run testado
[ ] Aprovacoes testadas
[ ] Logs revisados
```

Para ativar recarga real:

```text
[ ] Operador validou ICCIDs
[ ] Supervisor validou pendencias
[ ] Token Easy2Use correto
[ ] ENABLE_REAL_RECHARGE=true
[ ] Primeira recarga real acompanhada manualmente
```

