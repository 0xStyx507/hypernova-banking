import { useEffect, useState } from "react";
import { Pressable, ScrollView, Text, TextInput, View } from "react-native";
import { Balance, ChatResponse, MCPAccountOption, MCPAction, MCPConversationState, MobileApiError, mobileApi, formatMinor } from "../../api";

interface Message { role: "user" | "assistant"; text: string; data?: unknown; confirmation?: boolean; accountOptions?: MCPAccountOption[]; }
interface Props { accessToken: string; accountId?: string; accountBalances: Record<string, Balance>; mcpPinConfigured: boolean; initialAction?: MCPAction | null; onCreatePin: () => void; onPendingChange: (pending: boolean) => void; onConfirmed: (action: MCPAction) => void; onExpired: () => void; }

function isPendingAction(action?: MCPAction | null): boolean {
  return action?.status === "ready" || action?.status === "confirming";
}

function readableLabel(value: string): string { return value.replace(/_/g, " ").replace(/\b\w/g, (character) => character.toUpperCase()); }
function maskAccount(value: unknown): string { const text = String(value ?? ""); return text.length > 12 ? `${text.slice(0, 6)}…${text.slice(-4)}` : text; }
function operationLabel(value: unknown): string { return value === "deposit" ? "Depósito" : value === "withdrawal" ? "Retiro" : "Transferencia"; }
function statusLabel(value: unknown): string { return value === "succeeded" || value === "confirmed" ? "Completada" : value === "ready" ? "Lista para confirmar" : "Registrada"; }
function renderData(data: unknown, accountBalances: Record<string, Balance>): string[] {
  if (data === null || data === undefined) return [];
  if (typeof data === "object" && !Array.isArray(data)) {
    const object = data as Record<string, unknown>;
    if (Array.isArray(object.items)) {
      return object.items.slice(0, 5).flatMap((item, index) => {
        const row = item as Record<string, unknown>;
        if ("transfer_id" in row && "amount" in row) return [`${operationLabel(row.type)} · ${statusLabel(row.status)}`, `${row.direction === "credit" ? "+" : "−"}${formatMinor(String(row.amount ?? "0"))}`, `Referencia: ${maskAccount(row.transfer_id)} · ${row.created_at ? new Date(String(row.created_at)).toLocaleDateString("es-PA") : "fecha pendiente"}`];
        const balance = accountBalances[String(row.id ?? "")];
        return [`${String(row.display_name || `Cuenta ${index + 1}`)} · ${String(row.currency ?? "USD")}`, `Saldo disponible: ${balance ? formatMinor(balance.available_balance) : "pendiente"}`, `Cuenta: ${maskAccount(row.id)}`];
      });
    }
    if ("transfer_id" in object && "amount" in object && "type" in object) return [`${operationLabel(object.type)} · ${statusLabel(object.status)}`, `${object.direction === "credit" ? "+" : "−"}${formatMinor(String(object.amount))}`, `Referencia: ${maskAccount(object.transfer_id)} · ${object.created_at ? new Date(String(object.created_at)).toLocaleDateString("es-PA") : "fecha pendiente"}`];
    if ("available_balance" in object && "account_id" in object) return [`Saldo disponible: ${formatMinor(String(object.available_balance))}`, `Cuenta: ${maskAccount(object.account_id)}`];
  }
  if (Array.isArray(data)) return data.flatMap((item) => renderData(item, accountBalances));
  if (typeof data === "object") return Object.entries(data as Record<string, unknown>).flatMap(([key, value]) => {
    if (key.endsWith("_pending") || key.endsWith("_posted")) return [];
    if (key.endsWith("_id")) return [`${readableLabel(key)}: ${maskAccount(value)}`];
    if (key === "amount" && typeof value === "string") return [`${readableLabel(key)}: ${formatMinor(value)}`];
    if (typeof value === "object") return [`${readableLabel(key)}:`, ...renderData(value, accountBalances)];
    return [`${readableLabel(key)}: ${String(value)}`];
  });
  return [String(data)];
}

