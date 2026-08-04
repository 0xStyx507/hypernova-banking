import { KeyboardAvoidingView, Linking, Platform, Pressable, SafeAreaView, ScrollView, Text, TextInput, View, useWindowDimensions } from "react-native";
import { useEffect, useState } from "react";
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

/** Authenticated MFA setup. The authenticator deep-link remains the primary mobile action. */
export function MFAOnboarding(props: MFAOnboardingProps) {
  const { width } = useWindowDimensions();
  const compact = width < 380;
  const [remaining, setRemaining] = useState(() => props.enrollment ? Math.max(0, Date.parse(props.enrollment.expires_at) - Date.now()) : 0);

  useEffect(() => {
    if (!props.enrollment) return undefined;
    const update = () => setRemaining(Math.max(0, Date.parse(props.enrollment?.expires_at ?? "") - Date.now()));
    update();
    const timer = setInterval(update, 1000);
    return () => clearInterval(timer);
  }, [props.enrollment]);

  async function openAuthenticator() {
    if (!props.enrollment) return;
    try { await Linking.openURL(props.enrollment.otpauth_uri); }
    catch { /* Some authenticators do not register a deep link; manual setup remains available. */ }
  }

  return <SafeAreaView className="flex-1 bg-[#f7f9fb] dark:bg-[#07111f]">
    <KeyboardAvoidingView className="flex-1" behavior={Platform.OS === "ios" ? "padding" : undefined}>
      <ScrollView contentContainerStyle={{ flexGrow: 1, justifyContent: "center", padding: 20 }} keyboardShouldPersistTaps="handled">
        <View className="mb-6 flex-row items-center justify-between"><View><HyperBankLogo inverse /><Text className="mt-2 text-sm text-slate-500 dark:text-[#9fb0c5]">Cuenta de {maskEmail(props.user.email)}</Text></View><Pressable accessibilityRole="button" onPress={props.onLogout}><Text className="font-semibold text-[#2d73a5] dark:text-[#9fb0c5]">Salir</Text></Pressable></View>
        <View className="rounded-[28px] border border-slate-200 bg-white p-5 shadow-sm dark:border-[#213a55] dark:bg-[#0d1b2a]" style={{ padding: compact ? 18 : 24 }}>
          <Text className="text-xs font-bold uppercase tracking-[2px] text-[#5b20a3]">Seguridad de tu cuenta</Text>
          <Text className="mt-3 text-3xl font-bold text-[#24315e] dark:text-[#f6f8fc]" style={{ fontSize: compact ? 28 : 32 }}>Configurar MFA</Text>
          <Text className="mt-3 text-sm leading-6 text-slate-500 dark:text-[#c5d3e2]">Activa una segunda capa de protección para aprobar tus accesos y operaciones.</Text>
          <MfaStepper active={props.enrollment ? 1 : 0} />
          {props.loading && !props.enrollment ? <Text className="mt-7 rounded-2xl bg-slate-100 p-5 text-center text-sm text-slate-500 dark:bg-[#102033] dark:text-[#9fb0c5]">Preparando tu configuración segura…</Text> : null}
          {props.enrollment ? <View className="mt-7">
            <Text className="text-sm font-bold text-[#2d73a5] dark:text-[#7bc7ec]">1. Escanea o abre tu autenticador</Text>
            <Text className="mt-2 text-sm leading-6 text-slate-500 dark:text-[#c5d3e2]">Abre tu aplicación autenticadora para importar la cuenta automáticamente. Si no funciona, puedes introducir la clave manual.</Text>
            <Pressable className="mt-4 rounded-2xl bg-[#2d73a5] px-5 py-4" onPress={() => void openAuthenticator()}><Text className="text-center font-bold text-white">Abrir aplicación autenticadora</Text></Pressable>
            <Text selectable className="mt-4 rounded-2xl bg-[#f1f5f9] p-4 text-center font-mono text-base font-bold tracking-[2px] text-[#2d73a5] dark:bg-[#102033] dark:text-[#7bc7ec]">{props.enrollment.secret}</Text>
            <Text className="mt-2 text-xs leading-5 text-slate-400 dark:text-[#718399]">Clave manual de respaldo. No la compartas.</Text>
            <Text className="mt-6 text-sm font-bold text-[#2d73a5] dark:text-[#7bc7ec]">2. Confirma el código de seis dígitos</Text>
            <MfaCodeInput code={props.code} onChange={props.onCodeChange} />
            <Text className="mt-3 text-center text-xs text-slate-500 dark:text-[#9fb0c5]">Código válido durante {formatCountdown(remaining)}</Text>
            <Pressable className="mt-4 w-full rounded-2xl bg-[#16c1b5] px-5 py-4" disabled={props.busy || props.code.length !== 6} onPress={props.onVerify}><Text className="text-center font-bold text-[#24315e]">{props.busy ? "Verificando…" : "Activar MFA"}</Text></Pressable>
          </View> : null}
          {!props.loading && !props.enrollment ? <Pressable className="mt-7 rounded-2xl bg-[#2d73a5] px-5 py-4" disabled={props.busy} onPress={props.onBegin}><Text className="text-center font-bold text-white">Generar clave de configuración</Text></Pressable> : null}
          {props.notice ? <Text accessibilityRole="alert" className="mt-5 rounded-2xl bg-amber-50 p-4 text-sm leading-5 text-amber-800">{props.notice}</Text> : null}
        </View>
      </ScrollView>
    </KeyboardAvoidingView>
  </SafeAreaView>;
}

function MfaStepper({ active }: { active: number }) {
  return <View className="mt-6 flex-row items-start justify-between">{["Escanear", "Confirmar", "Listo"].map((label, index) => <View className="items-center" key={label}><View className={`h-8 w-8 items-center justify-center rounded-full ${index <= active ? "bg-[#16c1b5]" : "bg-[#e5ebf0] dark:bg-[#213a55]"}`}><Text className={`text-xs font-bold ${index <= active ? "text-[#24315e]" : "text-slate-500 dark:text-[#9fb0c5]"}`}>{index + 1}</Text></View><Text className={`mt-2 text-[10px] font-bold ${index <= active ? "text-[#2d73a5] dark:text-[#7bc7ec]" : "text-slate-400 dark:text-[#718399]"}`}>{label}</Text></View>)}</View>;
}

function MfaCodeInput({ code, onChange }: { code: string; onChange: (value: string) => void }) {
  return <View className="relative mt-3 flex-row justify-between"><TextInput className="absolute inset-0 z-10 opacity-0" value={code} onChangeText={(value) => onChange(sanitizeMfaCode(value))} keyboardType="number-pad" maxLength={6} autoComplete="sms-otp" accessibilityLabel="Código MFA de seis dígitos" /><>{Array.from({ length: 6 }, (_, index) => <View className={`h-14 w-[14%] items-center justify-center rounded-2xl border ${code[index] ? "border-[#16c1b5] bg-[#f0fcfa] dark:bg-[#173b42]" : "border-slate-200 bg-[#f8fafc] dark:border-[#344b65] dark:bg-[#102033]"}`} key={index}><Text className="text-xl font-bold text-[#24315e] dark:text-white">{code[index] ? "•" : ""}</Text></View>)}</></View>;
}

function formatCountdown(milliseconds: number): string {
  const seconds = Math.ceil(milliseconds / 1000);
  return `${Math.floor(seconds / 60).toString().padStart(2, "0")}:${(seconds % 60).toString().padStart(2, "0")}`;
}
