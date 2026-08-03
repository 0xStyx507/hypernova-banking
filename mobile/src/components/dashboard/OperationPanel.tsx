import { Pressable, Text, TextInput, View } from "react-native";
import { Account, OperationMode } from "../../api";
import { currencyInputToMinor, sanitizeCurrencyInput } from "../../money";

interface Props {
  accounts: Account[];
  activeAccount: Account | null;
  mode: OperationMode;
  amount: string;
  destination: string;
  transferTargetType: "own" | "external";
  transferConfirmationPin: string;
  mcpPinConfigured: boolean;
  mcpActionPending: boolean;
  busy: boolean;
  notice: string;
  onMode: (mode: OperationMode) => void;
  onAmount: (value: string) => void;
  onDestination: (value: string) => void;
  onTransferTargetTypeChange: (target: "own" | "external") => void;
  onTransferConfirmationPinChange: (pin: string) => void;
  onAccount: (id: string) => void;
  onSubmit: () => void;
}

const modes: Array<{ id: OperationMode; label: string }> = [
  { id: "deposit", label: "Depositar" },
  { id: "withdrawal", label: "Retirar" },
  { id: "transfer", label: "Transferir" },
];

/** Presents financial operations without deciding any ledger rule locally. */
export function OperationPanel(props: Props) {
  const destinations = props.accounts.filter((account) => account.id !== props.activeAccount?.id);
  const isExternal = props.mode === "transfer" && props.transferTargetType === "external";
  const canSubmit = Boolean(props.activeAccount && currencyInputToMinor(props.amount) !== "" && currencyInputToMinor(props.amount) !== "0" && (!isExternal || (props.destination && props.mcpPinConfigured && /^\d{4}$/u.test(props.transferConfirmationPin))));

  return (
    <View className="rounded-3xl bg-white p-4 dark:bg-[#142235]">
      <Text className="text-xs font-bold uppercase tracking-[2px] text-slate-400 dark:text-slate-300">Operaciones</Text>
      <Text className="mt-1 text-xl font-semibold text-[#2d73a5]">Mueve tu dinero con claridad</Text>

      <View className="mt-4 flex-row gap-2">
        {modes.map((item) => (
        <Pressable className={`flex-1 rounded-full px-2 py-3 ${props.mode === item.id ? "bg-[#2d73a5]" : "bg-slate-100 dark:bg-[#1d3047]"}`} disabled={props.mcpActionPending} key={item.id} onPress={() => props.onMode(item.id)}>
            <Text className={`text-center text-xs font-bold ${props.mode === item.id ? "text-white" : "text-slate-500 dark:text-slate-300"}`}>{item.label}</Text>
          </Pressable>
        ))}
      </View>

      <Text className="mt-5 text-xs font-bold uppercase tracking-wider text-slate-500 dark:text-slate-300">Cuenta origen</Text>
      <View className="mt-2 flex-row flex-wrap gap-2">
        {props.accounts.map((account) => (
          <Pressable className={`rounded-xl border px-3 py-3 ${account.id === props.activeAccount?.id ? "border-[#16c1b5] bg-[#e8f8f6]" : "border-slate-200"}`} disabled={props.mcpActionPending} key={account.id} onPress={() => props.onAccount(account.id)}>
            <Text className="text-xs font-semibold text-[#2d73a5]">{account.display_name || account.id.slice(0, 4)} · {account.id.slice(-4)}</Text>
          </Pressable>
        ))}
      </View>

      <Text className="mt-4 text-xs font-bold uppercase tracking-wider text-slate-500 dark:text-slate-300">Monto en USD</Text>
      <TextInput className="mt-2 rounded-2xl bg-slate-100 px-4 py-4 dark:bg-[#1d3047] dark:text-slate-100" editable={!props.mcpActionPending} keyboardType="decimal-pad" value={props.amount} onChangeText={(value) => props.onAmount(sanitizeCurrencyInput(value))} placeholder="10.00" accessibilityLabel="Monto en USD" />
      <Text className="mt-2 text-xs text-slate-400 dark:text-slate-300">Puedes escribir 10.50 o 10,50.</Text>

      {props.mode === "transfer" ? (
        <>
          <Text className="mt-4 text-xs font-bold uppercase tracking-wider text-slate-500 dark:text-slate-300">Tipo de destino</Text>
          <View className="mt-2 flex-row gap-2">
            <Pressable className={`flex-1 rounded-full px-3 py-3 ${!isExternal ? "bg-[#2d73a5]" : "bg-slate-100"}`} onPress={() => props.onTransferTargetTypeChange("own")}>
              <Text className={`text-center text-xs font-bold ${!isExternal ? "text-white" : "text-slate-500"}`}>Mis cuentas</Text>
            </Pressable>
            <Pressable className={`flex-1 rounded-full px-3 py-3 ${isExternal ? "bg-[#2d73a5]" : "bg-slate-100"}`} onPress={() => props.onTransferTargetTypeChange("external")}>
              <Text className={`text-center text-xs font-bold ${isExternal ? "text-white" : "text-slate-500"}`}>Otra cuenta</Text>
            </Pressable>
          </View>
          <Text className="mt-4 text-xs font-bold uppercase tracking-wider text-slate-500 dark:text-slate-300">Cuenta destino</Text>
          {isExternal ? (
            <TextInput className="mt-2 rounded-2xl bg-slate-100 px-4 py-4 dark:bg-[#1d3047] dark:text-slate-100" value={props.destination} onChangeText={props.onDestination} placeholder="UUID de la cuenta destino" autoCapitalize="none" autoCorrect={false} accessibilityLabel="Cuenta destino externa" />
          ) : (
            <View className="mt-2 flex-row flex-wrap gap-2">
              {destinations.map((account) => (
                <Pressable className={`rounded-xl border px-3 py-3 ${props.destination === account.id ? "border-[#16c1b5] bg-[#e8f8f6]" : "border-slate-200"}`} key={account.id} onPress={() => props.onDestination(account.id)}>
                  <Text className="text-xs font-semibold text-[#2d73a5]">{account.display_name || account.id.slice(0, 4)} · {account.id.slice(-4)}</Text>
                </Pressable>
              ))}
            </View>
          )}
          {isExternal ? (
            <>
              <Text className="mt-2 text-xs text-slate-500 dark:text-slate-300">Validaremos la cuenta antes de mover fondos.</Text>
              <TextInput className="mt-3 rounded-2xl bg-slate-100 px-4 py-4 text-center tracking-[5px]" value={props.transferConfirmationPin} onChangeText={(value) => props.onTransferConfirmationPinChange(value.replace(/\D/g, "").slice(0, 4))} keyboardType="number-pad" secureTextEntry maxLength={4} placeholder="PIN de 4 dígitos" accessibilityLabel="PIN de confirmación" />
              {!props.mcpPinConfigured ? <Text className="mt-2 text-xs text-amber-700 dark:text-amber-300">Configura tu PIN en Ajustes antes de confirmar.</Text> : null}
            </>
          ) : null}
          {!destinations.length && !isExternal ? <Text className="mt-2 text-xs text-slate-500 dark:text-slate-300">Abre otra cuenta desde Cuentas para transferir entre tus cuentas.</Text> : null}
        </>
      ) : null}

      <Text className="mt-4 rounded-2xl bg-slate-50 p-4 text-xs leading-5 text-slate-500 dark:bg-[#1d3047] dark:text-slate-300">{props.mode === "deposit" ? "Agrega fondos a la cuenta que elijas." : props.mode === "withdrawal" ? "Retira solo fondos disponibles." : "Revisa origen, destino y monto antes de confirmar."}</Text>
      {props.mcpActionPending ? <Text className="mt-3 rounded-xl bg-blue-50 p-3 text-sm text-blue-800 dark:bg-blue-950 dark:text-blue-200">Finaliza o cancela la operación pendiente desde el asistente antes de iniciar otra.</Text> : null}
      {props.notice ? <Text className="mt-3 rounded-xl bg-amber-50 p-3 text-sm text-amber-800 dark:bg-amber-950 dark:text-amber-200">{props.notice}</Text> : null}
      <Pressable className="mt-4 rounded-full bg-[#16c1b5] px-5 py-4" disabled={props.mcpActionPending || props.busy || !canSubmit} onPress={props.onSubmit}>
        <Text className="text-center font-semibold text-[#24315e]">{props.busy ? "Procesando…" : "Confirmar operación"}</Text>
      </Pressable>
    </View>
  );
}
