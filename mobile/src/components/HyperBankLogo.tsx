import { Text, View } from "react-native";

/** Compact native brand mark shared by authentication and dashboard headers. */
export function HyperBankLogo({ inverse = false }: { inverse?: boolean }) {
  return <View className="flex-row items-center"><View className={`h-9 w-9 items-center justify-center rounded-xl ${inverse ? "bg-white" : "bg-[#2d73a5]"}`}><Text className={`text-lg font-extrabold ${inverse ? "text-[#2d73a5]" : "text-white"}`}>H</Text></View><Text className={`ml-2 text-lg font-extrabold ${inverse ? "text-white" : "text-[#24315e] dark:text-slate-100"}`}>Hyper Bank</Text></View>;
}
