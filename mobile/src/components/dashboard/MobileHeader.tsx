import { Pressable, Text, View } from "react-native";
import { DashboardSection } from "../../types";
import { HyperBankLogo } from "../HyperBankLogo";

interface Props { name: string; email: string; onLogout: () => void; }

export function MobileHeader({ name, email, onLogout }: Props) {
  return <View className="border-b border-slate-200 bg-[#2d73a5] px-5 pb-4 pt-3 dark:border-slate-700">
    <View className="flex-row items-center justify-between">
      <View><HyperBankLogo inverse /><Text className="mt-1 text-xs text-[#d9f8f4]">Banca digital para tu día a día</Text></View>
      <View className="items-end"><Text className="max-w-[150px] text-right text-sm font-bold text-white" numberOfLines={1}>{name}</Text><Text className="max-w-[150px] text-right text-[10px] text-[#d9f8f4]" numberOfLines={1}>{email}</Text></View>
    </View>
    <View className="mt-4 flex-row items-center justify-end"><Pressable className="rounded-full bg-[#16c1b5] px-4 py-2" onPress={onLogout}><Text className="text-xs font-bold text-[#24315e]">Salir</Text></Pressable></View>
  </View>;
}

const navigation: Array<{ id: DashboardSection; label: string; icon: string }> = [
  { id: "accounts", label: "Cuentas", icon: "◎" },
  { id: "history", label: "Historial", icon: "≡" },
  { id: "operations", label: "Operar", icon: "↔" },
  { id: "settings", label: "Ajustes", icon: "⚙" },
];

export function MobileBottomNav({ active, onNavigate }: { active: DashboardSection; onNavigate: (section: DashboardSection) => void }) {
  return <View className="flex-row border-t border-slate-200 bg-white px-2 pb-2 pt-2 dark:border-slate-700 dark:bg-[#142235]">{navigation.map((item) => <Pressable className="flex-1 items-center rounded-2xl py-2" key={item.id} onPress={() => onNavigate(item.id)}><Text className={`text-xl ${active === item.id ? "text-[#16c1b5]" : "text-slate-400"}`}>{item.icon}</Text><Text className={`mt-1 text-[10px] font-bold ${active === item.id ? "text-[#2d73a5] dark:text-[#7bc7ec]" : "text-slate-400"}`}>{item.label}</Text></Pressable>)}</View>;
}
