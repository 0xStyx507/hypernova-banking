import { FormEvent, useEffect, useMemo, useState } from "react";
import { Account, Balance, MCPAccountOption, Transaction, User } from "../../api";
import { HistoryPagination } from "./HistoryPagination";
import { DashboardData, OperationMode } from "../../types";
import { HyperBankWordmark } from "../../components/brand/HyperBankWordmark";
import { currencyInputToMinor, sanitizeCurrencyInput } from "../../money";

interface DashboardPageProps extends DashboardData {
  user: User;
}

function formatMinorAmount(value: string): string {
  try {
    const minor = /[.,]/u.test(value) ? BigInt(currencyInputToMinor(value) || "0") : BigInt(value || "0");
    const negative = minor < 0n;
    const absolute = negative ? -minor : minor;
    const digits = absolute.toString().padStart(3, "0");
    const whole = digits.slice(0, -2).replace(/\B(?=(\d{3})+(?!\d))/g, ",");
    return `${negative ? "-" : ""}USD ${whole}.${digits.slice(-2)}`;
  } catch {
    return "USD 0.00";
  }
}

function transactionLabel(transaction: Transaction): string {
  if (transaction.type === "deposit") return "Depósito";
  if (transaction.type === "withdrawal") return "Retiro";
  return transaction.direction === "credit" ? "Transferencia recibida" : "Transferencia enviada";
}

type DashboardView = "home" | "history" | "settings" | OperationMode;

function isOperationView(view: DashboardView): view is OperationMode {
  return view === "deposit" || view === "withdraw" || view === "transfer";
}

function isPendingMCPAction(action: DashboardData["mcpAction"]): boolean {
  return action?.status === "ready" || action?.status === "confirming";
}

function viewFromPath(): DashboardView {
  const route = window.location.pathname.split("/").filter(Boolean).pop();
  if (route === "dashboard") return "home";
  if (route === "home" || route === "accounts") return "home";
  if (route === "settings") return "settings";
  if (route === "deposit" || route === "withdrawal" || route === "transfer") return route === "withdrawal" ? "withdraw" : route;
  return "history";
}

/** Composes the banking workspace from focused, presentation-only sections. */
export function DashboardPage(props: DashboardPageProps) {
  const { user, activeAccount, operationMode, onOperationModeChange } = props;
  const [assistantOpen, setAssistantOpen] = useState(false);
  const [view, setView] = useState<DashboardView>(() => viewFromPath());
  const [navigationNotice, setNavigationNotice] = useState("");

  useEffect(() => {
    const syncView = () => setView(viewFromPath());
    if (window.location.pathname === "/dashboard" || window.location.pathname === "/dashboard/") window.history.replaceState({}, "", "/dashboard/home");
    else if (!window.location.pathname.startsWith("/dashboard/")) window.history.replaceState({}, "", "/dashboard/history");
    window.addEventListener("popstate", syncView);
    return () => window.removeEventListener("popstate", syncView);
  }, []);

  useEffect(() => {
    if (isOperationView(view) && operationMode !== view) onOperationModeChange(view);
  }, [onOperationModeChange, operationMode, view]);

  function navigate(nextView: DashboardView) {
    if (isOperationView(nextView) && isPendingMCPAction(props.mcpAction) && nextView !== view) {
      setNavigationNotice("Tienes una operación pendiente. Confírmala o cancélala en el asistente antes de iniciar otra.");
      setAssistantOpen(true);
      return;
    }
    setNavigationNotice("");
    if (isOperationView(nextView)) props.onOperationModeChange(nextView);
    const route = nextView === "home" ? "home" : nextView === "withdraw" ? "withdrawal" : nextView;
    window.history.pushState({}, "", `/dashboard/${route}`);
    setView(nextView);
  }

  return (
    <main className="dashboard-page min-h-screen text-ink">
      <DashboardHeader user={user} view={view} onLogout={props.onLogout} onNavigate={navigate} />
      <div className="dashboard-container mx-auto space-y-6 px-4 py-5 sm:px-8 sm:py-8">
        {props.dashboardError && <p className="status-message status-error" role="alert">{props.dashboardError}</p>}
        {navigationNotice && <p className="status-message status-error" role="alert">{navigationNotice}</p>}
        {view === "home" && <AccountsHome {...props} />}
        {view === "history" && <TransactionHistoryPanel {...props} />}
        {isOperationView(view) && <OperationPage {...props} mode={view} onNavigate={navigate} />}
        {view === "settings" && <SettingsPanel {...props} />}
        <MCPPanel {...props} chatOpen={assistantOpen} onChatOpenChange={setAssistantOpen} onNavigateSettings={() => navigate("settings")} />
        <footer className="border-t border-slate-200 pt-5 text-xs text-slate-500">Tu información está protegida y tus movimientos quedan siempre disponibles para ti.</footer>
        {!activeAccount && <p className="sr-only">No hay una cuenta activa seleccionada.</p>}
      </div>
    </main>
  );
}

type NavIconName = "history" | "transfer" | "deposit" | "withdraw" | "settings";

