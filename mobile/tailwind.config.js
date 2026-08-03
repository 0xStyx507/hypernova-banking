/** @type {import('tailwindcss').Config} */
module.exports = {
  // The mobile UI is split into app routes and reusable feature components.
  // Include src so NativeWind generates styles for the actual screens.
  content: ["./app/**/*.{js,jsx,ts,tsx}", "./src/**/*.{js,jsx,ts,tsx}"],
  presets: [require("nativewind/preset")],
  theme: {
    extend: {
      colors: {
        hypernovaTeal: "#16c1b5",
        hypernovaBlue: "#2d73a5",
        hypernovaPurple: "#5b20a3",
        hypernovaInk: "#24315e",
        hypernovaBackground: "#f7f9fb",
      },
    },
  },
  plugins: [],
};
