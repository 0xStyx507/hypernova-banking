import { Text, View } from "react-native";
import { Transaction, formatMinor } from "../../api";

function isIncoming(item: Transaction): boolean {
  return item.type === "deposit" || item.direction === "credit";
}

/** Compact, color-coded activity chart that remains legible in light and dark themes. */
export function TransactionChart({ transactions }: { transactions: Transaction[] }) {
  const points = transactions.slice(0, 5).reverse();
  if (!points.length) return null;
  let max = 1n;
  for (const item of points) {
    try { const amount = BigInt(item.amount); if (amount > max) max = amount; } catch { /* Display fallback stays at the minimum bar height. */ }
  }

  return <View className="mt-5 rounded-3xl border border-slate-200 bg-[#fbfdff] p-4 dark:border-slate-700 dark:bg-[#142235]">
    <Text className="text-xs font-bold uppercase tracking-[2px] text-slate-400">Resumen visual</Text>
    <Text className="mt-1 text-lg font-semibold text-[#2d73a5] dark:text-[#7bc7ec]">Entradas y salidas</Text>
    <View className="mt-5 h-40 flex-row items-end justify-between gap-2">
      {points.map((item) => {
        let amount = 0n;
        try { amount = BigInt(item.amount); } catch { /* Keep the bar visible for malformed display data. */ }
        const height = Math.max(14, Number((amount * 100n) / max));
        const incoming = isIncoming(item);
        const barColor = incoming ? "#16c1b5" : "#8b5cf6";
        return <View className="flex-1 items-center" key={`${item.transfer_id}-chart`}>
          <Text className="mb-1 text-[9px] font-semibold text-slate-500 dark:text-slate-300" numberOfLines={1}>{formatMinor(item.amount)}</Text>
          <View className="w-full flex-1 justify-end rounded-t-xl bg-slate-200 dark:bg-slate-700"><View className="w-full rounded-t-xl" style={{ height: `${height}%`, backgroundColor: barColor }} /></View>
          <Text className="mt-1 text-[9px] text-slate-500 dark:text-slate-300">{new Date(item.created_at).toLocaleDateString("es-PA", { day: "2-digit", month: "short" })}</Text>
        </View>;
      })}
    </View>
    <View className="mt-4 flex-row flex-wrap gap-x-5 gap-y-2"><View className="flex-row items-center"><View className="h-3 w-3 rounded-sm bg-[#16c1b5]" /><Text className="ml-2 text-xs font-semibold text-slate-600 dark:text-slate-200">Depósitos</Text></View><View className="flex-row items-center"><View className="h-3 w-3 rounded-sm bg-[#8b5cf6]" /><Text className="ml-2 text-xs font-semibold text-slate-600 dark:text-slate-200">Retiros y transferencias</Text></View></View>
  </View>;
}