function NavIcon({ name }: { name: NavIconName }) {
  const paths: Record<NavIconName, string> = {
    history: "M5 5h14M5 12h14M5 19h9",
    transfer: "M7 7h10l-3-3M17 17H7l3 3M17 7l-3 3M7 17l3-3",
    deposit: "M12 4v12m0 0 4-4m-4 4-4-4M5 20h14",
    withdraw: "M12 20V8m0 0 4 4m-4-4-4 4M5 4h14",
    settings: "M12 8.5a3.5 3.5 0 1 0 0 7 3.5 3.5 0 0 0 0-7Zm0-5v2m0 13v2m8.5-8.5h-2m-13 0h-2m12.02-6.02-1.42 1.42M7.9 16.1l-1.42 1.42m11.04 0-1.42-1.42M7.9 7.9 6.48 6.48",
  };
  return <svg className="nav-icon" aria-hidden="true" viewBox="0 0 24 24" fill="none"><path d={paths[name]} stroke="currentColor" strokeLinecap="round" strokeLinejoin="round" strokeWidth="1.8" /></svg>;
}

function ChevronIcon({ direction = "down" }: { direction?: "up" | "down" }) {
  return <svg className="friendly-icon" aria-hidden="true" viewBox="0 0 24 24" fill="none"><path d={direction === "up" ? "m6 14 6-6 6 6" : "m6 10 6 6 6-6"} stroke="currentColor" strokeLinecap="round" strokeLinejoin="round" strokeWidth="2" /></svg>;
}

function MoreIcon() {
  return <svg className="friendly-icon" aria-hidden="true" viewBox="0 0 24 24" fill="currentColor"><circle cx="12" cy="5" r="1.6" /><circle cx="12" cy="12" r="1.6" /><circle cx="12" cy="19" r="1.6" /></svg>;
}

function PlusIcon() {
  return <svg className="friendly-icon" aria-hidden="true" viewBox="0 0 24 24" fill="none"><circle cx="12" cy="12" r="8.5" stroke="currentColor" strokeWidth="1.8" /><path d="M12 8v8M8 12h8" stroke="currentColor" strokeLinecap="round" strokeWidth="1.8" /></svg>;
}

function EyeIcon({ visible }: { visible: boolean }) {
  return <svg className="friendly-icon" aria-hidden="true" viewBox="0 0 24 24" fill="none"><path d="M3.5 12s3.1-5 8.5-5 8.5 5 8.5 5-3.1 5-8.5 5-8.5-5-8.5-5Z" stroke="currentColor" strokeLinecap="round" strokeLinejoin="round" strokeWidth="1.8" />{visible ? <circle cx="12" cy="12" r="2" fill="currentColor" /> : <path d="m5 5 14 14" stroke="currentColor" strokeLinecap="round" strokeWidth="1.8" />}</svg>;
}

function DashboardHeader({ user, view, onLogout, onNavigate }: { user: User; view: DashboardView; onLogout: () => void; onNavigate: (view: DashboardView) => void }) {
  const links: Array<{ view: DashboardView; label: string }> = [
    { view: "home", label: "Cuentas" },
    { view: "history", label: "Consultas" },
    { view: "transfer", label: "Transacciones" },
    { view: "deposit", label: "Depositar" },
    { view: "withdraw", label: "Retirar" },
    { view: "settings", label: "Configuraciones" },
  ];

  return <header className="dashboard-header">
    <div className="dashboard-header-inner">
      <div className="dashboard-header-bar">
        <HyperBankWordmark className="dashboard-brand dashboard-brand-light" />
        <div className="dashboard-header-account"><div><strong>{user.full_name}</strong><small>{user.email}</small></div><button className="dashboard-navbar-logout" onClick={onLogout} type="button">Cerrar sesión</button></div>
      </div>

      <nav className="dashboard-header-nav" aria-label="Navegación principal">
        {links.map((link) => <a className={`dashboard-header-nav-link ${view === link.view ? "dashboard-header-nav-link-active" : ""}`} href={`/dashboard/${link.view === "home" ? "home" : link.view === "withdraw" ? "withdrawal" : link.view}`} key={link.view} onClick={(event) => { event.preventDefault(); onNavigate(link.view); }}>{link.view !== "home" && <NavIcon name={link.view} />}{link.label}</a>)}
      </nav>
    </div>
  </header>;
}

function accountDisplayName(account: Account, index: number): string {
  if (account.display_name) return account.display_name;
  if (account.type === "checking") return index === 0 ? "Cuenta principal" : `Cuenta corriente ${index + 1}`;
  return "Cuenta bancaria";
}

function maskAccountNumber(accountId: string): string {
  if (accountId.length <= 10) return accountId;
  return `${accountId.slice(0, 4)}••••${accountId.slice(-4)}`;
}

