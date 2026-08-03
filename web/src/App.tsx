import { FormEvent, useCallback, useEffect, useMemo, useState } from "react";
import { Account, ApiError, Balance, HistoryResponse, MFAEnrollment, MFAStatus, MCPAction, MCPActionRequest, MCPAccountOption, MCPConversationState, OAuthProvider, Operation, apiClient } from "./api";
import { AuthPage } from "./features/auth/AuthPage";
import { MFAOnboarding } from "./features/auth/MFAOnboarding";
import { MFAStatusLoading } from "./features/auth/MFAStatusLoading";
import { MFAVerificationPage } from "./features/auth/MFAVerificationPage";
import { DashboardPage } from "./features/dashboard/DashboardPage";
import { AssistantReply, AuthField, AuthFieldErrors, AuthForm, AuthMode, FormNotice, OAuthPending, OperationMode, Session, TransferTargetType } from "./types";
import { clearStoredSession, readStoredSession, storeSession } from "./session";
import { readThemeMode, resolveTheme } from "./theme";
import type { ThemeMode } from "./theme";
import { currencyInputToMinor } from "./money";

const HISTORY_PAGE_SIZE = 5;

function formatMinorAmount(value: string): string {
  try {
    const minor = BigInt(value || "0");
    const negative = minor < 0n;
    const absolute = negative ? -minor : minor;
    const digits = absolute.toString().padStart(3, "0");
    const whole = digits.slice(0, -2).replace(/\B(?=(\d{3})+(?!\d))/g, ",");
    return `${negative ? "-" : ""}USD ${whole}.${digits.slice(-2)}`;
  } catch {
    return "USD 0.00";
  }
}

