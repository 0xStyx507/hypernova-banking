/** @type {import('tailwindcss').Config} */
export default {
  content: ["./index.html", "./src/**/*.{ts,tsx}"],
  theme: {
    extend: {
      colors: {
        ink: "#24315e",
        teal: "#16c1b5",
        blue: "#2d73a5",
        purple: "#5b20a3",
        cream: "#f7f9fb",
      },
    },
  },
  plugins: [],
};
