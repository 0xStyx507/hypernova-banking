import { FormEvent, useState } from "react";
import { Account, MCPActionRequest, MCPActionType, Transaction, User } from "../../api";
import { HistoryPagination } from "./HistoryPagination";
import { DashboardData, OperationMode } from "../../types";
import { HyperBankWordmark } from "../../components/brand/HyperBankWordmark";

interface DashboardPageProps extends DashboardData {
  user: User;
}

function formatMinorAmount(value: string): string {
  try {
    const minor = BigInt(value || "0");
    const negative = minor < 0n;
    const absolute = negative ? -minor : minor;
    const digits = absolute.toString().padStart(3, "0");
    const whole = digits.slice(0, -2).replace(/\B(?=(\d{3})+(?!\d))/g, ",");
    return `${negative ? "-" : ""}HNL ${whole}.${digits.slice(-2)}`;
  } catch {
    return "HNL 0.00";
  }
}

function transactionLabel(transaction: Transaction): string {
  if (transaction.type === "deposit") return "Depósito";
  if (transaction.type === "withdrawal") return "Retiro";
  return transaction.direction === "credit" ? "Transferencia recibida" : "Transferencia enviada";
}

/** Composes the banking workspace from focused, presentation-only sections. */
export function DashboardPage(props: DashboardPageProps) {
  const { user, activeAccount } = props;
  return (
    <main className="dashboard-page min-h-screen px-4 py-5 text-ink sm:px-8 sm:py-8">
      <div className="dashboard-container mx-auto space-y-6">
        <DashboardHeader user={user} onLogout={props.onLogout} />
        {props.dashboardError && <p className="status-message status-error" role="alert">{props.dashboardError}</p>}
        <div className="dashboard-grid grid gap-6 lg:grid-cols-[1.35fr_0.8fr]">
          <section className="dashboard-primary space-y-6">
            <AccountSummary {...props} />
            <ActivityPanel {...props} />
          </section>
          <OperationsPanel {...props} />
        </div>
        <MCPPanel {...props} />
        <footer className="border-t border-slate-200 pt-5 text-xs text-slate-500">Tu información está protegida y tus movimientos quedan siempre disponibles para ti.</footer>
        {!activeAccount && <p className="sr-only">No hay una cuenta activa seleccionada.</p>}
      </div>
    </main>
  );
}

function DashboardHeader({ user, onLogout }: { user: User; onLogout: () => void }) {
  return <header className="dashboard-header"><div className="dashboard-heading"><HyperBankWordmark className="dashboard-brand" /><p className="dashboard-kicker">Resumen financiero</p><h1>Hola, {user.full_name.split(" ")[0]}.</h1><p className="dashboard-subtitle">Tu posición consolidada y actividad reciente.</p></div><div className="dashboard-header-right"><div className="dashboard-actions"><span className="status-pill status-pill-success">MFA activo</span><span className="dashboard-user-email">{maskEmailForDisplay(user.email)}</span><button className="secondary-button" onClick={onLogout} type="button">Cerrar sesión</button></div></div></header>;
}

function maskEmailForDisplay(email: string): string {
  const [localPart, domain] = email.split("@", 2);
  if (!localPart || !domain) return "correo protegido";
  if (localPart.length <= 4) return `${localPart.slice(0, 1)}•••${localPart.slice(-1)}@${domain}`;
  return `${localPart.slice(0, 2)}•••${localPart.slice(-2)}@${domain}`;
}

function AccountSummary(props: DashboardPageProps) {
  const { activeAccount, accounts, balance, dashboardLoading } = props;
  return <section className="account-summary balance-card rounded-[1.75rem] bg-blue p-6 text-white shadow-xl sm:p-8">
    <div className="account-summary-top"><div><p className="text-sm text-slate-300">Saldo disponible</p>{dashboardLoading && !balance ? <div className="skeleton mt-5 h-12 w-56 bg-slate-700" /> : <p className="mt-4 text-5xl font-semibold tracking-tight">{formatMinorAmount(balance?.available_balance ?? "0")}</p>}<p className="mt-3 text-xs text-slate-400">Listo para usar cuando lo necesites</p></div><div className="account-summary-meta"><span className="rounded-full bg-white/10 px-3 py-1 text-xs font-bold">{activeAccount?.currency ?? "HNL"}</span><span className="account-status">{activeAccount?.status ?? "Sin cuenta activa"}</span></div></div>
    <div className="account-summary-number"><span>Número de cuenta</span><AccountIdentifier account={activeAccount} /></div>
    <div className="account-summary-stats"><div><p className="text-slate-400">Entradas</p><p className="mt-1 font-semibold">{formatMinorAmount(balance?.credits_posted ?? "0")}</p></div><div><p className="text-slate-400">Salidas</p><p className="mt-1 font-semibold">{formatMinorAmount(balance?.debits_posted ?? "0")}</p></div><label className="account-selector-label"><span>Cuenta activa</span><select aria-label="Seleccionar cuenta" value={activeAccount?.id ?? ""} onChange={(event) => props.onAccountChange(event.target.value)}>{accounts.map((account) => <option key={account.id} value={account.id}>{account.type} · {account.currency}</option>)}</select></label></div>
  </section>;
}

