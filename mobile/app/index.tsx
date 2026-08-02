import * as SecureStore from "expo-secure-store";
import { useEffect, useState } from "react";
import { KeyboardAvoidingView, Platform, Pressable, SafeAreaView, ScrollView, Text, TextInput, useWindowDimensions, View } from "react-native";
import QRCode from "react-native-qrcode-svg";
import { Account, Balance, MFAEnrollment, MFAStatus, MobileApiError, OperationMode, Transaction, User, createIdempotencyKey, formatMinor, mobileApi } from "../src/api";

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

  useEffect(() => { void restoreSession(); }, []);
  useEffect(() => { if (session) void loadDashboard(session); }, [session]);

  async function restoreSession() {
    const stored = await SecureStore.getItemAsync(sessionKey);
    if (!stored) return;
    try { setSession(JSON.parse(stored) as Session); } catch { await SecureStore.deleteItemAsync(sessionKey); }
  }

  async function saveSession(next: Session | null) {
    setSession(next);
    if (next) await SecureStore.setItemAsync(sessionKey, JSON.stringify(next));
    else await SecureStore.deleteItemAsync(sessionKey);
  }

  async function authenticate() {
    setBusy(true); setNotice("");
    try {
      if (authMode === "register") await mobileApi.register({ email, password, full_name: fullName });
      const tokens = await mobileApi.login({ email, password, mfa_code: authMFA || undefined });
      await saveSession({ accessToken: tokens.access_token, refreshToken: tokens.refresh_token, user: tokens.user });
      setPassword(""); setAuthMFA(""); setAuthNeedsMFA(false);
    } catch (error) {
      if (error instanceof MobileApiError && error.body.code === "mfa_required") setAuthNeedsMFA(true);
      setNotice(publicError(error));
    } finally { setBusy(false); }
  }

  async function loadDashboard(current: Session) {
    try {
      const response = await mobileApi.accounts(current.accessToken);
      const selected = response.items[0] ?? null;
      setAccounts(response.items); setAccount(selected);
      const status = await mobileApi.mfaStatus(current.accessToken);
      setMfaStatus(status);
      if (selected) {
        const [nextBalance, nextHistory] = await Promise.all([mobileApi.balance(selected.id, current.accessToken), mobileApi.history(selected.id, current.accessToken)]);
        setBalance(nextBalance); setHistory(nextHistory.items);
      }
    } catch (error) { setNotice(publicError(error)); }
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
    try { setMfaEnrollment(await mobileApi.enrollMFA(session.accessToken)); setNotice("Escanea el QR y confirma el código del autenticador."); }
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
    await saveSession(null); setAccounts([]); setAccount(null); setBalance(null); setHistory([]); setMfaStatus(null); setMfaEnrollment(null);
  }

  if (!session) return <AuthView mode={authMode} setMode={(next) => { setAuthMode(next); setNotice(""); setAuthNeedsMFA(false); }} email={email} setEmail={setEmail} password={password} setPassword={setPassword} fullName={fullName} setFullName={setFullName} mfaCode={authMFA} setMfaCode={setAuthMFA} needsMFA={authNeedsMFA} busy={busy} notice={notice} onSubmit={authenticate} />;

  return <SafeAreaView className="flex-1 bg-[#f7f5ef]"><KeyboardAvoidingView className="flex-1" behavior={Platform.OS === "ios" ? "padding" : undefined}><ScrollView contentContainerStyle={{ paddingBottom: 48, maxWidth: 720, width: "100%", alignSelf: "center" }} className={compact ? "px-4 pt-6" : "px-5 pt-8"}>
    <View className="flex-row items-center justify-between"><View><Text className="text-xs font-semibold uppercase tracking-[3px] text-slate-500">Hypernova</Text><Text className="mt-2 text-3xl font-semibold text-[#10233f]">Hola, {session.user.full_name.split(" ")[0]}.</Text></View><Pressable accessibilityRole="button" onPress={logout}><Text className="font-semibold text-slate-500">Salir</Text></Pressable></View>
    <View className="mt-7 rounded-3xl bg-[#10233f] p-7"><Text className="text-sm text-slate-300">Saldo disponible</Text><Text className={compact ? "mt-4 text-4xl font-semibold text-white" : "mt-4 text-5xl font-semibold text-white"}>{formatMinor(balance?.available_balance ?? "0")}</Text><Text className="mt-3 text-sm text-slate-300">Cuenta HNL · TigerBeetle</Text></View>
    <View className="mt-5 rounded-3xl bg-white p-5"><Text className="text-xs font-bold uppercase tracking-[2px] text-slate-400">Operar</Text><View className="mt-4 flex-row gap-2">{(["withdrawal", "transfer", "deposit"] as OperationMode[]).map((item) => <Pressable key={item} accessibilityRole="button" className={`flex-1 rounded-full px-2 py-3 ${mode === item ? "bg-[#10233f]" : "bg-slate-100"}`} onPress={() => { setMode(item); setNotice(""); }}><Text className={`text-center text-xs font-bold ${mode === item ? "text-white" : "text-slate-500"}`}>{item === "withdrawal" ? "Retirar" : item === "transfer" ? "Transferir" : "Depositar"}</Text></Pressable>)}</View><TextInput className="mt-5 rounded-2xl bg-slate-100 px-4 py-4" keyboardType="number-pad" value={amount} onChangeText={(value) => setAmount(value.replace(/\D/g, ""))} placeholder="Importe en unidades menores" accessibilityLabel="Importe" /><TextInput className="mt-3 rounded-2xl bg-slate-100 px-4 py-4" style={{ display: mode === "transfer" ? "flex" : "none" }} value={destination} onChangeText={setDestination} placeholder="UUID de cuenta destino" autoCapitalize="none" accessibilityLabel="Cuenta destino" /><Pressable className="mt-4 rounded-full bg-[#8cf0c5] px-5 py-4" disabled={busy} onPress={submitOperation}><Text className="text-center font-semibold text-[#10233f]">{busy ? "Procesando…" : "Confirmar operación"}</Text></Pressable></View>
    {notice ? <Text accessibilityRole="alert" className="mt-4 rounded-2xl bg-amber-50 p-4 text-sm text-amber-800">{notice}</Text> : null}
    <View className="mt-5 rounded-3xl bg-white p-5"><View className="flex-row items-center justify-between"><View><Text className="text-xs font-bold uppercase tracking-[2px] text-slate-400">Actividad</Text><Text className="mt-2 text-xl font-semibold text-[#10233f]">Historial reciente</Text></View><Text className="text-xs text-slate-400">{accounts.length} cuenta{accounts.length === 1 ? "" : "s"}</Text></View>{history.length === 0 ? <Text className="mt-4 text-slate-500">No hay movimientos recientes.</Text> : history.map((item) => <View className="flex-row items-center justify-between border-b border-slate-100 py-4" key={`${item.transfer_id}-${item.created_at}`}><View><Text className="font-semibold text-[#10233f]">{item.type}</Text><Text className="mt-1 text-xs text-slate-400">{new Date(item.created_at).toLocaleString("es-PA")}</Text></View><Text className="font-semibold text-[#10233f]">{item.direction === "credit" ? "+" : "−"}{formatMinor(item.amount)}</Text></View>)}</View>
    <View className="mt-5 rounded-3xl bg-white p-5"><View className="flex-row items-center justify-between"><View><Text className="text-xs font-bold uppercase tracking-[2px] text-slate-400">Seguridad</Text><Text className="mt-2 text-xl font-semibold text-[#10233f]">Autenticación multifactor</Text></View><Text className={mfaStatus?.enabled ? "rounded-full bg-emerald-100 px-3 py-2 text-xs font-bold text-emerald-700" : "rounded-full bg-slate-100 px-3 py-2 text-xs font-bold text-slate-500"}>{mfaStatus?.enabled ? "Activo" : "Pendiente"}</Text></View><Text className="mt-3 text-sm leading-6 text-slate-500">Usa un código TOTP de Google Authenticator o Microsoft Authenticator.</Text>{!mfaStatus?.enabled && !mfaEnrollment ? <Pressable className="mt-4 rounded-full bg-[#10233f] px-5 py-4" disabled={mfaBusy} onPress={beginMFA}><Text className="text-center font-semibold text-white">{mfaBusy ? "Generando…" : "Configurar MFA"}</Text></Pressable> : null}{mfaEnrollment ? <View className="mt-5 items-center"><QRCode value={mfaEnrollment.otpauth_uri} size={Math.min(220, width - 80)} /><Text className="mt-4 text-center text-xs text-slate-500">Escanea el QR y escribe el código actual.</Text><Text className="mt-3 break-all rounded-xl bg-slate-100 p-3 text-center font-mono text-xs text-slate-600">{mfaEnrollment.secret}</Text><TextInput className="mt-4 w-full rounded-2xl bg-slate-100 px-4 py-4 text-center" keyboardType="number-pad" maxLength={6} value={mfaCode} onChangeText={(value) => setMfaCode(value.replace(/\D/g, ""))} placeholder="000000" accessibilityLabel="Código MFA" /><Pressable className="mt-3 w-full rounded-full bg-[#8cf0c5] px-5 py-4" disabled={mfaBusy || mfaCode.length !== 6} onPress={verifyMFA}><Text className="text-center font-semibold text-[#10233f]">{mfaBusy ? "Verificando…" : "Activar MFA"}</Text></Pressable></View> : null}</View>
    <Text className="mt-6 text-center text-xs text-slate-400">Tokens protegidos por SecureStore · importes en unidades menores</Text>
  </ScrollView></KeyboardAvoidingView></SafeAreaView>;
}

