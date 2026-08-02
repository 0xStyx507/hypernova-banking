import { Stack } from "expo-router";
import "../global.css";

export default function RootLayout() {
  // Expo Router owns navigation and keeps screen transitions consistent.
  return <Stack screenOptions={{ headerShown: false }} />;
}
