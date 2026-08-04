import { Text, View } from "react-native";

export type FeedbackTone = "error" | "success" | "warning" | "info";

interface Props {
  tone: FeedbackTone;
  message: string;
  title?: string;
}

const defaultTitles: Record<FeedbackTone, string> = {
  error: "No pudimos completar la accion",
  success: "Listo",
  warning: "Revisa esta informacion",
  info: "Informacion",
};

/** Shared mobile feedback surface with screen-reader friendly semantics. */
export function FeedbackMessage({ tone, message, title }: Props) {
  const palette = tone === "error"
    ? { container: "border-red-200 bg-red-50", icon: "bg-red-600 text-white", text: "text-red-800" }
    : tone === "success"
      ? { container: "border-[#bde8e1] bg-[#effcf9]", icon: "bg-[#087e78] text-white", text: "text-[#087e78]" }
      : tone === "warning"
        ? { container: "border-amber-200 bg-amber-50", icon: "bg-amber-500 text-white", text: "text-amber-800" }
        : { container: "border-blue-200 bg-blue-50", icon: "bg-blue-600 text-white", text: "text-blue-800" };
  return <View className={`flex-row items-start rounded-2xl border p-3 ${palette.container}`} accessibilityRole={tone === "error" || tone === "warning" ? "alert" : "text"} accessibilityLiveRegion={tone === "error" || tone === "warning" ? "assertive" : "polite"}>
    <Text className={`mr-3 h-6 w-6 rounded-full text-center text-sm font-bold ${palette.icon}`}>{tone === "error" ? "!" : tone === "success" ? "✓" : "i"}</Text>
    <View className="flex-1"><Text className={`text-sm font-bold ${palette.text}`}>{title ?? defaultTitles[tone]}</Text><Text className={`mt-1 text-sm leading-5 ${palette.text}`}>{message}</Text></View>
  </View>;
}
