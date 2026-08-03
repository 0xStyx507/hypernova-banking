import * as SecureStore from "expo-secure-store";
import { useEffect, useState } from "react";
import { Linking } from "react-native";
import { useColorScheme as useNativeColorScheme } from "nativewind";
import { Account, Balance, History, MCPAction, MFAEnrollment, MFAStatus, MobileApiError, OAuthProvider, OperationMode, User, createIdempotencyKey, mobileApi } from "../src/api";
import { maskEmail, sanitizeMfaCode } from "../src/auth";
import { AuthView } from "../src/components/AuthView";
import { MFAOnboarding } from "../src/components/MFAOnboarding";
import { MFAStatusGate } from "../src/components/MFAStatusGate";
import { MFAVerificationView } from "../src/components/MFAVerificationView";
import { MobileDashboard } from "../src/components/dashboard/MobileDashboard";
import { DashboardSection } from "../src/types";
import { currencyInputToMinor } from "../src/money";

type AuthMode = "login" | "register";

interface Session { accessToken: string; refreshToken: string; user: User }

const sessionKey = "hypernova.mobile.session";
const themeKey = "hypernova.mobile.theme";
type ThemeMode = "system" | "light" | "dark";

/** Mobile entry screen: authentication, balance, operations, history and MFA. */
export default function HomeScreen() {
  const colorScheme = useNativeColorScheme();
  const [themeMode, setThemeMode] = useState<ThemeMode>("system");
  const [session, setSession] = useState<Session | null>(null);
  const [authMode, setAuthMode] = useState<AuthMode>("login");
  const [authNeedsMFA, setAuthNeedsMFA] = useState(false);
  const [oauthPending, setOauthPending] = useState<{ provider: OAuthProvider; code: string } | null>(null);
  const [email, setEmail] = useState("");
  const [password, setPassword] = useState("");
  const [fullName, setFullName] = useState("");
  const [authMFA, setAuthMFA] = useState("");
  const [accounts, setAccounts] = useState<Account[]>([]);
  const [accountBalances, setAccountBalances] = useState<Record<string, Balance>>({});
  const [account, setAccount] = useState<Account | null>(null);
  const [balance, setBalance] = useState<Balance | null>(null);
  const [history, setHistory] = useState<History | null>(null);
  const [historyPages, setHistoryPages] = useState<History[]>([]);
  const [historyPage, setHistoryPage] = useState(1);
  const [historyBusy, setHistoryBusy] = useState(false);
  const [section, setSection] = useState<DashboardSection>("accounts");
  const [mode, setMode] = useState<OperationMode>("withdrawal");
  const [amount, setAmount] = useState("");
  const [destination, setDestination] = useState("");
  const [transferTargetType, setTransferTargetType] = useState<"own" | "external">("own");
  const [transferConfirmationPin, setTransferConfirmationPin] = useState("");
  const [busy, setBusy] = useState(false);
  const [notice, setNotice] = useState("");
  const [mfaStatus, setMfaStatus] = useState<MFAStatus | null>(null);
  const [mfaEnrollment, setMfaEnrollment] = useState<MFAEnrollment | null>(null);
  const [mfaCode, setMfaCode] = useState("");
  const [mfaBusy, setMfaBusy] = useState(false);
  const [mfaLoading, setMfaLoading] = useState(false);
  const [mfaCheckError, setMfaCheckError] = useState("");
  const [accountBusy, setAccountBusy] = useState(false);
  const [accountNotice, setAccountNotice] = useState("");
  const [accountRenameBusyId, setAccountRenameBusyId] = useState("");
  const [profileFullName, setProfileFullName] = useState("");
  const [profileBusy, setProfileBusy] = useState(false);
  const [profileNotice, setProfileNotice] = useState("");
  const [mcpPin, setMcpPin] = useState("");
  const [mcpPinConfigured, setMcpPinConfigured] = useState(false);
  const [mcpPinBusy, setMcpPinBusy] = useState(false);
  const [mcpPinNotice, setMcpPinNotice] = useState("");
  const [mcpActionPending, setMcpActionPending] = useState(false);
  const [mcpAction, setMcpAction] = useState<MCPAction | null>(null);

  useEffect(() => {
    void restoreSession();
    void SecureStore.getItemAsync(themeKey).then((saved) => {
      if (saved === "light" || saved === "dark" || saved === "system") setThemeMode(saved);
    });
  }, []);
  useEffect(() => {
    colorScheme.setColorScheme(themeMode);
    void SecureStore.setItemAsync(themeKey, themeMode);
  }, [colorScheme, themeMode]);
  useEffect(() => {
    const consumeOAuthUrl = (url: string) => {
      const query = new URL(url).searchParams;
      const provider = query.get("oauth_provider");
      const code = query.get("oauth_code");
      if ((provider !== "google" && provider !== "github") || typeof code !== "string") return;
      setBusy(true); setNotice("");
      void mobileApi.exchangeOAuth(provider, code)
        .then(async (tokens) => {
          await saveSession({ accessToken: tokens.access_token, refreshToken: tokens.refresh_token, user: tokens.user });
        })
        .catch((error) => {
          if (error instanceof MobileApiError && error.body.code === "mfa_required") {
            setOauthPending({ provider, code }); setAuthNeedsMFA(true); setAuthMode("login");
            setNotice("");
          } else setNotice(publicError(error));
        })
        .finally(() => setBusy(false));
    };
    const subscription = Linking.addEventListener("url", ({ url }) => consumeOAuthUrl(url));
    void Linking.getInitialURL().then((url) => { if (url) consumeOAuthUrl(url); });
    return () => subscription.remove();
  }, []);
  useEffect(() => { if (session) void loadMFA(session); }, [session]);
  useEffect(() => { if (session && mfaStatus?.enabled) void loadDashboard(session); }, [session, mfaStatus?.enabled]);
  useEffect(() => { if (session && mfaStatus?.enabled) void loadMCPPIN(); }, [session, mfaStatus?.enabled]);

  async function restoreSession() {
    const stored = await SecureStore.getItemAsync(sessionKey);
    if (!stored) return;
    try {
      setMfaLoading(true);
      const previous = JSON.parse(stored) as Partial<Session>;
      if (!previous.refreshToken) throw new Error("stored session has no refresh token");
      // Rotate the refresh token while restoring a native session. This avoids
      // trusting an expired access token and removes a stale/replayed session
      // instead of keeping the user on a half-authenticated screen.
      const tokens = await mobileApi.refresh(previous.refreshToken);
      await saveSession({ accessToken: tokens.access_token, refreshToken: tokens.refresh_token, user: tokens.user });
    } catch { await SecureStore.deleteItemAsync(sessionKey); setMfaLoading(false); }
  }

  async function saveSession(next: Session | null) {
    setMfaCheckError("");
    setMfaStatus(null);
    setMfaEnrollment(null);
    if (next) setMfaLoading(true);
    setSession(next);
    if (next) await SecureStore.setItemAsync(sessionKey, JSON.stringify(next));
    else await SecureStore.deleteItemAsync(sessionKey);
  }

  async function authenticate() {
    setBusy(true); setNotice("");
    try {
      const tokens = oauthPending
        ? await mobileApi.exchangeOAuth(oauthPending.provider, oauthPending.code, authMFA)
        : (authMode === "register" ? (await mobileApi.register({ email, password, full_name: fullName }), await mobileApi.login({ email, password, mfa_code: authMFA || undefined })) : await mobileApi.login({ email, password, mfa_code: authMFA || undefined }));
      await saveSession({ accessToken: tokens.access_token, refreshToken: tokens.refresh_token, user: tokens.user });
      setPassword(""); setAuthMFA(""); setAuthNeedsMFA(false); setOauthPending(null);
    } catch (error) {
      if (error instanceof MobileApiError && error.body.code === "mfa_required") {
        setAuthNeedsMFA(true);
        setNotice("");
      } else {
        setNotice(publicError(error));
      }
    } finally { setBusy(false); }
  }

  async function loadDashboard(current: Session) {
    try {
      const response = await mobileApi.accounts(current.accessToken);
      // Keep the account selected by the customer after a mutation or refresh.
      // Falling back to the first account is only needed when the previous
      // selection no longer exists (for example, after closing an account).
      const selected = response.items.find((item) => item.id === account?.id) ?? response.items[0] ?? null;
      const balanceEntries = await Promise.all(response.items.map(async (item) => { try { return [item.id, await mobileApi.balance(item.id, current.accessToken)] as const; } catch { return null; } }));
      setAccountBalances(Object.fromEntries(balanceEntries.filter((entry): entry is readonly [string, Balance] => entry !== null)));
      setAccounts(response.items); setAccount(selected); setProfileFullName(current.user.full_name); setHistoryPages([]); setHistoryPage(1);
      if (selected) await loadAccountData(selected.id, current.accessToken);
    } catch (error) { setNotice(publicError(error)); }
  }

  async function loadAccountData(accountId: string, accessToken: string) {
    const [nextBalance, nextHistory] = await Promise.all([mobileApi.balance(accountId, accessToken), mobileApi.history(accountId, accessToken)]);
    setBalance(nextBalance); setAccountBalances((current) => ({ ...current, [accountId]: nextBalance })); setHistory(nextHistory); setHistoryPages([]); setHistoryPage(1);
  }

  async function selectAccount(accountId: string) {
    if (!session) return;
    const selected = accounts.find((item) => item.id === accountId) ?? null;
    setAccount(selected); setHistory(null); setHistoryPages([]); setHistoryPage(1); setNotice("");
    if (selected) { try { await loadAccountData(selected.id, session.accessToken); } catch (error) { setNotice(publicError(error)); } }
  }

  async function createAccount() {
    if (!session) return;
    setAccountBusy(true); setAccountNotice("");
    try { const created = await mobileApi.createAccount(session.accessToken); const nextAccounts = [...accounts, created]; setAccounts(nextAccounts); setAccount(created); await loadAccountData(created.id, session.accessToken); setAccountNotice("Tu nueva cuenta USD está lista."); }
    catch (error) { setAccountNotice(publicError(error)); }
    finally { setAccountBusy(false); }
  }

  async function renameAccount(accountId: string, displayName: string) {
    if (!session || displayName.trim().length < 2) { setAccountNotice("Escribe un nombre de al menos dos caracteres."); return; }
    setAccountRenameBusyId(accountId); setAccountNotice("");
    try { const updated = await mobileApi.renameAccount(accountId, displayName.trim(), session.accessToken); setAccounts((current) => current.map((item) => item.id === accountId ? updated : item)); setAccountNotice("Nombre de cuenta actualizado."); }
    catch (error) { setAccountNotice(publicError(error)); }
    finally { setAccountRenameBusyId(""); }
  }

  async function loadMCPPIN() {
    if (!session || !mfaStatus?.enabled) return;
    try { const status = await mobileApi.mcpPINStatus(session.accessToken); const pending = await mobileApi.getPendingMCPAction(session.accessToken); setMcpPinConfigured(status.configured); setMcpAction(pending.action); setMcpActionPending(pending.action?.status === "ready"); }
    catch (error) { setMcpPinNotice(publicError(error)); }
  }

  async function saveMCPPIN() {
    if (!session || !/^\d{4}$/u.test(mcpPin)) { setMcpPinNotice("El PIN debe contener exactamente cuatro dígitos."); return; }
    setMcpPinBusy(true); setMcpPinNotice("");
    try { await mobileApi.setMCPPIN(mcpPin, session.accessToken); setMcpPin(""); setMcpPinConfigured(true); setMcpPinNotice("PIN activo durante tres minutos."); }
    catch (error) { setMcpPinNotice(publicError(error)); }
    finally { setMcpPinBusy(false); }
  }

  async function nextHistory() {
    if (!session || !account || !history?.next_cursor || historyBusy) return;
    setHistoryBusy(true);
    try { setHistoryPages((current) => [...current, history]); setHistory(await mobileApi.history(account.id, session.accessToken, { cursor: history.next_cursor, limit: 5 })); setHistoryPage((current) => current + 1); }
    catch (error) { setNotice(publicError(error)); }
    finally { setHistoryBusy(false); }
  }

  function previousHistory() {
    if (!historyPages.length || historyBusy) return;
    const previous = historyPages[historyPages.length - 1]; setHistoryPages((current) => current.slice(0, -1)); setHistory(previous); setHistoryPage((current) => Math.max(1, current - 1));
  }

  async function updateProfile() {
    if (!session || profileFullName.trim().length < 2) { setProfileNotice("Escribe un nombre válido."); return; }
    setProfileBusy(true); setProfileNotice("");
    try { const user = await mobileApi.updateProfile(profileFullName.trim(), session.accessToken); const next = { ...session, user }; setSession(next); await SecureStore.setItemAsync(sessionKey, JSON.stringify(next)); setProfileFullName(user.full_name); setProfileNotice("Tus datos fueron actualizados."); }
    catch (error) { setProfileNotice(publicError(error)); }
    finally { setProfileBusy(false); }
  }

  async function loadMFA(current: Session) {
    setMfaLoading(true);
    setMfaCheckError("");
    try {
      const status = await mobileApi.mfaStatus(current.accessToken);
      setMfaStatus(status);
      if (!status.enabled) {
        // MFA must be completed before account data becomes available.
        setMfaEnrollment(await mobileApi.enrollMFA(current.accessToken));
      }
    } catch (error) {
      const message = publicError(error);
      setMfaCheckError(message);
      setNotice(message);
    }
    finally { setMfaLoading(false); }
  }

  async function submitOperation() {
    if (!session || !account || !amount) return;
    const minorAmount = currencyInputToMinor(amount);
    if (!minorAmount || minorAmount === "0") { setNotice("Escribe un monto válido mayor que USD 0.00."); return; }
    setBusy(true); setNotice("");
    try {
      const key = createIdempotencyKey();
      if (mode === "deposit") await mobileApi.deposit(account.id, minorAmount, session.accessToken, key);
      if (mode === "withdrawal") await mobileApi.withdraw(account.id, minorAmount, session.accessToken, key);
      if (mode === "transfer") await mobileApi.transfer(account.id, destination, minorAmount, session.accessToken, key, transferTargetType === "external" ? transferConfirmationPin : undefined, transferTargetType);
      setAmount(""); setDestination(""); setTransferConfirmationPin(""); setNotice("Operación registrada correctamente."); await loadDashboard(session);
    } catch (error) { setNotice(publicError(error)); } finally { setBusy(false); }
  }

  async function beginMFA() {
    if (!session) return;
    setMfaBusy(true); setNotice("");
    try { setMfaEnrollment(await mobileApi.enrollMFA(session.accessToken)); setNotice("Guarda la clave manual en tu aplicación autenticadora y confirma el código actual."); }
    catch (error) { setNotice(publicError(error)); }
    finally { setMfaBusy(false); }
  }

  async function verifyMFA() {
    if (!session || mfaCode.length !== 6) return;
    setMfaBusy(true); setNotice("");
    try { setMfaStatus(await mobileApi.verifyMFA(mfaCode, session.accessToken)); setMfaEnrollment(null); setMfaCode(""); setNotice("MFA activado correctamente."); }
    catch (error) { setNotice(publicError(error)); }
    finally { setMfaBusy(false); }
  }

  async function logout() {
    await mobileApi.logout(session?.accessToken ?? "").catch(() => undefined);
    await saveSession(null); setAccounts([]); setAccountBalances({}); setAccount(null); setBalance(null); setHistory(null); setHistoryPages([]); setHistoryPage(1); setMfaCode(""); setMfaLoading(false); setSection("accounts"); setMcpPin(""); setMcpPinConfigured(false); setMcpPinNotice(""); setMcpActionPending(false); setMcpAction(null);
  }

  function navigateDashboard(nextSection: DashboardSection) {
    if (mcpActionPending && nextSection === "operations" && section !== "operations") {
      setNotice("Tienes una operación pendiente en el asistente. Confírmala o cancélala antes de iniciar otra.");
      return;
    }
    setNotice("");
    setSection(nextSection);
  }

  if (!session && (authNeedsMFA || oauthPending)) return <MFAVerificationView accountLabel={email ? maskEmail(email) : "tu cuenta"} code={authMFA} busy={busy} notice={notice} onCodeChange={(value) => { setAuthMFA(sanitizeMfaCode(value)); setNotice(""); }} onSubmit={authenticate} onBack={() => { setAuthNeedsMFA(false); setOauthPending(null); setAuthMFA(""); setNotice(""); }} />;

  if (!session) return <AuthView mode={authMode} setMode={(next) => { setAuthMode(next); setNotice(""); setAuthNeedsMFA(false); setOauthPending(null); }} email={email} setEmail={setEmail} password={password} setPassword={setPassword} fullName={fullName} setFullName={setFullName} busy={busy} notice={notice} onSubmit={authenticate} onOAuth={(provider) => { setBusy(true); void Linking.openURL(mobileApi.oauthStartUrl(provider, "hypernova://oauth")).catch((error) => setNotice(publicError(error))).finally(() => setBusy(false)); }} />;

  if (!mfaStatus) return <MFAStatusGate loading={mfaLoading} notice={mfaCheckError} onRetry={() => { void loadMFA(session); }} onLogout={logout} />;

  if (!mfaStatus.enabled) return <MFAOnboarding user={session.user} enrollment={mfaEnrollment} code={mfaCode} busy={mfaBusy} loading={mfaLoading} notice={notice} onCodeChange={setMfaCode} onBegin={beginMFA} onVerify={verifyMFA} onLogout={logout} />;

  return <MobileDashboard themeMode={themeMode} onThemeModeChange={setThemeMode} user={session.user} accessToken={session.accessToken} accounts={accounts} accountBalances={accountBalances} activeAccount={account} balance={balance} history={history} historyPage={historyPage} historyBusy={historyBusy} hasMoreHistory={Boolean(history?.has_more)} section={section} operationMode={mode} operationAmount={amount} destinationAccountId={destination} transferTargetType={transferTargetType} transferConfirmationPin={transferConfirmationPin} operationBusy={busy} notice={notice} accountBusy={accountBusy} accountNotice={accountNotice} accountRenameBusyId={accountRenameBusyId} profileFullName={profileFullName || session.user.full_name} profileBusy={profileBusy} profileNotice={profileNotice} mcpPin={mcpPin} mcpPinConfigured={mcpPinConfigured} mcpPinBusy={mcpPinBusy} mcpPinNotice={mcpPinNotice} mcpActionPending={mcpActionPending} mcpAction={mcpAction} onNavigate={navigateDashboard} onAccountChange={(accountId) => { void selectAccount(accountId); }} onCreateAccount={() => { void createAccount(); }} onRenameAccount={(accountId, name) => { void renameAccount(accountId, name); }} onOperationModeChange={(nextMode) => { setMode(nextMode); setNotice(""); }} onAmountChange={setAmount} onDestinationChange={setDestination} onTransferTargetTypeChange={(target) => { setTransferTargetType(target); setDestination(""); setTransferConfirmationPin(""); }} onTransferConfirmationPinChange={(pin) => setTransferConfirmationPin(pin)} onOperation={() => { void submitOperation(); }} onNextHistory={() => { void nextHistory(); }} onPreviousHistory={previousHistory} onProfileNameChange={setProfileFullName} onProfileSubmit={() => { void updateProfile(); }} onMCPPINChange={(pin) => setMcpPin(pin.replace(/\D/g, "").slice(0, 4))} onSetMCPPIN={() => { void saveMCPPIN(); }} onLogout={() => { void logout(); }} onMCPActionPendingChange={setMcpActionPending} onMCPActionExpired={() => { setMcpAction(null); setMcpActionPending(false); setNotice("La operación pendiente expiró. Puedes iniciar otra operación."); }} onMCPActionConfirmed={(action) => { setMcpAction(action); setMcpActionPending(false); const accountID = action.payload.account_id || action.payload.source_account_id; const selected = accountID ? accounts.find((item) => item.id === accountID) : undefined; if (selected && session) { setAccount(selected); void loadAccountData(selected.id, session.accessToken).then(() => new Promise((resolve) => setTimeout(resolve, 250))).then(() => loadAccountData(selected.id, session.accessToken)); } else if (session) void loadDashboard(session); }} />;
}


