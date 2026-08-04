import { Text, View } from "react-native";
import { Transaction, formatMinor } from "../../api";

function isIncoming(item: Transaction): boolean { return item.type === "deposit" || item.direction === "credit"; }

function amountRatio(item: Transaction, maximum: bigint): number {
  try { return maximum > 0n ? Number(BigInt(item.amount) * 100n / maximum) / 100 : 0; } catch { return 0; }
}

function LineLane({ items, incoming }: { items: Transaction[]; incoming: boolean }) {
  const maximum = items.reduce((current, item) => { try { const amount = BigInt(item.amount); return amount > current ? amount : current; } catch { return current; } }, 1n);
  const points = items.map((item, index) => ({ x: items.length <= 1 ? 50 : 4 + (index * 92) / (items.length - 1), y: incoming ? 82 - amountRatio(item, maximum) * 64 : 18 + amountRatio(item, maximum) * 64 }));
  return <View className="relative mt-2 h-20 overflow-hidden rounded-xl bg-[#f4f8fb] dark:bg-[#17264b]">
    <View className="absolute inset-x-2 top-1/2 h-px bg-[#d9e4ed] dark:bg-[#304979]" />
    {points.slice(1).map((point, index) => { const previous = points[index]; const dx = point.x - previous.x; const dy = point.y - previous.y; const length = Math.sqrt(dx * dx + dy * dy); const angle = Math.atan2(dy, dx) * (180 / Math.PI); return <View className={`absolute h-[2px] rounded-full ${incoming ? "bg-[#14c7bc]" : "bg-[#7b35cc]"}`} key={`${items[index].transfer_id}-segment`} style={{ left: `${previous.x}%`, top: `${previous.y}%`, transform: [{ rotate: `${angle}deg` }], width: `${length}%` }} />; })}
    {points.map((point, index) => <View className={`absolute h-2.5 w-2.5 rounded-full border-2 border-white dark:border-[#17264b] ${incoming ? "bg-[#14c7bc]" : "bg-[#7b35cc]"}`} key={`${items[index].transfer_id}-point`} style={{ left: `${point.x}%`, top: `${point.y}%` }} />)}
  </View>;
}

/** Compact home/history chart with separate rising deposits and falling withdrawals. */
export function TransactionChart({ transactions }: { transactions: Transaction[] }) {
  const points = transactions.slice(0, 6).reverse();
  const deposits = points.filter(isIncoming);
  const withdrawals = points.filter((item) => !isIncoming(item));
  if (!points.length) return null;
  return <View className="mt-5 rounded-3xl border border-[#d9e4ed] bg-white p-4 dark:border-[#2d456e] dark:bg-[#1b2a55]
    "><View className="flex-row items-center justify-between"><View><Text className="text-xs font-bold uppercase tracking-[2px] text-slate-400 dark:text-[#9fb0c5]">Actividad financiera</Text><Text className="mt-1 text-base font-bold text-[#24315e] dark:text-[#f6f8fc]">Depositos y retiros</Text></View><View className="items-end"><Text className="text-[10px] font-bold text-[#14c7bc]">Depositos ↑</Text><Text className="mt-1 text-[10px] font-bold text-[#7b35cc]">Retiros ↓</Text></View></View>{deposits.length ? <LineLane items={deposits} incoming /> : null}{withdrawals.length ? <LineLane items={withdrawals} incoming={false} /> : null}<View className="mt-3 flex-row justify-between"><Text className="text-[9px] text-slate-400 dark:text-[#9fb0c5]">{points[0] ? formatMinor(points[0].amount) : ""}</Text><Text className="text-[9px] text-slate-400 dark:text-[#9fb0c5]">{points[points.length - 1] ? formatMinor(points[points.length - 1].amount) : ""}</Text></View></View>;
}
