import { KeyboardAvoidingView, Platform, Pressable, ScrollView, Text, View } from "react-native";
import { SafeAreaView } from "react-native-safe-area-context";
import { formatMinor, Transaction } from "../../api";
import { ChatbotFab } from "./ChatbotFab";
import { AccountList } from "./AccountList";
import { HistoryPanel } from "./HistoryPanel";
import { MobileBottomNav, MobileHeader } from "./MobileHeader";
import { OperationPanel } from "./OperationPanel";
import { SettingsPanel } from "./SettingsPanel";
import { TransactionChart } from "./TransactionChart";
import { MobileDashboardProps } from "./types";

/** Authenticated mobile workspace. It mirrors the web sections without copying financial rules. */
export function MobileDashboard(props: MobileDashboardProps) {
  return <SafeAreaView className="flex-1 bg-[#f7f9fb] dark:bg-[#07111f]" edges={["top"]}>
    <KeyboardAvoidingView className="flex-1" behavior={Platform.OS === "ios" ? "padding" : undefined}>
      <View className="flex-1">
        <MobileHeader name={props.user.full_name} email={props.user.email} onLogout={props.onLogout} onThemeToggle={props.onThemeToggle} />
        <ScrollView className="flex-1" contentContainerStyle={{ padding: 19, paddingBottom: 150 }} keyboardShouldPersistTaps="handled">
          {props.section === "accounts" ? <HomeSection {...props} /> : null}
          {props.section === "history" ? <HistoryPanel history={props.history} page={props.historyPage} busy={props.historyBusy} onPrevious={props.onPreviousHistory} onNext={props.onNextHistory} /> : null}
          {props.section === "operations" ? <OperationPanel accounts={props.accounts} activeAccount={props.activeAccount} mode={props.operationMode} amount={props.operationAmount} destination={props.destinationAccountId} transferTargetType={props.transferTargetType} transferConfirmationPin={props.transferConfirmationPin} mcpPinConfigured={props.mcpPinConfigured} mcpActionPending={props.mcpActionPending} busy={props.operationBusy} notice={props.operationNotice} onMode={props.onOperationModeChange} onAmount={props.onAmountChange} onDestination={props.onDestinationChange} onTransferTargetTypeChange={props.onTransferTargetTypeChange} onTransferConfirmationPinChange={props.onTransferConfirmationPinChange} onAccount={props.onAccountChange} onSubmit={props.onOperation} /> : null}
          {props.section === "settings" ? <SettingsPanel user={props.user} name={props.profileFullName} busy={props.profileBusy} notice={props.profileNotice} onName={props.onProfileNameChange} onSave={props.onProfileSubmit} mcpPin={props.mcpPin} mcpPinConfigured={props.mcpPinConfigured} mcpPinExpiresAt={props.mcpPinExpiresAt} mcpPinBusy={props.mcpPinBusy} mcpPinNotice={props.mcpPinNotice} onMCPPINChange={props.onMCPPINChange} onSetMCPPIN={props.onSetMCPPIN} /> : null}
        </ScrollView>
        <MobileBottomNav active={props.section} onNavigate={props.onNavigate} />
        <ChatbotFab accessToken={props.accessToken} accountId={props.activeAccount?.id} accountBalances={props.accountBalances} mcpPinConfigured={props.mcpPinConfigured} initialAction={props.mcpAction} onCreatePin={() => props.onNavigate("settings")} onPendingChange={props.onMCPActionPendingChange} onConfirmed={props.onMCPActionConfirmed} onExpired={props.onMCPActionExpired} />
      </View>
    </KeyboardAvoidingView>
  </SafeAreaView>;
}