function AuthView(props: { mode: AuthMode; setMode: (mode: AuthMode) => void; email: string; setEmail: (value: string) => void; password: string; setPassword: (value: string) => void; fullName: string; setFullName: (value: string) => void; mfaCode: string; setMfaCode: (value: string) => void; needsMFA: boolean; busy: boolean; notice: string; onSubmit: () => void }) {
  return <SafeAreaView className="flex-1 bg-[#f7f5ef]"><KeyboardAvoidingView className="flex-1" behavior={Platform.OS === "ios" ? "padding" : undefined}><ScrollView contentContainerStyle={{ flexGrow: 1, justifyContent: "center", paddingBottom: 40 }} className="px-5"><Text className="text-xs font-semibold uppercase tracking-[3px] text-slate-500">Hypernova Banking</Text><Text className="mt-4 text-5xl font-semibold text-[#10233f]">Tu dinero, claro.</Text><Text className="mt-4 text-base leading-6 text-slate-500">Control financiero con seguridad reforzada y operaciones verificables.</Text><View className="mt-8 rounded-3xl bg-white p-6"><View className="flex-row gap-2">{(["login", "register"] as AuthMode[]).map((mode) => <Pressable key={mode} className={`flex-1 rounded-full px-3 py-3 ${props.mode === mode ? "bg-[#10233f]" : "bg-slate-100"}`} onPress={() => props.setMode(mode)}><Text className={`text-center text-xs font-bold ${props.mode === mode ? "text-white" : "text-slate-500"}`}>{mode === "login" ? "Entrar" : "Crear cuenta"}</Text></Pressable>)}</View>{props.mode === "register" ? <TextInput className="mt-6 rounded-2xl bg-slate-100 px-4 py-4" value={props.fullName} onChangeText={props.setFullName} placeholder="Nombre completo" autoComplete="name" /> : null}<TextInput className="mt-4 rounded-2xl bg-slate-100 px-4 py-4" value={props.email} onChangeText={props.setEmail} placeholder="Email" autoCapitalize="none" keyboardType="email-address" autoComplete="email" /><TextInput className="mt-4 rounded-2xl bg-slate-100 px-4 py-4" value={props.password} onChangeText={props.setPassword} placeholder="Contraseña" secureTextEntry autoCapitalize="none" autoComplete="password" />{props.needsMFA ? <TextInput className="mt-4 rounded-2xl bg-slate-100 px-4 py-4 text-center" value={props.mfaCode} onChangeText={(value) => props.setMfaCode(value.replace(/\D/g, ""))} placeholder="Código MFA de 6 dígitos" keyboardType="number-pad" maxLength={6} autoComplete="sms-otp" /> : null}<Pressable className="mt-6 rounded-full bg-[#8cf0c5] px-5 py-4" disabled={props.busy} onPress={props.onSubmit}><Text className="text-center font-semibold text-[#10233f]">{props.busy ? "Procesando…" : props.mode === "login" ? "Iniciar sesión" : "Crear cuenta HNL"}</Text></Pressable>{props.notice ? <Text accessibilityRole="alert" className="mt-4 text-sm text-red-700">{props.notice}</Text> : null}</View></ScrollView></KeyboardAvoidingView></SafeAreaView>;
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
