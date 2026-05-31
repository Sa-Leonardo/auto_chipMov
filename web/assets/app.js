const state = {
  key: localStorage.getItem("chipmov.adminKey") || "",
  iccids: [],
  summary: [],
  approvals: [],
  operations: [],
  reports: [],
  nextRun: null,
};

const cnpjNames = {
  "58420964000179": "MOV Fit",
  "15070244000118": "MOV Fibra",
};

const els = {
  adminKey: document.querySelector("#adminKey"),
  saveKeyBtn: document.querySelector("#saveKeyBtn"),
  refreshBtn: document.querySelector("#refreshBtn"),
  syncSubscribersBtn: document.querySelector("#syncSubscribersBtn"),
  syncLastRechargeBtn: document.querySelector("#syncLastRechargeBtn"),
  createApprovalsBtn: document.querySelector("#createApprovalsBtn"),
  dryRunBtn: document.querySelector("#dryRunBtn"),
  manualRechargeForm: document.querySelector("#manualRechargeForm"),
  manualIccid: document.querySelector("#manualIccid"),
  manualQuantity: document.querySelector("#manualQuantity"),
  manualDryRun: document.querySelector("#manualDryRun"),
  confirmModal: document.querySelector("#confirmModal"),
  confirmTitle: document.querySelector("#confirmTitle"),
  confirmMessage: document.querySelector("#confirmMessage"),
  confirmCancelBtn: document.querySelector("#confirmCancelBtn"),
  confirmOkBtn: document.querySelector("#confirmOkBtn"),
  statusMessage: document.querySelector("#statusMessage"),
  metricTracked: document.querySelector("#metricTracked"),
  metricPending: document.querySelector("#metricPending"),
  metricDue: document.querySelector("#metricDue"),
  metricNext: document.querySelector("#metricNext"),
  summaryBody: document.querySelector("#summaryBody"),
  nextRunBox: document.querySelector("#nextRunBox"),
  iccidFilter: document.querySelector("#iccidFilter"),
  statusFilter: document.querySelector("#statusFilter"),
  iccidsBody: document.querySelector("#iccidsBody"),
  approvalsBody: document.querySelector("#approvalsBody"),
  operationsBody: document.querySelector("#operationsBody"),
  reportFilter: document.querySelector("#reportFilter"),
  reportsBody: document.querySelector("#reportsBody"),
};

els.adminKey.value = state.key;
els.confirmModal.hidden = true;

els.saveKeyBtn.addEventListener("click", () => {
  state.key = els.adminKey.value.trim();
  localStorage.setItem("chipmov.adminKey", state.key);
  showMessage("Chave salva no navegador.");
});