function LegacyHomeSection(props: MobileDashboardProps) {
  const firstName = props.user.full_name.trim().split(/\s+/u)[0] || "cliente";
  const incoming = sumTransactions(props.history?.items ?? [], (item) => item.direction === "credit");
  const outgoing = sumTransactions(props.history?.items ?? [], (item) => item.direction !== "credit");
  const recentItems = props.history?.items.slice(0, 3) ?? [];
  const activeAccountLabel = props.activeAccount?.id ? maskAccount(props.activeAccount.id) : "Cuenta principal";

  return <View>
    <Text className="text-xs font-bold uppercase tracking-[2px] text-slate-400 dark:text-[#718399]">Resumen financiero</Text>
    <Text className="mt-1 text-3xl font-bold text-[#24315e] dark:text-[#f6f8fc]">Hola, {firstName}</Text>
    <Text className="mt-1 text-sm text-slate-500 dark:text-[#9fb0c5]">Tus finanzas están protegidas</Text>

    <View className="mt-5 rounded-[24px] bg-[#4f8cff] p-5 shadow-sm dark:bg-[#4f8cff]">
      <View className="flex-row items-start justify-between">
        <View className="rounded-full bg-[#734fff] px-3 py-2"><Text className="text-[10px] font-bold text-white">●  {activeAccountLabel}</Text></View>
        <Text className="rounded-2xl bg-[#734fff] px-3 py-2 text-sm text-white">◉</Text>
      </View>
      <Text className="mt-6 text-xs font-medium text-white/85">Saldo disponible</Text>
      <Text className="mt-2 text-4xl font-bold tracking-tight text-white">{formatMinor(props.balance?.available_balance ?? "0")}</Text>
      <Text className="mt-3 text-xs font-medium text-white/85">+6.8% este mes</Text>
    </View>

    <QuickActions onNavigate={props.onNavigate} onModeChange={props.onOperationModeChange} />

    <View className="mt-5 flex-row gap-2">
      <MetricCard label="Ingresos" value={incoming} accent="teal" />
      <MetricCard label="Gastos" value={outgoing} accent="yellow" />
      <MetricCard label="Balance" value={subtractMinor(incoming, outgoing)} accent="cyan" />
    </View>

    <View className="mt-6 rounded-[22px] bg-white p-4 dark:bg-[#0d1b2a]">
      <View className="flex-row items-center justify-between"><Text className="text-base font-bold text-[#24315e] dark:text-[#f6f8fc]">Últimos movimientos</Text><Pressable onPress={() => props.onNavigate("history")}><Text className="text-xs font-bold text-[#4f8cff]">Ver todos</Text></Pressable></View>
      {recentItems.length ? recentItems.map((item) => <MobileMovement item={item} key={`${item.transfer_id}-${item.created_at}`} />) : <Text className="mt-4 text-sm text-slate-500 dark:text-[#9fb0c5]">Todavía no hay movimientos.</Text>}
    </View>

    <View className="mt-5"><AccountList accounts={props.accounts} accountBalances={props.accountBalances} activeAccount={props.activeAccount} busy={props.accountBusy} renameBusyId={props.accountRenameBusyId} notice={props.accountNotice} onSelect={props.onAccountChange} onCreate={props.onCreateAccount} onRename={props.onRenameAccount} /></View>
  </View>;
}