function AccountsHome(props: DashboardPageProps) {
  const activeId = props.activeAccount?.id;
  const activeBalance = props.balance?.available_balance ?? "0";
  const [showAccountNumbers, setShowAccountNumbers] = useState(false);
  const [editingAccountId, setEditingAccountId] = useState("");
  const [editingName, setEditingName] = useState("");
  const totalBalance = useMemo(() => Object.values(props.accountBalances).reduce((total, current) => { try { return total + BigInt(current.available_balance); } catch { return total; } }, 0n).toString(), [props.accountBalances]);
  function startRename(account: Account, index: number) { setEditingAccountId(account.id); setEditingName(accountDisplayName(account, index)); }
  return <section className="bank-home space-y-6">
    <div className="bank-page-heading"><h1>Cuentas personales</h1><button className="account-visibility-button" aria-label={showAccountNumbers ? "Ocultar números de cuenta" : "Mostrar números de cuenta"} onClick={() => setShowAccountNumbers((current) => !current)} type="button"><EyeIcon visible={showAccountNumbers} /></button></div>
    <div className="bank-account-section surface">
      <div className="bank-table-header"><span><ChevronIcon direction="up" /> Cuentas de depósito</span><span>Saldo capital</span><span>Disponible</span><span aria-hidden="true" /></div>
      {props.accounts.map((account, index) => {
        const selected = account.id === activeId;
        return <div className={`bank-account-row ${selected ? "bank-account-row-active" : ""}`} key={account.id}>
          <button className="bank-account-name" onClick={() => props.onAccountChange(account.id)} type="button">
            <span className="bank-row-chevron"><ChevronIcon /></span>
            <span><strong>{accountDisplayName(account, index)}</strong><small title={account.id}>{showAccountNumbers ? account.id : maskAccountNumber(account.id)}</small></span>
          </button>
          <span className="bank-account-amount">{props.accountBalances[account.id] ? formatMinorAmount(props.accountBalances[account.id].balance) : selected ? formatMinorAmount(activeBalance) : "—"}</span>
          <span className="bank-account-amount">{props.accountBalances[account.id] ? formatMinorAmount(props.accountBalances[account.id].available_balance) : selected ? formatMinorAmount(activeBalance) : "—"}</span>
          <button className="bank-row-menu" aria-label={`Editar ${accountDisplayName(account, index)}`} onClick={() => startRename(account, index)} type="button"><MoreIcon /></button>
        </div>;
      })}
      {editingAccountId && <div className="account-rename-panel"><label><span className="field-label">Nombre de la cuenta</span><input value={editingName} maxLength={48} onChange={(event) => setEditingName(event.target.value)} /></label><div className="operation-actions"><button className="secondary-button" onClick={() => setEditingAccountId("")} type="button">Cancelar</button><button className="primary-button" disabled={props.accountRenameBusyId === editingAccountId} onClick={() => { props.onRenameAccount(editingAccountId, editingName); setEditingAccountId(""); }} type="button">{props.accountRenameBusyId === editingAccountId ? "Guardando…" : "Guardar nombre"}</button><button className="secondary-button danger-button" disabled={props.accountDeleteBusyId === editingAccountId || props.accountBalances[editingAccountId]?.available_balance !== "0" || props.accountBalances[editingAccountId]?.credits_pending !== "0" || props.accountBalances[editingAccountId]?.debits_pending !== "0"} onClick={() => props.onDeleteAccount(editingAccountId)} type="button">{props.accountDeleteBusyId === editingAccountId ? "Cerrando…" : "Cerrar cuenta"}</button></div><small className="field-help">Solo se puede cerrar cuando el saldo USD y los movimientos pendientes están en cero.</small></div>}
      <div className="bank-account-footer"><button className="bank-add-account" disabled={props.accountBusy} onClick={props.onCreateAccount} type="button"><PlusIcon /> {props.accountBusy ? "Abriendo cuenta…" : "Abrir nueva cuenta"}</button></div>
      <div className="bank-account-total"><span>Total de cuentas</span><strong>{Object.keys(props.accountBalances).length ? formatMinorAmount(totalBalance) : "—"}</strong><strong>{Object.keys(props.accountBalances).length ? formatMinorAmount(totalBalance) : "—"}</strong><span /></div>
    </div>
    {props.accountNotice && <p className={`status-message ${props.accountNotice.tone === "error" ? "status-error" : "status-success"}`} role="alert">{props.accountNotice.message}</p>}
  </section>;
}

function SettingsPanel(props: DashboardPageProps) {
  return <section className="dashboard-settings-page surface p-5 sm:p-8"><div className="dashboard-page-title"><p className="dashboard-kicker">Configuraciones</p><h2>Tu perfil y seguridad</h2><p>Actualiza tus datos personales y revisa la protección de tu cuenta.</p></div><div className="theme-settings" aria-labelledby="theme-settings-title"><div><p className="dashboard-kicker">Apariencia</p><h3 id="theme-settings-title">Tema de la aplicación</h3><p>Usa el tema del dispositivo o elige una apariencia.</p></div><div className="theme-options" role="group" aria-label="Tema de la aplicación">{([['system','Sistema'],['light','Claro'],['dark','Oscuro']] as const).map(([mode,label]) => <button className={props.themeMode === mode ? "theme-option-active" : ""} key={mode} onClick={() => props.onThemeModeChange(mode)} type="button">{label}</button>)}</div></div><form className="profile-form" onSubmit={props.onProfileSubmit}><label><span className="field-label">Nombre completo</span><input autoComplete="name" required minLength={2} maxLength={120} value={props.profileFullName} onChange={(event) => props.onProfileNameChange(event.target.value)} /></label><label><span className="field-label">Correo electrónico</span><input autoComplete="email" value={props.user.email} disabled /></label><p className="profile-help">El correo no se modifica desde aquí porque requiere un proceso de verificación.</p>{props.profileNotice && <p className={`status-message ${props.profileNotice.tone === "error" ? "status-error" : "status-success"}`} role="alert">{props.profileNotice.message}</p>}<button className="primary-button" disabled={props.profileBusy} type="submit">{props.profileBusy ? "Guardando…" : "Guardar cambios"}</button></form><div className="settings-grid"><article><span className="settings-icon">✓</span><div><h3>Autenticación multifactor</h3><p>Tu cuenta tiene una segunda capa de protección activa.</p></div><strong>Activa</strong></article><article><span className="settings-icon">⌁</span><div><h3>Sesión protegida</h3><p>La sesión se renueva de forma segura mientras utilizas la aplicación.</p></div><strong>Segura</strong></article></div><form className="mcp-pin-settings" onSubmit={props.onSetMCPPIN}><div><p className="dashboard-kicker">Confirmaciones</p><h3>PIN del asistente</h3><p>Se solicita solo al confirmar una operación y vence automáticamente en tres minutos.</p></div><label><span className="field-label">PIN de 4 dígitos</span><input autoComplete="new-password" inputMode="numeric" maxLength={4} pattern="[0-9]{4}" type="password" value={props.mcpPin} onChange={(event) => props.onMCPPINChange(event.target.value)} placeholder="••••" /></label>{props.mcpPinNotice && <p className={`status-message ${props.mcpPinNotice.tone === "error" ? "status-error" : "status-success"}`} role="alert">{props.mcpPinNotice.message}</p>}<button className="primary-button" disabled={props.mcpPinBusy} type="submit">{props.mcpPinBusy ? "Guardando…" : props.mcpPinConfigured ? "Renovar PIN" : "Crear PIN"}</button></form></section>;
}

