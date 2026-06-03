import { useEffect, useMemo, useState } from 'react'
import type { FormEvent, ReactNode } from 'react'
import {
  Activity, AlertTriangle, BarChart3, BookOpen, CheckCircle2, Database, FileDown,
  Gauge, KeyRound, LayoutDashboard, ListChecks, Loader2, LogOut, RadioTower,
  RefreshCw, Search, Settings, ShieldCheck, Signal, Smartphone, UserPlus, XCircle,
} from 'lucide-react'
import type { LucideIcon } from 'lucide-react'
import {
  Area, AreaChart, Bar, BarChart, CartesianGrid, ResponsiveContainer, Tooltip, XAxis, YAxis,
} from 'recharts'
import './index.css'

type Role = 'admin' | 'supervisor' | 'operator' | 'viewer'
type User = { id?: number; name: string; email: string; role: Role; active?: boolean }
type ICCID = {
  id: number; cnpj: string; subscriber_name: string; sim_card: string; phone_number: string;
  contract_status: string; plan_name: string; last_recharge_at?: string; next_recharge_due_at?: string;
}
type Operation = { id: number; sim_card: string; cnpj: string; quantity: number; status: string; trigger_type: string; created_at: string; error_message?: string; easy2use_user_message?: string }
type Approval = { id: number; sim_card: string; cnpj: string; subscriber_name: string; status: string; quantity: number; reason: string; created_at: string }
type NextRun = { today: string; next_recharge_due_at?: string | null; iccids_due_count: number; actionable_iccids_count: number; next_recharge_iccids: ICCID[] }
type UserForm = { name: string; email: string; password: string; role: Role; active: boolean }
type AuditLog = { id: number; user_id?: number; action: string; resource: string; resource_id?: string; metadata?: string; created_at: string }
type AttendanceItem = { subscriber_name: string; cnpj: string; document: string; sim_card: string; phone_number: string; contract_number: string; contract_status: string; plan_name: string; recharge_allowed: boolean }
type Availability = { available: boolean; message?: string; error?: string; value?: string | number; unit?: string; real_time?: boolean }
type LastRecharge = { ultima_recarga?: string; LastRecharge?: string; error?: string }
type ConsumptionSummary = { contracts: number; internet_total: number; upload_total: number; download_total: number; voice_seconds: number; voice_minutes: number; sms_count: number }
type UsageHistory = { available: boolean; message?: string; error?: string; period: string; summary?: ConsumptionSummary; results?: object[] }
type AttendanceDetail = { item: AttendanceItem; last_recharge: LastRecharge; balance: Availability; usage_history: UsageHistory; operations: Operation[]; audit: AuditLog[] }
type DashboardAlert = { level: string; message: string }
type DashboardSummary = { total_iccids: number; status_counts: Record<string, number>; pending_approvals: number; due_recharges: number; actionable_iccids_count: number; recent_operations: Operation[]; important_alerts: DashboardAlert[] }

const API = import.meta.env.VITE_API_BASE_URL || ''
const nav = [
  ['dashboard', 'Dashboard', LayoutDashboard],
  ['attendance', 'Atendimento ICCID', Search],
  ['iccids', 'ICCIDs', Smartphone],
  ['manual', 'Recarga Manual', RadioTower],
  ['approvals', 'Aprovacoes', ListChecks],
  ['operations', 'Operacoes', Activity],
  ['reports', 'Relatorios', BarChart3],
  ['docs', 'Documentacao', BookOpen],
  ['settings', 'Configuracoes', Settings],
] as const

function request<T>(path: string, options: RequestInit = {}): Promise<T> {
  const token = localStorage.getItem('chipmov.accessToken')
  return fetch(`${API}${path}`, {
    ...options,
    headers: {
      'Content-Type': 'application/json',
      ...(token ? { Authorization: `Bearer ${token}` } : {}),
      ...options.headers,
    },
  }).then(async (res) => {
    const text = await res.text()
    const data = text ? JSON.parse(text) : {}
    if (!res.ok) throw new Error(data.error || res.statusText)
    return data
  })
}