els.refreshBtn.addEventListener("click", refreshAll);
els.syncSubscribersBtn.addEventListener("click", () => confirmAndRun(
  "Sincronizar assinantes agora? Isso atualiza a base local com os dados da plataforma.",
  "Sincronizando assinantes...",
  () => api("/sync/assinantes", { method: "POST" }),
));
els.syncLastRechargeBtn.addEventListener("click", () => confirmAndRun(
  "Sincronizar ultima recarga agora? Essa acao consulta a API externa para os ICCIDs salvos e pode levar alguns minutos.",
  "Sincronizando ultima recarga...",
  () => api("/sync/ultima-recarga", { method: "POST" }),
));
els.createApprovalsBtn.addEventListener("click", () => confirmAndRun(
  "Criar pendencias de aprovacao para ICCIDs vencidos? Nenhuma recarga real sera feita.",
  "Criando pendencias...",
  () => api("/automation/check-recharges", {
  method: "POST",
  body: { create_approvals: true },
})));
els.dryRunBtn.addEventListener("click", () => runAction("Simulando rotina...", () => api("/automation/check-recharges", {
  method: "POST",
  body: { dry_run: true },
})));
els.manualRechargeForm.addEventListener("submit", (event) => {
  event.preventDefault();
  const iccid = els.manualIccid.value.trim();
  const quantity = Number.parseInt(els.manualQuantity.value, 10);
  const dryRun = els.manualDryRun.checked;
  if (!iccid || !Number.isInteger(quantity) || quantity < 1) {
    showMessage("Informe um ICCID e uma quantidade inteira maior ou igual a 1.", true);
    return;
  }
  const label = dryRun ? "Simulando recarga manual..." : "Executando recarga manual real...";
  const execute = () => runAction(label, () => api(`/iccids/${encodeURIComponent(iccid)}/saldo`, {
    method: "POST",
    body: { quantity, dry_run: dryRun },
  }));
  if (dryRun) {
    confirmModal({
      title: "Confirmar simulacao",
      message: `Simular adicao de saldo?\n\nICCID: ${iccid}\nQuantidade solicitada: ${quantity} GB\n\nNenhuma chamada sera enviada para a API real.`,
      confirmText: "Simular",
    }).then((confirmed) => {
      if (confirmed) {
        execute();
      }
    });
    return;
  }
  const message = `Confirmar recarga REAL?\n\nICCID: ${iccid}\nQuantidade solicitada: ${quantity} GB\n\nA plataforma pode creditar uma franquia diferente conforme operadora/plano.`;
  confirmModal({
    title: "Confirmar recarga real",
    message,
    confirmText: "Adicionar saldo",
    danger: true,
  }).then((confirmed) => {
    if (confirmed) {
      execute();
    } else {
      showMessage("Recarga manual cancelada.");
    }
  });
});
els.iccidFilter.addEventListener("input", renderICCIDs);
els.statusFilter.addEventListener("change", renderICCIDs);
els.reportFilter.addEventListener("input", renderReports);

async function api(path, options = {}) {
  if (!state.key) {
    throw new Error("Informe a chave interna antes de chamar a API.");
  }
  const init = {
    method: options.method || "GET",
    headers: {
      "x-api-key": state.key,
      "Content-Type": "application/json",
    },
  };
  if (options.body) {
    init.body = JSON.stringify(options.body);
  }
  const response = await fetch(path, init);
  const text = await response.text();
  const data = text ? JSON.parse(text) : {};
  if (!response.ok) {
    const detail = data.error || data.message || response.statusText;
    throw new Error(`${response.status} ${detail}`);
  }
  return data;
}

async function runAction(label, fn) {
  showMessage(label);
  try {
    const result = await fn();
    const summary = actionSummary(result);
    showMessage(summary);
    await resultModal({
      title: result.status === "success" ? "Acao concluida" : "Resultado",
      message: summary,
      danger: false,
    });
    await refreshAll(false);
  } catch (error) {
    showMessage(error.message, true);
    await resultModal({
      title: "Erro na operacao",
      message: error.message,
      danger: true,
    });
  }
}

async function confirmAndRun(message, label, fn, options = {}) {
  const confirmed = await confirmModal({
    title: options.title || "Confirmar acao",
    message,
    confirmText: options.confirmText || "Confirmar",
    danger: options.danger || false,
  });
  if (!confirmed) {
    showMessage("Acao cancelada.");
    return;
  }
  runAction(label, fn);
}

function confirmModal({ title, message, confirmText = "Confirmar", danger = false }) {
  return new Promise((resolve) => {
    els.confirmTitle.textContent = title;
    els.confirmMessage.textContent = message;
    els.confirmOkBtn.textContent = confirmText;
    els.confirmOkBtn.classList.toggle("danger", danger);
    els.confirmModal.hidden = false;

    const cleanup = (value) => {
      els.confirmModal.hidden = true;
      els.confirmOkBtn.classList.remove("danger");
      els.confirmOkBtn.removeEventListener("click", onConfirm);
      els.confirmCancelBtn.removeEventListener("click", onCancel);
      els.confirmModal.removeEventListener("click", onBackdrop);
      document.removeEventListener("keydown", onKeydown);
      resolve(value);
    };

    const onConfirm = () => cleanup(true);
    const onCancel = () => cleanup(false);
    const onBackdrop = (event) => {
      if (event.target === els.confirmModal) cleanup(false);
    };
    const onKeydown = (event) => {
      if (event.key === "Escape") cleanup(false);
    };

    els.confirmOkBtn.addEventListener("click", onConfirm);
    els.confirmCancelBtn.addEventListener("click", onCancel);
    els.confirmModal.addEventListener("click", onBackdrop);
    document.addEventListener("keydown", onKeydown);
    els.confirmCancelBtn.focus();
  });
}

