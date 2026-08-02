/** @type {import('tailwindcss').Config} */
module.exports = {
  content: ["./app/**/*.{js,jsx,ts,tsx}", "./components/**/*.{js,jsx,ts,tsx}"],
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
