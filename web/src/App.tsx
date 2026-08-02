import { FormEvent, useCallback, useEffect, useMemo, useState } from "react";
import { Account, ApiError, Balance, HistoryResponse, MFAEnrollment, MFAStatus, MCPAction, MCPActionRequest, MCPTool, OAuthProvider, Operation, apiClient } from "./api";
import { AuthPage } from "./features/auth/AuthPage";
import { MFAOnboarding } from "./features/auth/MFAOnboarding";
import { MFAStatusLoading } from "./features/auth/MFAStatusLoading";
import { MFAVerificationPage } from "./features/auth/MFAVerificationPage";
import { DashboardPage } from "./features/dashboard/DashboardPage";
import { AuthField, AuthFieldErrors, AuthForm, AuthMode, FormNotice, OAuthPending, OperationMode, Session } from "./types";
import { clearStoredSession, readStoredSession, storeSession } from "./session";

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
    const messages: Record<string, string> = {
      invalid_registration: "Revisa los datos: el nombre, correo o contraseña no cumplen los requisitos.",
      invalid_login: "Revisa tu correo y contraseña antes de intentar nuevamente.",
      invalid_request: "No pudimos leer los datos. Revisa los campos e inténtalo otra vez.",
      email_already_in_use: "Ese correo ya tiene una cuenta. Prueba iniciar sesión.",
      invalid_credentials: "El correo o la contraseña no coinciden.",
      invalid_access_token: "Tu sesión expiró. Inicia sesión nuevamente.",
      mfa_unavailable: "No pudimos verificar la protección de tu cuenta. Inténtalo de nuevo.",
      demo_deposit_disabled: "El depósito de demostración está desactivado en este entorno.",
      insufficient_funds: "Fondos insuficientes para completar la operación.",
      mfa_required: "Escribe el código de seis dígitos de tu autenticador.",
      mfa_invalid_code: "El código MFA no es válido o ya expiró. Revisa tu autenticador.",
      mfa_enrollment_expired: "El enrolamiento MFA expiró. Genera un QR nuevo.",
      mfa_already_enabled: "El MFA ya está activo para esta cuenta.",
      oauth_not_configured: "Este proveedor todavía no está configurado en el entorno.",
      oauth_invalid_redirect: "El retorno OAuth no está permitido por la configuración del servidor.",
      oauth_email_conflict: "Ese email ya pertenece a una cuenta vinculada. Inicia sesión y vincula el proveedor desde seguridad.",
      idempotency_key_reused: "La operación ya existe con otra solicitud.",
    };
    return messages[error.response.code] ?? error.response.error;
  }
  return "No pudimos completar la solicitud. Intenta nuevamente.";
}

function sanitizeFullName(value: string): string {
  return value.replace(/[^\p{L} ]/gu, "").replace(/\s{2,}/g, " ").slice(0, 120);
}

function isValidEmail(value: string): boolean {
  const normalized = value.trim().toLowerCase();
  return normalized.length <= 320 && /^[^\s@]+@[^\s@]+\.[^\s@]{2,}$/u.test(normalized);
}

function createIdempotencyKey(): string {
  if (typeof crypto.randomUUID === "function") return crypto.randomUUID();
  const bytes = new Uint8Array(16);
  crypto.getRandomValues(bytes);
  bytes[6] = (bytes[6] & 0x0f) | 0x40;
  bytes[8] = (bytes[8] & 0x3f) | 0x80;
  return Array.from(bytes, (byte) => byte.toString(16).padStart(2, "0")).join("").replace(/^(.{8})(.{4})(.{4})(.{4})(.{12})$/, "$1-$2-$3-$4-$5");
}

function operationLabel(operation: Operation): string {
  if (operation.type === "deposit") return "Depósito acreditado";
  if (operation.type === "withdrawal") return "Retiro realizado";
  return "Transferencia enviada";
}

function sessionFromTokens(tokens: { access_token: string; refresh_token: string; access_expires_at: string; refresh_expires_at: string; user: Session["user"] }): Session {
  return { accessToken: tokens.access_token, refreshToken: tokens.refresh_token, user: tokens.user, accessExpiresAt: tokens.access_expires_at, refreshExpiresAt: tokens.refresh_expires_at };
}