function resultModal({ title, message, danger = false }) {
  return new Promise((resolve) => {
    els.confirmTitle.textContent = title;
    els.confirmMessage.textContent = message;
    els.confirmOkBtn.textContent = "Ok";
    els.confirmOkBtn.classList.toggle("danger", danger);
    els.confirmCancelBtn.hidden = true;
    els.confirmModal.hidden = false;

    const cleanup = () => {
      els.confirmModal.hidden = true;
      els.confirmCancelBtn.hidden = false;
      els.confirmOkBtn.classList.remove("danger");
      els.confirmOkBtn.removeEventListener("click", onConfirm);
      els.confirmModal.removeEventListener("click", onBackdrop);
      document.removeEventListener("keydown", onKeydown);
      resolve();
    };

    const onConfirm = () => cleanup();
    const onBackdrop = (event) => {
      if (event.target === els.confirmModal) cleanup();
    };
    const onKeydown = (event) => {
      if (event.key === "Escape" || event.key === "Enter") cleanup();
    };

    els.confirmOkBtn.addEventListener("click", onConfirm);
    els.confirmModal.addEventListener("click", onBackdrop);
    document.addEventListener("keydown", onKeydown);
    els.confirmOkBtn.focus();
  });
}

async function refreshAll(showOk = true) {
  try {
    const [iccids, summary, approvals, operations, nextRun] = await Promise.all([
      api("/iccids"),
      api("/iccids/summary"),
      api("/recharge-approvals?status=pending"),
      api("/operacoes?limit=500"),
      api("/automation/next-run"),
    ]);
    state.iccids = iccids.items || [];
    state.summary = summary.items || [];
    state.approvals = approvals.items || [];
    state.reports = operations.items || [];
    state.operations = state.reports.slice(0, 20);
    state.nextRun = nextRun;
    renderAll();
    if (showOk) {
      showMessage("Dados atualizados.");
    }
  } catch (error) {
    showMessage(error.message, true);
  }
}

function renderAll() {
  els.metricTracked.textContent = state.iccids.length;
  els.metricPending.textContent = state.approvals.length;
  els.metricDue.textContent = state.nextRun?.iccids_due_count ?? "-";
  els.metricNext.textContent = formatDate(state.nextRun?.next_recharge_due_at);
  renderSummary();
  renderNextRun();
  renderICCIDs();
  renderApprovals();
  renderOperations();
  renderReports();
}

function renderSummary() {
  els.summaryBody.innerHTML = "";
  if (!state.summary.length) {
    els.summaryBody.append(emptyRow(3, "Nenhum resumo local."));
    return;
  }
  for (const item of state.summary) {
    els.summaryBody.append(row([
      formatCNPJ(item.cnpj),
      badge(item.contract_status),
      item.count,
    ]));
  }
}

function renderNextRun() {
  const next = state.nextRun;
  if (!next || !next.next_recharge_due_at) {
    els.nextRunBox.innerHTML = `<div class="muted">Nenhuma próxima recarga acionável.</div>`;
    return;
  }
  const items = next.next_recharge_iccids || [];
  els.nextRunBox.innerHTML = `
    <div class="next-date">${escapeHTML(formatDate(next.next_recharge_due_at))}</div>
    <div class="muted" style="margin-bottom: 8px;">${items.length} ICCID(s) nessa data</div>
    <div style="display:flex; flex-direction:column;">
      ${items.slice(0, 5).map((item) => `
        <div class="next-list-item">
          <strong style="color:var(--accent); font-family:monospace;">${escapeHTML(item.sim_card)}</strong> 
          <span class="muted" style="margin-left: 8px;">(${escapeHTML(formatCNPJ(item.cnpj))})</span>
        </div>
      `).join("")}
    </div>
  `;
}

