import * as SecureStore from "expo-secure-store";
import { useEffect, useState } from "react";
import { KeyboardAvoidingView, Linking, Platform, Pressable, SafeAreaView, ScrollView, Text, TextInput, useWindowDimensions, View } from "react-native";
import { Account, Balance, MFAEnrollment, MFAStatus, MobileApiError, OAuthProvider, OperationMode, Transaction, User, createIdempotencyKey, formatMinor, mobileApi } from "../src/api";
import { maskEmail, sanitizeMfaCode } from "../src/auth";
import { AuthView } from "../src/components/AuthView";
import { MFAOnboarding } from "../src/components/MFAOnboarding";
import { MFAStatusGate } from "../src/components/MFAStatusGate";
import { MFAVerificationView } from "../src/components/MFAVerificationView";

type AuthMode = "login" | "register";

interface Session { accessToken: string; refreshToken: string; user: User }

const sessionKey = "hypernova.mobile.session";

/** Mobile entry screen: authentication, balance, operations, history and MFA. */
export default function HomeScreen() {
  const { width } = useWindowDimensions();
  const compact = width < 390;
  const [session, setSession] = useState<Session | null>(null);
  const [authMode, setAuthMode] = useState<AuthMode>("login");
  const [authNeedsMFA, setAuthNeedsMFA] = useState(false);
  const [oauthPending, setOauthPending] = useState<{ provider: OAuthProvider; code: string } | null>(null);
  const [email, setEmail] = useState("");
  const [password, setPassword] = useState("");
  const [fullName, setFullName] = useState("");
  const [authMFA, setAuthMFA] = useState("");
  const [accounts, setAccounts] = useState<Account[]>([]);
  const [account, setAccount] = useState<Account | null>(null);
  const [balance, setBalance] = useState<Balance | null>(null);
  const [history, setHistory] = useState<Transaction[]>([]);
  const [mode, setMode] = useState<OperationMode>("withdrawal");
  const [amount, setAmount] = useState("");
  const [destination, setDestination] = useState("");
  const [busy, setBusy] = useState(false);
  const [notice, setNotice] = useState("");
  const [mfaStatus, setMfaStatus] = useState<MFAStatus | null>(null);
  const [mfaEnrollment, setMfaEnrollment] = useState<MFAEnrollment | null>(null);
  const [mfaCode, setMfaCode] = useState("");
  const [mfaBusy, setMfaBusy] = useState(false);
  const [mfaLoading, setMfaLoading] = useState(false);
  const [mfaCheckError, setMfaCheckError] = useState("");

  useEffect(() => { void restoreSession(); }, []);
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

  async function restoreSession() {
    const stored = await SecureStore.getItemAsync(sessionKey);
    if (!stored) return;
    try {
      setMfaLoading(true);
      setSession(JSON.parse(stored) as Session);
    } catch { await SecureStore.deleteItemAsync(sessionKey); }
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
      const selected = response.items[0] ?? null;
      setAccounts(response.items); setAccount(selected);
      if (selected) {
        const [nextBalance, nextHistory] = await Promise.all([mobileApi.balance(selected.id, current.accessToken), mobileApi.history(selected.id, current.accessToken)]);
        setBalance(nextBalance); setHistory(nextHistory.items);
      }
    } catch (error) { setNotice(publicError(error)); }
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
    setBusy(true); setNotice("");
    try {
      const key = createIdempotencyKey();
      if (mode === "deposit") await mobileApi.deposit(account.id, amount, session.accessToken, key);
      if (mode === "withdrawal") await mobileApi.withdraw(account.id, amount, session.accessToken, key);
      if (mode === "transfer") await mobileApi.transfer(account.id, destination, amount, session.accessToken, key);
      setAmount(""); setDestination(""); setNotice("Operación registrada correctamente."); await loadDashboard(session);
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
    await saveSession(null); setAccounts([]); setAccount(null); setBalance(null); setHistory([]); setMfaCode(""); setMfaLoading(false);
  }

  if (!session && (authNeedsMFA || oauthPending)) return <MFAVerificationView accountLabel={email ? maskEmail(email) : "tu cuenta"} code={authMFA} busy={busy} notice={notice} onCodeChange={(value) => { setAuthMFA(sanitizeMfaCode(value)); setNotice(""); }} onSubmit={authenticate} onBack={() => { setAuthNeedsMFA(false); setOauthPending(null); setAuthMFA(""); setNotice(""); }} />;

  if (!session) return <AuthView mode={authMode} setMode={(next) => { setAuthMode(next); setNotice(""); setAuthNeedsMFA(false); setOauthPending(null); }} email={email} setEmail={setEmail} password={password} setPassword={setPassword} fullName={fullName} setFullName={setFullName} busy={busy} notice={notice} onSubmit={authenticate} onOAuth={(provider) => { setBusy(true); void Linking.openURL(mobileApi.oauthStartUrl(provider, "hypernova://oauth")).catch((error) => setNotice(publicError(error))).finally(() => setBusy(false)); }} />;

  if (!mfaStatus) return <MFAStatusGate loading={mfaLoading} notice={mfaCheckError} onRetry={() => { void loadMFA(session); }} onLogout={logout} />;

  if (!mfaStatus.enabled) return <MFAOnboarding user={session.user} enrollment={mfaEnrollment} code={mfaCode} busy={mfaBusy} loading={mfaLoading} notice={notice} onCodeChange={setMfaCode} onBegin={beginMFA} onVerify={verifyMFA} onLogout={logout} />;

  return <SafeAreaView className="flex-1 bg-[#f7f9fb]"><KeyboardAvoidingView className="flex-1" behavior={Platform.OS === "ios" ? "padding" : undefined}><ScrollView contentContainerStyle={{ paddingBottom: 48, maxWidth: 720, width: "100%", alignSelf: "center" }} className={compact ? "px-4 pt-6" : "px-5 pt-8"}>
    <View className="flex-row items-center justify-between"><View><Text className="text-xs font-semibold uppercase tracking-[3px] text-slate-500">Hypernova · Resumen</Text><Text className="mt-2 text-3xl font-semibold text-[#2d73a5]">Hola, {session.user.full_name.split(" ")[0]}.</Text><Text className="mt-2 text-sm text-slate-500">Tu posición y actividad reciente.</Text></View><View className="items-end"><Text className="mb-2 rounded-full bg-[#dff7f3] px-3 py-1 text-xs font-bold text-[#087e78]">MFA activo</Text><Pressable accessibilityRole="button" onPress={logout}><Text className="font-semibold text-slate-500">Salir</Text></Pressable></View></View>
    <View className="mt-7 rounded-3xl bg-[#2d73a5] p-7"><View className="flex-row items-start justify-between"><View><Text className="text-sm text-slate-300">Saldo disponible</Text><Text className={compact ? "mt-4 text-4xl font-semibold text-white" : "mt-4 text-5xl font-semibold text-white"}>{formatMinor(balance?.available_balance ?? "0")}</Text></View><Text className="rounded-full bg-white/10 px-3 py-1 text-xs font-bold text-white">HNL</Text></View><Text className="mt-4 text-xs text-slate-400">Fondos disponibles · cuenta protegida</Text></View>
    <View className="mt-5 rounded-3xl bg-white p-5"><Text className="text-xs font-bold uppercase tracking-[2px] text-slate-400">Operar</Text><View className="mt-4 flex-row gap-2">{(["withdrawal", "transfer", "deposit"] as OperationMode[]).map((item) => <Pressable key={item} accessibilityRole="button" className={`flex-1 rounded-full px-2 py-3 ${mode === item ? "bg-[#2d73a5]" : "bg-slate-100"}`} onPress={() => { setMode(item); setNotice(""); }}><Text className={`text-center text-xs font-bold ${mode === item ? "text-white" : "text-slate-500"}`}>{item === "withdrawal" ? "Retirar" : item === "transfer" ? "Transferir" : "Depositar"}</Text></Pressable>)}</View><TextInput className="mt-5 rounded-2xl bg-slate-100 px-4 py-4" keyboardType="number-pad" value={amount} onChangeText={(value) => setAmount(value.replace(/\D/g, ""))} placeholder="Importe en unidades menores" accessibilityLabel="Importe" /><TextInput className="mt-3 rounded-2xl bg-slate-100 px-4 py-4" style={{ display: mode === "transfer" ? "flex" : "none" }} value={destination} onChangeText={setDestination} placeholder="UUID de cuenta destino" autoCapitalize="none" accessibilityLabel="Cuenta destino" /><Pressable className="mt-4 rounded-full bg-[#16c1b5] px-5 py-4" disabled={busy} onPress={submitOperation}><Text className="text-center font-semibold text-[#2d73a5]">{busy ? "Procesando…" : "Confirmar operación"}</Text></Pressable></View>
    {notice ? <Text accessibilityRole="alert" className="mt-4 rounded-2xl bg-amber-50 p-4 text-sm text-amber-800">{notice}</Text> : null}
    <View className="mt-5 rounded-3xl bg-white p-5"><View className="flex-row items-center justify-between"><View><Text className="text-xs font-bold uppercase tracking-[2px] text-slate-400">Actividad</Text><Text className="mt-2 text-xl font-semibold text-[#2d73a5]">Historial reciente</Text></View><Text className="text-xs text-slate-400">{accounts.length} cuenta{accounts.length === 1 ? "" : "s"}</Text></View>{history.length === 0 ? <Text className="mt-4 text-slate-500">No hay movimientos recientes.</Text> : history.map((item) => <View className="flex-row items-center justify-between border-b border-slate-100 py-4" key={`${item.transfer_id}-${item.created_at}`}><View><Text className="font-semibold text-[#2d73a5]">{item.type}</Text><Text className="mt-1 text-xs text-slate-400">{new Date(item.created_at).toLocaleString("es-PA")}</Text></View><Text className="font-semibold text-[#2d73a5]">{item.direction === "credit" ? "+" : "−"}{formatMinor(item.amount)}</Text></View>)}</View>
    <Text className="mt-6 text-center text-xs text-slate-400">Tokens protegidos por SecureStore · importes en unidades menores</Text>
  </ScrollView></KeyboardAvoidingView></SafeAreaView>;
}


function publicError(error: unknown): string {
  if (error instanceof MobileApiError) {
    if (error.body.code === "insufficient_funds") return "Fondos insuficientes.";
    if (error.body.code === "demo_deposit_disabled") return "El depósito demo está desactivado en este entorno.";
    if (error.body.code === "mfa_required") return "Escribe el código de tu autenticador para continuar.";
    if (error.body.code === "mfa_invalid_code") return "El código MFA no es válido o expiró.";
    return error.body.error;
  }
  return "No pudimos completar la solicitud.";
}
