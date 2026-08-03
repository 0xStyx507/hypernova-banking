import { useState } from "react";
import { Pressable, Text, TextInput, View } from "react-native";
import { Account, Balance, formatMinor } from "../../api";

function accountName(account: Account, index: number): string { return account.display_name || (index === 0 ? "Cuenta principal" : `Cuenta corriente ${index + 1}`); }
function maskAccount(id: string): string { return id.length > 10 ? `${id.slice(0, 4)}••••${id.slice(-4)}` : id; }

interface Props { accounts: Account[]; accountBalances: Record<string, Balance>; activeAccount: Account | null; busy: boolean; renameBusyId: string; notice: string; onSelect: (id: string) => void; onCreate: () => void; onRename: (id: string, name: string) => void; }

export function AccountList({ accounts, accountBalances, activeAccount, busy, renameBusyId, notice, onSelect, onCreate, onRename }: Props) {
  const [showNumbers, setShowNumbers] = useState(false);
  const [editingId, setEditingId] = useState("");
  const [editingName, setEditingName] = useState("");
  return <View className="rounded-3xl bg-white p-4 shadow-sm dark:bg-[#142235]">
    <View className="mb-3 flex-row items-center justify-between"><View><Text className="text-xs font-bold uppercase tracking-[2px] text-slate-400">Cuentas personales</Text><Text className="mt-1 text-lg font-semibold text-[#2d73a5]">Tus cuentas de depósito</Text></View><Pressable className="rounded-full bg-[#2d73a5] px-3 py-2" accessibilityLabel={showNumbers ? "Ocultar cuentas" : "Ver cuentas"} onPress={() => setShowNumbers((current) => !current)}><EyeIcon visible={showNumbers} /></Pressable></View>
    {accounts.map((item, index) => { const selected = item.id === activeAccount?.id; const itemBalance = accountBalances[item.id]; return <View key={item.id}><Pressable className={`mb-2 rounded-2xl border p-4 ${selected ? "border-[#16c1b5] bg-[#f0fcfa] dark:bg-[#173b42]" : "border-slate-200 bg-white dark:border-slate-700 dark:bg-[#1d3047]"}`} onPress={() => onSelect(item.id)}><View className="flex-row items-start justify-between"><View className="flex-1"><Text className="font-semibold text-[#2d73a5] dark:text-[#7bc7ec]">{accountName(item, index)}</Text><Text className="mt-1 text-xs text-slate-400">{showNumbers ? item.id : maskAccount(item.id)} · {item.status === "active" ? "Activa" : item.status}</Text></View><View className="items-end"><Text className="text-sm font-bold text-[#24315e] dark:text-slate-100">{itemBalance ? formatMinor(itemBalance.available_balance) : "—"}</Text><Pressable className="mt-2" onPress={() => { setEditingId(item.id); setEditingName(accountName(item, index)); }}><Text className="text-xs font-bold text-[#5b20a3] dark:text-[#d0a8ff]">Editar nombre</Text></Pressable></View></View></Pressable>{editingId === item.id ? <View className="mb-3 rounded-2xl bg-[#f4fafb] p-3 dark:bg-[#173b42]"><TextInput className="rounded-xl bg-white px-3 py-3 dark:bg-[#142235] dark:text-slate-100" value={editingName} onChangeText={setEditingName} maxLength={48} placeholder="Nombre de cuenta" /><View className="mt-2 flex-row justify-end gap-2"><Pressable className="rounded-full bg-slate-100 px-3 py-2 dark:bg-[#243a53]" onPress={() => setEditingId("")}><Text className="text-xs font-bold text-slate-600 dark:text-slate-200">Cancelar</Text></Pressable><Pressable className="rounded-full bg-[#16c1b5] px-3 py-2" disabled={renameBusyId === item.id} onPress={() => { onRename(item.id, editingName); setEditingId(""); }}><Text className="text-xs font-bold text-[#24315e]">{renameBusyId === item.id ? "Guardando…" : "Guardar"}</Text></Pressable></View></View> : null}</View>; })}
    <Pressable className="mt-1 flex-row items-center rounded-2xl bg-[#e8f8f6] px-4 py-3" disabled={busy} onPress={onCreate}><Text className="mr-2 text-xl font-bold text-[#2d73a5]">＋</Text><Text className="font-semibold text-[#2d73a5]">{busy ? "Abriendo cuenta…" : "Abrir nueva cuenta"}</Text></Pressable>
    {notice ? <Text className="mt-3 rounded-xl bg-amber-50 p-3 text-sm text-amber-800">{notice}</Text> : null}
  </View>;
}

function EyeIcon({ visible }: { visible: boolean }) {
  return <Text className="text-base text-white">{visible ? "◉" : "◌"}</Text>;
}
