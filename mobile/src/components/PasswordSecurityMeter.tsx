import { Text, View } from "react-native";

interface Props { value: string }

/** Shows password requirements without logging or persisting the secret. */
export function PasswordSecurityMeter({ value }: Props) {
  if (!value) return null;
  const lengthOk = value.length >= 8;
  const hasNumber = /\d/u.test(value);
  const hasMixedCase = /[a-z]/u.test(value) && /[A-Z]/u.test(value);
  const hasSymbol = /[^A-Za-z0-9]/u.test(value);
  const score = [lengthOk, hasNumber, hasMixedCase, hasSymbol].filter(Boolean).length;
  const tone = score <= 1 ? "bg-red-400" : score <= 2 ? "bg-amber-400" : "bg-[#16c1b5]";
  const label = !lengthOk ? "Debe tener al menos 8 caracteres" : score >= 3 ? "Seguridad alta" : "Puedes hacerla mas segura";
  return <View className="mt-3 rounded-2xl border border-slate-200 bg-white p-3">
    <View className="h-2 overflow-hidden rounded-full bg-slate-100"><View className={`h-full rounded-full ${tone}`} style={{ width: `${Math.max(25, score * 25)}%` }} /></View>
    <Text className="mt-2 text-xs font-bold text-[#2d73a5]">{label}</Text>
    <View className="mt-2 flex-row flex-wrap gap-x-3 gap-y-1"><Text className={`text-xs ${lengthOk ? "text-[#087e78]" : "text-slate-400"}`}>{lengthOk ? "✓" : "○"} 8 caracteres</Text><Text className={`text-xs ${hasNumber ? "text-[#087e78]" : "text-slate-400"}`}>{hasNumber ? "✓" : "○"} Un numero</Text><Text className={`text-xs ${hasMixedCase ? "text-[#087e78]" : "text-slate-400"}`}>{hasMixedCase ? "✓" : "○"} Mayuscula y minuscula</Text><Text className={`text-xs ${hasSymbol ? "text-[#087e78]" : "text-slate-400"}`}>{hasSymbol ? "✓" : "○"} Simbolo</Text></View>
  </View>;
}
