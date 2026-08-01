import { Pressable, Text, View } from "react-native";

export default function HomeScreen() {
  return (
    <View className="flex-1 bg-[#f7f5ef] px-6 pt-20">
      <Text className="text-xs font-semibold uppercase tracking-[4px] text-slate-500">Hypernova</Text>
      <Text className="mt-3 text-4xl font-semibold text-[#10233f]">Tu dinero, claro.</Text>

      <View className="mt-10 rounded-3xl bg-[#10233f] p-7">
        <Text className="text-sm text-slate-300">Balance disponible</Text>
        <Text className="mt-4 text-5xl font-semibold text-white">$0.00</Text>
        <Text className="mt-3 text-sm text-slate-300">Fase 0 · cuenta aún no conectada</Text>
      </View>

      <Pressable className="mt-6 rounded-full bg-[#8cf0c5] px-5 py-4" accessibilityRole="button">
        <Text className="text-center font-semibold text-[#10233f]">Iniciar sesión</Text>
      </Pressable>
    </View>
  );
}