function App() {
  const [user, setUser] = useState<User | null>(null)
  const [loading, setLoading] = useState(true)
  const [page, setPage] = useState('dashboard')
  const [toast, setToast] = useState('')

  useEffect(() => {
    request<{ user: User }>('/api/me')
      .then((r) => setUser(r.user))
      .catch(() => setUser(null))
      .finally(() => setLoading(false))
  }, [])

  if (loading) return <FullLoader />
  if (!user) return <Login onLogin={setUser} />

  return (
    <div className="app-shell">
      <aside className="sidebar">
        <div className="brand"><div className="brand-mark">CM</div><div><strong>chip-MOV</strong><span>Recargas preventivas</span></div></div>
        <nav>{nav.map(([id, label, Icon]) => <button key={id} className={page === id ? 'active' : ''} onClick={() => setPage(id)}><Icon size={18} />{label}</button>)}</nav>
      </aside>
      <main className="main">
        <header className="topbar">
          <div><span className="eyebrow">Operacao interna</span><h1>{nav.find(([id]) => id === page)?.[1]}</h1></div>
          <div className="userbox"><ShieldCheck size={18} /><span>{user.name}</span><small>{roleLabel(user.role)}</small><button title="Sair" onClick={() => logout(setUser)}><LogOut size={16} /></button></div>
        </header>
        {toast && <div className="toast">{toast}</div>}
        {page === 'dashboard' && <Dashboard setToast={setToast} />}
        {page === 'attendance' && <AttendanceICCID setToast={setToast} />}
        {page === 'iccids' && <ICCIDs />}
        {page === 'manual' && <ManualRecharge setToast={setToast} />}
        {page === 'approvals' && <Approvals setToast={setToast} />}
        {page === 'operations' && <Operations />}
        {page === 'reports' && <Reports />}
        {page === 'docs' && <Docs />}
        {page === 'settings' && <SettingsPage user={user} setToast={setToast} />}
      </main>
    </div>
  )
}

function Login({ onLogin }: { onLogin: (u: User) => void }) {
  const [email, setEmail] = useState('admin@chipmov.local')
  const [password, setPassword] = useState('admin12345')
  const [error, setError] = useState('')
  const [busy, setBusy] = useState(false)
  async function submit(e: FormEvent) {
    e.preventDefault(); setBusy(true); setError('')
    try {
      const data = await request<{ access_token: string; refresh_token: string; user: User }>('/api/auth/login', {
        method: 'POST', body: JSON.stringify({ email, password }),
      })
      localStorage.setItem('chipmov.accessToken', data.access_token)
      localStorage.setItem('chipmov.refreshToken', data.refresh_token)
      onLogin(data.user)
    } catch (err) { setError((err as Error).message) } finally { setBusy(false) }
  }
  return <div className="login-screen"><form className="login-card" onSubmit={submit}><div className="brand-mark large">CM</div><h1>chip-MOV</h1><p>Acesso seguro ao sistema de recargas preventivas</p><label>Email<input value={email} onChange={(e) => setEmail(e.target.value)} /></label><label>Senha<input type="password" value={password} onChange={(e) => setPassword(e.target.value)} /></label>{error && <div className="error">{error}</div>}<button disabled={busy}>{busy ? <Loader2 className="spin" size={18} /> : <KeyRound size={18} />} Entrar</button></form></div>
}

