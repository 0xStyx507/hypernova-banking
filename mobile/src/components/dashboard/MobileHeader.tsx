import { Pressable, Text, View } from "react-native";
import { DashboardSection } from "../../types";

interface Props { name: string; email: string; onLogout: () => void; onThemeToggle: () => void; }

export function MobileHeader({ name, email, onLogout, onThemeToggle }: Props) {
  const initials = name.split(/\s+/u).map((part) => part[0]).join("").slice(0, 2).toUpperCase();
  return <View className="bg-transparent px-5 pb-2 pt-4">
    <View className="flex-row items-center justify-between">
      <View className="flex-1"><Text className="text-xl font-bold text-[#24315e] dark:text-[#f6f8fc]">Hola, {name.split(/\s+/u)[0] || "cliente"}</Text><Text className="mt-1 text-[10px] font-medium text-[#16c1b5]">Tus finanzas están protegidas</Text></View>
      <View className="flex-row items-center gap-2"><Pressable accessibilityLabel="Cambiar tema" className="h-10 w-10 items-center justify-center rounded-2xl bg-[#eef3f8] dark:bg-[#102033]" onPress={onThemeToggle}><Text className="text-base text-[#24315e] dark:text-[#f6f8fc]">☼</Text></Pressable><Pressable accessibilityLabel="Notificaciones" className="h-10 w-10 items-center justify-center rounded-2xl bg-[#eef3f8] dark:bg-[#102033]"><Text className="text-sm text-[#24315e] dark:text-[#f6f8fc]">♢</Text></Pressable><Pressable accessibilityLabel={`Cerrar sesión de ${email}`} className="h-10 w-10 items-center justify-center rounded-2xl bg-[#4f8cff]" onPress={onLogout}><Text className="text-xs font-bold text-white">{initials}</Text></Pressable></View>
    </View>
  </View>;
}

const navigation: Array<{ id: DashboardSection; label: string; icon: string }> = [
  { id: "accounts", label: "Inicio", icon: "⌂" },
  { id: "history", label: "Historial", icon: "▣" },
  { id: "operations", label: "Operar", icon: "↔" },
  { id: "settings", label: "Ajustes", icon: "⚙" },
];

export function MobileBottomNav({ active, onNavigate }: { active: DashboardSection; onNavigate: (section: DashboardSection) => void }) {
  return <View className="mx-5 flex-row rounded-3xl bg-white px-2 pb-2 pt-2 shadow-sm dark:bg-[#0d1b2a]">{navigation.map((item) => <Pressable className="flex-1 items-center rounded-2xl py-2" key={item.id} onPress={() => onNavigate(item.id)}><Text className={`text-xl ${active === item.id ? "text-[#38d9ff]" : "text-slate-400 dark:text-[#718399]"}`}>{item.icon}</Text><Text className={`mt-1 text-[10px] font-bold ${active === item.id ? "text-[#24315e] dark:text-[#f6f8fc]" : "text-slate-400 dark:text-[#9fb0c5]"}`}>{item.label}</Text></Pressable>)}</View>;
}
