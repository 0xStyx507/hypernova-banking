import * as SecureStore from "expo-secure-store";
import { useEffect, useState } from "react";
import { Pressable, SafeAreaView, ScrollView, Text, TextInput, View } from "react-native";
import { Account, Balance, MobileApiError, OperationMode, Transaction, User, createIdempotencyKey, formatMinor, mobileApi } from "../src/api";

type AuthMode = "login" | "register";
interface Session { accessToken: string; refreshToken: string; user: User }

const sessionKey = "hypernova.mobile.session";

export default function HomeScreen() {
  const [session, setSession] = useState<Session | null>(null);
  const [authMode, setAuthMode] = useState<AuthMode>("login");
  const [email, setEmail] = useState("");
  const [password, setPassword] = useState("");
  const [fullName, setFullName] = useState("");
  const [accounts, setAccounts] = useState<Account[]>([]);
  const [account, setAccount] = useState<Account | null>(null);
  const [balance, setBalance] = useState<Balance | null>(null);
  const [history, setHistory] = useState<Transaction[]>([]);
  const [mode, setMode] = useState<OperationMode>("withdrawal");
  const [amount, setAmount] = useState("");
  const [destination, setDestination] = useState("");
  const [busy, setBusy] = useState(false);
  const [notice, setNotice] = useState("");

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
      const tokens = await mobileApi.login({ email, password });
      await saveSession({ accessToken: tokens.access_token, refreshToken: tokens.refresh_token, user: tokens.user });
      setPassword("");
    } catch (error) { setNotice(publicError(error)); } finally { setBusy(false); }
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

  async function logout() { await mobileApi.logout(session?.accessToken ?? "").catch(() => undefined); await saveSession(null); setAccounts([]); setAccount(null); setBalance(null); setHistory([]); }

  if (!session) return <AuthView mode={authMode} setMode={setAuthMode} email={email} setEmail={setEmail} password={password} setPassword={setPassword} fullName={fullName} setFullName={setFullName} busy={busy} notice={notice} onSubmit={authenticate} />;

  return <SafeAreaView className="flex-1 bg-[#f7f5ef]"><ScrollView contentContainerStyle={{ paddingBottom: 40 }} className="px-5 pt-8"><View className="flex-row items-center justify-between"><View><Text className="text-xs font-semibold uppercase tracking-[3px] text-slate-500">Hypernova</Text><Text className="mt-2 text-3xl font-semibold text-[#10233f]">Hola, {session.user.full_name.split(" ")[0]}.</Text></View><Pressable onPress={logout}><Text className="font-semibold text-slate-500">Salir</Text></Pressable></View><View className="mt-8 rounded-3xl bg-[#10233f] p-7"><Text className="text-sm text-slate-300">Saldo disponible</Text><Text className="mt-4 text-5xl font-semibold text-white">{formatMinor(balance?.available_balance ?? "0")}</Text><Text className="mt-3 text-sm text-slate-300">Cuenta HNL · TigerBeetle</Text></View><View className="mt-5 rounded-3xl bg-white p-5"><Text className="text-xs font-bold uppercase tracking-[2px] text-slate-400">Operar</Text><View className="mt-4 flex-row gap-2">{(["withdrawal", "transfer", "deposit"] as OperationMode[]).map((item) => <Pressable key={item} className={`flex-1 rounded-full px-2 py-3 ${mode === item ? "bg-[#10233f]" : "bg-slate-100"}`} onPress={() => setMode(item)}><Text className={`text-center text-xs font-bold ${mode === item ? "text-white" : "text-slate-500"}`}>{item === "withdrawal" ? "Retirar" : item === "transfer" ? "Transferir" : "Depositar"}</Text></Pressable>)}</View><TextInput className="mt-5 rounded-2xl bg-slate-100 px-4 py-4" keyboardType="number-pad" value={amount} onChangeText={(value) => setAmount(value.replace(/\D/g, ""))} placeholder="Importe en unidades menores" /><TextInput className="mt-3 rounded-2xl bg-slate-100 px-4 py-4" style={{ display: mode === "transfer" ? "flex" : "none" }} value={destination} onChangeText={setDestination} placeholder="UUID de cuenta destino" autoCapitalize="none" /><Pressable className="mt-4 rounded-full bg-[#8cf0c5] px-5 py-4" disabled={busy} onPress={submitOperation}><Text className="text-center font-semibold text-[#10233f]">{busy ? "Procesando…" : "Confirmar operación"}</Text></Pressable></View>{notice ? <Text className="mt-4 rounded-2xl bg-amber-50 p-4 text-sm text-amber-800">{notice}</Text> : null}<View className="mt-5 rounded-3xl bg-white p-5"><Text className="text-xs font-bold uppercase tracking-[2px] text-slate-400">Actividad</Text>{history.length === 0 ? <Text className="mt-4 text-slate-500">No hay movimientos recientes.</Text> : history.map((item) => <View className="flex-row items-center justify-between border-b border-slate-100 py-4" key={`${item.transfer_id}-${item.created_at}`}><View><Text className="font-semibold text-[#10233f]">{item.type}</Text><Text className="mt-1 text-xs text-slate-400">{new Date(item.created_at).toLocaleString("es-PA")}</Text></View><Text className="font-semibold text-[#10233f]">{item.direction === "credit" ? "+" : "−"}{formatMinor(item.amount)}</Text></View>)}</View><Text className="mt-6 text-center text-xs text-slate-400">Tokens protegidos por SecureStore · HNL minor units</Text></ScrollView></SafeAreaView>;
}