function displayError(error: unknown): string {
  if (error instanceof ApiError) {
    const messages: Record<string, string> = {
      invalid_registration: "Revisa los datos: el nombre, correo o contraseña no cumplen los requisitos.",
      invalid_profile: "Revisa tu nombre e inténtalo nuevamente.",
      invalid_login: "Revisa tu correo y contraseña antes de intentar nuevamente.",
      invalid_request: "No pudimos leer los datos. Revisa los campos e inténtalo otra vez.",
      invalid_currency: "La cuenta debe crearse en USD. Actualiza la aplicación y vuelve a intentarlo.",
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
      invalid_mcp_pin: "El PIN debe tener exactamente cuatro dígitos y coincidir con el configurado.",
      mcp_pin_not_configured: "Configura tu PIN de cuatro dígitos antes de confirmar una operación.",
      mcp_pin_expired: "Tu PIN de confirmación venció. Genera uno nuevo para continuar.",
      mcp_pin_locked: "El PIN quedó bloqueado temporalmente por varios intentos. Espera unos minutos antes de volver a intentarlo.",
      mcp_pin_unavailable: "No pudimos consultar tu PIN de confirmación. Inténtalo nuevamente.",
      mcp_pin_error: "No pudimos guardar tu PIN de confirmación. Inténtalo nuevamente.",
      oauth_not_configured: "Este proveedor todavía no está configurado en el entorno.",
      oauth_invalid_redirect: "El retorno OAuth no está permitido por la configuración del servidor.",
      oauth_email_conflict: "Ese email ya pertenece a una cuenta vinculada. Inicia sesión y vincula el proveedor desde seguridad.",
      idempotency_key_reused: "La operación ya existe con otra solicitud.",
      account_not_found: "La cuenta seleccionada no está disponible. Actualiza tus cuentas e inténtalo nuevamente.",
      external_account_required: "La cuenta destino no está disponible para transferir.",
      external_account_not_found: "No encontramos la cuenta destino. Revísala e inténtalo nuevamente.",
      external_transfer_pin_required: "Escribe tu PIN de confirmación para transferir a otra cuenta.",
      invalid_transfer_type: "Selecciona un tipo de transferencia válido.",
      account_not_empty: "La cuenta solo se puede cerrar cuando su saldo USD y movimientos pendientes están en cero.",
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
  const [themeMode, setThemeMode] = useState<ThemeMode>(() => readThemeMode());
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
  const [accountBalances, setAccountBalances] = useState<Record<string, Balance>>({});
  const [selectedAccountId, setSelectedAccountId] = useState("");
  const [accountBusy, setAccountBusy] = useState(false);
  const [accountNotice, setAccountNotice] = useState<FormNotice | null>(null);
  const [accountRenameBusyId, setAccountRenameBusyId] = useState("");
  const [accountDeleteBusyId, setAccountDeleteBusyId] = useState("");
  const [profileFullName, setProfileFullName] = useState("");
  const [profileBusy, setProfileBusy] = useState(false);
  const [profileNotice, setProfileNotice] = useState<FormNotice | null>(null);
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
  const [transferTargetType, setTransferTargetType] = useState<TransferTargetType>("own");
  const [transferConfirmationPin, setTransferConfirmationPin] = useState("");
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
  const [mcpError, setMcpError] = useState("");
  const [assistantInput, setAssistantInput] = useState("");
  const [assistantReply, setAssistantReply] = useState<AssistantReply | null>(null);
  const [assistantConversation, setAssistantConversation] = useState<MCPConversationState | null>(null);
  const [assistantAccountOptions, setAssistantAccountOptions] = useState<MCPAccountOption[]>([]);
  const [assistantBusy, setAssistantBusy] = useState(false);
  const [mcpAction, setMcpAction] = useState<MCPAction | null>(null);
  const [mcpActionBusy, setMcpActionBusy] = useState(false);
  const [mcpActionNotice, setMcpActionNotice] = useState<FormNotice | null>(null);
  const [mcpPinConfigured, setMcpPinConfigured] = useState(false);
  const [mcpPinExpiresAt, setMcpPinExpiresAt] = useState<string | undefined>();
  const [mcpPin, setMcpPin] = useState("");
  const [mcpConfirmationPin, setMcpConfirmationPin] = useState("");
  const [mcpPinBusy, setMcpPinBusy] = useState(false);
  const [mcpPinNotice, setMcpPinNotice] = useState<FormNotice | null>(null);

  useEffect(() => {
    const media = window.matchMedia("(prefers-color-scheme: dark)");
    const applyTheme = () => {
      document.documentElement.dataset.theme = resolveTheme(themeMode);
    };
    applyTheme();
    if (themeMode === "system") {
      media.addEventListener("change", applyTheme);
    }
    window.localStorage.setItem("hypernova.theme", themeMode);
    return () => media.removeEventListener("change", applyTheme);
  }, [themeMode]);

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
      const balanceEntries = await Promise.all(response.items.map(async (account) => {
        try { return [account.id, await apiClient.getBalance(account.id, { accessToken: session.accessToken })] as const; }
        catch { return null; }
      }));
      setAccountBalances(Object.fromEntries(balanceEntries.filter((entry): entry is readonly [string, Balance] => entry !== null)));
      setSelectedAccountId((current) => current || response.items[0]?.id || "");
      setProfileFullName((current) => current || session.user.full_name);
      setDashboardError("");
    } catch (error) { setDashboardError(displayError(error)); }
  }, [sessionReady, session, mfaStatus?.enabled]);

  const loadAccountData = useCallback(async (accountID = selectedAccountId) => {
    if (!sessionReady || !session || !mfaStatus?.enabled || !accountID) return;
    setDashboardLoading(true);
    try {
      const [nextBalance, nextHistory] = await Promise.all([apiClient.getBalance(accountID, { accessToken: session.accessToken }), apiClient.getHistory(accountID, { accessToken: session.accessToken, limit: HISTORY_PAGE_SIZE })]);
      setBalance(nextBalance);
      setAccountBalances((current) => ({ ...current, [accountID]: nextBalance }));
      setHistory(nextHistory);
      setHistoryPages([nextHistory]);
      setHistoryPage(1);
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
    try {
      const pinStatus = await apiClient.getMCPPINStatus({ accessToken: session.accessToken });
      setMcpPinConfigured(pinStatus.configured);
      setMcpPinExpiresAt(pinStatus.expires_at);
      const pending = await apiClient.getPendingMCPAction({ accessToken: session.accessToken });
      setMcpAction(pending.action);
      setMcpError("");
    }
    catch (error) { setMcpError(displayError(error)); }
  }, [sessionReady, session, mfaStatus?.enabled]);

  useEffect(() => { void loadMCP(); }, [loadMCP]);

  useEffect(() => {
    if (!mcpAction || (mcpAction.status !== "ready" && mcpAction.status !== "confirming")) return;
    const expiresAt = Date.parse(mcpAction.expires_at);
    if (!Number.isFinite(expiresAt)) return;
    const expireAction = () => {
      setMcpAction(null);
      setAssistantConversation(null);
      setAssistantAccountOptions([]);
      setMcpConfirmationPin("");
      setMcpActionNotice({ tone: "error", message: "La operación pendiente expiró. Puedes iniciar una nueva operación." });
    };
    const delay = expiresAt - Date.now();
    if (delay <= 0) { expireAction(); return; }
    const timer = window.setTimeout(expireAction, delay);
    return () => window.clearTimeout(timer);
  }, [mcpAction]);

  useEffect(() => {
    if (!mcpPinExpiresAt) return;
    const expiresAt = Date.parse(mcpPinExpiresAt);
    if (!Number.isFinite(expiresAt)) return;
    const expirePIN = () => {
      setMcpPinConfigured(false);
      setMcpPinNotice({ tone: "error", message: "Tu PIN venció. Genera uno nuevo para confirmar operaciones." });
    };
    const delay = expiresAt - Date.now();
    if (delay <= 0) {
      expirePIN();
      return;
    }
    const timer = window.setTimeout(expirePIN, delay);
    return () => window.clearTimeout(timer);
  }, [mcpPinExpiresAt]);

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
        setAuthNotice({ tone: "success", message: `Cuenta USD ${registration.account.status === "active" ? "lista" : "en provisión"}.` });
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
    setSession(null); setAccounts([]); setAccountBalances({}); setBalance(null); setHistory(null); setHistoryPages([]); setHistoryPage(1); setSelectedAccountId(""); setAccountNotice(null); setAccountRenameBusyId(""); setAccountDeleteBusyId(""); setProfileFullName(""); setProfileNotice(null); setMfaStatus(null); setMfaGateReady(false); setMfaEnrollment(null); setMfaCode(""); setMfaLoading(false); setMfaNotice(null); setMcpPinConfigured(false); setMcpPinExpiresAt(undefined); setMcpPin(""); setMcpConfirmationPin(""); setMcpPinNotice(null); setAssistantConversation(null); setAssistantAccountOptions([]);
  }

  async function handleCreateAccount() {
    if (!session) return;
    setAccountBusy(true);
    setAccountNotice(null);
    try {
      const account = await apiClient.createAccount({ currency: "USD" }, { accessToken: session.accessToken });
      setAccounts((current) => [...current, account]);
      setSelectedAccountId(account.id);
      setAccountNotice({ tone: "success", message: "Tu nueva cuenta USD está lista." });
    } catch (error) {
      setAccountNotice({ tone: "error", message: displayError(error) });
    } finally {
      setAccountBusy(false);
    }
  }

  async function handleRenameAccount(accountId: string, displayName: string) {
    if (!session || displayName.trim().length < 2) {
      setAccountNotice({ tone: "error", message: "Escribe un nombre de al menos dos caracteres." });
      return;
    }
    setAccountRenameBusyId(accountId);
    setAccountNotice(null);
    try {
      const updated = await apiClient.renameAccount(accountId, displayName.trim(), { accessToken: session.accessToken });
      setAccounts((current) => current.map((account) => account.id === updated.id ? updated : account));
      setAccountNotice({ tone: "success", message: "Nombre de cuenta actualizado." });
    } catch (error) { setAccountNotice({ tone: "error", message: displayError(error) }); }
    finally { setAccountRenameBusyId(""); }
  }

  async function handleDeleteAccount(accountId: string) {
    if (!session) return;
    const account = accounts.find((item) => item.id === accountId);
    const currentBalance = accountBalances[accountId];
    if (!account || !currentBalance || currentBalance.available_balance !== "0" || currentBalance.credits_pending !== "0" || currentBalance.debits_pending !== "0") {
      setAccountNotice({ tone: "error", message: "Solo puedes cerrar una cuenta con saldo USD y movimientos pendientes en cero." });
      return;
    }
    if (!window.confirm(`¿Cerrar ${account.display_name || "esta cuenta"}? Conservaremos su historial, pero ya no recibirá nuevas operaciones.`)) return;
    setAccountDeleteBusyId(accountId);
    setAccountNotice(null);
    try {
      await apiClient.closeAccount(accountId, { accessToken: session.accessToken });
      const remaining = accounts.filter((item) => item.id !== accountId);
      setAccounts(remaining);
      setAccountBalances((current) => { const next = { ...current }; delete next[accountId]; return next; });
      setSelectedAccountId((current) => current === accountId ? (remaining[0]?.id ?? "") : current);
      setAccountNotice({ tone: "success", message: "La cuenta fue cerrada de forma segura." });
    } catch (error) {
      setAccountNotice({ tone: "error", message: displayError(error) });
    } finally {
      setAccountDeleteBusyId("");
    }
  }

  async function handleProfileSubmit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (!session || profileFullName.trim().length < 2) {
      setProfileNotice({ tone: "error", message: "Escribe un nombre válido para continuar." });
      return;
    }
    setProfileBusy(true);
    setProfileNotice(null);
    try {
      const user = await apiClient.updateProfile({ full_name: sanitizeFullName(profileFullName) }, { accessToken: session.accessToken });
      const nextSession = { ...session, user };
      establishSession(nextSession);
      setProfileFullName(user.full_name);
      setProfileNotice({ tone: "success", message: "Tus datos personales fueron actualizados." });
    } catch (error) {
      setProfileNotice({ tone: "error", message: displayError(error) });
    } finally {
      setProfileBusy(false);
    }
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
    const minorAmount = currencyInputToMinor(operationAmount);
    if (!minorAmount || minorAmount === "0") {
      setOperationNotice({ tone: "error", message: "Escribe un monto válido mayor que USD 0.00." });
      return;
    }
    setOperationBusy(true); setOperationNotice(null);
    const idempotencyKey = createIdempotencyKey();
    try {
      let operation: Operation;
      if (operationMode === "deposit") operation = await apiClient.deposit(activeAccount.id, { amount: minorAmount, currency: "USD" }, { accessToken: session.accessToken, idempotencyKey });
      else if (operationMode === "withdraw") operation = await apiClient.withdraw(activeAccount.id, { amount: minorAmount, currency: "USD" }, { accessToken: session.accessToken, idempotencyKey });
      else operation = await apiClient.transfer({ source_account_id: activeAccount.id, destination_account_id: destinationAccountId, amount: minorAmount, currency: "USD", transfer_type: transferTargetType, confirmation_pin: transferTargetType === "external" ? transferConfirmationPin : undefined }, { accessToken: session.accessToken, idempotencyKey });
      setOperationNotice({ tone: "success", message: `${operationLabel(operation)} por ${formatMinorAmount(operation.amount)}.` }); setOperationAmount(""); setDestinationAccountId(""); setTransferConfirmationPin(""); await loadAccountData();
    } catch (error) {
      if (error instanceof ApiError && (error.status === 401 || error.response.code === "mfa_required")) { await handleLogout(); setAuthNotice({ tone: "error", message: displayError(error) }); return; }
      setOperationNotice({ tone: "error", message: displayError(error) });
    } finally { setOperationBusy(false); }
  }

  async function loadMoreHistory() {
    if (!session || !selectedAccountId || !history?.next_cursor) return;
    setHistoryPageBusy(true);
    try { const nextPage = await apiClient.getHistory(selectedAccountId, { accessToken: session.accessToken, limit: HISTORY_PAGE_SIZE, cursor: history.next_cursor }); setHistoryPages((current) => [...current, nextPage]); setHistory(nextPage); setHistoryPage((current) => current + 1); }
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

  async function sendAssistantMessage(message: string) {
    if (!session || !message.trim()) return;
    setAssistantBusy(true);
    try { const response = await apiClient.sendChatMessage(message.trim(), activeAccount?.id, assistantConversation, { accessToken: session.accessToken }); setAssistantReply({ id: createIdempotencyKey(), message: response.message, data: response.read_only_data, confirmation: response.requires_confirmation, conversation: response.conversation, accountOptions: response.account_options ?? [] }); setAssistantConversation(response.conversation ?? null); setAssistantAccountOptions(response.account_options ?? []); setMcpAction(response.action ?? null); setMcpActionNotice(null); setAssistantInput(""); }
    catch (error) { if (error instanceof ApiError && error.status === 401) { await handleLogout(); setAuthNotice({ tone: "error", message: displayError(error) }); return; } setAssistantReply({ id: createIdempotencyKey(), message: displayError(error), confirmation: false }); setMcpError(""); }
    finally { setAssistantBusy(false); }
  }

  function handleAssistantSubmit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    void sendAssistantMessage(assistantInput);
  }

  function handleAssistantAccountSelect(accountId: string) {
    void sendAssistantMessage(`cuenta ${accountId}`);
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
    if (!/^\d{4}$/u.test(mcpConfirmationPin)) {
      setMcpActionNotice({ tone: "error", message: "Escribe el PIN de cuatro dígitos para confirmar." });
      return;
    }
    setMcpActionBusy(true);
    setMcpActionNotice(null);
    try {
      const affectedAccountID = mcpAction.payload.account_id || mcpAction.payload.source_account_id || selectedAccountId;
      const confirmed = await apiClient.confirmMCPAction(mcpAction.id, mcpConfirmationPin, { accessToken: session.accessToken });
      setMcpAction(confirmed);
      setAssistantReply({ id: createIdempotencyKey(), message: "Operación confirmada correctamente. Aquí tienes tu comprobante.", data: confirmed.operation, confirmation: false });
      setAssistantConversation(null);
      setAssistantAccountOptions([]);
      setMcpConfirmationPin("");
      if (affectedAccountID && affectedAccountID !== selectedAccountId) setSelectedAccountId(affectedAccountID);
      await loadAccountData(affectedAccountID);
      // TigerBeetle balance reads can become visible a few milliseconds before
      // the account-transfer index is returned. Re-read the same account once
      // so the history panel reflects the confirmed movement without inventing
      // a client-side transaction.
      await new Promise((resolve) => window.setTimeout(resolve, 250));
      await loadAccountData(affectedAccountID);
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
      await apiClient.cancelMCPAction(mcpAction.id, { accessToken: session.accessToken });
      setMcpAction(null);
      setAssistantConversation(null);
      setAssistantAccountOptions([]);
      setMcpConfirmationPin("");
      setAssistantReply({ id: createIdempotencyKey(), message: "Operación cancelada. Puedes iniciar otra cuando quieras.", confirmation: false });
    } catch (error) {
      setMcpActionNotice({ tone: "error", message: displayError(error) });
    } finally {
      setMcpActionBusy(false);
    }
  }

  async function handleSetMCPPIN(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (!session || !/^\d{4}$/u.test(mcpPin)) {
      setMcpPinNotice({ tone: "error", message: "El PIN debe contener exactamente cuatro dígitos." });
      return;
    }
    setMcpPinBusy(true);
    setMcpPinNotice(null);
    try {
      await apiClient.setMCPPIN(mcpPin, { accessToken: session.accessToken });
      const pinStatus = await apiClient.getMCPPINStatus({ accessToken: session.accessToken });
      setMcpPinConfigured(pinStatus.configured);
      setMcpPinExpiresAt(pinStatus.expires_at);
      setMcpPin("");
      setMcpPinNotice({ tone: "success", message: "PIN activo durante tres minutos." });
    } catch (error) {
      setMcpPinNotice({ tone: "error", message: displayError(error) });
    } finally {
      setMcpPinBusy(false);
    }
  }

  if (!sessionReady) return <MFAStatusLoading notice={null} onLogout={handleLogout} />;
  if (!session) {
    if (authNeedsMFA || oauthPending) return <MFAVerificationPage email={authForm.email} code={authForm.mfaCode} busy={authBusy} notice={authNotice} fieldError={authFieldErrors.mfaCode} oauth={Boolean(oauthPending)} onCodeChange={(value) => handleAuthFieldChange("mfaCode", value)} onSubmit={handleAuth} onBack={() => { setAuthNeedsMFA(false); setOauthPending(null); setAuthForm((current) => ({ ...current, mfaCode: "" })); setAuthFieldErrors({}); setAuthNotice(null); }} />;
    return <AuthPage mode={authMode} busy={authBusy} notice={authNotice} fieldErrors={authFieldErrors} form={authForm} oauthPending={oauthPending} onModeChange={(mode) => { setAuthMode(mode); setAuthNotice(null); setAuthFieldErrors({}); }} onFieldChange={handleAuthFieldChange} onFullNameChange={handleFullNameChange} onFieldBlur={validateAuthField} onSubmit={handleAuth} onOAuth={(provider) => apiClient.startOAuth(provider)} />;
  }
  if (!mfaGateReady || mfaStatus === null) return <MFAStatusLoading notice={mfaNotice} onLogout={handleLogout} />;
  if (!mfaStatus.enabled) return <MFAOnboarding user={session.user} enrollment={mfaEnrollment} code={mfaCode} busy={mfaBusy} loading={mfaLoading} notice={mfaNotice} onCodeChange={setMfaCode} onBegin={beginMFAEnrollment} onVerify={verifyMFAEnrollment} onLogout={handleLogout} />;
  return <DashboardPage themeMode={themeMode} onThemeModeChange={setThemeMode} user={session.user} accounts={accounts} accountBalances={accountBalances} activeAccount={activeAccount} balance={balance} history={history} historyPage={historyPage} dashboardLoading={dashboardLoading} dashboardError={dashboardError} operationMode={operationMode} operationAmount={operationAmount} destinationAccountId={destinationAccountId} transferTargetType={transferTargetType} transferConfirmationPin={transferConfirmationPin} operationBusy={operationBusy} operationNotice={operationNotice} exportBusy={exportBusy} accountBusy={accountBusy} accountNotice={accountNotice} accountRenameBusyId={accountRenameBusyId} accountDeleteBusyId={accountDeleteBusyId} profileFullName={profileFullName || session.user.full_name} profileBusy={profileBusy} profileNotice={profileNotice} mcpError={mcpError} assistantInput={assistantInput} assistantReply={assistantReply} assistantConversation={assistantConversation} assistantAccountOptions={assistantAccountOptions} assistantBusy={assistantBusy} mcpAction={mcpAction} mcpActionBusy={mcpActionBusy} mcpActionNotice={mcpActionNotice} mcpPinConfigured={mcpPinConfigured} mcpPinExpiresAt={mcpPinExpiresAt} mcpPin={mcpPin} mcpConfirmationPin={mcpConfirmationPin} mcpPinBusy={mcpPinBusy} mcpPinNotice={mcpPinNotice} historyHasMore={Boolean(history?.has_more)} historyPageBusy={historyPageBusy} onAccountChange={(accountId) => { setSelectedAccountId(accountId); setDestinationAccountId(""); setAssistantConversation(null); setAssistantAccountOptions([]); }} onCreateAccount={() => void handleCreateAccount()} onRenameAccount={(accountId, displayName) => void handleRenameAccount(accountId, displayName)} onDeleteAccount={(accountId) => void handleDeleteAccount(accountId)} onOperationModeChange={(mode) => { setOperationMode(mode); setOperationNotice(null); }} onAmountChange={setOperationAmount} onDestinationChange={setDestinationAccountId} onTransferTargetTypeChange={(target) => { setTransferTargetType(target); setDestinationAccountId(""); setTransferConfirmationPin(""); }} onTransferConfirmationPinChange={(pin) => setTransferConfirmationPin(pin.replace(/\D/g, "").slice(0, 4))} onOperation={handleOperation} onExport={handleExport} onPreviousHistory={loadPreviousHistory} onNextHistory={loadMoreHistory} onAssistantInput={setAssistantInput} onAssistantSubmit={handleAssistantSubmit} onAssistantAccountSelect={handleAssistantAccountSelect} onPrepareMCPAction={(request) => void handlePrepareMCPAction(request)} onConfirmMCPAction={() => void handleConfirmMCPAction()} onCancelMCPAction={() => void handleCancelMCPAction()} onMCPPINChange={(pin) => setMcpPin(pin.replace(/\D/g, "").slice(0, 4))} onMCPConfirmationPINChange={(pin) => setMcpConfirmationPin(pin.replace(/\D/g, "").slice(0, 4))} onSetMCPPIN={handleSetMCPPIN} onProfileNameChange={setProfileFullName} onProfileSubmit={handleProfileSubmit} onLogout={handleLogout} />;
}

export default App;