function AccountIdentifier({ account }: { account?: Account }) {
  const [copied, setCopied] = useState(false);
  if (!account) return <strong className="account-number">—</strong>;
  const accountId = account.id;
  async function copyAccount() {
    if (!navigator.clipboard) return;
    await navigator.clipboard.writeText(accountId);
    setCopied(true);
    window.setTimeout(() => setCopied(false), 1600);
  }
  return <div className="account-identifier"><strong className="account-number" title={accountId}>{accountId}</strong><button className="account-copy-button" onClick={() => void copyAccount()} type="button">{copied ? "Copiado" : "Copiar"}</button></div>;
}

function OperationsPanel(props: DashboardPageProps) {
  return <section className="dashboard-operation-card surface h-fit p-5 sm:p-7"><p className="text-sm font-bold uppercase tracking-[0.18em] text-slate-400">Operaciones</p><h2 className="mt-2 text-xl font-semibold">Mueve tus fondos</h2><div className="mt-5 grid grid-cols-3 rounded-full bg-slate-100 p-1">{(["deposit", "withdraw", "transfer"] as OperationMode[]).map((mode) => <button key={mode} className={`rounded-full px-2 py-2 text-xs font-bold ${props.operationMode === mode ? "bg-blue text-white" : "text-slate-500"}`} onClick={() => props.onOperationModeChange(mode)} type="button">{mode === "deposit" ? "Agregar" : mode === "withdraw" ? "Retirar" : "Enviar"}</button>)}</div><form className="mt-6 space-y-5" onSubmit={props.onOperation}><label><span className="field-label">Monto en HNL</span><input required inputMode="numeric" pattern="[1-9][0-9]*" value={props.operationAmount} onChange={(event) => props.onAmountChange(event.target.value.replace(/\D/g, ""))} placeholder="100000" /></label>{props.operationMode === "transfer" && <label><span className="field-label">Cuenta de destino</span><input required value={props.destinationAccountId} onChange={(event) => props.onDestinationChange(event.target.value)} placeholder="Identificador de la cuenta" /></label>}<div className="rounded-2xl bg-slate-50 p-4 text-xs leading-5 text-slate-500">{props.operationMode === "deposit" ? "Agrega dinero a tu cuenta de forma sencilla." : props.operationMode === "withdraw" ? "Solo puedes retirar fondos disponibles." : "Revisa los datos antes de enviar dinero."}</div>{props.operationNotice && <p className={`status-message ${props.operationNotice.tone === "error" ? "status-error" : "status-success"}`} role="alert">{props.operationNotice.message}</p>}<button className="primary-button w-full" disabled={props.operationBusy || !props.activeAccount} type="submit">{props.operationBusy ? "Enviando…" : "Continuar"}</button></form></section>;
}

function ActivityPanel(props: DashboardPageProps) {
  return <section className="dashboard-activity-panel surface p-5 sm:p-7"><div className="flex flex-wrap items-start justify-between gap-4"><div><p className="text-sm font-bold uppercase tracking-[0.18em] text-slate-400">Actividad</p><h2 className="mt-2 text-xl font-semibold">Historial reciente</h2></div><div className="flex flex-wrap items-center gap-2"><span className="rounded-full bg-slate-100 px-3 py-1 text-xs font-bold text-slate-500">Movimientos HNL</span><button className="secondary-button" disabled={props.exportBusy || !props.activeAccount} onClick={props.onExport} type="button">{props.exportBusy ? "Preparando…" : "Descargar movimientos"}</button></div></div>{props.history?.items.length ? <ActivityChart transactions={props.history.items} /> : null}{!props.history && props.dashboardLoading ? <div className="mt-6 space-y-3"><div className="skeleton h-12 w-full" /><div className="skeleton h-12 w-full" /></div> : props.history?.items.length ? <div className="mt-5 divide-y divide-slate-100">{props.history.items.map((transaction) => <TransactionRow key={`${transaction.transfer_id}-${transaction.created_at}`} transaction={transaction} />)}</div> : <div className="mt-6 rounded-2xl bg-slate-50 p-6 text-center text-sm text-slate-500">Todavía no hay movimientos en esta cuenta.</div>}{props.history && <HistoryPagination page={props.historyPage} hasMore={props.historyHasMore} busy={props.historyPageBusy} onPrevious={props.onPreviousHistory} onNext={props.onNextHistory} />}</section>;
}

