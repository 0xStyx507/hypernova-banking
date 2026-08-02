module.exports = function (api) {
  api.cache(true);
  return {
    // NativeWind v4 uses Expo's JSX import-source transform. The legacy
    // `nativewind/babel` preset pulls in the Reanimated 4 Worklets plugin,
    // which is not part of this Expo SDK 52 / Reanimated 3 stack.
    presets: [["babel-preset-expo", { jsxImportSource: "nativewind" }]],
  };
};
