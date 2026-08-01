import { FormEvent, useCallback, useEffect, useMemo, useState } from "react";
import {
  Account,
  ApiError,
  Balance,
  HistoryResponse,
  Operation,
  Transaction,
  User,
  apiClient,
} from "./api";

type AuthMode = "login" | "register";
type OperationMode = "deposit" | "withdraw" | "transfer";

interface Session {
  accessToken: string;
  refreshToken: string;
  user: User;
}

interface FormNotice {
  tone: "error" | "success";
  message: string;
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

function displayError(error: unknown): string {
  if (error instanceof ApiError) {
    if (error.response.code === "demo_deposit_disabled") {
      return "El depósito de demostración está desactivado en este entorno.";
    }
    if (error.response.code === "insufficient_funds") return "Fondos insuficientes para completar la operación.";
    if (error.response.code === "idempotency_key_reused") return "La operación ya existe con otra solicitud.";
    return error.response.error;
  }
  return "No pudimos completar la solicitud. Intenta nuevamente.";
}

function operationLabel(operation: Operation): string {
  if (operation.type === "deposit") return "Depósito acreditado";
  if (operation.type === "withdrawal") return "Retiro realizado";
  return "Transferencia enviada";
}

function transactionLabel(transaction: Transaction): string {
  if (transaction.type === "deposit") return "Depósito";
  if (transaction.type === "withdrawal") return "Retiro";
  return transaction.direction === "credit" ? "Transferencia recibida" : "Transferencia enviada";
}

/** Creates a cryptographically strong idempotency key for a write operation. */
function createIdempotencyKey(): string {
  if (typeof crypto.randomUUID === "function") {
    return crypto.randomUUID();
  }

  const bytes = new Uint8Array(16);
  crypto.getRandomValues(bytes);
  bytes[6] = (bytes[6] & 0x0f) | 0x40;
  bytes[8] = (bytes[8] & 0x3f) | 0x80;

  return Array.from(bytes, (byte) => byte.toString(16).padStart(2, "0"))
    .join("")
    .replace(/^(.{8})(.{4})(.{4})(.{4})(.{12})$/, "$1-$2-$3-$4-$5");
}

function App() {
  const [session, setSession] = useState<Session | null>(null);
  const [authMode, setAuthMode] = useState<AuthMode>("login");
  const [authBusy, setAuthBusy] = useState(false);
  const [authNotice, setAuthNotice] = useState<FormNotice | null>(null);
  const [authForm, setAuthForm] = useState({ email: "", password: "", fullName: "" });
  const [accounts, setAccounts] = useState<Account[]>([]);
  const [selectedAccountId, setSelectedAccountId] = useState("");
  const [balance, setBalance] = useState<Balance | null>(null);
  const [history, setHistory] = useState<HistoryResponse | null>(null);
  const [dashboardLoading, setDashboardLoading] = useState(false);
  const [dashboardError, setDashboardError] = useState("");
  const [operationMode, setOperationMode] = useState<OperationMode>("deposit");
  const [operationAmount, setOperationAmount] = useState("");
  const [destinationAccountId, setDestinationAccountId] = useState("");
  const [operationBusy, setOperationBusy] = useState(false);
  const [operationNotice, setOperationNotice] = useState<FormNotice | null>(null);

  const activeAccount = useMemo(
    () => accounts.find((account) => account.id === selectedAccountId) ?? accounts[0],
    [accounts, selectedAccountId],
  );

  const loadAccounts = useCallback(async () => {
    if (!session) return;
    try {
      const response = await apiClient.listAccounts({ accessToken: session.accessToken });
      setAccounts(response.items);
      setSelectedAccountId((current) => current || response.items[0]?.id || "");
      setDashboardError("");
    } catch (error) {
      setDashboardError(displayError(error));
    }
  }, [session]);

  const loadAccountData = useCallback(async () => {
    if (!session || !selectedAccountId) return;
    setDashboardLoading(true);
    try {
      const [nextBalance, nextHistory] = await Promise.all([
        apiClient.getBalance(selectedAccountId, { accessToken: session.accessToken }),
        apiClient.getHistory(selectedAccountId, { accessToken: session.accessToken, limit: 8 }),
      ]);
      setBalance(nextBalance);
      setHistory(nextHistory);
      setDashboardError("");
    } catch (error) {
      setDashboardError(displayError(error));
    } finally {
      setDashboardLoading(false);
    }
  }, [session, selectedAccountId]);

  useEffect(() => {
    void loadAccounts();
  }, [loadAccounts]);

  useEffect(() => {
    void loadAccountData();
  }, [loadAccountData]);

  async function handleAuth(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    setAuthBusy(true);
    setAuthNotice(null);
    try {
      if (authMode === "register") {
        const registration = await apiClient.register({
          email: authForm.email,
          password: authForm.password,
          full_name: authForm.fullName,
        });
        const tokens = await apiClient.login({ email: authForm.email, password: authForm.password });
        setSession({ accessToken: tokens.access_token, refreshToken: tokens.refresh_token, user: tokens.user });
        setAuthNotice({ tone: "success", message: `Cuenta HNL ${registration.account.status === "active" ? "lista" : "en provisión"}.` });
      } else {
        const tokens = await apiClient.login({ email: authForm.email, password: authForm.password });
        setSession({ accessToken: tokens.access_token, refreshToken: tokens.refresh_token, user: tokens.user });
      }
      setAuthForm({ email: "", password: "", fullName: "" });
    } catch (error) {
      setAuthNotice({ tone: "error", message: displayError(error) });
    } finally {
      setAuthBusy(false);
    }
  }

  async function handleLogout() {
    if (session) await apiClient.logout(session.accessToken).catch(() => undefined);
    setSession(null);
    setAccounts([]);
    setBalance(null);
    setHistory(null);
    setSelectedAccountId("");
  }

  async function handleOperation(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (!session || !activeAccount) return;
    setOperationBusy(true);
    setOperationNotice(null);
    const idempotencyKey = createIdempotencyKey();
    try {
      let operation: Operation;
      if (operationMode === "deposit") {
        operation = await apiClient.deposit(activeAccount.id, { amount: operationAmount, currency: "HNL" }, { accessToken: session.accessToken, idempotencyKey });
      } else if (operationMode === "withdraw") {
        operation = await apiClient.withdraw(activeAccount.id, { amount: operationAmount, currency: "HNL" }, { accessToken: session.accessToken, idempotencyKey });
      } else {
        operation = await apiClient.transfer({ source_account_id: activeAccount.id, destination_account_id: destinationAccountId, amount: operationAmount, currency: "HNL" }, { accessToken: session.accessToken, idempotencyKey });
      }
      setOperationNotice({ tone: "success", message: `${operationLabel(operation)} por ${formatMinorAmount(operation.amount)}.` });
      setOperationAmount("");
      setDestinationAccountId("");
      await loadAccountData();
    } catch (error) {
      setOperationNotice({ tone: "error", message: displayError(error) });
    } finally {
      setOperationBusy(false);
    }
  }

  async function loadMoreHistory() {
    if (!session || !selectedAccountId || !history?.next_cursor) return;
    try {
      const nextPage = await apiClient.getHistory(selectedAccountId, { accessToken: session.accessToken, limit: 8, cursor: history.next_cursor });
      setHistory({ items: [...history.items, ...nextPage.items], has_more: nextPage.has_more, next_cursor: nextPage.next_cursor });
    } catch (error) {
      setDashboardError(displayError(error));
    }
  }

  if (!session) {
    return (
      <main className="min-h-screen px-5 py-8 text-ink sm:px-10 sm:py-12">
        <div className="mx-auto grid max-w-6xl gap-10 lg:grid-cols-[1.05fr_0.95fr] lg:items-center">
          <section className="space-y-7">
            <div>
              <p className="text-sm font-bold uppercase tracking-[0.28em] text-slate-500">Hypernova Banking</p>
              <h1 className="mt-5 max-w-xl text-5xl font-semibold leading-[0.98] tracking-tight sm:text-7xl">Tu dinero, claro y bajo control.</h1>
              <p className="mt-6 max-w-lg text-lg leading-8 text-slate-600">Una cuenta HNL con saldos verificables, operaciones idempotentes e historial transparente.</p>
            </div>
            <div className="flex flex-wrap gap-3 text-sm font-semibold text-slate-600">
              <span className="rounded-full bg-white px-4 py-2 shadow-sm">Ledger TigerBeetle</span>
              <span className="rounded-full bg-white px-4 py-2 shadow-sm">Tokens opacos</span>
              <span className="rounded-full bg-white px-4 py-2 shadow-sm">HNL minor units</span>
            </div>
          </section>

          <section className="surface p-6 sm:p-8">
            <div className="flex rounded-full bg-slate-100 p-1" role="tablist" aria-label="Acceso">
              {(["login", "register"] as AuthMode[]).map((mode) => (
                <button key={mode} className={`flex-1 rounded-full px-4 py-2 text-sm font-bold ${authMode === mode ? "bg-ink text-white" : "text-slate-500"}`} onClick={() => { setAuthMode(mode); setAuthNotice(null); }} role="tab" aria-selected={authMode === mode} type="button">
                  {mode === "login" ? "Iniciar sesión" : "Crear cuenta"}
                </button>
              ))}
            </div>
            <form className="mt-7 space-y-5" onSubmit={handleAuth}>
              {authMode === "register" && <label><span className="field-label">Nombre completo</span><input required minLength={2} value={authForm.fullName} onChange={(event) => setAuthForm({ ...authForm, fullName: event.target.value })} autoComplete="name" /></label>}
              <label><span className="field-label">Email</span><input required type="email" value={authForm.email} onChange={(event) => setAuthForm({ ...authForm, email: event.target.value })} autoComplete="email" /></label>
              <label><span className="field-label">Contraseña</span><input required minLength={8} type="password" value={authForm.password} onChange={(event) => setAuthForm({ ...authForm, password: event.target.value })} autoComplete={authMode === "login" ? "current-password" : "new-password"} /></label>
              {authNotice && <p className={`status-message ${authNotice.tone === "error" ? "status-error" : "status-success"}`} role="alert">{authNotice.message}</p>}
              <button className="primary-button w-full" disabled={authBusy} type="submit">{authBusy ? "Procesando…" : authMode === "login" ? "Entrar al dashboard" : "Crear mi cuenta HNL"}</button>
            </form>
            <p className="mt-6 text-center text-xs leading-5 text-slate-500">Las credenciales se envían únicamente por HTTPS en producción y los tokens permanecen en memoria durante esta sesión.</p>
          </section>
        </div>
      </main>
    );
  }

  return (
    <main className="min-h-screen px-4 py-5 text-ink sm:px-8 sm:py-8">
      <div className="mx-auto max-w-7xl space-y-6">
        <header className="flex flex-col gap-4 sm:flex-row sm:items-center sm:justify-between">
          <div><p className="text-sm font-bold uppercase tracking-[0.24em] text-slate-500">Hypernova</p><h1 className="mt-1 text-3xl font-semibold tracking-tight">Hola, {session.user.full_name.split(" ")[0]}.</h1></div>
          <div className="flex items-center gap-3"><span className="hidden text-sm text-slate-500 sm:inline">{session.user.email}</span><button className="secondary-button" onClick={handleLogout} type="button">Cerrar sesión</button></div>
        </header>

        {dashboardError && <p className="status-message status-error" role="alert">{dashboardError}</p>}

        <div className="grid gap-6 lg:grid-cols-[1.25fr_0.75fr]">
          <section className="space-y-6">
            <div className="rounded-[1.75rem] bg-ink p-6 text-white shadow-xl sm:p-9">
              <div className="flex flex-wrap items-start justify-between gap-4"><div><p className="text-sm text-slate-300">Saldo disponible</p>{dashboardLoading && !balance ? <div className="skeleton mt-5 h-12 w-56 bg-slate-700" /> : <p className="mt-4 text-5xl font-semibold tracking-tight">{formatMinorAmount(balance?.available_balance ?? "0")}</p>}</div><span className="rounded-full bg-white/10 px-3 py-1 text-xs font-bold">{activeAccount?.currency ?? "HNL"}</span></div>
              <div className="mt-8 grid gap-4 border-t border-white/10 pt-5 text-sm sm:grid-cols-2"><div><p className="text-slate-400">Créditos registrados</p><p className="mt-1 font-semibold">{formatMinorAmount(balance?.credits_posted ?? "0")}</p></div><div><p className="text-slate-400">Débitos registrados</p><p className="mt-1 font-semibold">{formatMinorAmount(balance?.debits_posted ?? "0")}</p></div></div>
            </div>

            <section className="surface p-5 sm:p-7"><div className="flex flex-col gap-4 sm:flex-row sm:items-center sm:justify-between"><div><p className="text-sm font-bold uppercase tracking-[0.18em] text-slate-400">Cuenta</p><h2 className="mt-2 text-xl font-semibold">Tu cuenta de uso diario</h2></div>{accounts.length > 0 && <select aria-label="Seleccionar cuenta" className="sm:max-w-xs" value={activeAccount?.id ?? ""} onChange={(event) => setSelectedAccountId(event.target.value)}>{accounts.map((account) => <option key={account.id} value={account.id}>{account.type} · {account.currency} · {account.status}</option>)}</select>}</div></section>

            <section className="surface p-5 sm:p-7"><div className="flex flex-wrap items-start justify-between gap-4"><div><p className="text-sm font-bold uppercase tracking-[0.18em] text-slate-400">Actividad</p><h2 className="mt-2 text-xl font-semibold">Historial reciente</h2></div><span className="rounded-full bg-slate-100 px-3 py-1 text-xs font-bold text-slate-500">Unidades menores</span></div>{!history && dashboardLoading ? <div className="mt-6 space-y-3"><div className="skeleton h-12 w-full" /><div className="skeleton h-12 w-full" /></div> : history?.items.length ? <div className="mt-5 divide-y divide-slate-100">{history.items.map((transaction) => <TransactionRow key={`${transaction.transfer_id}-${transaction.created_at}`} transaction={transaction} />)}</div> : <div className="mt-6 rounded-2xl bg-slate-50 p-6 text-center text-sm text-slate-500">Todavía no hay movimientos en esta cuenta.</div>}{history?.has_more && <button className="secondary-button mt-5 w-full" onClick={loadMoreHistory} type="button">Cargar movimientos anteriores</button>}</section>
          </section>

          <section className="surface h-fit p-5 sm:p-7"><p className="text-sm font-bold uppercase tracking-[0.18em] text-slate-400">Operar</p><h2 className="mt-2 text-xl font-semibold">Mueve tus fondos</h2><div className="mt-5 grid grid-cols-3 rounded-full bg-slate-100 p-1">{(["deposit", "withdraw", "transfer"] as OperationMode[]).map((mode) => <button key={mode} className={`rounded-full px-2 py-2 text-xs font-bold ${operationMode === mode ? "bg-ink text-white" : "text-slate-500"}`} onClick={() => { setOperationMode(mode); setOperationNotice(null); }} type="button">{mode === "deposit" ? "Depositar" : mode === "withdraw" ? "Retirar" : "Transferir"}</button>)}</div><form className="mt-6 space-y-5" onSubmit={handleOperation}><label><span className="field-label">Importe HNL (unidades menores)</span><input required inputMode="numeric" pattern="[1-9][0-9]*" value={operationAmount} onChange={(event) => setOperationAmount(event.target.value.replace(/\D/g, ""))} placeholder="100000" /></label>{operationMode === "transfer" && <label><span className="field-label">Cuenta destino</span><input required value={destinationAccountId} onChange={(event) => setDestinationAccountId(event.target.value)} placeholder="UUID de la cuenta" /></label>}<div className="rounded-2xl bg-slate-50 p-4 text-xs leading-5 text-slate-500">{operationMode === "deposit" ? "El depósito de demostración está protegido por configuración del entorno." : operationMode === "withdraw" ? "TigerBeetle rechaza débitos que superen los créditos disponibles." : "La cuenta origen siempre es la cuenta seleccionada."}</div>{operationNotice && <p className={`status-message ${operationNotice.tone === "error" ? "status-error" : "status-success"}`} role="alert">{operationNotice.message}</p>}<button className="primary-button w-full" disabled={operationBusy || !activeAccount} type="submit">{operationBusy ? "Enviando…" : "Confirmar operación"}</button></form></section>
        </div>
        <footer className="border-t border-slate-200 pt-5 text-xs text-slate-500">Los importes financieros se manejan como unidades enteras y cada mutación usa una clave de idempotencia.</footer>
      </div>
    </main>
  );
}

function TransactionRow({ transaction }: { transaction: Transaction }) {
  return <div className="flex items-center justify-between gap-3 py-4"><div className="min-w-0"><p className="truncate text-sm font-bold">{transactionLabel(transaction)}</p><p className="mt-1 text-xs text-slate-400">{new Date(transaction.created_at).toLocaleString("es-PA")}</p></div><p className={`shrink-0 text-sm font-bold ${transaction.direction === "credit" ? "text-emerald-700" : "text-ink"}`}>{transaction.direction === "credit" ? "+" : "−"}{formatMinorAmount(transaction.amount)}</p></div>;
}

export default App;
