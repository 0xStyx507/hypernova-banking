/** @type {import('tailwindcss').Config} */
export default {
  content: ["./index.html", "./src/**/*.{ts,tsx}"],
  theme: {
    extend: {
      colors: {
        ink: "#10233f",
        mint: "#8cf0c5",
        cream: "#f7f5ef",
      },
    },
  },
  plugins: [],
};

