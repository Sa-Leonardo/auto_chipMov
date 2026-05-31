import { useEffect, useMemo, useState } from 'react'
import type { FormEvent } from 'react'
import {
  Activity, AlertTriangle, BarChart3, BookOpen, CheckCircle2, Database, FileDown,
  Gauge, KeyRound, LayoutDashboard, ListChecks, Loader2, LogOut, RadioTower,
  RefreshCw, Search, Settings, ShieldCheck, Signal, Smartphone, XCircle,
} from 'lucide-react'
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

const API = import.meta.env.VITE_API_BASE_URL || ''
const nav = [
  ['dashboard', 'Dashboard', LayoutDashboard],
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
  const [data, setData] = useState<any>(null)
  const [live, setLive] = useState('desconectado')
  const load = () => request('/api/dashboard/summary').then(setData)
  const syncSubscribers = async () => {
    setToast('Sincronizando assinantes...')
    const result = await request<any>('/sync/assinantes', { method: 'POST' })
    await load()
    setToast(`Sincronizacao concluida: ${result.saved || 0} ICCIDs salvos`)
  }
  useEffect(() => { load(); const ws = new WebSocket(`${location.protocol === 'https:' ? 'wss' : 'ws'}://${location.host}/api/ws`); ws.onopen = () => setLive('online'); ws.onclose = () => setLive('offline'); return () => ws.close() }, [])
  const chart = Object.entries(data?.status_counts || {}).map(([name, value]) => ({ name: name || 'Sem status', value }))
  return <section className="stack">
    <div className="actionbar"><div className="tools"><button onClick={() => load().then(() => setToast('Dashboard atualizado'))}><RefreshCw size={16} /> Atualizar</button><button onClick={syncSubscribers}><Database size={16} /> Sincronizar assinantes</button></div><span className={`live ${live}`}>WebSocket {live}</span></div>
    <div className="metrics">
      <Metric icon={Database} label="Total de ICCIDs" value={data?.total_iccids ?? '-'} />
      <Metric icon={Signal} label="Em uso" value={data?.status_counts?.['EM USO'] ?? 0} />
      <Metric icon={AlertTriangle} label="Bloqueados" value={data?.status_counts?.BLOQUEADO ?? 0} />
      <Metric icon={XCircle} label="Cancelados" value={data?.status_counts?.CANCELADO ?? 0} />
      <Metric icon={Gauge} label="Proximas recargas" value={data?.actionable_iccids_count ?? 0} />
    </div>
    <div className="grid two"><Panel title="Status dos ICCIDs"><ResponsiveContainer height={260}><BarChart data={chart}><CartesianGrid strokeDasharray="3 3" /><XAxis dataKey="name" /><YAxis /><Tooltip /><Bar dataKey="value" fill="#2dd4bf" /></BarChart></ResponsiveContainer></Panel><Panel title="Ultimas Operacoes"><DataRows rows={data?.recent_operations || []} keys={['id', 'sim_card', 'status', 'trigger_type']} /></Panel></div>
    <Panel title="Alertas importantes">{(data?.important_alerts || []).length ? data.important_alerts.map((a: any, i: number) => <div className="alert" key={i}><AlertTriangle size={16} />{a.message}</div>) : <Empty text="Nenhum alerta importante." />}</Panel>
  </section>
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
  const submit = async (e: FormEvent) => { e.preventDefault(); if (!confirm(`${dryRun ? 'Simular' : 'Executar'} recarga para ${iccid}?`)) return; await request(`/iccids/${encodeURIComponent(iccid)}/saldo`, { method: 'POST', body: JSON.stringify({ quantity, dry_run: dryRun }) }); setToast('Operacao processada'); request<{ items: Operation[] }>('/operacoes?limit=20').then(r => setHistory(r.items || [])) }
  useEffect(() => { request<{ items: Operation[] }>('/operacoes?limit=20').then(r => setHistory(r.items || [])) }, [])
  return <div className="grid two"><Panel title="Recarga manual"><form className="form" onSubmit={submit}><label>ICCID<input value={iccid} onChange={e => setIccid(e.target.value)} required /></label><label>Quantidade GB<input type="number" min={1} value={quantity} onChange={e => setQuantity(Number(e.target.value))} /></label><label className="check"><input type="checkbox" checked={dryRun} onChange={e => setDryRun(e.target.checked)} /> Simular sem chamar API real</label><button><RadioTower size={16} /> Processar</button></form></Panel><Panel title="Historico de recargas manuais"><DataRows rows={history.filter(h => h.trigger_type === 'manual')} keys={['id', 'sim_card', 'quantity', 'status', 'created_at']} /></Panel></div>
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
  return <section className="stack"><Panel title="Dashboard analitico"><ResponsiveContainer height={260}><AreaChart data={trend}><XAxis dataKey="name" /><YAxis /><Tooltip /><Area dataKey="value" stroke="#38bdf8" fill="#0ea5e9" fillOpacity={0.2} /></AreaChart></ResponsiveContainer></Panel><Panel title="Relatorios por recarga" tools={<button onClick={() => exportCSV(items, 'relatorio-recargas.csv')}><FileDown size={16} /> CSV</button>}><DataRows rows={items} keys={['created_at', 'cnpj', 'sim_card', 'quantity', 'status', 'trigger_type', 'easy2use_user_message']} /></Panel></section>
}

