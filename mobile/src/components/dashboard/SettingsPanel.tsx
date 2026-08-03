import { Pressable, Text, TextInput, View } from "react-native";
import { User } from "../../api";

interface Props {
  user: User;
  name: string;
  busy: boolean;
  notice: string;
  onName: (name: string) => void;
  onSave: () => void;
  mcpPin: string;
  mcpPinConfigured: boolean;
  mcpPinBusy: boolean;
  mcpPinNotice: string;
  onMCPPINChange: (pin: string) => void;
  onSetMCPPIN: () => void;
}

export function SettingsPanel(props: Props) {
  const { user, name, busy, notice, onName, onSave, mcpPin, mcpPinConfigured, mcpPinBusy, mcpPinNotice, onMCPPINChange, onSetMCPPIN } = props;
  return <View className="rounded-3xl bg-white p-5">
    <Text className="text-xs font-bold uppercase tracking-[2px] text-slate-400">Configuraciones</Text>
    <Text className="mt-1 text-2xl font-semibold text-[#2d73a5]">Tu perfil y seguridad</Text>
    <Text className="mt-2 text-sm leading-5 text-slate-500">Mantén tus datos personales actualizados.</Text>
    <Text className="mt-6 text-xs font-bold uppercase tracking-wider text-slate-500">Nombre completo</Text>
    <TextInput className="mt-2 rounded-2xl bg-slate-100 px-4 py-4 text-[#24315e]" value={name} onChangeText={onName} maxLength={120} autoCapitalize="words" autoComplete="name" />
    <Text className="mt-4 text-xs font-bold uppercase tracking-wider text-slate-500">Correo electrónico</Text>
    <TextInput className="mt-2 rounded-2xl bg-slate-100 px-4 py-4 text-slate-500" value={user.email} editable={false} autoComplete="email" />
    <Text className="mt-2 text-xs leading-5 text-slate-400">El correo requiere un proceso de verificación y no se cambia desde aquí.</Text>
    {notice ? <Text className="mt-3 rounded-xl bg-amber-50 p-3 text-sm text-amber-800">{notice}</Text> : null}
    <Pressable className="mt-5 rounded-full bg-[#16c1b5] px-5 py-4" disabled={busy} onPress={onSave}><Text className="text-center font-semibold text-[#24315e]">{busy ? "Guardando…" : "Guardar cambios"}</Text></Pressable>
    <View className="mt-6 rounded-2xl bg-[#f0fcfa] p-4">
      <Text className="font-semibold text-[#2d73a5]">PIN del asistente</Text>
      <Text className="mt-1 text-sm leading-5 text-slate-500">{mcpPinConfigured ? "Tu PIN está activo durante tres minutos." : "Crea un PIN para confirmar operaciones desde el chatbot."}</Text>
      <TextInput className="mt-3 rounded-xl bg-white px-4 py-3 text-center tracking-[5px]" value={mcpPin} onChangeText={onMCPPINChange} keyboardType="number-pad" maxLength={4} secureTextEntry placeholder="••••" autoComplete="password" accessibilityLabel="PIN del asistente" />
      <Pressable className="mt-3 rounded-full bg-[#2d73a5] px-5 py-3" disabled={mcpPinBusy} onPress={onSetMCPPIN}><Text className="text-center text-sm font-bold text-white">{mcpPinBusy ? "Guardando…" : mcpPinConfigured ? "Renovar PIN" : "Crear PIN"}</Text></Pressable>
      {mcpPinNotice ? <Text className="mt-3 rounded-xl bg-amber-50 p-3 text-sm text-amber-800">{mcpPinNotice}</Text> : null}
    </View>
  </View>;
}