type OperationStep = "details" | "confirm" | "receipt";

function OperationPage(props: DashboardPageProps & { mode: OperationMode; onNavigate: (view: DashboardView) => void }) {
  const [step, setStep] = useState<OperationStep>("details");
  const [submittedAmount, setSubmittedAmount] = useState("");
  const labels: Record<OperationMode, { kicker: string; title: string; description: string }> = {
    deposit: { kicker: "Depositar", title: "Agrega fondos a tu cuenta", description: "Registra un depósito y revisa el resultado en tu historial." },
    withdraw: { kicker: "Retirar", title: "Retira fondos disponibles", description: "Solicita un retiro de forma clara y segura." },
    transfer: { kicker: "Transferir", title: "Envía dinero a otra cuenta", description: "Revisa los datos de destino antes de confirmar." },
  };
  const content = labels[props.mode];
  useEffect(() => {
    if (props.operationNotice?.tone === "success") setStep("receipt");
  }, [props.operationNotice]);

  function submitStep(event: FormEvent<HTMLFormElement>) {
    if (step === "details") {
      event.preventDefault();
      setStep("confirm");
      return;
    }
    setSubmittedAmount(props.operationAmount);
    props.onOperation(event);
  }

  return <section id={`${props.mode}-page`} className="dashboard-operation-page surface p-5 sm:p-8">
    <div className="dashboard-page-title">
      <p className="dashboard-kicker">{content.kicker}</p>
      <h2>{content.title}</h2>
      <p>{content.description}</p>
    </div>
    <OperationSteps step={step} />
    {step === "receipt" ? <OperationReceipt {...props} amount={submittedAmount || props.operationAmount} onNavigate={props.onNavigate} /> : <OperationsPanel {...props} step={step} onSubmit={submitStep} onBack={() => setStep("details")} onCancel={() => props.onNavigate("history")} onNavigate={props.onNavigate} />}
  </section>;
}

function OperationSteps({ step }: { step: OperationStep }) {
  const steps: Array<{ id: OperationStep; label: string }> = [{ id: "details", label: "Completar datos" }, { id: "confirm", label: "Confirmar" }, { id: "receipt", label: "Comprobante" }];
  return <div className="operation-steps" aria-label="Progreso de la operación">{steps.map((item, index) => <div className={`operation-step ${step === item.id ? "operation-step-current" : ""} ${index < steps.findIndex((current) => current.id === step) ? "operation-step-complete" : ""}`} key={item.id}><span>{index + 1}</span><small>{item.label}</small></div>)}</div>;
}

