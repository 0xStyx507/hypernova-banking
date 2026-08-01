import { Stack } from "expo-router";
import "../global.css";

export default function RootLayout() {
  // Expo Router owns navigation; feature screens will be added in later phases.
  return <Stack screenOptions={{ headerShown: false }} />;
}
