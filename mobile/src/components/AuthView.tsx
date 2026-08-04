import { KeyboardAvoidingView, Platform, Pressable, SafeAreaView, ScrollView, Text, TextInput, View, useWindowDimensions } from "react-native";
import { OAuthProvider } from "../api";
import { HyperBankLogo } from "./HyperBankLogo";
import { PasswordSecurityMeter } from "./PasswordSecurityMeter";

type AuthMode = "login" | "register";

interface AuthViewProps {
  mode: AuthMode;
  setMode: (mode: AuthMode) => void;
  email: string;
  setEmail: (value: string) => void;
  password: string;
  setPassword: (value: string) => void;
  passwordConfirmation: string;
  setPasswordConfirmation: (value: string) => void;
  fullName: string;
  setFullName: (value: string) => void;
  busy: boolean;
  notice: string;
  onSubmit: () => void;
  onOAuth: (provider: OAuthProvider) => void;
}

/** Login and registration form. MFA is intentionally rendered on a separate screen. */
export function AuthView(props: AuthViewProps) {
  const { width } = useWindowDimensions();
  const compact = width < 380;
  return (
    <SafeAreaView className="flex-1 bg-[#f7f9fb] dark:bg-[#0d1726]">
      <KeyboardAvoidingView className="flex-1" behavior={Platform.OS === "ios" ? "padding" : undefined}>
        <ScrollView contentContainerStyle={{ flexGrow: 1, justifyContent: "center", paddingBottom: 40 }} className="px-5">
          <HyperBankLogo />
          <Text className="mt-4 font-semibold text-[#2d73a5] dark:text-[#7bc7ec]" style={{ fontSize: compact ? 38 : 48, lineHeight: compact ? 44 : 54 }}>Banca digital para tu día a día</Text>
          <Text className="mt-4 text-base leading-6 text-slate-500 dark:text-slate-300">Control financiero con seguridad reforzada y operaciones verificables.</Text>

          <View className="mt-8 rounded-3xl bg-white p-5 dark:bg-[#142235]" style={{ padding: compact ? 18 : 24 }}>
            <View className="flex-row gap-2">
              {(["login", "register"] as AuthMode[]).map((mode) => (
                <Pressable key={mode} className={`flex-1 rounded-full px-3 py-3 ${props.mode === mode ? "bg-[#2d73a5]" : "bg-slate-100 dark:bg-[#1d3047]"}`} onPress={() => props.setMode(mode)}>
                  <Text className={`text-center text-xs font-bold ${props.mode === mode ? "text-white" : "text-slate-500 dark:text-slate-300"}`}>{mode === "login" ? "Entrar" : "Crear cuenta"}</Text>
                </Pressable>
              ))}
            </View>

            {props.mode === "register" ? <TextInput className="mt-6 rounded-2xl bg-slate-100 px-4 py-4 dark:bg-[#1d3047] dark:text-slate-100" value={props.fullName} onChangeText={props.setFullName} placeholder="Nombre completo" autoComplete="name" /> : null}
            <TextInput className="mt-4 rounded-2xl bg-slate-100 px-4 py-4 dark:bg-[#1d3047] dark:text-slate-100" value={props.email} onChangeText={props.setEmail} placeholder="Email" autoCapitalize="none" keyboardType="email-address" autoComplete="email" />
            <TextInput className="mt-4 rounded-2xl bg-slate-100 px-4 py-4 dark:bg-[#1d3047] dark:text-slate-100" value={props.password} onChangeText={props.setPassword} placeholder="Contraseña" secureTextEntry autoCapitalize="none" autoComplete={props.mode === "login" ? "password" : "new-password"} maxLength={72} />
            {props.mode === "register" ? <PasswordSecurityMeter value={props.password} /> : null}
            {props.mode === "register" ? <TextInput className={`mt-4 rounded-2xl px-4 py-4 dark:bg-[#1d3047] dark:text-slate-100 ${props.passwordConfirmation && props.passwordConfirmation !== props.password ? "border border-red-300 bg-red-50" : "bg-slate-100"}`} value={props.passwordConfirmation} onChangeText={props.setPasswordConfirmation} placeholder="Confirmar contraseña" secureTextEntry autoCapitalize="none" autoComplete="new-password" maxLength={72} /> : null}
            {props.mode === "register" && props.passwordConfirmation && props.passwordConfirmation !== props.password ? <Text className="mt-2 text-xs text-red-700">Las contraseñas no coinciden.</Text> : null}

            <Pressable className="mt-6 rounded-full bg-[#16c1b5] px-5 py-4" disabled={props.busy} onPress={props.onSubmit}>
              <Text className="text-center font-semibold text-[#2d73a5]">{props.busy ? "Procesando…" : props.mode === "login" ? "Iniciar sesión" : "Crear cuenta USD"}</Text>
            </Pressable>
            {props.notice ? <Text accessibilityRole="alert" className="mt-4 text-sm text-red-700">{props.notice}</Text> : null}

            <View className="my-5 flex-row items-center"><View className="h-px flex-1 bg-slate-200 dark:bg-slate-700" /><Text className="mx-3 text-[10px] font-bold uppercase tracking-[1px] text-slate-400">O también</Text><View className="h-px flex-1 bg-slate-200 dark:bg-slate-700" /></View>
            <View className="flex-row gap-3">
              <Pressable className="flex-1 rounded-2xl bg-slate-100 px-3 py-4 dark:bg-[#1d3047]" disabled={props.busy} onPress={() => props.onOAuth("google")}><Text className="text-center text-xs font-bold text-slate-500 dark:text-slate-200">G · Google</Text></Pressable>
              <Pressable className="flex-1 rounded-2xl bg-slate-100 px-3 py-4 dark:bg-[#1d3047]" disabled={props.busy} onPress={() => props.onOAuth("github")}><Text className="text-center text-xs font-bold text-slate-500 dark:text-slate-200">GH · GitHub</Text></Pressable>
            </View>
            <Text className="mt-3 text-center text-xs leading-5 text-slate-400">El proveedor confirma tu identidad; Hypernova nunca recibe tu contraseña externa.</Text>
          </View>
        </ScrollView>
      </KeyboardAvoidingView>
    </SafeAreaView>
  );
}
