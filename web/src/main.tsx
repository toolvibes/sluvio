import { StrictMode } from "react";
import { createRoot } from "react-dom/client";
import App from "./App";
import { I18nProvider } from "./i18n";
import "./styles.css";

// Apply a stored theme before the first paint.
try {
  const theme = localStorage.getItem("sluvio.theme");
  if (theme === "light" || theme === "dark") document.documentElement.setAttribute("data-theme", theme);
} catch {
  /* storage unavailable */
}

createRoot(document.getElementById("root")!).render(
  <StrictMode>
    <I18nProvider>
      <App />
    </I18nProvider>
  </StrictMode>,
);
