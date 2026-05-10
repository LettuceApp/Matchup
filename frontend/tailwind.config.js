/** @type {import('tailwindcss').Config} */
module.exports = {
  content: ["./src/**/*.{js,jsx,ts,tsx}"],
  // `dark:` utilities apply when an ancestor has [data-theme="dark"].
  // utils/theme.js sets that attribute on <html> from localStorage; the
  // CSS-variable theme tokens in styles/theme.css already key off the
  // same attribute, so Tailwind's dark variant + our token system
  // share one source of truth and toggle in lockstep.
  darkMode: ['selector', '[data-theme="dark"]'],
  theme: {
    extend: {},
  },
  plugins: [],
};