function renderICCIDs() {
  const search = els.iccidFilter.value.trim().toLowerCase();
  const status = els.statusFilter.value;
  const filtered = state.iccids.filter((item) => {
    const haystack = `${item.sim_card} ${item.cnpj} ${item.contract_status} ${item.phone_number}`.toLowerCase();
    const matchesSearch = !search || haystack.includes(search);
    const matchesStatus = !status || item.contract_status === status;
    return matchesSearch && matchesStatus;
  });
  els.iccidsBody.innerHTML = "";
  if (!filtered.length) {
    els.iccidsBody.append(emptyRow(6, "Nenhum ICCID encontrado."));
    return;
  }
  for (const item of filtered) {
    els.iccidsBody.append(row([
      item.sim_card,
      formatCNPJ(item.cnpj),
      item.phone_number || "-",
      badge(item.contract_status),
      formatDate(item.last_recharge_at),
      formatDate(item.next_recharge_due_at),
    ]));
  }
}

function renderApprovals() {
  els.approvalsBody.innerHTML = "";
  if (!state.approvals.length) {
    els.approvalsBody.append(emptyRow(5, "Nenhuma pendencia."));
    return;
  }
  for (const item of state.approvals) {
    const actions = document.createElement("div");
    actions.className = "row-actions";
    const approve = document.createElement("button");
    approve.type = "button";
    approve.textContent = "Aprovar";
    approve.addEventListener("click", () => confirmAndRun(
      `Aprovar e executar recarga real para este ICCID?\n\nICCID: ${item.sim_card}\nCNPJ: ${formatCNPJ(item.cnpj)}\nQuantidade solicitada: ${item.quantity} GB`,
      `Aprovando pendencia ${item.id}...`,
      () => api(`/recharge-approvals/${item.id}/approve`, { method: "POST" }),
      { title: "Aprovar recarga real", confirmText: "Aprovar e recarregar", danger: true },
    ));
    const reject = document.createElement("button");
    reject.type = "button";
    reject.className = "danger";
    reject.textContent = "Rejeitar";
    reject.addEventListener("click", () => confirmAndRun(
      `Rejeitar esta pendencia?\n\nICCID: ${item.sim_card}`,
      `Rejeitando pendencia ${item.id}...`,
      () => api(`/recharge-approvals/${item.id}/reject`, { method: "POST" }),
      { title: "Rejeitar pendencia", confirmText: "Rejeitar", danger: true },
    ));
    actions.append(approve, reject);
    els.approvalsBody.append(row([
      item.id,
      item.sim_card,
      formatCNPJ(item.cnpj),
      item.quantity,
      actions,
    ]));
  }
}

function renderOperations() {
  els.operationsBody.innerHTML = "";
  if (!state.operations.length) {
    els.operationsBody.append(emptyRow(4, "Nenhuma operacao recente."));
    return;
  }
  for (const item of state.operations) {
    els.operationsBody.append(row([
      item.id,
      item.sim_card,
      badge(item.status),
      item.trigger_type,
    ]));
  }
}

