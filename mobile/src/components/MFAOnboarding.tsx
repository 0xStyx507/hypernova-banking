import { KeyboardAvoidingView, Linking, Platform, Pressable, SafeAreaView, ScrollView, Text, TextInput, View, useWindowDimensions } from "react-native";
import { MFAEnrollment, User } from "../api";
import { maskEmail, sanitizeMfaCode } from "../auth";
import { HyperBankLogo } from "./HyperBankLogo";

interface MFAOnboardingProps {
  user: User;
  enrollment: MFAEnrollment | null;
  code: string;
  busy: boolean;
  loading: boolean;
  notice: string;
  onCodeChange: (value: string) => void;
  onBegin: () => void;
  onVerify: () => void;
  onLogout: () => void;
}

/** Authenticated MFA setup. Mobile uses a manual secret and never renders a QR. */
export function MFAOnboarding(props: MFAOnboardingProps) {
  const { width } = useWindowDimensions();
  const compact = width < 380;
  async function openAuthenticator() {
    if (!props.enrollment) return;
    try { await Linking.openURL(props.enrollment.otpauth_uri); }
    catch { /* Some authenticators do not register a deep link; manual setup remains available. */ }
  }
  return (
    <SafeAreaView className="flex-1 bg-[#5b20a3] dark:bg-[#24103f]">
      <KeyboardAvoidingView className="flex-1" behavior={Platform.OS === "ios" ? "padding" : undefined}>
        <ScrollView contentContainerStyle={{ flexGrow: 1, justifyContent: "center", padding: 20 }} keyboardShouldPersistTaps="handled">
          <View className="mb-6 flex-row items-center justify-between">
            <View><HyperBankLogo inverse /><Text className="mt-2 text-sm text-slate-300">Cuenta de {maskEmail(props.user.email)}</Text></View>
            <Pressable accessibilityRole="button" onPress={props.onLogout}><Text className="font-semibold text-slate-300">Salir</Text></Pressable>
          </View>
          <View className="rounded-3xl bg-white p-5 dark:bg-[#142235]" style={{ padding: compact ? 18 : 24 }}>
            <Text className="text-xs font-bold uppercase tracking-[2px] text-slate-400">Paso 1 de 1 · Seguridad</Text>
            <Text className="mt-3 text-3xl font-semibold text-[#2d73a5] dark:text-[#7bc7ec]" style={{ fontSize: compact ? 28 : 32 }}>Protege tu cuenta antes de continuar.</Text>
            <Text className="mt-3 text-sm leading-6 text-slate-500 dark:text-slate-300">Configura tu autenticador con Google Authenticator o Microsoft Authenticator. Puedes abrirlo directamente o usar una clave manual.</Text>
            {props.loading && !props.enrollment ? <Text className="mt-7 rounded-2xl bg-slate-100 p-5 text-center text-sm text-slate-500 dark:bg-[#1d3047] dark:text-slate-300">Preparando tu configuración segura…</Text> : null}
            {props.enrollment ? <View className="mt-7">
              <Text className="text-sm font-semibold text-[#2d73a5]">1. Agrega una cuenta manualmente</Text>
              <Text className="mt-2 text-sm leading-6 text-slate-500 dark:text-slate-300">Abre tu autenticador para configurarlo automáticamente. Si no funciona, selecciona “introducir clave” y usa esta alternativa:</Text>
              <Pressable className="mt-4 rounded-full bg-[#2d73a5] px-5 py-4" onPress={() => void openAuthenticator()}><Text className="text-center font-bold text-white">Abrir aplicación autenticadora</Text></Pressable>
              <Text selectable className="mt-4 rounded-2xl bg-slate-100 p-4 text-center font-mono text-base font-bold tracking-[2px] text-[#2d73a5] dark:bg-[#1d3047] dark:text-[#7bc7ec]">{props.enrollment.secret}</Text>
              <Text className="mt-3 text-xs leading-5 text-slate-400">La clave se muestra una sola vez. No la compartas; quedará protegida después de activar MFA.</Text>
              <Text className="mt-6 text-sm font-semibold text-[#2d73a5] dark:text-[#7bc7ec]">2. Confirma el código actual</Text>
              <TextInput className="mt-3 w-full rounded-2xl border border-slate-200 bg-slate-100 px-4 py-4 text-center text-lg tracking-[4px] dark:border-slate-700 dark:bg-[#1d3047] dark:text-slate-100" keyboardType="number-pad" maxLength={6} value={props.code} onChangeText={(value) => props.onCodeChange(sanitizeMfaCode(value))} placeholder="000000" autoComplete="sms-otp" accessibilityLabel="Código MFA" />
              <Pressable className="mt-3 w-full rounded-full bg-[#16c1b5] px-5 py-4" disabled={props.busy || props.code.length !== 6} onPress={props.onVerify}><Text className="text-center font-semibold text-[#2d73a5]">{props.busy ? "Verificando…" : "Activar y entrar"}</Text></Pressable>
            </View> : null}
            {!props.loading && !props.enrollment ? <Pressable className="mt-7 rounded-full bg-[#2d73a5] px-5 py-4" disabled={props.busy} onPress={props.onBegin}><Text className="text-center font-semibold text-white">Generar clave de configuración</Text></Pressable> : null}
            {props.notice ? <Text accessibilityRole="alert" className="mt-5 rounded-2xl bg-amber-50 p-4 text-sm text-amber-800">{props.notice}</Text> : null}
          </View>
        </ScrollView>
      </KeyboardAvoidingView>
    </SafeAreaView>
  );
}