function Dashboard({ setToast }: { setToast: (s: string) => void }) {
  const [data, setData] = useState<DashboardSummary | null>(null)
  const [nextRun, setNextRun] = useState<NextRun | null>(null)
  const [live, setLive] = useState('desconectado')
  const load = () => request<DashboardSummary>('/api/dashboard/summary').then(setData)
  const loadNextRun = () => request<NextRun>('/automation/next-run').then(setNextRun)
  const loadAll = () => Promise.all([load(), loadNextRun()])
  const syncSubscribers = async () => {
    setToast('Sincronizando assinantes...')
    const result = await request<{ saved?: number }>('/sync/assinantes', { method: 'POST' })
    await loadAll()
    setToast(`Sincronizacao concluida: ${result.saved || 0} ICCIDs salvos`)
  }
  useEffect(() => { loadAll(); const ws = new WebSocket(`${location.protocol === 'https:' ? 'wss' : 'ws'}://${location.host}/api/ws`); ws.onopen = () => setLive('online'); ws.onclose = () => setLive('offline'); return () => ws.close() }, [])
  const chart = Object.entries(data?.status_counts || {}).map(([name, value]) => ({ name: name || 'Sem status', value }))
  const nextRows = nextRun?.next_recharge_iccids || []
  const alerts = data?.important_alerts || []
  return <section className="stack">
    <div className="actionbar"><div className="tools"><button onClick={() => loadAll().then(() => setToast('Dashboard atualizado'))}><RefreshCw size={16} /> Atualizar</button><button onClick={syncSubscribers}><Database size={16} /> Sincronizar assinantes</button></div><span className={`live ${live}`}>WebSocket {live}</span></div>
    <div className="metrics">
      <Metric icon={Database} label="Total de ICCIDs" value={data?.total_iccids ?? '-'} />
      <Metric icon={Signal} label="Em uso" value={data?.status_counts?.['EM USO'] ?? 0} />
      <Metric icon={AlertTriangle} label="Bloqueados" value={data?.status_counts?.BLOQUEADO ?? 0} />
      <Metric icon={XCircle} label="Cancelados" value={data?.status_counts?.CANCELADO ?? 0} />
      <Metric icon={Gauge} label="Proximas recargas" value={data?.actionable_iccids_count ?? 0} />
    </div>
    <Panel title="ICCIDs da proxima recarga" tools={<span className="panel-note">{nextRun?.next_recharge_due_at ? `Data: ${format(nextRun.next_recharge_due_at)}` : 'Sem proxima data'}</span>}>
      {nextRows.length ? <DataRows rows={nextRows} keys={['sim_card', 'cnpj', 'subscriber_name', 'contract_status', 'next_recharge_due_at']} /> : <Empty text="Nenhum ICCID elegivel encontrado para a proxima recarga." />}
    </Panel>
    <div className="grid two"><Panel title="Status dos ICCIDs"><ResponsiveContainer height={260}><BarChart data={chart}><CartesianGrid strokeDasharray="3 3" /><XAxis dataKey="name" /><YAxis /><Tooltip /><Bar dataKey="value" fill="#a855f7" /></BarChart></ResponsiveContainer></Panel><Panel title="Ultimas Operacoes"><DataRows rows={data?.recent_operations || []} keys={['id', 'sim_card', 'status', 'trigger_type']} /></Panel></div>
    <Panel title="Alertas importantes">{alerts.length ? alerts.map((a, i) => <div className="alert" key={i}><AlertTriangle size={16} />{a.message}</div>) : <Empty text="Nenhum alerta importante." />}</Panel>
  </section>
}