function App() {
  const [session, setSession] = useState<Session | null>(() => readStoredSession());
  const [sessionReady, setSessionReady] = useState(false);
  const [authMode, setAuthMode] = useState<AuthMode>("login");
  const [authBusy, setAuthBusy] = useState(false);
  const [authNotice, setAuthNotice] = useState<FormNotice | null>(null);
  const [authFieldErrors, setAuthFieldErrors] = useState<AuthFieldErrors>({});
  const [authNeedsMFA, setAuthNeedsMFA] = useState(false);
  const [oauthPending, setOauthPending] = useState<OAuthPending | null>(null);
  const [authForm, setAuthForm] = useState<AuthForm>({ email: "", password: "", fullName: "", mfaCode: "" });
  const [accounts, setAccounts] = useState<Account[]>([]);
  const [selectedAccountId, setSelectedAccountId] = useState("");
  const [balance, setBalance] = useState<Balance | null>(null);
  const [history, setHistory] = useState<HistoryResponse | null>(null);
  const [historyPages, setHistoryPages] = useState<HistoryResponse[]>([]);
  const [historyPage, setHistoryPage] = useState(1);
  const [historyPageBusy, setHistoryPageBusy] = useState(false);
  const [dashboardLoading, setDashboardLoading] = useState(false);
  const [dashboardError, setDashboardError] = useState("");
  const [operationMode, setOperationMode] = useState<OperationMode>("deposit");
  const [operationAmount, setOperationAmount] = useState("");
  const [destinationAccountId, setDestinationAccountId] = useState("");
  const [operationBusy, setOperationBusy] = useState(false);
  const [operationNotice, setOperationNotice] = useState<FormNotice | null>(null);
  const [exportBusy, setExportBusy] = useState(false);
  const [mfaStatus, setMfaStatus] = useState<MFAStatus | null>(null);
  const [mfaGateReady, setMfaGateReady] = useState(false);
  const [mfaEnrollment, setMfaEnrollment] = useState<MFAEnrollment | null>(null);
  const [mfaCode, setMfaCode] = useState("");
  const [mfaBusy, setMfaBusy] = useState(false);
  const [mfaLoading, setMfaLoading] = useState(false);
  const [mfaNotice, setMfaNotice] = useState<FormNotice | null>(null);
  const [mcpTools, setMcpTools] = useState<MCPTool[]>([]);
  const [mcpLoading, setMcpLoading] = useState(false);
  const [mcpError, setMcpError] = useState("");
  const [assistantInput, setAssistantInput] = useState("");
  const [assistantReply, setAssistantReply] = useState<{ message: string; data?: unknown; confirmation: boolean } | null>(null);
  const [assistantBusy, setAssistantBusy] = useState(false);
  const [mcpAction, setMcpAction] = useState<MCPAction | null>(null);
  const [mcpActionBusy, setMcpActionBusy] = useState(false);
  const [mcpActionNotice, setMcpActionNotice] = useState<FormNotice | null>(null);

  function establishSession(nextSession: Session) {
    storeSession(nextSession);
    setSession(nextSession);
  }

  useEffect(() => {
    const stored = readStoredSession();
    if (!stored) {
      setSessionReady(true);
      return;
    }
    void apiClient.refresh(stored.refreshToken).then((tokens) => {
      establishSession(sessionFromTokens(tokens));
    }).catch(() => {
      clearStoredSession();
      setSession(null);
    }).finally(() => setSessionReady(true));
  }, []);

  useEffect(() => {
    if (!sessionReady || !session) return;
    const expiresAt = session.accessExpiresAt ? Date.parse(session.accessExpiresAt) : NaN;
    if (!Number.isFinite(expiresAt)) return;
    const refreshIn = Math.max(5_000, expiresAt - Date.now() - 60_000);
    const timer = window.setTimeout(() => {
      void apiClient.refresh(session.refreshToken).then((tokens) => {
        establishSession(sessionFromTokens(tokens));
      }).catch(() => {
        clearStoredSession();
        setSession(null);
      });
    }, refreshIn);
    return () => window.clearTimeout(timer);
  }, [sessionReady, session]);

  useEffect(() => {
    const params = new URLSearchParams(window.location.search);
    const provider = params.get("oauth_provider") as OAuthProvider | null;
    const code = params.get("oauth_code");
    if (!code || (provider !== "google" && provider !== "github")) return;
    window.history.replaceState({}, document.title, `${window.location.pathname}${window.location.hash}`);
    setAuthBusy(true);
    void apiClient.exchangeOAuth(provider, code).then((tokens) => {
      setMfaStatus(null);
      setMfaGateReady(false);
      establishSession(sessionFromTokens(tokens));
      setAuthNotice({ tone: "success", message: "Acceso federado confirmado." });
    }).catch((error) => {
      if (error instanceof ApiError && error.response.code === "mfa_required") {
        setOauthPending({ provider, code });
        setAuthNeedsMFA(true);
        setAuthMode("login");
        setAuthNotice({ tone: "success", message: "Confirma el código de tu autenticador para terminar." });
      } else setAuthNotice({ tone: "error", message: displayError(error) });
    }).finally(() => setAuthBusy(false));
  }, []);

  const activeAccount = useMemo(() => accounts.find((account) => account.id === selectedAccountId) ?? accounts[0], [accounts, selectedAccountId]);

  const loadAccounts = useCallback(async () => {
    if (!sessionReady || !session || !mfaStatus?.enabled) return;
    try {
      const response = await apiClient.listAccounts({ accessToken: session.accessToken });
      setAccounts(response.items);
      setSelectedAccountId((current) => current || response.items[0]?.id || "");
      setDashboardError("");
    } catch (error) { setDashboardError(displayError(error)); }
  }, [sessionReady, session, mfaStatus?.enabled]);

  const loadAccountData = useCallback(async () => {
    if (!sessionReady || !session || !mfaStatus?.enabled || !selectedAccountId) return;
    setDashboardLoading(true);
    try {
      const [nextBalance, nextHistory] = await Promise.all([apiClient.getBalance(selectedAccountId, { accessToken: session.accessToken }), apiClient.getHistory(selectedAccountId, { accessToken: session.accessToken, limit: 8 })]);
      setBalance(nextBalance);
      setHistory(nextHistory);
      setHistoryPages([nextHistory]);
      setDashboardError("");
    } catch (error) { setDashboardError(displayError(error)); }
    finally { setDashboardLoading(false); }
  }, [sessionReady, session, mfaStatus?.enabled, selectedAccountId]);

  useEffect(() => { void loadAccounts(); }, [loadAccounts]);
  useEffect(() => { void loadAccountData(); }, [loadAccountData]);

  const loadMFA = useCallback(async () => {
    if (!sessionReady || !session) return;
    setMfaGateReady(false);
    setMfaLoading(true);
    try {
      const status = await apiClient.getMFAStatus({ accessToken: session.accessToken });
      setMfaStatus(status);
      if (!status.enabled) setMfaEnrollment(await apiClient.enrollMFA({ accessToken: session.accessToken }));
    } catch (error) { setMfaNotice({ tone: "error", message: displayError(error) }); }
    finally { setMfaLoading(false); setMfaGateReady(true); }
  }, [sessionReady, session]);

  useEffect(() => { void loadMFA(); }, [loadMFA]);

  const loadMCP = useCallback(async () => {
    if (!sessionReady || !session || !mfaStatus?.enabled) return;
    setMcpLoading(true);
    try { setMcpTools((await apiClient.listMCPTools({ accessToken: session.accessToken })).tools); setMcpError(""); }
    catch (error) { setMcpError(displayError(error)); }
    finally { setMcpLoading(false); }
  }, [sessionReady, session, mfaStatus?.enabled]);

  useEffect(() => { void loadMCP(); }, [loadMCP]);

  function validateAuthFields(): boolean {
    const errors: AuthFieldErrors = {};
    if (!oauthPending) {
      if (authMode === "register" && authForm.fullName.trim().length < 2) errors.fullName = "Escribe al menos dos letras en tu nombre.";
      if (!isValidEmail(authForm.email)) errors.email = "Escribe un correo electrónico válido.";
      if (!authForm.password) errors.password = "Escribe tu contraseña.";
      else if (authMode === "register" && (authForm.password.length < 8 || new TextEncoder().encode(authForm.password).length > 72)) errors.password = "La contraseña debe tener entre 8 y 72 caracteres.";
    }
    if ((authMode === "login" && authNeedsMFA) || oauthPending) if (!/^\d{6}$/u.test(authForm.mfaCode)) errors.mfaCode = "Escribe los 6 dígitos de tu autenticador.";
    setAuthFieldErrors(errors);
    return Object.keys(errors).length === 0;
  }

  function validateAuthField(field: AuthField) {
    const errors = { ...authFieldErrors };
    delete errors[field];
    if (field === "fullName" && authMode === "register" && authForm.fullName.trim().length < 2) errors.fullName = "Escribe al menos dos letras en tu nombre.";
    if (field === "email" && !oauthPending && !isValidEmail(authForm.email)) errors.email = "Escribe un correo electrónico válido.";
    if (field === "password" && authMode === "register" && (authForm.password.length < 8 || new TextEncoder().encode(authForm.password).length > 72)) errors.password = "La contraseña debe tener entre 8 y 72 caracteres.";
    if (field === "mfaCode" && ((authMode === "login" && authNeedsMFA) || oauthPending) && !/^\d{6}$/u.test(authForm.mfaCode)) errors.mfaCode = "Escribe los 6 dígitos de tu autenticador.";
    setAuthFieldErrors(errors);
  }

  async function handleAuth(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (!validateAuthFields()) return;
    const email = authForm.email.trim().toLowerCase();
    setAuthBusy(true);
    setAuthNotice(null);
    try {
      if (oauthPending) {
        const tokens = await apiClient.exchangeOAuth(oauthPending.provider, oauthPending.code, authForm.mfaCode);
        establishSession(sessionFromTokens(tokens));
        setMfaStatus(null); setMfaGateReady(false); setOauthPending(null);
      } else if (authMode === "register") {
        const registration = await apiClient.register({ email, password: authForm.password, full_name: authForm.fullName });
        const tokens = await apiClient.login({ email, password: authForm.password });
        establishSession(sessionFromTokens(tokens));
        setMfaStatus(null); setMfaGateReady(false);
        setAuthNotice({ tone: "success", message: `Cuenta HNL ${registration.account.status === "active" ? "lista" : "en provisión"}.` });
      } else {
        const tokens = await apiClient.login({ email, password: authForm.password, mfa_code: authForm.mfaCode || undefined });
        establishSession(sessionFromTokens(tokens));
        setMfaStatus(null); setMfaGateReady(false);
      }
      setAuthNeedsMFA(false); setAuthForm({ email: "", password: "", fullName: "", mfaCode: "" }); setAuthFieldErrors({});
    } catch (error) {
      if (error instanceof ApiError && error.response.code === "mfa_required") setAuthNeedsMFA(true);
      if (error instanceof ApiError && error.response.code === "mfa_invalid_code") { setAuthFieldErrors((current) => ({ ...current, mfaCode: "El código no es válido. Revisa tu autenticador e inténtalo otra vez." })); setAuthNotice(null); }
      else setAuthNotice({ tone: "error", message: displayError(error) });
    } finally { setAuthBusy(false); }
  }

  async function handleLogout() {
    if (session) await apiClient.logout(session.accessToken).catch(() => undefined);
    clearStoredSession();
    setSession(null); setAccounts([]); setBalance(null); setHistory(null); setHistoryPages([]); setHistoryPage(1); setSelectedAccountId(""); setMfaStatus(null); setMfaGateReady(false); setMfaEnrollment(null); setMfaCode(""); setMfaLoading(false); setMfaNotice(null);
  }

  function handleFullNameChange(value: string) {
    const sanitized = sanitizeFullName(value);
    setAuthForm((current) => ({ ...current, fullName: sanitized }));
    setAuthFieldErrors((current) => ({ ...current, fullName: value === sanitized ? undefined : "El nombre solo admite letras, ñ, acentos y espacios." }));
  }

  function handleAuthFieldChange(field: AuthField, value: string) {
    setAuthForm((current) => ({ ...current, [field]: value }));
    setAuthFieldErrors((current) => ({ ...current, [field]: undefined }));
  }

  async function beginMFAEnrollment() {
    if (!session) return;
    setMfaBusy(true); setMfaNotice(null);
    try { setMfaEnrollment(await apiClient.enrollMFA({ accessToken: session.accessToken })); setMfaNotice({ tone: "success", message: "Escanea el QR y confirma con el código actual de tu autenticador." }); }
    catch (error) { setMfaNotice({ tone: "error", message: displayError(error) }); }
    finally { setMfaBusy(false); }
  }

  async function verifyMFAEnrollment(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (!session) return;
    if (mfaCode.length !== 6) { setMfaNotice({ tone: "error", message: "Escribe los 6 dígitos de tu autenticador." }); return; }
    setMfaBusy(true); setMfaNotice(null);
    try { setMfaStatus(await apiClient.verifyMFA(mfaCode, { accessToken: session.accessToken })); setMfaEnrollment(null); setMfaCode(""); setMfaNotice({ tone: "success", message: "MFA activado. Tu próximo inicio de sesión pedirá un código." }); }
    catch (error) { setMfaNotice({ tone: "error", message: displayError(error) }); }
    finally { setMfaBusy(false); }
  }

  async function handleOperation(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (!session || !activeAccount) return;
    setOperationBusy(true); setOperationNotice(null);
    const idempotencyKey = createIdempotencyKey();
    try {
      let operation: Operation;
      if (operationMode === "deposit") operation = await apiClient.deposit(activeAccount.id, { amount: operationAmount, currency: "HNL" }, { accessToken: session.accessToken, idempotencyKey });
      else if (operationMode === "withdraw") operation = await apiClient.withdraw(activeAccount.id, { amount: operationAmount, currency: "HNL" }, { accessToken: session.accessToken, idempotencyKey });
      else operation = await apiClient.transfer({ source_account_id: activeAccount.id, destination_account_id: destinationAccountId, amount: operationAmount, currency: "HNL" }, { accessToken: session.accessToken, idempotencyKey });
      setOperationNotice({ tone: "success", message: `${operationLabel(operation)} por ${formatMinorAmount(operation.amount)}.` }); setOperationAmount(""); setDestinationAccountId(""); await loadAccountData();
    } catch (error) {
      if (error instanceof ApiError && (error.status === 401 || error.response.code === "mfa_required")) { await handleLogout(); setAuthNotice({ tone: "error", message: displayError(error) }); return; }
      setOperationNotice({ tone: "error", message: displayError(error) });
    } finally { setOperationBusy(false); }
  }

  async function loadMoreHistory() {
    if (!session || !selectedAccountId || !history?.next_cursor) return;
    setHistoryPageBusy(true);
    try { const nextPage = await apiClient.getHistory(selectedAccountId, { accessToken: session.accessToken, limit: 8, cursor: history.next_cursor }); setHistoryPages((current) => [...current, nextPage]); setHistory(nextPage); setHistoryPage((current) => current + 1); }
    catch (error) { setDashboardError(displayError(error)); }
    finally { setHistoryPageBusy(false); }
  }

  function loadPreviousHistory() {
    if (historyPage <= 1) return;
    const previousPage = historyPages[historyPage - 2];
    if (!previousPage) return;
    setHistory(previousPage); setHistoryPage((current) => current - 1);
  }

  async function handleExport() {
    if (!session || !activeAccount) return;
    setExportBusy(true);
    try { const blob = await apiClient.exportHistory(activeAccount.id, { accessToken: session.accessToken }); const url = URL.createObjectURL(blob); const link = document.createElement("a"); link.href = url; link.download = "hypernova-transactions.csv"; link.click(); URL.revokeObjectURL(url); }
    catch (error) { setDashboardError(displayError(error)); }
    finally { setExportBusy(false); }
  }

  async function handleAssistantSubmit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (!session || !assistantInput.trim()) return;
    setAssistantBusy(true);
    try { const response = await apiClient.sendChatMessage(assistantInput.trim(), { accessToken: session.accessToken }); setAssistantReply({ message: response.message, data: response.read_only_data, confirmation: response.requires_confirmation }); setAssistantInput(""); }
    catch (error) { if (error instanceof ApiError && error.status === 401) { await handleLogout(); setAuthNotice({ tone: "error", message: displayError(error) }); return; } setMcpError(displayError(error)); }
    finally { setAssistantBusy(false); }
  }

  async function handlePrepareMCPAction(request: MCPActionRequest) {
    if (!session) return;
    setMcpActionBusy(true);
    setMcpActionNotice(null);
    try {
      setMcpAction(await apiClient.prepareMCPAction(request, { accessToken: session.accessToken }));
    } catch (error) {
      setMcpActionNotice({ tone: "error", message: displayError(error) });
    } finally {
      setMcpActionBusy(false);
    }
  }

  async function handleConfirmMCPAction() {
    if (!session || !mcpAction) return;
    setMcpActionBusy(true);
    setMcpActionNotice(null);
    try {
      setMcpAction(await apiClient.confirmMCPAction(mcpAction.id, { accessToken: session.accessToken }));
      await loadAccountData();
    } catch (error) {
      setMcpActionNotice({ tone: "error", message: displayError(error) });
    } finally {
      setMcpActionBusy(false);
    }
  }

  async function handleCancelMCPAction() {
    if (!session || !mcpAction) return;
    setMcpActionBusy(true);
    setMcpActionNotice(null);
    try {
      setMcpAction(await apiClient.cancelMCPAction(mcpAction.id, { accessToken: session.accessToken }));
    } catch (error) {
      setMcpActionNotice({ tone: "error", message: displayError(error) });
    } finally {
      setMcpActionBusy(false);
    }
  }

  if (!sessionReady) return <MFAStatusLoading notice={null} onLogout={handleLogout} />;
  if (!session) {
    if (authNeedsMFA || oauthPending) return <MFAVerificationPage email={authForm.email} code={authForm.mfaCode} busy={authBusy} notice={authNotice} fieldError={authFieldErrors.mfaCode} oauth={Boolean(oauthPending)} onCodeChange={(value) => handleAuthFieldChange("mfaCode", value)} onSubmit={handleAuth} onBack={() => { setAuthNeedsMFA(false); setOauthPending(null); setAuthForm((current) => ({ ...current, mfaCode: "" })); setAuthFieldErrors({}); setAuthNotice(null); }} />;
    return <AuthPage mode={authMode} busy={authBusy} notice={authNotice} fieldErrors={authFieldErrors} form={authForm} oauthPending={oauthPending} onModeChange={(mode) => { setAuthMode(mode); setAuthNotice(null); setAuthFieldErrors({}); }} onFieldChange={handleAuthFieldChange} onFullNameChange={handleFullNameChange} onFieldBlur={validateAuthField} onSubmit={handleAuth} onOAuth={(provider) => apiClient.startOAuth(provider)} />;
  }
  if (!mfaGateReady || mfaStatus === null) return <MFAStatusLoading notice={mfaNotice} onLogout={handleLogout} />;
  if (!mfaStatus.enabled) return <MFAOnboarding user={session.user} enrollment={mfaEnrollment} code={mfaCode} busy={mfaBusy} loading={mfaLoading} notice={mfaNotice} onCodeChange={setMfaCode} onBegin={beginMFAEnrollment} onVerify={verifyMFAEnrollment} onLogout={handleLogout} />;
  return <DashboardPage user={session.user} accounts={accounts} activeAccount={activeAccount} balance={balance} history={history} historyPage={historyPage} dashboardLoading={dashboardLoading} dashboardError={dashboardError} operationMode={operationMode} operationAmount={operationAmount} destinationAccountId={destinationAccountId} operationBusy={operationBusy} operationNotice={operationNotice} exportBusy={exportBusy} mcpTools={mcpTools} mcpLoading={mcpLoading} mcpError={mcpError} assistantInput={assistantInput} assistantReply={assistantReply} assistantBusy={assistantBusy} mcpAction={mcpAction} mcpActionBusy={mcpActionBusy} mcpActionNotice={mcpActionNotice} historyHasMore={Boolean(history?.has_more)} historyPageBusy={historyPageBusy} onAccountChange={(accountId) => { setSelectedAccountId(accountId); setBalance(null); setHistory(null); setHistoryPages([]); setHistoryPage(1); }} onOperationModeChange={(mode) => { setOperationMode(mode); setOperationNotice(null); }} onAmountChange={setOperationAmount} onDestinationChange={setDestinationAccountId} onOperation={handleOperation} onExport={handleExport} onPreviousHistory={loadPreviousHistory} onNextHistory={loadMoreHistory} onAssistantInput={setAssistantInput} onAssistantSubmit={handleAssistantSubmit} onPrepareMCPAction={(request) => void handlePrepareMCPAction(request)} onConfirmMCPAction={() => void handleConfirmMCPAction()} onCancelMCPAction={() => void handleCancelMCPAction()} onLogout={handleLogout} />;
}

export default App;