function HomeSection(props: MobileDashboardProps) {
  const firstName = props.user.full_name.trim().split(/\s+/u)[0] || "cliente";
  const incoming = sumTransactions(props.history?.items ?? [], (item) => item.direction === "credit");
  const outgoing = sumTransactions(props.history?.items ?? [], (item) => item.direction !== "credit");
  const recentItems = props.history?.items.slice(0, 6) ?? [];
  const activeAccountLabel = props.activeAccount?.id ? maskAccount(props.activeAccount.id) : "Cuenta principal";

  return <View>
    <Text className="text-xs font-bold uppercase tracking-[2px] text-slate-400 dark:text-[#718399]">Resumen financiero</Text>
    <Text className="mt-1 text-3xl font-bold text-[#24315e] dark:text-[#f6f8fc]">Hola, {firstName}</Text>
    <Text className="mt-1 text-sm text-slate-500 dark:text-[#9fb0c5]">Tus finanzas estan protegidas</Text>
    <View className="mt-5 rounded-[24px] bg-[#337eaf] p-5 shadow-sm dark:bg-[#337eaf]">
      <View className="flex-row items-start justify-between"><View className="rounded-full bg-[#1b2a57] px-3 py-2"><Text className="text-[10px] font-bold text-white">• {activeAccountLabel}</Text></View><Text className="rounded-2xl bg-[#1b2a57] px-3 py-2 text-sm text-white">◉</Text></View>
      <Text className="mt-6 text-xs font-medium text-white/85">Saldo disponible</Text>
      <Text className="mt-2 text-4xl font-bold tracking-tight text-white">{formatMinor(props.balance?.available_balance ?? "0").replace("USD", "B/.")}</Text>
      <Text className="mt-3 text-xs font-medium text-white/85">+6.8% este mes</Text>
      <View className="mt-4 flex-row gap-2"><Pressable className="flex-1 rounded-xl bg-white px-3 py-3" onPress={() => { props.onOperationModeChange("deposit"); props.onNavigate("operations"); }}><Text className="text-center text-xs font-bold text-[#337eaf]">Depositar</Text></Pressable><Pressable className="flex-1 rounded-xl bg-[#14c7bc] px-3 py-3" onPress={() => { props.onOperationModeChange("transfer"); props.onNavigate("operations"); }}><Text className="text-center text-xs font-bold text-[#24315e]">Transferir</Text></Pressable></View>
    </View>
    <View className="mt-4"><AccountList accounts={props.accounts} accountBalances={props.accountBalances} activeAccount={props.activeAccount} busy={props.accountBusy} renameBusyId={props.accountRenameBusyId} notice={props.accountNotice} onSelect={props.onAccountChange} onCreate={props.onCreateAccount} onRename={props.onRenameAccount} /></View>
    <QuickActions onNavigate={props.onNavigate} onModeChange={props.onOperationModeChange} />
    <View className="mt-5 flex-row gap-2"><MetricCard label="Ingresos" value={incoming} accent="teal" /><MetricCard label="Gastos" value={outgoing} accent="yellow" /><MetricCard label="Balance" value={subtractMinor(incoming, outgoing)} accent="cyan" /></View>
    {recentItems.length > 0 ? <TransactionChart transactions={recentItems} /> : null}
    <View className="mt-5 rounded-[22px] bg-white p-4 dark:bg-[#1b2a55]"><View className="flex-row items-center justify-between"><Text className="text-base font-bold text-[#24315e] dark:text-[#f6f8fc]">Ultimos movimientos</Text><Pressable onPress={() => props.onNavigate("history")}><Text className="text-xs font-bold text-[#4f8cff]">Ver todos</Text></Pressable></View>{recentItems.length ? recentItems.slice(0, 2).map((item) => <MobileMovement item={item} key={`${item.transfer_id}-${item.created_at}`} />) : <Text className="mt-4 text-sm text-slate-500 dark:text-[#9fb0c5]">Todavia no hay movimientos.</Text>}</View>
  </View>;
}

function LegacyQuickActions({ onNavigate, onModeChange }: { onNavigate: (section: MobileDashboardProps["section"]) => void; onModeChange: (mode: MobileDashboardProps["operationMode"]) => void }) {
  const actions: Array<{ label: string; icon: string; mode?: MobileDashboardProps["operationMode"]; section: MobileDashboardProps["section"] }> = [
    { label: "Depositar", icon: "↓", mode: "deposit", section: "operations" },
    { label: "Retirar", icon: "↑", mode: "withdrawal", section: "operations" },
    { label: "Transferir", icon: "↔", mode: "transfer", section: "operations" },
    { label: "Consultar", icon: "⌁", section: "history" },
  ];
  return <View className="mt-5 flex-row justify-between">{actions.map((action) => <Pressable className="w-[23%] items-center" key={action.label} onPress={() => { if (action.mode) onModeChange(action.mode); onNavigate(action.section); }}><View className="h-12 w-12 items-center justify-center rounded-2xl bg-[#102a45] dark:bg-[#102033]"><Text className="text-lg font-bold text-[#38d9ff]">{action.icon}</Text></View><Text className="mt-2 text-[10px] font-medium text-slate-500 dark:text-[#9fb0c5]">{action.label}</Text></Pressable>)}</View>;
}