function AttendanceICCID({ setToast }: { setToast: (s: string) => void }) {
  const [q, setQ] = useState('')
  const [searchType, setSearchType] = useState('auto')
  const [results, setResults] = useState<AttendanceItem[]>([])
  const [detail, setDetail] = useState<AttendanceDetail | null>(null)
  const [quantity, setQuantity] = useState(1)
  const [note, setNote] = useState('')
  const [dryRun, setDryRun] = useState(true)
  const [period, setPeriod] = useState(currentMonth())
  const [realTimeBalance, setRealTimeBalance] = useState(false)
  const [error, setError] = useState('')
  const [busy, setBusy] = useState(false)
  const search = async (e?: FormEvent) => {
    e?.preventDefault(); setBusy(true); setError(''); setDetail(null)
    try {
      const data = await request<{ items: AttendanceItem[] }>(`/api/attendance/search?q=${encodeURIComponent(q)}&type=${encodeURIComponent(searchType)}&period=${encodeURIComponent(period)}`)
      setResults(data.items || [])
      setToast(`${data.items?.length || 0} resultado(s) encontrado(s)`)
    } catch (err) { setError((err as Error).message) } finally { setBusy(false) }
  }
  const openDetail = async (simCard: string) => {
    setBusy(true); setError('')
    try {
      const data = await request<AttendanceDetail>(`/api/attendance/iccids/${encodeURIComponent(simCard)}?period=${encodeURIComponent(period)}&real_time=${realTimeBalance ? 'true' : 'false'}`)
      setDetail(data)
      setQuantity(1); setNote(''); setDryRun(true)
    } catch (err) { setError((err as Error).message) } finally { setBusy(false) }
  }
  const recharge = async (e: FormEvent) => {
    e.preventDefault()
    if (!detail) return
    const simCard = detail.item.sim_card
    if (!confirm(`${dryRun ? 'Simular' : 'Executar'} recarga avulsa de ${quantity}GB para ${simCard}?`)) return
    setBusy(true); setError('')
    try {
      await request(`/api/attendance/iccids/${encodeURIComponent(simCard)}/recharge`, { method: 'POST', body: JSON.stringify({ quantity, dry_run: dryRun, note }) })
      setToast('Recarga avulsa processada')
      await openDetail(simCard)
    } catch (err) { const message = (err as Error).message; setError(message); setToast(`Falha na recarga avulsa: ${message}`) } finally { setBusy(false) }
  }
  const item = detail?.item
  const providerAlerts = detail ? [
    detail.balance?.error || detail.balance?.message,
    detail.usage_history?.error || detail.usage_history?.message,
  ].filter(Boolean) : []
  return <section className="stack">
    <Panel title="Busca de cliente ou ICCID" tools={<span className="panel-note">ICCID, CPF/CNPJ ou nome</span>}>
      <form className="form inline-form" onSubmit={search}>
        <label>Tipo<select value={searchType} onChange={e => setSearchType(e.target.value)}><option value="auto">Automatico</option><option value="iccid">ICCID</option><option value="document">CPF/CNPJ</option><option value="name">Nome</option></select></label>
        <label>Pesquisa<input value={q} onChange={e => setQ(e.target.value)} placeholder="Digite ICCID, documento ou nome" required /></label>
        <button disabled={busy}>{busy ? <Loader2 className="spin" size={16} /> : <Search size={16} />} Consultar</button>
      </form>
      {error && <div className="error">{error}</div>}
    </Panel>
    {!item && <Panel title="Resultados"><AttendanceResultCards rows={results} onOpen={openDetail} /></Panel>}
    {item && detail && <div className="attendance-layout">
      <div className="attendance-main">
        <Panel title="Ficha do cliente" tools={<button onClick={() => openDetail(item.sim_card)}><RefreshCw size={16} /> Atualizar</button>}>
          <div className="stack compact">
            {providerAlerts.map((message, index) => <div className="alert slim" key={index}><AlertTriangle size={15} /> {message}</div>)}
            <div className="form inline-form attendance-filters">
              <label>Periodo<input type="month" value={period} onChange={e => setPeriod(e.target.value)} /></label>
              <label className="check"><input type="checkbox" checked={realTimeBalance} onChange={e => setRealTimeBalance(e.target.checked)} /> Saldo em tempo real</label>
            </div>
            <div className="client-card">
              <div>
                <span>Cliente</span>
                <strong>{item.subscriber_name || '-'}</strong>
                <small>{item.document || item.cnpj || '-'}</small>
              </div>
              <div>
                <span>Telefone</span>
                <strong>{item.phone_number || '-'}</strong>
                <small>{item.plan_name || '-'}</small>
              </div>
              <div>
                <span>ICCID</span>
                <strong>{item.sim_card}</strong>
                <small>Contrato {item.contract_number || '-'}</small>
              </div>
            </div>
            <div className="kpi-row">
              <Metric icon={Signal} label="Status" value={item.contract_status || '-'} />
              <Metric icon={Database} label="Ultima recarga" value={detail.last_recharge?.ultima_recarga || detail.last_recharge?.LastRecharge || '-'} />
              <Metric icon={Gauge} label="Saldo atual" value={detail.balance?.available ? `${detail.balance.value} GB` : 'Indisponivel'} />
            </div>
            <ConsumptionSummaryCard usage={detail.usage_history} />
          </div>
        </Panel>
        <Panel title="Recarga avulsa">
          <form className="form" onSubmit={recharge}>
            <div className="recharge-row">
              <label>Quantidade GB<input type="number" min={1} value={quantity} onChange={e => setQuantity(Number(e.target.value))} /></label>
              <label className="check"><input type="checkbox" checked={dryRun} onChange={e => setDryRun(e.target.checked)} /> Simular</label>
            </div>
            <label>Motivo ou observacao<input value={note} onChange={e => setNote(e.target.value)} placeholder="Ex.: atendimento solicitado pelo cliente" /></label>
            {!item.recharge_allowed && <div className="error">Este ICCID nao esta ativo para recarga: {item.contract_status}</div>}
            <button disabled={busy || !item.recharge_allowed}><RadioTower size={16} /> Processar recarga</button>
          </form>
        </Panel>
      </div>
      <aside className="attendance-side">
        <Panel title="Historico de recargas">
          <OperationCards rows={detail.operations || []} />
        </Panel>
      </aside>
      <div className="attendance-audit">
        <Panel title="Auditoria de atendimento">
          <AuditRows rows={detail.audit || []} />
        </Panel>
      </div>
    </div>}
  </section>
}