function OperationsPanel(props: DashboardPageProps & { step: OperationStep; onSubmit: (event: FormEvent<HTMLFormElement>) => void; onBack: () => void; onCancel: () => void; onNavigate: (view: DashboardView) => void }) {
  const isExternalTransfer = props.operationMode === "transfer" && props.transferTargetType === "external";
  return <div className="dashboard-operation-card mt-6 p-0">
    {props.step === "confirm" ? <div className="operation-confirmation"><p className="dashboard-kicker">Revisión</p><h3>Confirma los datos de tu operación</h3><dl><div><dt>Cuenta origen</dt><dd>{props.activeAccount?.id ?? "—"}</dd></div>{props.operationMode === "transfer" && <div><dt>Cuenta destino</dt><dd>{props.destinationAccountId || "—"}</dd></div>}<div><dt>Monto</dt><dd>{formatMinorAmount(props.operationAmount)}</dd></div><div><dt>Tipo</dt><dd>{props.operationMode === "deposit" ? "Depósito" : props.operationMode === "withdraw" ? "Retiro" : "Transferencia"}</dd></div></dl>{isExternalTransfer && <label className="transfer-pin-field"><span className="field-label">PIN de confirmación</span><input required inputMode="numeric" maxLength={4} pattern="[0-9]{4}" type="password" value={props.transferConfirmationPin} onChange={(event) => props.onTransferConfirmationPinChange(event.target.value)} placeholder="••••" />{!props.mcpPinConfigured && <small className="field-help">Configura primero tu PIN en Configuraciones.</small>}</label>}{props.operationNotice && <p className={`status-message ${props.operationNotice.tone === "error" ? "status-error" : "status-success"}`} role="alert">{props.operationNotice.message}</p>}<form onSubmit={props.onSubmit}><div className="operation-actions"><button className="secondary-button" onClick={props.onBack} type="button">Volver</button>{isExternalTransfer && !props.mcpPinConfigured && <button className="secondary-button" onClick={() => props.onNavigate("settings")} type="button">Crear PIN</button>}<button className="primary-button" disabled={props.operationBusy || !props.activeAccount || (isExternalTransfer && (!props.mcpPinConfigured || !/^\d{4}$/u.test(props.transferConfirmationPin)))} type="submit">{props.operationBusy ? "Procesando…" : "Confirmar operación"}</button></div></form></div> : <>
    <p className="text-sm font-bold uppercase tracking-[0.18em] text-slate-400">{props.operationMode === "transfer" ? "Tu transferencia" : props.operationMode === "deposit" ? "Tu depósito" : "Tu retiro"}</p>
    <h2 className="mt-2 text-xl font-semibold">{props.operationMode === "transfer" ? "¿A dónde quieres enviar tu dinero?" : props.operationMode === "deposit" ? "Pon tu dinero en orden" : "Retira con tranquilidad"}</h2>
      <form className="mt-6 space-y-5" onSubmit={props.onSubmit}>
        <label>
          <span className="field-label">Cuenta origen</span>
        <select aria-label="Cuenta origen" value={props.activeAccount?.id ?? ""} onChange={(event) => props.onAccountChange(event.target.value)}>{props.accounts.map((account) => <option key={account.id} value={account.id}>{maskAccountNumber(account.id)} · {account.type}</option>)}</select>
        </label>
        <label>
          <span className="field-label">Monto en USD</span>
          <input required inputMode="decimal" type="text" pattern="[0-9]+([.,][0-9]{1,2})?" value={props.operationAmount}
            onChange={(event) => props.onAmountChange(sanitizeCurrencyInput(event.target.value))} placeholder="10.00" aria-describedby="amount-help" />
          <small id="amount-help" className="field-help">Puedes escribir, por ejemplo, 10.50 o 10,50.</small>
        </label>
        {props.operationMode === "transfer" && <>
          <div className="transfer-target-switch" role="tablist" aria-label="Tipo de cuenta destino">
            <button className={props.transferTargetType === "own" ? "active" : ""} onClick={() => props.onTransferTargetTypeChange("own")} type="button">Entre mis cuentas</button>
            <button className={props.transferTargetType === "external" ? "active" : ""} onClick={() => props.onTransferTargetTypeChange("external")} type="button">A otra cuenta</button>
          </div>
          {isExternalTransfer ? <label>
            <span className="field-label">Cuenta destino externa</span>
            <input required value={props.destinationAccountId} onChange={(event) => props.onDestinationChange(event.target.value.trim())} placeholder="UUID de la cuenta destino" autoComplete="off" />
            <small className="field-help">Validaremos que exista y esté activa antes de solicitar la confirmación.</small>
          </label> : <label>
            <span className="field-label">Cuenta destino propia</span>
            <select required value={props.destinationAccountId} onChange={(event) => props.onDestinationChange(event.target.value)}>
              <option value="">Selecciona una cuenta</option>
              {props.accounts.filter((account) => account.id !== props.activeAccount?.id).map((account) => <option key={account.id} value={account.id}>{maskAccountNumber(account.id)} · {accountDisplayName(account, props.accounts.indexOf(account))}</option>)}
            </select>
            {props.accounts.length < 2 && <small className="field-help">Abre otra cuenta desde Cuentas para realizar una transferencia entre tus cuentas.</small>}
          </label>}
        </>}
      <div className="rounded-2xl bg-slate-50 p-4 text-xs leading-5 text-slate-500">{props.operationMode === "deposit" ? "Elige la cuenta que quieres fortalecer y escribe el monto que deseas agregar."
        : props.operationMode === "withdraw" ? "Elige tu cuenta, indica el monto y conserva el control de tus fondos." : "Elige desde dónde enviar y revisa la cuenta destino antes de avanzar."}
        </div><div className="operation-actions"><button className="secondary-button" onClick={props.onCancel} type="button">Cancelar</button><button className="primary-button" disabled={props.operationBusy || !props.activeAccount} type="submit">Siguiente</button></div>
      </form></>}
  </div>;
}

function OperationReceipt(props: DashboardPageProps & { mode: OperationMode; amount: string; onNavigate: (view: DashboardView) => void }) {
  return <div className="operation-receipt surface"><div className="operation-receipt-icon" aria-hidden="true">✓</div><p className="dashboard-kicker">Comprobante</p><h3>Operación realizada</h3><p>{props.operationNotice?.message ?? "Tu operación fue procesada correctamente."}</p><dl><div><dt>Tipo</dt><dd>{props.mode === "deposit" ? "Depósito" : props.mode === "withdraw" ? "Retiro" : "Transferencia"}</dd></div><div><dt>Monto</dt><dd>{formatMinorAmount(props.amount)}</dd></div><div><dt>Fecha</dt><dd>{new Date().toLocaleString("es-PA")}</dd></div></dl><button className="primary-button" onClick={() => props.onNavigate("history")} type="button">Ver historial</button></div>;
}