/** Floating assistant. Critical operations still require an explicit PIN. */
export function ChatbotFab(props: Props) {
  const { accessToken, accountId, accountBalances, mcpPinConfigured, initialAction, onCreatePin, onPendingChange, onConfirmed, onExpired } = props;
  const [open, setOpen] = useState(false);
  const [input, setInput] = useState("");
  const [confirmationPin, setConfirmationPin] = useState("");
  const [busy, setBusy] = useState(false);
  const [messages, setMessages] = useState<Message[]>([]);
  const [action, setAction] = useState<MCPAction | null>(initialAction ?? null);
  const [conversation, setConversation] = useState<MCPConversationState | null>(null);
  const [actionNotice, setActionNotice] = useState("");
  const [secureOpen, setSecureOpen] = useState(false);

  useEffect(() => { onPendingChange(isPendingAction(action)); }, [action, onPendingChange]);
  useEffect(() => { if (initialAction && !action) setAction(initialAction); }, [action, initialAction]);
  useEffect(() => {
    const pendingAction = action;
    if (!pendingAction || !isPendingAction(pendingAction)) return;
    const expiresAt = Date.parse(pendingAction.expires_at);
    if (!Number.isFinite(expiresAt)) return;
    const expireAction = () => { setAction(null); setConversation(null); setConfirmationPin(""); setActionNotice("La operación pendiente expiró. Puedes iniciar una nueva operación."); onExpired(); };
    const delay = expiresAt - Date.now();
    if (delay <= 0) { expireAction(); return; }
    const timer = setTimeout(expireAction, delay);
    return () => clearTimeout(timer);
  }, [action, onExpired]);

  useEffect(() => { setConversation(null); setAction(null); setConfirmationPin(""); }, [accountId]);

  async function send(messageOverride?: string) {
    const text = (messageOverride ?? input).trim();
    if (!text || busy) return;
    setInput(""); setMessages((current) => [...current, { role: "user", text }]); setBusy(true);
    try {
      const reply: ChatResponse = await mobileApi.chat(text, accessToken, accountId, conversation);
      setAction(reply.action ?? null); setConversation(reply.conversation ?? null); setActionNotice("");
      setMessages((current) => [...current, { role: "assistant", text: reply.message, data: reply.read_only_data, confirmation: reply.requires_confirmation, accountOptions: reply.account_options }]);
    } catch (error) {
      setMessages((current) => [...current, { role: "assistant", text: error instanceof MobileApiError ? error.body.error : "No pudimos responder ahora." }]);
    } finally { setBusy(false); }
  }

  async function confirmAction() {
    if (!action) return;
    if (!mcpPinConfigured) { onCreatePin(); return; }
    if (!/^\d{4}$/u.test(confirmationPin)) { setActionNotice("Escribe los cuatro dígitos de tu PIN."); return; }
    setBusy(true); setActionNotice("");
    try {
      const confirmed = await mobileApi.confirmMCPAction(action.id, confirmationPin, accessToken);
      setAction(confirmed); setConversation(null); setConfirmationPin(""); onConfirmed(confirmed);
      setMessages((current) => [...current, { role: "assistant", text: "Operación confirmada. El comprobante quedó registrado.", data: confirmed.operation }]);
    } catch (error) { setActionNotice(error instanceof MobileApiError ? error.body.error : "No pudimos confirmar la operación."); }
    finally { setBusy(false); }
  }

  async function cancelAction() {
    if (!action || busy) return;
    setBusy(true); setActionNotice("");
    try {
      await mobileApi.cancelMCPAction(action.id, accessToken);
      setAction(null); setConversation(null); setConfirmationPin("");
      setMessages((current) => [...current, { role: "assistant", text: "Cancelé la operación. Puedes iniciar otra cuando quieras." }]);
    } catch (error) { setActionNotice(error instanceof MobileApiError ? error.body.error : "No pudimos cancelar la operación."); }
    finally { setBusy(false); }
  }

  return <>
    {secureOpen && action && isPendingAction(action) ? <SecureConfirmationScreen action={action} pin={confirmationPin} busy={busy} configured={mcpPinConfigured} notice={actionNotice} onPinChange={(value) => setConfirmationPin(value.replace(/\D/g, "").slice(0, 4))} onConfirm={() => void confirmAction()} onCancel={() => void cancelAction()} onClose={() => setSecureOpen(false)} onCreatePin={onCreatePin} /> : null}
    {open ? <View className="absolute bottom-36 right-2 z-20 w-[calc(100%-16px)] max-w-[430px] overflow-hidden rounded-3xl border border-slate-200 bg-white shadow-lg dark:border-[#213a55] dark:bg-[#0d1b2a]">
      <View className="flex-row items-center justify-between bg-[#5b20a3] px-5 py-4"><View><Text className="text-lg font-bold text-white">Asistente Hypernova</Text><Text className="mt-1 text-xs text-[#a8e5df]">Conectado y listo</Text></View><Pressable onPress={() => setOpen(false)}><Text className="text-2xl text-white">×</Text></Pressable></View>
      <ScrollView className="max-h-96 px-4 py-4" contentContainerStyle={{ gap: 10 }}>
        {!messages.length ? <><View className="self-start rounded-2xl rounded-tl-sm bg-slate-100 px-4 py-3 dark:bg-[#1d3047]"><Text className="text-base leading-6 text-slate-700 dark:text-slate-100">¡Hola! Puedo consultar tus cuentas, revisar movimientos o preparar una operación.</Text><Text className="mt-2 text-xs leading-5 text-slate-500 dark:text-slate-300">Elige una consulta rápida o escribe lo que necesitas.</Text></View><View className="mt-2 flex-row flex-wrap gap-2"><Pressable className="rounded-full border border-[#cfe9e5] bg-white px-3 py-2" disabled={busy} onPress={() => void send("¿Cuál es mi saldo disponible?")}><Text className="text-xs font-bold text-[#2d73a5]">Ver saldo</Text></Pressable><Pressable className="rounded-full border border-[#cfe9e5] bg-white px-3 py-2" disabled={busy} onPress={() => void send("¿Qué movimientos tuve este mes?")}><Text className="text-xs font-bold text-[#2d73a5]">Movimientos</Text></Pressable><Pressable className="rounded-full border border-[#cfe9e5] bg-white px-3 py-2" disabled={busy} onPress={() => void send("¿Cuánto gasté este mes?")}><Text className="text-xs font-bold text-[#2d73a5]">Resumen</Text></Pressable></View></> : messages.map((message, index) => <View className={`max-w-[88%] rounded-2xl px-4 py-3 ${message.role === "user" ? "self-end bg-[#e8f8f6]" : "self-start bg-slate-100 dark:bg-[#1d3047]"}`} key={`${message.role}-${index}`}><Text className="text-sm leading-5 text-slate-700 dark:text-slate-100">{message.text}</Text>{message.accountOptions?.length ? <View className="mt-3 w-full gap-2"><Text className="text-xs font-bold uppercase tracking-wider text-slate-500 dark:text-slate-300">Elige una cuenta</Text>{message.accountOptions.map((account) => <Pressable className="flex-row items-center justify-between rounded-xl border border-[#d8eceb] bg-white px-3 py-3 dark:bg-[#142235]" disabled={busy} key={account.id} onPress={() => void send(`cuenta ${account.id}`)}><View><Text className="text-sm font-bold text-[#2d73a5] dark:text-[#7bc7ec]">{account.display_name || "Cuenta"}</Text><Text className="mt-1 text-xs text-slate-500 dark:text-slate-300">{account.currency} · {maskAccount(account.id)}</Text></View><Text className="text-xl text-[#2d73a5]">›</Text></Pressable>)}</View> : null}{message.data ? <View className="mt-2 w-full rounded-xl bg-white p-3 dark:bg-[#142235]">{renderData(message.data, props.accountBalances).map((line, lineIndex) => <Text className="mt-1 text-xs text-slate-600 dark:text-slate-300" key={`${line}-${lineIndex}`}>{line}</Text>)}</View> : null}{message.confirmation ? <Text className="mt-2 text-xs font-semibold text-[#5b20a3]">La operación requiere confirmación explícita con tu PIN.</Text> : null}</View>)}
        {isPendingAction(action) ? <View className="rounded-2xl border border-[#d8eceb] bg-[#f4fafb] p-4 dark:bg-[#173b42]"><Text className="font-semibold text-[#2d73a5] dark:text-[#7bc7ec]">{action?.status === "confirming" ? "Recuperar confirmación" : "Resumen listo para confirmar"}</Text><Text className="mt-1 text-xs font-bold text-slate-700 dark:text-slate-100">{action?.payload.action === "deposit" ? "Depósito" : action?.payload.action === "withdrawal" ? "Retiro" : "Transferencia"}</Text><Text className="mt-1 text-base font-bold text-[#2d73a5] dark:text-[#7bc7ec]">{formatMinor(action?.payload.amount ?? "0")}</Text><Text className="mt-1 text-xs text-slate-600 dark:text-slate-300">Revisa los datos y confirma con tu PIN.</Text>{mcpPinConfigured ? <><TextInput className="mt-3 rounded-xl bg-white px-4 py-3 text-center tracking-[5px] dark:bg-[#142235] dark:text-slate-100" value={confirmationPin} onChangeText={(value) => setConfirmationPin(value.replace(/\D/g, "").slice(0, 4))} keyboardType="number-pad" secureTextEntry maxLength={4} autoComplete="password" placeholder="PIN de 4 dígitos" accessibilityLabel="PIN de confirmación" /><Pressable className="mt-3 rounded-full bg-[#16c1b5] px-4 py-3" disabled={busy} onPress={() => void confirmAction()}><Text className="text-center font-bold text-[#24315e]">{busy ? "Confirmando…" : action?.status === "confirming" ? "Reintentar confirmación" : "Confirmar operación"}</Text></Pressable></> : <Pressable className="mt-3 rounded-full bg-[#2d73a5] px-4 py-3" onPress={onCreatePin}><Text className="text-center font-bold text-white">Crear PIN en Ajustes</Text></Pressable>}<Pressable className="mt-2 px-4 py-2" disabled={busy} onPress={() => void cancelAction()}><Text className="text-center text-xs font-bold text-slate-500 underline dark:text-slate-300">Cancelar operación</Text></Pressable>{actionNotice ? <Text className="mt-2 text-xs text-red-700">{actionNotice}</Text> : null}</View> : null}
        {isPendingAction(action) ? <Pressable className="mx-1 rounded-full border border-[#2d73a5] px-4 py-3" onPress={() => setSecureOpen(true)}><Text className="text-center text-xs font-bold text-[#2d73a5]">Revisar en pantalla segura</Text></Pressable> : null}
      </ScrollView>
      <View className="flex-row items-center border-t border-slate-200 p-3 dark:border-slate-700"><TextInput className="flex-1 rounded-full border-2 border-[#2d73a5] bg-white px-4 py-3 dark:bg-[#1d3047] dark:text-slate-100" editable={!busy && !isPendingAction(action)} value={input} onChangeText={setInput} placeholder={isPendingAction(action) ? "Confirma o cancela la operación…" : "Escribe tu consulta…"} onSubmitEditing={() => void send()} returnKeyType="send" /><Pressable className="ml-2 h-11 w-11 items-center justify-center rounded-full bg-[#16c1b5]" disabled={busy || isPendingAction(action)} onPress={() => void send()}><Text className="text-2xl text-[#24315e]">›</Text></Pressable></View>
    </View> : null}
    <Text className="absolute bottom-28 right-24 z-20 rounded-full border border-slate-200 bg-white px-3 py-2 text-xs font-bold text-[#24315e] shadow-lg dark:border-[#213a55] dark:bg-[#0d1b2a] dark:text-[#f6f8fc]">Asistente</Text><Pressable accessibilityRole="button" accessibilityLabel={open ? "Cerrar chat" : "Abrir chat"} className="absolute bottom-24 right-5 z-20 h-16 w-16 items-center justify-center rounded-full bg-[#5b20a3] shadow-lg" onPress={() => setOpen((current) => !current)}><Text className="text-3xl text-white">{open ? "×" : "✦"}</Text></Pressable>
  </>;
}