function AttendanceResultCards({ rows, onOpen }: { rows: AttendanceItem[]; onOpen: (simCard: string) => void }) {
  if (!rows.length) return <Empty text="Nenhum registro encontrado." />
  return <div className="result-cards">{rows.map((row) => <article className="result-card" key={row.sim_card}>
    <div className="result-card-main">
      <div>
        <strong>{row.subscriber_name || 'Cliente sem nome'}</strong>
        <span>{row.document || row.cnpj || '-'}</span>
      </div>
      <button onClick={() => onOpen(row.sim_card)}><Search size={16} /> Abrir</button>
    </div>
    <div className="result-highlight">
      <span>{row.phone_number || '-'}</span>
      <mark className={statusClass(row.contract_status)}>{row.contract_status || '-'}</mark>
    </div>
    <small>ICCID {row.sim_card}</small>
    <small>{row.plan_name || 'Plano nao informado'}</small>
  </article>)}</div>
}

function ConsumptionSummaryCard({ usage }: { usage: UsageHistory }) {
  if (!usage?.available || !usage.summary) return <div className="alert slim"><AlertTriangle size={15} /> {usage?.message || usage?.error || 'Historico de uso indisponivel.'}</div>
  const s = usage.summary
  return <div className="usage-summary">
    <div><span>Periodo</span><strong>{usage.period}</strong></div>
    <div><span>Internet</span><strong>{formatNumber(s.internet_total)} MB</strong></div>
    <div><span>Upload</span><strong>{formatNumber(s.upload_total)} MB</strong></div>
    <div><span>Download</span><strong>{formatNumber(s.download_total)} MB</strong></div>
    <div><span>Voz</span><strong>{s.voice_minutes} min</strong></div>
    <div><span>SMS</span><strong>{s.sms_count}</strong></div>
  </div>
}

function OperationCards({ rows }: { rows: Operation[] }) {
  if (!rows.length) return <Empty text="Nenhuma recarga encontrada para este ICCID." />
  return <div className="operation-list">{rows.map((op) => <article className="operation-card" key={op.id}>
    <div>
      <strong>#{op.id} · {op.quantity} GB</strong>
      <span>{format(op.created_at)}</span>
    </div>
    <mark className={statusClass(op.status)}>{op.status}</mark>
    {(op.easy2use_user_message || op.error_message) && <small>{op.easy2use_user_message || op.error_message}</small>}
  </article>)}</div>
}

function AuditRows({ rows }: { rows: AuditLog[] }) {
  if (!rows.length) return <Empty text="Nenhum evento de auditoria para este atendimento." />
  return <div className="audit-list">{rows.map((row) => <details className="audit-row" key={row.id}>
    <summary>
      <span>#{row.id}</span>
      <span>{format(row.created_at)}</span>
      <mark className={statusClass(row.action)}>{row.action}</mark>
      <button type="button">Ver detalhes</button>
    </summary>
    <pre>{formatAuditDetails(row)}</pre>
  </details>)}</div>
}

