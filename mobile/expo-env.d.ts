declare namespace NodeJS {
  interface ProcessEnv {
    EXPO_PUBLIC_API_BASE_URL?: string;
  }
}

// NativeWind's JSX transform adds className at runtime. Keep the prop visible
// to TypeScript even when the Expo plugin has not generated its declaration.
import "react-native";

declare module "react-native" {
  interface ViewProps { className?: string }
  interface TextProps { className?: string }
  interface TextInputProps { className?: string }
  interface ScrollViewProps { className?: string }
  interface PressableProps { className?: string }
}
