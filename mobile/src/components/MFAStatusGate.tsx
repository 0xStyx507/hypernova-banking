import { Pressable, SafeAreaView, Text, View } from "react-native";

interface MFAStatusGateProps {
  notice: string;
  loading: boolean;
  onRetry: () => void;
  onLogout: () => void;
}

/** Prevents the enrollment screen from flashing while MFA status is loading. */
export function MFAStatusGate(props: MFAStatusGateProps) {
  return (
    <SafeAreaView className="flex-1 items-center justify-center bg-[#f7f9fb] px-5">
      <View className="w-full max-w-md rounded-3xl bg-white p-7">
        <View className="h-12 w-12 items-center justify-center rounded-2xl bg-[#dff7f3]">
          <Text className="text-xl font-bold text-[#087e78]">✓</Text>
        </View>
        <Text className="mt-5 text-2xl font-semibold text-[#1e315f]">Comprobando tu seguridad</Text>
        <Text className="mt-3 text-sm leading-6 text-slate-500">
          {props.notice || "Estamos verificando el estado de tu autenticación multifactor."}
        </Text>
        {props.notice ? <Pressable className="mt-5 rounded-full bg-[#2d73a5] px-5 py-4" onPress={props.onRetry}><Text className="text-center font-semibold text-white">Intentar de nuevo</Text></Pressable> : null}
        <Pressable className="mt-3 rounded-full bg-slate-100 px-5 py-4" onPress={props.onLogout}><Text className="text-center font-semibold text-[#1e315f]">Salir</Text></Pressable>
        {props.loading ? <Text className="mt-4 text-center text-xs text-slate-400">Cargando…</Text> : null}
      </View>
    </SafeAreaView>
  );
}