function ICCIDs() {
  const [items, setItems] = useState<ICCID[]>([])
  const [q, setQ] = useState(''); const [status, setStatus] = useState('')
  useEffect(() => { request<{ items: ICCID[] }>('/iccids').then((r) => setItems(r.items || [])) }, [])
  const filtered = useMemo(() => items.filter(i => `${i.sim_card} ${i.cnpj} ${i.contract_status} ${i.subscriber_name}`.toLowerCase().includes(q.toLowerCase()) && (!status || i.contract_status === status)), [items, q, status])
  return <Panel title="Tabela completa de ICCIDs" tools={<><SearchBox value={q} onChange={setQ} /><select value={status} onChange={(e) => setStatus(e.target.value)}><option value="">Todos</option><option>EM USO</option><option>BLOQUEADO</option><option>CANCELADO</option><option>PORTOUT</option></select><button onClick={() => exportCSV(filtered, 'iccids.csv')}><FileDown size={16} /> CSV</button></>}><DataRows rows={filtered} keys={['sim_card', 'cnpj', 'subscriber_name', 'contract_status', 'last_recharge_at', 'next_recharge_due_at']} /></Panel>
}

function ManualRecharge({ setToast }: { setToast: (s: string) => void }) {
  const [iccid, setIccid] = useState(''); const [quantity, setQuantity] = useState(1); const [dryRun, setDryRun] = useState(true); const [history, setHistory] = useState<Operation[]>([])
  const [error, setError] = useState('')
  const loadHistory = () => request<{ items: Operation[] }>('/operacoes?limit=20').then(r => setHistory(r.items || []))
  const submit = async (e: FormEvent) => {
    e.preventDefault(); setError('')
    const cleanICCID = iccid.trim()
    if (!confirm(`${dryRun ? 'Simular' : 'Executar'} recarga para ${cleanICCID}?`)) return
    try {
      await request(`/iccids/${encodeURIComponent(cleanICCID)}/saldo`, { method: 'POST', body: JSON.stringify({ quantity, dry_run: dryRun }) })
      setToast('Operacao processada')
    } catch (err) {
      const message = (err as Error).message
      setError(message)
      setToast(`Falha na recarga: ${message}`)
    } finally {
      loadHistory()
    }
  }
  useEffect(() => { loadHistory() }, [])
  return <div className="grid two"><Panel title="Recarga manual"><form className="form" onSubmit={submit}><label>ICCID<input value={iccid} onChange={e => setIccid(e.target.value)} required /></label><label>Quantidade GB<input type="number" min={1} value={quantity} onChange={e => setQuantity(Number(e.target.value))} /></label><label className="check"><input type="checkbox" checked={dryRun} onChange={e => setDryRun(e.target.checked)} /> Simular sem chamar API real</label>{error && <div className="error">{error}</div>}<button><RadioTower size={16} /> Processar</button></form></Panel><Panel title="Historico de recargas manuais"><DataRows rows={history.filter(h => h.trigger_type === 'manual')} keys={['id', 'sim_card', 'quantity', 'status', 'created_at', 'error_message', 'easy2use_user_message']} /></Panel></div>
}

function Approvals({ setToast }: { setToast: (s: string) => void }) {
  const [items, setItems] = useState<Approval[]>([])
  const load = () => request<{ items: Approval[] }>('/recharge-approvals?status=pending').then(r => setItems(r.items || []))
  useEffect(() => { load() }, [])
  const act = async (id: number, action: 'approve' | 'reject') => { await request(`/recharge-approvals/${id}/${action}`, { method: 'POST' }); setToast(`Aprovacao ${action === 'approve' ? 'aprovada' : 'rejeitada'}`); load() }
  return <Panel title="Operacoes pendentes"><DataRows rows={items} keys={['id', 'sim_card', 'cnpj', 'quantity', 'reason']} renderActions={(row: Approval) => <><button onClick={() => act(row.id, 'approve')}><CheckCircle2 size={16} /></button><button className="danger" onClick={() => act(row.id, 'reject')}><XCircle size={16} /></button></>} /></Panel>
}

function Operations() {
  const [items, setItems] = useState<Operation[]>([]); const [q, setQ] = useState('')
  useEffect(() => { request<{ items: Operation[] }>('/operacoes?limit=500').then(r => setItems(r.items || [])) }, [])
  const filtered = items.filter(i => JSON.stringify(i).toLowerCase().includes(q.toLowerCase()))
  return <Panel title="Logs completos do sistema" tools={<SearchBox value={q} onChange={setQ} />}><DataRows rows={filtered} keys={['id', 'sim_card', 'cnpj', 'quantity', 'status', 'trigger_type', 'created_at', 'error_message']} /></Panel>
}