function publicError(error: unknown): string {
    if (error instanceof MobileApiError) {
    if (error.body.code === "insufficient_funds") return "Fondos insuficientes.";
    if (error.body.code === "demo_deposit_disabled") return "El depósito demo está desactivado en este entorno.";
    if (error.body.code === "mfa_required") return "Escribe el código de tu autenticador para continuar.";
    if (error.body.code === "mfa_invalid_code") return "El código MFA no es válido o expiró.";
    if (error.body.code === "mcp_pin_error" || error.body.code === "mcp_pin_unavailable") return "No pudimos guardar tu PIN de confirmación. Inténtalo nuevamente.";
    if (error.body.code === "mcp_pin_not_configured") return "Configura tu PIN en Ajustes antes de confirmar una operación.";
    if (error.body.code === "mcp_pin_expired") return "Tu PIN venció. Genera uno nuevo en Ajustes.";
    if (error.body.code === "mcp_pin_invalid") return "El PIN no coincide. Revísalo e inténtalo nuevamente.";
    if (error.body.code === "external_account_not_found") return "No encontramos la cuenta destino.";
    if (error.body.code === "account_not_found") return "La cuenta seleccionada no está disponible. Actualiza tus cuentas e inténtalo nuevamente.";
    if (error.body.code === "external_transfer_pin_required") return "Escribe tu PIN para transferir a otra cuenta.";
    if (error.body.code === "invalid_transfer_type") return "Selecciona un tipo de transferencia válido.";
    if (error.body.code === "invalid_currency") return "Las cuentas Hyper Bank usan USD.";
    return error.body.error;
  }
  return "No pudimos completar la solicitud.";
}