function renderReports() {
  els.reportsBody.innerHTML = "";
  const search = els.reportFilter.value.trim().toLowerCase();
  const filtered = state.reports.filter((item) => {
    const haystack = `${item.sim_card} ${item.cnpj} ${formatCNPJ(item.cnpj)} ${item.status} ${item.trigger_type} ${item.easy2use_user_message || ""} ${item.error_message || ""}`.toLowerCase();
    return !search || haystack.includes(search);
  });
  if (!filtered.length) {
    els.reportsBody.append(emptyRow(7, "Nenhum relatorio encontrado."));
    return;
  }
  for (const item of filtered) {
    els.reportsBody.append(row([
      formatDateTime(item.created_at),
      item.sim_card,
      formatCNPJ(item.cnpj),
      item.quantity,
      badge(item.status),
      item.trigger_type,
      item.easy2use_user_message || item.error_message || "-",
    ]));
  }
}

function row(values) {
  const tr = document.createElement("tr");
  for (const value of values) {
    const td = document.createElement("td");
    if (value instanceof HTMLElement) {
      td.append(value);
    } else {
      td.textContent = String(value ?? "-");
    }
    tr.append(td);
  }
  return tr;
}

function emptyRow(cols, message) {
  const tr = document.createElement("tr");
  const td = document.createElement("td");
  td.colSpan = cols;
  td.className = "muted";
  td.textContent = message;
  tr.append(td);
  return tr;
}

function badge(status) {
  const value = status || "-";
  const span = document.createElement("span");
  span.className = "badge";
  if (value === "EM USO" || value === "success") span.classList.add("ok");
  if (value === "BLOQUEADO" || value === "pending") span.classList.add("warn");
  if (value === "CANCELADO" || value === "failed") span.classList.add("danger");
  span.textContent = value;
  return span;
}

function formatDate(value) {
  if (!value) return "-";
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return String(value).slice(0, 10);
  return new Intl.DateTimeFormat("pt-BR").format(date);
}

function formatDateTime(value) {
  if (!value) return "-";
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return String(value);
  return new Intl.DateTimeFormat("pt-BR", {
    dateStyle: "short",
    timeStyle: "short",
  }).format(date);
}

function formatCNPJ(value) {
  const raw = String(value || "");
  const name = cnpjNames[raw];
  return name ? `${name} - ${raw}` : raw || "-";
}

function showMessage(message, isError = false) {
  els.statusMessage.hidden = false;
  els.statusMessage.classList.toggle("error", isError);
  els.statusMessage.textContent = message;
}

function actionSummary(result) {
  if (result.saved !== undefined) {
    return `Sincronizacao concluida. Salvos: ${result.saved}. Contratos permitidos: ${result.allowed_contracts ?? "-"}. Ignorados: ${result.skipped ?? "-"}.`;
  }
  if (result.updated !== undefined) {
    return `Ultima recarga sincronizada. Atualizados: ${result.updated}. Falhas: ${result.failed}.`;
  }
  if (result.created_approvals !== undefined) {
    return `Pendencias processadas. Criadas: ${result.created_approvals}. Existentes: ${result.existing_approvals}.`;
  }
  if (result.checked !== undefined) {
    return `Rotina simulada. Verificados: ${result.checked}. Pendentes: ${result.results?.length ?? 0}.`;
  }
  if (result.dry_run) {
    return `Simulacao concluida para ${result.sim_card ?? result.iccid?.sim_card ?? "ICCID informado"}. Nenhuma recarga real foi feita.`;
  }
  if (result.status) {
    const dateInfo = result.next_recharge_due_at ? ` Proxima recarga: ${formatDate(result.next_recharge_due_at)}.` : "";
    if (result.status === "success" && result.sim_card) {
      return `Saldo adicionado com sucesso. ICCID: ${result.sim_card}. Quantidade solicitada: ${result.quantity} GB.${dateInfo}`;
    }
    return `Acao concluida: ${result.status}.${dateInfo}`;
  }
  return "Acao concluida.";
}

function escapeHTML(value) {
  return String(value ?? "")
    .replaceAll("&", "&amp;")
    .replaceAll("<", "&lt;")
    .replaceAll(">", "&gt;")
    .replaceAll('"', "&quot;")
    .replaceAll("'", "&#039;");
}

if (state.key) {
  refreshAll(false);
}
