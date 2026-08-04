import { useEffect, useState } from "react";
import { Pressable, Text, TextInput, View } from "react-native";
import { User } from "../../api";
import { FeedbackMessage } from "../feedback/FeedbackMessage";

interface Props {
  user: User;
  name: string;
  busy: boolean;
  notice: string;
  onName: (name: string) => void;
  onSave: () => void;
  mcpPin: string;
  mcpPinConfigured: boolean;
  mcpPinExpiresAt?: string;
  mcpPinBusy: boolean;
  mcpPinNotice: string;
  onMCPPINChange: (pin: string) => void;
  onSetMCPPIN: () => void;
}

function formatPinCountdown(milliseconds: number): string {
  const totalSeconds = Math.max(0, Math.ceil(milliseconds / 1000));
  const minutes = Math.floor(totalSeconds / 60).toString().padStart(2, "0");
  const seconds = (totalSeconds % 60).toString().padStart(2, "0");
  return `${minutes}:${seconds}`;
}

export function SettingsPanel(props: Props) {
  const { user, name, busy, notice, onName, onSave, mcpPin, mcpPinConfigured, mcpPinExpiresAt, mcpPinBusy, mcpPinNotice, onMCPPINChange, onSetMCPPIN } = props;
  const [now, setNow] = useState(() => Date.now());
  const expiresAt = mcpPinExpiresAt ? new Date(mcpPinExpiresAt).getTime() : 0;
  const remaining = mcpPinConfigured && expiresAt ? Math.max(0, expiresAt - now) : 0;
  const pinExpired = mcpPinConfigured && Boolean(expiresAt) && remaining === 0;

  useEffect(() => {
    if (!mcpPinConfigured || !expiresAt) return undefined;
    const timer = setInterval(() => setNow(Date.now()), 1000);
    return () => clearInterval(timer);
  }, [expiresAt, mcpPinConfigured]);

  const status = pinExpired ? "Vencido" : mcpPinConfigured ? "Activo" : "No configurado";
  const statusStyle = pinExpired ? "bg-red-50 text-red-700" : mcpPinConfigured ? "bg-[#dff7f3] text-[#087e78]" : "bg-slate-100 text-slate-600";

  return <View className="rounded-3xl border border-slate-200 bg-white p-5">
    <Text className="text-xs font-bold uppercase tracking-[2px] text-slate-400">Configuraciones</Text>
    <Text className="mt-1 text-2xl font-semibold text-[#2d73a5]">Tu perfil y seguridad</Text>
    <Text className="mt-2 text-sm leading-5 text-slate-500">Manten tus datos personales actualizados y controla tus confirmaciones.</Text>
    <Text className="mt-6 text-xs font-bold uppercase tracking-wider text-slate-500">Nombre completo</Text>
    <TextInput className="mt-2 rounded-2xl border border-slate-200 bg-slate-50 px-4 py-4 text-[#24315e]" value={name} onChangeText={onName} maxLength={120} autoCapitalize="words" autoComplete="name" />
    <Text className="mt-4 text-xs font-bold uppercase tracking-wider text-slate-500">Correo electronico</Text>
    <TextInput className="mt-2 rounded-2xl border border-slate-200 bg-slate-100 px-4 py-4 text-slate-500" value={user.email} editable={false} autoComplete="email" />
    <Text className="mt-2 text-xs leading-5 text-slate-400">El correo requiere verificacion y no se cambia desde aqui.</Text>
    {notice ? <FeedbackMessage tone={notice.includes("actualizados") ? "success" : "error"} message={notice} /> : null}
    <Pressable className="mt-5 rounded-full bg-[#16c1b5] px-5 py-4" disabled={busy} onPress={onSave}><Text className="text-center font-semibold text-[#24315e]">{busy ? "Guardando..." : "Guardar cambios"}</Text></Pressable>
    <View className="mt-6 rounded-3xl border border-[#cfe9e5] bg-[#f8fffe] p-4">
      <View className="flex-row items-start justify-between"><View className="flex-1 pr-3"><Text className="text-xs font-bold uppercase tracking-wider text-[#087e78]">Confirmaciones seguras</Text><Text className="mt-1 text-lg font-semibold text-[#24315e]">PIN del asistente</Text><Text className="mt-1 text-sm leading-5 text-slate-500">Se solicita al confirmar operaciones y vence automaticamente cada tres minutos.</Text></View><Text className={`rounded-full px-3 py-2 text-xs font-bold ${statusStyle}`}>{status}</Text></View>
      <View className="mt-4 rounded-2xl border border-[#d8eceb] bg-white p-3" accessibilityLiveRegion="polite"><Text className="text-xs text-slate-500">{pinExpired ? "Crea un PIN nuevo para continuar." : mcpPinConfigured ? "PIN protegido y listo para confirmar." : "Configura un PIN para autorizar operaciones."}</Text>{mcpPinConfigured && !pinExpired ? <Text className="mt-1 text-sm font-bold text-[#2d73a5]">Expira en {formatPinCountdown(remaining)}</Text> : null}</View>
      <TextInput className="mt-4 rounded-2xl border border-slate-200 bg-white px-4 py-4 text-center text-[#24315e]" value={mcpPin} onChangeText={onMCPPINChange} keyboardType="number-pad" maxLength={4} secureTextEntry placeholder="••••" autoComplete="password" accessibilityLabel="PIN del asistente" />
      <Pressable className="mt-3 rounded-full bg-[#2d73a5] px-5 py-4" disabled={mcpPinBusy} onPress={onSetMCPPIN}><Text className="text-center text-sm font-bold text-white">{mcpPinBusy ? "Guardando..." : mcpPinConfigured && !pinExpired ? "Renovar PIN" : "Crear PIN"}</Text></Pressable>
      <Text className="mt-3 text-xs leading-5 text-slate-400">Tu PIN nunca se muestra ni se guarda en texto visible. Cinco intentos incorrectos activan un bloqueo temporal.</Text>
      {mcpPinNotice ? <FeedbackMessage tone={mcpPinNotice.toLowerCase().includes("activo") ? "success" : "error"} message={mcpPinNotice} /> : null}
    </View>
  </View>;
}