function TransactionHistoryPanel(props: DashboardPageProps) {
  const [filter, setFilter] = useState<"all" | "deposit" | "withdrawal" | "transfer">("all");
  const [fromDate, setFromDate] = useState("");
  const [toDate, setToDate] = useState("");
  const transactions = props.history?.items.slice(0, 5) ?? [];
  const filteredTransactions = transactions.filter((transaction) => {
    const transactionDate = transaction.created_at.slice(0, 10);
    return (filter === "all" || transaction.type === filter) && (!fromDate || transactionDate >= fromDate) && (!toDate || transactionDate <= toDate);
  });
  return <section id="history" className="dashboard-history-panel surface p-5 sm:p-7">
    <div className="history-heading-row"><div><p className="dashboard-kicker">Consultas</p><h2>Historial de transacciones</h2><p>Consulta, filtra y descarga los movimientos de tu cuenta.</p></div><button className="secondary-button" disabled={props.exportBusy || !props.activeAccount} onClick={props.onExport} type="button">{props.exportBusy ? "Preparando…" : "Imprimir / descargar"}</button></div>
    <form className="history-filter-panel" onSubmit={(event) => event.preventDefault()}><label><span>Desde</span><input type="date" value={fromDate} onChange={(event) => setFromDate(event.target.value)} /></label><label><span>Hasta</span><input type="date" value={toDate} onChange={(event) => setToDate(event.target.value)} /></label><label><span>Tipo de transacción</span><select value={filter} onChange={(event) => setFilter(event.target.value as typeof filter)}><option value="all">TODOS</option><option value="deposit">DEPÓSITOS</option><option value="withdrawal">RETIROS</option><option value="transfer">TRANSFERENCIAS</option></select></label><button className="primary-button" type="submit">Buscar</button></form>
    {!props.history && props.dashboardLoading ? <div className="mt-6 space-y-3"><div className="skeleton h-12 w-full" /><div className="skeleton h-12 w-full" /></div> : filteredTransactions.length ? <div className="transaction-table" role="table" aria-label="Historial de transacciones"><div className="transaction-table-header" role="row"><span>Fecha</span><span>Destino / tipo</span><span>Descripción</span><span>Estado</span><span>Monto</span></div>{filteredTransactions.map((transaction) => <TransactionRow key={`${transaction.transfer_id}-${transaction.created_at}`} transaction={transaction} />)}</div> : <div className="mt-6 rounded-2xl bg-slate-50 p-6 text-center text-sm text-slate-500">{filter === "all" ? "Todavía no hay transacciones en esta cuenta." : "No hay transacciones con estos filtros en la página actual."}</div>}
    {props.history && <HistoryPagination page={props.historyPage} hasMore={props.historyHasMore} busy={props.historyPageBusy} onPrevious={props.onPreviousHistory} onNext={props.onNextHistory} />}
    {filteredTransactions.length > 0 && <section className="history-chart-section"><div><p className="dashboard-kicker">Resumen de la página</p><h3>Movimientos recientes</h3><p>La gráfica representa los movimientos de la página actual.</p></div><ActivityChart transactions={filteredTransactions} /></section>}
  </section>;
}

function ActivityChart({ transactions }: { transactions: Transaction[] }) {
  const points = transactions.slice(0, 5).reverse();
  if (!points.length) return null;
  const maximum = points.reduce((current, transaction) => { try { const amount = BigInt(transaction.amount); return amount > current ? amount : current; } catch { return current; } }, 0n);
  const credits = points.filter((transaction) => transaction.direction === "credit").length;
  return <div className="activity-chart" aria-label="Gráfico de importes de los últimos movimientos" role="img"><div className="activity-chart-header"><span>Importe por movimiento</span><span>{credits} entradas · {points.length - credits} salidas</span></div><div className="activity-chart-plot"><div className="activity-chart-gridline activity-chart-gridline-top" /><div className="activity-chart-gridline activity-chart-gridline-middle" /><div className="activity-chart-bars">{points.map((transaction) => { let amount = 0n; try { amount = BigInt(transaction.amount); } catch { /* Ignore malformed display data. */ } const percentage = maximum > 0n ? Number((amount * 100n) / maximum) : 0; const shortDate = new Date(transaction.created_at).toLocaleDateString("es-PA", { day: "2-digit", month: "short" }); return <div className="activity-chart-column" key={`${transaction.transfer_id}-chart`}><span className="activity-chart-value">{formatMinorAmount(transaction.amount)}</span><div className="activity-chart-track"><div className={`activity-chart-bar ${transaction.direction === "credit" ? "activity-chart-bar-credit" : "activity-chart-bar-debit"}`} style={{ height: `${Math.max(10, percentage)}%` }} /></div><span className="activity-chart-date">{shortDate}</span></div>; })}</div></div><div className="activity-chart-legend"><span><i className="activity-legend-dot activity-legend-credit" />Entradas</span><span><i className="activity-legend-dot activity-legend-debit" />Salidas</span></div></div>;
}

function TransactionRow({ transaction }: { transaction: Transaction }) {
  const label = transactionLabel(transaction);
  return <div className="transaction-table-row" role="row"><span className="transaction-date"><ChevronIcon /> {new Date(transaction.created_at).toLocaleDateString("es-PA")}</span><span><strong>{transaction.type === "transfer" ? "Cuenta destino" : "Cuenta activa"}</strong><small>{label}</small></span><span><strong>{label}</strong><small>Referencia: {transaction.transfer_id.slice(0, 8)}…</small></span><span className="transaction-status">REGISTRADA</span><strong className={`transaction-amount ${transaction.direction === "credit" ? "transaction-credit" : "transaction-debit"}`}>{transaction.direction === "credit" ? "+" : "−"}{formatMinorAmount(transaction.amount)}</strong></div>;
}

type ChatMessage = {
  id: string;
  role: "user" | "assistant";
  text: string;
  data?: unknown;
  confirmation?: boolean;
  accountOptions?: MCPAccountOption[];
};

interface MCPPanelProps extends DashboardPageProps {
  chatOpen: boolean;
  onChatOpenChange: (open: boolean) => void;
  onNavigateSettings: () => void;
}

function readableDataLabel(key: string): string {
  return key.replace(/_/g, " ").replace(/\b\w/g, (character) => character.toUpperCase());
}

function assistantAccountLabel(value: unknown): string {
  if (typeof value !== "string") return "Cuenta seleccionada";
  return value.length > 12 ? `${value.slice(0, 6)}…${value.slice(-4)}` : value;
}

function assistantMoney(value: unknown): string {
  if (typeof value !== "string" && typeof value !== "number") return "—";
  try { return formatMinorAmount(String(value)); } catch { return "—"; }
}