function ActivityChart({ transactions }: { transactions: Transaction[] }) {
  const points = transactions.slice(0, 6).reverse();
  if (!points.length) return null;
  const maximum = points.reduce((current, transaction) => { try { const amount = BigInt(transaction.amount); return amount > current ? amount : current; } catch { return current; } }, 0n);
  return <div className="activity-chart-inline" aria-label="Gráfico de importes recientes" role="img">{points.map((transaction) => { let amount = 0n; try { amount = BigInt(transaction.amount); } catch { /* Ignore malformed display data. */ } const percentage = maximum > 0n ? Number((amount * 100n) / maximum) : 0; return <div className="flex h-full flex-1 flex-col items-center justify-end gap-2" key={`${transaction.transfer_id}-chart`}><div className={`w-full max-w-10 rounded-t-xl ${transaction.direction === "credit" ? "bg-teal" : "bg-purple"}`} style={{ height: `${Math.max(12, percentage)}%` }} title={formatMinorAmount(transaction.amount)} /><span className="text-[10px] font-bold uppercase text-slate-400">{transaction.direction === "credit" ? "C" : "D"}</span></div>; })}</div>;
}

function TransactionRow({ transaction }: { transaction: Transaction }) {
  return <div className="flex items-center justify-between gap-3 py-4"><div className="min-w-0"><p className="truncate text-sm font-bold">{transactionLabel(transaction)}</p><p className="mt-1 text-xs text-slate-400">{new Date(transaction.created_at).toLocaleString("es-PA")}</p></div><p className={`shrink-0 text-sm font-bold ${transaction.direction === "credit" ? "text-[#087e78]" : "text-purple"}`}>{transaction.direction === "credit" ? "+" : "−"}{formatMinorAmount(transaction.amount)}</p></div>;
}