interface SecureConfirmationProps {
  action: MCPAction;
  pin: string;
  busy: boolean;
  configured: boolean;
  notice: string;
  onPinChange: (value: string) => void;
  onConfirm: () => void;
  onCancel: () => void;
  onClose: () => void;
  onCreatePin: () => void;
}

/** Full-screen review state from the Figma secure-confirmation flow. */
function SecureConfirmationScreen(props: SecureConfirmationProps) {
  const [remaining, setRemaining] = useState(() => Math.max(0, Date.parse(props.action.expires_at) - Date.now()));
  useEffect(() => {
    const timer = setInterval(() => setRemaining(Math.max(0, Date.parse(props.action.expires_at) - Date.now())), 1000);
    return () => clearInterval(timer);
  }, [props.action.expires_at]);

  return <View className="absolute inset-0 z-40 bg-[#f7f9fb] dark:bg-[#07111f]">
    <ScrollView contentContainerStyle={{ padding: 20, paddingTop: 58, paddingBottom: 36 }} keyboardShouldPersistTaps="handled">
      <View className="flex-row items-center"><Pressable accessibilityLabel="Volver" className="mr-3 h-10 w-10 items-center justify-center rounded-2xl bg-white dark:bg-[#0d1b2a]" onPress={props.onClose}><Text className="text-xl text-[#24315e] dark:text-white">‹</Text></Pressable><View><Text className="text-xl font-bold text-[#24315e] dark:text-[#f6f8fc]">Confirmación segura</Text><Text className="mt-1 text-xs text-slate-500 dark:text-[#9fb0c5]">Operación preparada por MCP</Text></View></View>
      <View className="mt-6 rounded-[24px] bg-white p-5 dark:bg-[#0d1b2a]"><Text className="text-xs font-bold uppercase tracking-[1.5px] text-[#5b20a3]">{operationLabel(props.action.action)} preparada</Text><Text className="mt-2 text-3xl font-bold text-[#24315e] dark:text-[#f6f8fc]">{formatMinor(props.action.payload.amount)}</Text><Text className="mt-1 text-xs text-slate-500 dark:text-[#9fb0c5]">Revisa todos los datos antes de confirmar.</Text><View className="mt-5 rounded-2xl bg-[#f4f7fb] p-4 dark:bg-[#102033]"><ReviewRow label="Cuenta origen" value={maskAccount(props.action.payload.source_account_id ?? props.action.payload.account_id)} /><ReviewRow label="Cuenta destino" value={maskAccount(props.action.payload.destination_account_id ?? "No aplica")} /><ReviewRow label="Comisión" value="Sin comisión" /><ReviewRow label="Saldo resultante" value="Se actualizará al confirmar" /></View></View>
      <View className="mt-5 rounded-[22px] border border-[#d8eceb] bg-[#f4fafb] p-4 dark:border-[#215b62] dark:bg-[#173b42]"><Text className="font-semibold text-[#24315e] dark:text-[#f6f8fc]">Tu asistente espera tu autorización</Text><Text className="mt-2 text-xs leading-5 text-slate-600 dark:text-[#c5d3e2]">El asistente no ejecutará la operación hasta validar tu PIN. Puedes cancelar esta operación en cualquier momento.</Text></View>
      <View className="mt-5 rounded-[22px] bg-white p-5 dark:bg-[#0d1b2a]"><Text className="text-xs font-bold uppercase tracking-[1.5px] text-[#2d73a5]">PIN de confirmación</Text><TextInput className="mt-3 rounded-2xl border border-slate-200 bg-[#f4f7fb] px-4 py-4 text-center text-xl tracking-[8px] dark:border-[#344b65] dark:bg-[#102033] dark:text-white" value={props.pin} onChangeText={props.onPinChange} keyboardType="number-pad" secureTextEntry maxLength={4} placeholder="••••" placeholderTextColor="#9fb0c5" accessibilityLabel="PIN de confirmación" />{props.notice ? <Text accessibilityRole="alert" className="mt-3 rounded-2xl bg-red-50 p-3 text-xs leading-5 text-red-700">{props.notice}</Text> : null}<Text className="mt-3 text-center text-xs text-slate-500 dark:text-[#9fb0c5]">Expira en {formatCountdown(remaining)}</Text><Pressable className="mt-4 rounded-2xl bg-[#16c1b5] px-5 py-4" disabled={props.busy} onPress={props.configured ? props.onConfirm : props.onCreatePin}><Text className="text-center font-bold text-[#24315e]">{props.busy ? "Validando…" : props.configured ? "Confirmar transferencia" : "Crear PIN en Ajustes"}</Text></Pressable><Pressable className="mt-3 rounded-2xl bg-[#eef3f8] px-5 py-4 dark:bg-[#102033]" disabled={props.busy} onPress={props.onCancel}><Text className="text-center font-bold text-[#24315e] dark:text-[#f6f8fc]">Cancelar operación</Text></Pressable></View>
    </ScrollView>
  </View>;
}

function ReviewRow({ label, value }: { label: string; value: string }) {
  return <View className="flex-row items-center justify-between border-b border-slate-200 py-2 last:border-0 dark:border-[#213a55]"><Text className="text-xs text-slate-500 dark:text-[#9fb0c5]">{label}</Text><Text className="max-w-[58%] text-right text-xs font-bold text-[#24315e] dark:text-[#f6f8fc]">{value}</Text></View>;
}

function formatCountdown(milliseconds: number): string {
  const seconds = Math.ceil(milliseconds / 1000);
  return `${Math.floor(seconds / 60).toString().padStart(2, "0")}:${(seconds % 60).toString().padStart(2, "0")}`;
}