function AuthView(props: { mode: AuthMode; setMode: (mode: AuthMode) => void; email: string; setEmail: (value: string) => void; password: string; setPassword: (value: string) => void; fullName: string; setFullName: (value: string) => void; busy: boolean; notice: string; onSubmit: () => void }) {
  return <SafeAreaView className="flex-1 bg-[#f7f5ef]"><ScrollView contentContainerStyle={{ paddingBottom: 40 }} className="px-5 pt-16"><Text className="text-xs font-semibold uppercase tracking-[3px] text-slate-500">Hypernova Banking</Text><Text className="mt-4 text-5xl font-semibold text-[#10233f]">Tu dinero, claro.</Text><View className="mt-10 rounded-3xl bg-white p-6"><View className="flex-row gap-2">{(["login", "register"] as AuthMode[]).map((mode) => <Pressable key={mode} className={`flex-1 rounded-full px-3 py-3 ${props.mode === mode ? "bg-[#10233f]" : "bg-slate-100"}`} onPress={() => props.setMode(mode)}><Text className={`text-center text-xs font-bold ${props.mode === mode ? "text-white" : "text-slate-500"}`}>{mode === "login" ? "Entrar" : "Crear cuenta"}</Text></Pressable>)}</View>{props.mode === "register" ? <TextInput className="mt-6 rounded-2xl bg-slate-100 px-4 py-4" value={props.fullName} onChangeText={props.setFullName} placeholder="Nombre completo" /> : null}<TextInput className="mt-4 rounded-2xl bg-slate-100 px-4 py-4" value={props.email} onChangeText={props.setEmail} placeholder="Email" autoCapitalize="none" keyboardType="email-address" /><TextInput className="mt-4 rounded-2xl bg-slate-100 px-4 py-4" value={props.password} onChangeText={props.setPassword} placeholder="Contraseña" secureTextEntry autoCapitalize="none" /><Pressable className="mt-6 rounded-full bg-[#8cf0c5] px-5 py-4" disabled={props.busy} onPress={props.onSubmit}><Text className="text-center font-semibold text-[#10233f]">{props.busy ? "Procesando…" : props.mode === "login" ? "Iniciar sesión" : "Crear cuenta HNL"}</Text></Pressable>{props.notice ? <Text className="mt-4 text-sm text-red-700">{props.notice}</Text> : null}</View></ScrollView></SafeAreaView>;
}

function publicError(error: unknown): string {
  if (error instanceof MobileApiError) {
    if (error.body.code === "insufficient_funds") return "Fondos insuficientes.";
    if (error.body.code === "demo_deposit_disabled") return "El depósito demo está desactivado en este entorno.";
    return error.body.error;
  }
  return "No pudimos completar la solicitud.";
}