function Reports() {
  const [items, setItems] = useState<Operation[]>([])
  useEffect(() => { request<{ items: Operation[] }>('/operacoes?limit=500').then(r => setItems(r.items || [])) }, [])
  const trend = items.slice(0, 20).reverse().map(i => ({ name: `#${i.id}`, value: i.quantity || 0 }))
  return <section className="stack"><Panel title="Dashboard analitico"><ResponsiveContainer height={260}><AreaChart data={trend}><XAxis dataKey="name" /><YAxis /><Tooltip /><Area dataKey="value" stroke="#38bdf8" fill="#a855f7" fillOpacity={0.2} /></AreaChart></ResponsiveContainer></Panel><Panel title="Relatorios por recarga" tools={<button onClick={() => exportCSV(items, 'relatorio-recargas.csv')}><FileDown size={16} /> CSV</button>}><DataRows rows={items} keys={['created_at', 'cnpj', 'sim_card', 'quantity', 'status', 'trigger_type', 'easy2use_user_message']} /></Panel></section>
}

function Docs() {
  const sections = ['O sistema monitora ICCIDs autorizados por CNPJ e evita perda de linha por falta de recarga.', 'Fluxo: sincronizar assinantes, calcular proxima recarga, criar Aprovacao, executar recarga e auditar.', 'Status: EM USO pode ser recarregado; BLOQUEADO exige analise; CANCELADO e PORTOUT nao devem receber recarga automatica.', 'API: endpoints REST protegidos por JWT e compativeis temporariamente com x-api-key para automacoes.', 'Boas praticas: testar em dry-run, revisar Aprovacoes, manter tokens no .env e acompanhar Operacoes recentes.']
  return <Panel title="Documentacao interna do sistema">{sections.map((s, i) => <div className="doc-row" key={s}><strong>{i + 1}</strong><p>{s}</p></div>)}</Panel>
}

function SettingsPage({ user, setToast }: { user: User; setToast: (s: string) => void }) {
  const [users, setUsers] = useState<User[]>([])
  const [form, setForm] = useState<UserForm>({ name: '', email: '', password: '', role: 'operator', active: true })
  const [error, setError] = useState('')
  const [busy, setBusy] = useState(false)
  const loadUsers = () => request<{ items: User[] }>('/api/users').then(r => setUsers(r.items || [])).catch(() => setUsers([]))
  useEffect(() => { loadUsers() }, [])
  const createUser = async (e: FormEvent) => {
    e.preventDefault(); setBusy(true); setError('')
    try {
      const data = await request<{ user: User }>('/api/users', { method: 'POST', body: JSON.stringify(form) })
      setUsers((items) => [...items, data.user].sort((a, b) => a.name.localeCompare(b.name)))
      setForm({ name: '', email: '', password: '', role: 'operator', active: true })
      setToast(`Usuario criado: ${data.user.name} (${roleLabel(data.user.role)})`)
    } catch (err) { setError((err as Error).message) } finally { setBusy(false) }
  }
  return <div className="grid two">
    <Panel title="Seguranca e sessao"><DataRows rows={[user]} keys={['email', 'role', 'active']} /><button onClick={() => setToast('Configuracoes salvas localmente')}>Salvar Configuracoes</button></Panel>
    <Panel title="Novo usuario">
      {user.role === 'admin' ? <form className="form" onSubmit={createUser}>
        <label>Nome<input value={form.name} onChange={e => setForm({ ...form, name: e.target.value })} required /></label>
        <label>Email<input type="email" value={form.email} onChange={e => setForm({ ...form, email: e.target.value })} required /></label>
        <label>Senha inicial<input type="password" minLength={8} value={form.password} onChange={e => setForm({ ...form, password: e.target.value })} required /></label>
        <label>Tipo de usuario<select value={form.role} onChange={e => setForm({ ...form, role: e.target.value as Role })}>{roleOptions.map(role => <option key={role} value={role}>{roleLabel(role)}</option>)}</select></label>
        <label className="check"><input type="checkbox" checked={form.active} onChange={e => setForm({ ...form, active: e.target.checked })} /> Usuario ativo</label>
        {error && <div className="error">{error}</div>}
        <button disabled={busy}>{busy ? <Loader2 className="spin" size={16} /> : <UserPlus size={16} />} Criar usuario</button>
      </form> : <Empty text="Somente administradores podem criar novos usuarios." />}
    </Panel>
    <Panel title="Usuarios e permissoes"><DataRows rows={users} keys={['name', 'email', 'role', 'active']} /></Panel>
  </div>
}