function assistantActionLabel(action: string): string {
  if (action === "deposit") return "Depósito";
  if (action === "withdrawal") return "Retiro";
  return "Transferencia";
}

function isAssistantRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === "object" && value !== null && !Array.isArray(value);
}

function assistantStatusLabel(value: unknown): string {
  if (value === "succeeded" || value === "confirmed") return "Completada";
  if (value === "ready") return "Lista para confirmar";
  return "Registrada";
}

function AssistantDataView({ data, accountBalances = {} }: { data: unknown; accountBalances?: Record<string, Balance> }) {
  if (data === null || data === undefined) return null;
  if (isAssistantRecord(data) && Array.isArray(data.items)) {
    if (!data.items.length) return <p className="assistant-data-empty">No hay información para mostrar.</p>;
    const first = data.items[0];
    if (isAssistantRecord(first) && "transfer_id" in first) return <div className="assistant-transaction-list">{data.items.slice(0, 5).map((item, index) => { const transaction = item as Record<string, unknown>; return <div className="assistant-transaction-item" key={String(transaction.transfer_id ?? index)}><div><strong>{readableDataLabel(String(transaction.type ?? "Movimiento"))}</strong><small>{transaction.created_at ? new Date(String(transaction.created_at)).toLocaleDateString("es-PA") : "Fecha no disponible"}</small></div><strong className={transaction.direction === "credit" ? "transaction-credit" : "transaction-debit"}>{transaction.direction === "credit" ? "+" : "−"}{assistantMoney(transaction.amount)}</strong></div>; })}</div>;
    return <div className="assistant-account-list">{data.items.slice(0, 5).map((item, index) => { const account = item as Record<string, unknown>; const accountID = String(account.id ?? ""); const balance = accountBalances[accountID]; return <div className="assistant-account-item" key={accountID || index}><div><strong>{String(account.display_name || `Cuenta ${index + 1}`)}</strong><small>{assistantAccountLabel(account.id)} · {String(account.currency ?? "USD")}</small></div><div className="assistant-account-balance"><strong>{balance ? assistantMoney(balance.available_balance) : "Saldo pendiente"}</strong><span>{String(account.status ?? "Activa")}</span></div></div>; })}</div>;
  }
  if (isAssistantRecord(data) && "transfer_id" in data && "amount" in data && "type" in data) return <div className="assistant-operation-receipt"><div className="assistant-operation-heading"><span>{assistantActionLabel(String(data.type))}</span><strong>{assistantStatusLabel(data.status)}</strong></div><div className="assistant-operation-amount">{data.direction === "debit" ? "−" : "+"}{assistantMoney(data.amount)}</div><div className="assistant-operation-details"><span>Moneda<strong>{String(data.currency ?? "USD")}</strong></span><span>Referencia<strong>{assistantAccountLabel(data.transfer_id)}</strong></span><span>Fecha<strong>{data.created_at ? new Date(String(data.created_at)).toLocaleString("es-PA", { dateStyle: "medium", timeStyle: "short" }) : "Pendiente"}</strong></span></div></div>;
  if (isAssistantRecord(data) && "available_balance" in data && "account_id" in data) return <div className="assistant-balance-summary"><div><span>Saldo disponible</span><strong>{assistantMoney(data.available_balance)}</strong></div><small>Cuenta {assistantAccountLabel(data.account_id)}</small></div>;
  if (Array.isArray(data)) return <ul className="assistant-data-list">{data.map((item, index) => <li key={index}><AssistantDataView data={item} accountBalances={accountBalances} /></li>)}</ul>;
  if (isAssistantRecord(data)) return <div className="assistant-data-grid">{Object.entries(data).filter(([key]) => !key.endsWith("_pending") && !key.endsWith("_posted")).map(([key, value]) => <div className="assistant-data-item" key={key}><span>{readableDataLabel(key)}</span><div className="assistant-data-value">{key.endsWith("_id") ? assistantAccountLabel(value) : <AssistantDataView data={value} accountBalances={accountBalances} />}</div></div>)}</div>;
  return <span>{String(data)}</span>;
}

function MCPActionConfirmation({ props }: { props: MCPPanelProps }) {
  const action = props.mcpAction;
  if (!action || !isPendingMCPAction(action)) return null;
  const recovering = action.status === "confirming";
  return <div className="chatbot-pin-request"><div className="assistant-action-summary"><strong>{assistantActionLabel(action.action)} {recovering ? "en validación" : "preparada"}</strong><span>{assistantMoney(action.payload.amount)}</span><small>{recovering ? "La validación anterior se interrumpió. Puedes reintentar o cancelar cuando haya vencido el bloqueo breve." : "Revisa los datos antes de confirmar."}</small></div><p>La operación necesita tu PIN de cuatro dígitos.</p>{!props.mcpPinConfigured ? <button className="secondary-button" onClick={props.onNavigateSettings} type="button">Crear PIN en configuraciones</button> : <form onSubmit={(event) => { event.preventDefault(); props.onConfirmMCPAction(); }}><input inputMode="numeric" maxLength={4} type="password" value={props.mcpConfirmationPin} onChange={(event) => props.onMCPConfirmationPINChange(event.target.value)} placeholder="••••" aria-label="PIN de confirmación" /><button className="primary-button" disabled={props.mcpActionBusy} type="submit">{props.mcpActionBusy ? "Validando…" : recovering ? "Reintentar confirmación" : "Confirmar"}</button></form>}<button className="chatbot-cancel-button" disabled={props.mcpActionBusy} onClick={props.onCancelMCPAction} type="button">Cancelar operación</button>{props.mcpActionNotice && <p className={`status-message ${props.mcpActionNotice.tone === "error" ? "status-error" : "status-success"}`} role="alert">{props.mcpActionNotice.message}</p>}</div>;
}