function Docs() {
  const sections = ['O sistema monitora ICCIDs autorizados por CNPJ e evita perda de linha por falta de recarga.', 'Fluxo: sincronizar assinantes, calcular proxima recarga, criar Aprovacao, executar recarga e auditar.', 'Status: EM USO pode ser recarregado; BLOQUEADO exige analise; CANCELADO e PORTOUT nao devem receber recarga automatica.', 'API: endpoints REST protegidos por JWT e compativeis temporariamente com x-api-key para automacoes.', 'Boas praticas: testar em dry-run, revisar Aprovacoes, manter tokens no .env e acompanhar Operacoes recentes.']
  return <Panel title="Documentacao interna do sistema">{sections.map((s, i) => <div className="doc-row" key={s}><strong>{i + 1}</strong><p>{s}</p></div>)}</Panel>
}

function SettingsPage({ user, setToast }: { user: User; setToast: (s: string) => void }) {
  const [users, setUsers] = useState<User[]>([])
  useEffect(() => { request<{ items: User[] }>('/api/users').then(r => setUsers(r.items || [])).catch(() => setUsers([])) }, [])
  return <div className="grid two"><Panel title="Seguranca e sessao"><DataRows rows={[user]} keys={['email', 'role', 'active']} /><button onClick={() => setToast('Configuracoes salvas localmente')}>Salvar Configuracoes</button></Panel><Panel title="Usuarios e permissoes"><DataRows rows={users} keys={['name', 'email', 'role', 'active']} /></Panel></div>
}

function Metric({ icon: Icon, label, value }: { icon: any; label: string; value: any }) { return <article className="metric"><Icon size={20} /><span>{label}</span><strong>{value}</strong></article> }
function Panel({ title, tools, children }: any) { return <section className="panel"><div className="panel-head"><h2>{title}</h2><div className="tools">{tools}</div></div>{children}</section> }
function DataRows({ rows, keys, renderActions }: any) { if (!rows?.length) return <Empty text="Nenhum registro encontrado." />; return <div className="table-wrap"><table><thead><tr>{keys.map((k: string) => <th key={k}>{k}</th>)}{renderActions && <th>Acoes</th>}</tr></thead><tbody>{rows.map((r: any, i: number) => <tr key={r.id || i}>{keys.map((k: string) => <td key={k}>{format(r[k])}</td>)}{renderActions && <td className="row-actions">{renderActions(r)}</td>}</tr>)}</tbody></table></div> }
function SearchBox({ value, onChange }: any) { return <div className="search"><Search size={16} /><input value={value} onChange={(e) => onChange(e.target.value)} placeholder="Busca avancada" /></div> }
function Empty({ text }: { text: string }) { return <div className="empty">{text}</div> }
function FullLoader() { return <div className="login-screen"><Loader2 className="spin" /></div> }
function format(v: any) { if (v === null || v === undefined || v === '') return '-'; if (typeof v === 'boolean') return v ? 'sim' : 'nao'; return String(v).slice(0, 80) }
function roleLabel(r: Role) { return ({ admin: 'Admin', supervisor: 'Supervisor', operator: 'Operador', viewer: 'Visualizacao' } as Record<Role, string>)[r] }
function logout(setUser: (u: User | null) => void) { localStorage.removeItem('chipmov.accessToken'); localStorage.removeItem('chipmov.refreshToken'); setUser(null) }
function exportCSV(rows: any[], filename: string) { const keys = Object.keys(rows[0] || {}); const csv = [keys.join(','), ...rows.map(r => keys.map(k => JSON.stringify(r[k] ?? '')).join(','))].join('\n'); const url = URL.createObjectURL(new Blob([csv])); const a = document.createElement('a'); a.href = url; a.download = filename; a.click(); URL.revokeObjectURL(url) }

export default App