function Metric({ icon: Icon, label, value }: { icon: LucideIcon; label: string; value: ReactNode }) { return <article className="metric"><Icon size={20} /><span>{label}</span><strong>{value}</strong></article> }
function Panel({ title, tools, children }: { title: string; tools?: ReactNode; children: ReactNode }) { return <section className="panel"><div className="panel-head"><h2>{title}</h2><div className="tools">{tools}</div></div>{children}</section> }
function DataRows<T extends object>({ rows, keys, renderActions }: { rows?: T[]; keys: string[]; renderActions?: (row: T) => ReactNode }) {
  if (!rows?.length) return <Empty text="Nenhum registro encontrado." />
  return <div className="table-wrap"><table><thead><tr>{keys.map((k) => <th key={k}>{k}</th>)}{renderActions && <th>Acoes</th>}</tr></thead><tbody>{rows.map((r, i) => {
    const row = r as Record<string, unknown>
    return <tr key={String(row.id || i)}>{keys.map((k) => <td key={k}>{format(row[k])}</td>)}{renderActions && <td className="row-actions">{renderActions(r)}</td>}</tr>
  })}</tbody></table></div>
}
function SearchBox({ value, onChange }: { value: string; onChange: (value: string) => void }) { return <div className="search"><Search size={16} /><input value={value} onChange={(e) => onChange(e.target.value)} placeholder="Busca avancada" /></div> }
function Empty({ text }: { text: string }) { return <div className="empty">{text}</div> }
function FullLoader() { return <div className="login-screen"><Loader2 className="spin" /></div> }
function format(v: unknown) { if (v === null || v === undefined || v === '') return '-'; if (typeof v === 'boolean') return v ? 'sim' : 'nao'; return String(v).slice(0, 80) }
function formatNumber(v: number) { return Number.isFinite(v) ? v.toLocaleString('pt-BR', { maximumFractionDigits: 2 }) : '0' }
function currentMonth() { const now = new Date(); return `${now.getFullYear()}-${String(now.getMonth() + 1).padStart(2, '0')}` }
function statusClass(value: string) {
  const clean = value.toLowerCase()
  if (clean.includes('success') || clean.includes('em uso') || clean.includes('checked')) return 'status-ok'
  if (clean.includes('failed') || clean.includes('erro') || clean.includes('cancelado') || clean.includes('blocked')) return 'status-danger'
  if (clean.includes('pending') || clean.includes('dry') || clean.includes('bloqueado')) return 'status-warn'
  return 'status-info'
}
function formatAuditDetails(row: AuditLog) {
  let metadata: unknown
  try {
    metadata = row.metadata ? JSON.parse(row.metadata) : {}
  } catch {
    metadata = row.metadata || {}
  }
  return JSON.stringify({ ...row, metadata }, null, 2)
}
const roleOptions: Role[] = ['admin', 'supervisor', 'operator', 'viewer']
function roleLabel(r: Role) { return ({ admin: 'Admin', supervisor: 'Supervisor', operator: 'Operador', viewer: 'Visualizacao' } as Record<Role, string>)[r] }
function logout(setUser: (u: User | null) => void) { localStorage.removeItem('chipmov.accessToken'); localStorage.removeItem('chipmov.refreshToken'); setUser(null) }
function exportCSV(rows: object[], filename: string) { const keys = Object.keys(rows[0] || {}); const csv = [keys.join(','), ...rows.map(r => { const row = r as Record<string, unknown>; return keys.map(k => JSON.stringify(row[k] ?? '')).join(',') })].join('\n'); const url = URL.createObjectURL(new Blob([csv])); const a = document.createElement('a'); a.href = url; a.download = filename; a.click(); URL.revokeObjectURL(url) }

export default App