function MCPPanel(props: DashboardPageProps) {
  const [actionType, setActionType] = useState<MCPActionType>("deposit");
  const [actionAmount, setActionAmount] = useState("");
  const [actionDestination, setActionDestination] = useState("");

  function submitAction(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (!props.activeAccount || !actionAmount) return;
    const request: MCPActionRequest = {
      action: actionType,
      amount: actionAmount,
      currency: "HNL",
      ...(actionType === "transfer"
        ? { source_account_id: props.activeAccount.id, destination_account_id: actionDestination }
        : { account_id: props.activeAccount.id }),
    };
    props.onPrepareMCPAction(request);
  }

  const actionLabel = props.mcpAction?.action === "withdrawal" ? "Retiro" : props.mcpAction?.action === "transfer" ? "Transferencia" : "Depósito";
  const actionReady = props.mcpAction?.status === "ready" || props.mcpAction?.status === "confirming";

  return <section className="dashboard-tools-panel surface p-5 sm:p-7" aria-labelledby="mcp-title">
    <div className="flex flex-col gap-4 lg:flex-row lg:items-start lg:justify-between"><div><div className="flex items-center gap-3"><p className="text-sm font-bold uppercase tracking-[0.18em] text-slate-400">Asistente Hyper Bank</p><span className="status-pill status-pill-success">Disponible</span></div><h2 id="mcp-title" className="mt-2 text-2xl font-semibold">Herramientas para entender tu cuenta</h2><p className="mt-2 max-w-2xl text-sm leading-6 text-slate-500">Consulta tus datos o prepara una operación. El dinero solo se mueve después de revisar el resumen y confirmar de forma explícita.</p></div><span className="rounded-full bg-slate-100 px-3 py-1 text-xs font-bold text-slate-500">{props.mcpLoading ? "Sincronizando…" : `${props.mcpTools.length} herramientas disponibles`}</span></div>
    {props.mcpError && <p className="status-message status-error mt-5" role="alert">{props.mcpError}</p>}
    <div className="mt-6 grid gap-3 md:grid-cols-2 lg:grid-cols-3">{props.mcpTools.map((tool) => <div className="rounded-2xl border border-slate-200 bg-slate-50 p-4" key={tool.name}><div className="flex items-center justify-between gap-3"><p className="font-semibold text-ink">{tool.name}</p><span className={`rounded-full px-2 py-1 text-[10px] font-bold uppercase tracking-wide ${tool.read_only ? "bg-teal/15 text-teal" : "bg-purple/10 text-purple"}`}>{tool.read_only ? "Lectura" : "Confirmación"}</span></div><p className="mt-2 text-xs leading-5 text-slate-500">{tool.description}</p></div>)}</div>
    <div className="mcp-action-box mt-6 rounded-2xl border border-blue/15 bg-blue/5 p-5"><div className="flex flex-wrap items-start justify-between gap-3"><div><p className="text-sm font-bold uppercase tracking-[0.16em] text-blue">Operación con confirmación</p><p className="mt-2 text-sm text-slate-600">Prepara un resumen inmutable antes de enviar la solicitud al ledger.</p></div>{props.mcpAction && <span className="status-pill status-pill-neutral">{props.mcpAction.status}</span>}</div><form className="mt-5 grid gap-4 md:grid-cols-3" onSubmit={submitAction}><label><span className="field-label">Tipo de operación</span><select value={actionType} onChange={(event) => setActionType(event.target.value as MCPActionType)}><option value="deposit">Depósito</option><option value="withdrawal">Retiro</option><option value="transfer">Transferencia</option></select></label><label><span className="field-label">Monto en HNL</span><input required inputMode="numeric" pattern="[1-9][0-9]*" value={actionAmount} onChange={(event) => setActionAmount(event.target.value.replace(/\D/g, ""))} placeholder="100000" /></label>{actionType === "transfer" && <label><span className="field-label">Cuenta de destino</span><input required value={actionDestination} onChange={(event) => setActionDestination(event.target.value)} placeholder="Identificador de cuenta" /></label>}<button className="primary-button md:col-span-3" disabled={props.mcpActionBusy || !props.activeAccount} type="submit">{props.mcpActionBusy ? "Preparando…" : "Preparar operación"}</button></form>{props.mcpAction && <div className="mt-5 rounded-2xl bg-white p-4"><p className="text-sm font-semibold">{actionLabel} por {formatMinorAmount(props.mcpAction.payload.amount)}</p><p className="mt-1 text-xs text-slate-500">Revisa los datos. La acción vence {new Date(props.mcpAction.expires_at).toLocaleTimeString("es-PA")} y todavía no ha movido fondos.</p>{props.mcpActionNotice && <p className={`status-message mt-4 ${props.mcpActionNotice.tone === "error" ? "status-error" : "status-success"}`} role="alert">{props.mcpActionNotice.message}</p>}<div className="mt-4 flex flex-wrap gap-3"><button className="primary-button" disabled={props.mcpActionBusy || !actionReady} onClick={props.onConfirmMCPAction} type="button">{props.mcpActionBusy ? "Confirmando…" : "Confirmar y ejecutar"}</button><button className="secondary-button" disabled={props.mcpActionBusy || !actionReady} onClick={props.onCancelMCPAction} type="button">Cancelar</button></div></div>}</div>
    <form className="mt-6 flex flex-col gap-3 sm:flex-row" onSubmit={props.onAssistantSubmit}><label className="sr-only" htmlFor="assistant-message">Consulta al asistente</label><input id="assistant-message" value={props.assistantInput} onChange={(event) => props.onAssistantInput(event.target.value)} placeholder="Ej. ¿Cuál es mi saldo disponible?" maxLength={2000} /><button className="primary-button shrink-0 sm:w-48" disabled={props.assistantBusy || !props.assistantInput.trim()} type="submit">{props.assistantBusy ? "Consultando…" : "Preguntar"}</button></form>{props.assistantReply && <div className="mt-5 rounded-2xl border border-blue/20 bg-blue/5 p-5"><p className="text-sm font-semibold text-ink">{props.assistantReply.message}</p>{props.assistantReply.confirmation && <p className="mt-3 text-xs font-semibold text-purple">Esta intención requiere revisar y confirmar la operación antes de ejecutarse.</p>}{props.assistantReply.data !== undefined && <details className="mt-4"><summary className="cursor-pointer text-xs font-bold text-blue">Ver detalles</summary><pre className="mt-3 max-h-48 overflow-auto rounded-xl bg-slate-950 p-4 text-xs leading-5 text-slate-100">{JSON.stringify(props.assistantReply.data, null, 2)}</pre></details>}</div>}
  </section>;
}
