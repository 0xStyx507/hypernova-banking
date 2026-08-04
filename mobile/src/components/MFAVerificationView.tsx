import { KeyboardAvoidingView, Platform, Pressable, SafeAreaView, ScrollView, Text, TextInput, View } from "react-native";

interface MFAVerificationViewProps {
  accountLabel: string;
  code: string;
  busy: boolean;
  notice: string;
  onCodeChange: (value: string) => void;
  onSubmit: () => void;
  onBack: () => void;
}

/**
 * Dedicated login challenge. It deliberately contains no QR or enrollment
 * material: setup is a separate authenticated flow.
 */
export function MFAVerificationView(props: MFAVerificationViewProps) {
  const validCode = props.code.length === 6;

  return (
    <SafeAreaView className="flex-1 bg-[#f7f9fb] dark:bg-[#07111f]">
      <KeyboardAvoidingView className="flex-1" behavior={Platform.OS === "ios" ? "padding" : undefined}>
        <ScrollView contentContainerStyle={{ flexGrow: 1, justifyContent: "center", padding: 20 }} keyboardShouldPersistTaps="handled">
          <View className="mb-8 items-center">
            <View className="h-12 w-12 items-center justify-center rounded-2xl bg-[#2d73a5]">
              <Text className="text-xl font-bold text-white">H</Text>
            </View>
            <Text className="mt-3 text-lg font-bold text-[#1e315f]">Hyper Bank</Text>
          </View>

          <View className="rounded-[28px] border border-slate-200 bg-white p-6 shadow-sm dark:border-[#213a55] dark:bg-[#0d1b2a]">
            <Text className="text-center text-xs font-bold uppercase tracking-[2px] text-[#5b20a3]">Verificación de seguridad</Text>
            <Text className="mt-3 text-center text-3xl font-semibold text-[#1e315f]">Confirma tu identidad</Text>
            <Text className="mt-3 text-center text-sm leading-6 text-slate-500">
              Tu cuenta ({props.accountLabel}) necesita un código de seguridad antes de continuar.
            </Text>

            <Text className="mt-7 text-xs font-bold uppercase tracking-[1px] text-[#2d73a5]">Código de 6 dígitos</Text>
            <TextInput
              className={`mt-2 rounded-2xl border px-4 py-4 text-center text-lg tracking-[4px] ${props.notice ? "border-red-400 bg-red-50" : "border-slate-200 bg-white"}`}
              value={props.code}
              onChangeText={props.onCodeChange}
              placeholder="000000"
              placeholderTextColor="#94a3b8"
              keyboardType="number-pad"
              maxLength={6}
              autoComplete="sms-otp"
              autoFocus
              accessibilityLabel="Código de autenticación de 6 dígitos"
              accessibilityHint="Escribe el código de tu aplicación autenticadora"
            />
            {props.notice ? <Text accessibilityRole="alert" className="mt-2 rounded-2xl bg-red-50 px-3 py-3 text-sm leading-5 text-red-700">{props.notice}</Text> : <Text className="mt-2 px-1 text-xs leading-5 text-slate-500">Abre tu aplicación autenticadora y escribe el código vigente.</Text>}

            <Pressable
              className={`mt-4 rounded-full px-5 py-4 ${validCode && !props.busy ? "bg-[#16c1b5]" : "bg-[#a8e5df]"}`}
              disabled={!validCode || props.busy}
              onPress={props.onSubmit}
              accessibilityRole="button"
              accessibilityState={{ disabled: !validCode || props.busy }}
            >
              <Text className="text-center font-semibold text-[#1e315f]">{props.busy ? "Verificando…" : "Continuar"}</Text>
            </Pressable>

            <Pressable className="mt-3 rounded-full bg-slate-100 px-5 py-4" onPress={props.onBack} accessibilityRole="button">
              <Text className="text-center font-semibold text-[#1e315f]">Volver al inicio de sesión</Text>
            </Pressable>
          </View>
        </ScrollView>
      </KeyboardAvoidingView>
    </SafeAreaView>
  );
}