function MCPPanel(props: MCPPanelProps) {
  const { chatOpen, onChatOpenChange } = props;
  const [chatMessages, setChatMessages] = useState<ChatMessage[]>([]);

  useEffect(() => {
    const reply = props.assistantReply;
    if (!reply) return;
    setChatMessages((current) => {
      const last = current[current.length - 1];
      if (last?.role === "assistant" && last.id === reply.id) return current;
      return [...current, { id: reply.id, role: "assistant", text: reply.message, data: reply.data, confirmation: reply.confirmation, accountOptions: reply.accountOptions }];
    });
  }, [props.assistantReply]);

  function submitChat(event: FormEvent<HTMLFormElement>) {
    const message = props.assistantInput.trim();
    if (message) setChatMessages((current) => [...current, { id: `user-${Date.now()}-${current.length}`, role: "user", text: message }]);
    props.onAssistantSubmit(event);
  }

  useEffect(() => {
    if (!chatOpen) return;
    function closeChatOnEscape(event: KeyboardEvent) {
      if (event.key === "Escape") onChatOpenChange(false);
    }
    window.addEventListener("keydown", closeChatOnEscape);
    return () => window.removeEventListener("keydown", closeChatOnEscape);
  }, [chatOpen, onChatOpenChange]);

  return <section id="assistant" className="chatbot-shell" aria-label="Asistente Hyper Bank">
    <span className="chatbot-fab-label" aria-hidden="true">Asistente</span>
    <button className="chatbot-fab" aria-expanded={props.chatOpen} aria-controls="hyper-bank-chat" aria-label={props.chatOpen ? "Cerrar asistente Hyper Bank" : "Abrir asistente Hyper Bank"} onClick={() => props.onChatOpenChange(!props.chatOpen)} type="button">
      <svg aria-hidden="true" viewBox="0 0 24 24" fill="none"><path d="M5 6.75A2.75 2.75 0 0 1 7.75 4h8.5A2.75 2.75 0 0 1 19 6.75v6.5A2.75 2.75 0 0 1 16.25 16H11l-4.25 3v-3.1A2.75 2.75 0 0 1 5 13.25v-6.5Z" stroke="currentColor" strokeWidth="1.8" strokeLinejoin="round" /><path d="M8.5 9.8h7M8.5 12.2h4.5" stroke="currentColor" strokeWidth="1.5" strokeLinecap="round" /></svg>
      <span className="chatbot-fab-dot" aria-hidden="true" />
    </button>
    {props.chatOpen && <div id="hyper-bank-chat" className="chatbot-popover" role="dialog" aria-modal="false" aria-labelledby="mcp-title" aria-describedby="mcp-description">
      <div className="chatbot-popover-header"><div><h2 id="mcp-title">Te ayudamos</h2><p id="mcp-description" className="sr-only">Escribe una consulta para recibir ayuda sobre tu cuenta.</p></div><button className="chatbot-popover-close" aria-label="Cerrar chat" onClick={() => props.onChatOpenChange(false)} type="button">×</button></div>
      <div className="mcp-chat-window chatbot-chat-body">
        <div className="mcp-chat-history" aria-live="polite">
          {!chatMessages.length && <div className="mcp-chat-bubble mcp-chat-bubble-assistant"><p>¡Hola! ¿En qué puedo ayudarte?</p></div>}
          {chatMessages.map((message) => <div className={`mcp-chat-bubble ${message.role === "user" ? "mcp-chat-bubble-user" : "mcp-chat-bubble-assistant"}`} key={message.id}><p>{message.text}</p>{message.accountOptions?.length ? <div className="assistant-account-options"><span className="assistant-data-title">Elige una cuenta</span>{message.accountOptions.map((account) => <button className="assistant-account-option" key={account.id} disabled={props.assistantBusy} onClick={() => props.onAssistantAccountSelect(account.id)} type="button"><span><strong>{account.display_name || "Cuenta"}</strong><small>{account.currency} · {assistantAccountLabel(account.id)}</small></span><span aria-hidden="true">›</span></button>)}</div> : null}{message.data !== undefined && <div className="assistant-data-card"><p className="assistant-data-title">Resumen</p><AssistantDataView data={message.data} accountBalances={props.accountBalances} /></div>}{message.confirmation && <MCPActionConfirmation props={props} />}</div>)}
          {isPendingMCPAction(props.mcpAction) && !chatMessages.some((message) => message.confirmation) && <div className="mcp-chat-bubble mcp-chat-bubble-assistant"><MCPActionConfirmation props={props} /></div>}
        </div>
        <form className="mcp-chat-input chatbot-chat-input" onSubmit={submitChat}><label className="sr-only" htmlFor="assistant-message">Consulta al asistente</label><input id="assistant-message" disabled={props.assistantBusy || isPendingMCPAction(props.mcpAction)} value={props.assistantInput} onChange={(event) => props.onAssistantInput(event.target.value)} placeholder={isPendingMCPAction(props.mcpAction) ? "Confirma o cancela la operación…" : "Escribe tu mensaje…"} maxLength={2000} /><button className="chatbot-send-button" aria-label="Enviar mensaje" disabled={props.assistantBusy || isPendingMCPAction(props.mcpAction) || !props.assistantInput.trim()} type="submit">{props.assistantBusy ? "…" : "›"}</button></form>
      </div></div>}
  </section>;
}