function QuickActions({ onNavigate, onModeChange }: { onNavigate: (section: MobileDashboardProps["section"]) => void; onModeChange: (mode: MobileDashboardProps["operationMode"]) => void }) {
  return <View className="mt-5 flex-row justify-start gap-8"><Pressable className="items-center" onPress={() => { onModeChange("withdrawal"); onNavigate("operations"); }}><View className="h-12 w-12 items-center justify-center rounded-2xl bg-[#e5f2f7] dark:bg-[#243b68]"><Text className="text-lg font-bold text-[#2d73a5] dark:text-[#38d9ff]">↑</Text></View><Text className="mt-2 text-[10px] font-medium text-slate-500 dark:text-[#9fb0c5]">Retirar</Text></Pressable><Pressable className="items-center" onPress={() => onNavigate("history")}><View className="h-12 w-12 items-center justify-center rounded-2xl bg-[#e5f2f7] dark:bg-[#243b68]"><Text className="text-lg font-bold text-[#2d73a5] dark:text-[#38d9ff]">⌁</Text></View><Text className="mt-2 text-[10px] font-medium text-slate-500 dark:text-[#9fb0c5]">Consultar</Text></Pressable></View>;
}

function MetricCard({ label, value, accent }: { label: string; value: string; accent: "teal" | "yellow" | "cyan" }) {
  const accentClass = accent === "teal" ? "bg-[#35d49a]" : accent === "yellow" ? "bg-[#f4cf4a]" : "bg-[#38d9ff]";
  return <View className="min-w-0 flex-1 rounded-2xl bg-[#1b2a55] p-3 dark:bg-[#1b2a55]"><Text className="text-[10px] text-[#9fb0c5]">{label}</Text><Text className="mt-1 text-sm font-bold text-white">{formatMinor(value).replace("USD", "B/.")}</Text><View className={`mt-2 h-1 w-10 rounded-full ${accentClass}`} /></View>;
}

function MobileMovement({ item }: { item: Transaction }) {
  const incoming = item.direction === "credit";
  const label = item.type === "deposit" ? "Depósito" : item.type === "withdrawal" ? "Retiro" : incoming ? "Transferencia recibida" : "Transferencia enviada";
  return <View className="mt-3 flex-row items-center rounded-2xl bg-[#f4f7fb] p-3 dark:bg-[#102033]"><View className={`h-10 w-10 items-center justify-center rounded-2xl ${incoming ? "bg-[#102a45]" : "bg-[#172b43]"}`}><Text className={`text-base font-bold ${incoming ? "text-[#35d49a]" : "text-white"}`}>{incoming ? "↓" : "↑"}</Text></View><View className="ml-3 flex-1"><Text className="text-xs font-bold text-[#24315e] dark:text-[#f6f8fc]">{label}</Text><Text className="mt-1 text-[10px] text-slate-400 dark:text-[#9fb0c5]">{new Date(item.created_at).toLocaleDateString("es-PA", { day: "2-digit", month: "short" })}</Text></View><Text className={`text-xs font-bold ${incoming ? "text-[#35d49a]" : "text-[#f6f8fc]"}`}>{incoming ? "+" : "−"}{formatMinor(item.amount)}</Text></View>;
}

function sumTransactions(items: Transaction[], predicate: (item: Transaction) => boolean): string {
  return items.filter(predicate).reduce((total, item) => { try { return total + BigInt(item.amount); } catch { return total; } }, 0n).toString();
}

function subtractMinor(first: string, second: string): string {
  try { return (BigInt(first) - BigInt(second)).toString(); } catch { return "0"; }
}

function maskAccount(value: string): string { return value.length > 10 ? `•••• ${value.slice(-4)}` : value; }
