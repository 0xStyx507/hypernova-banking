import { StrictMode } from "react";
import { createRoot } from "react-dom/client";
import "./styles.css";
import App from "./App";
import { installClientErrorLogging } from "./logger";

const removeClientErrorLogging = installClientErrorLogging();
window.addEventListener("beforeunload", removeClientErrorLogging, { once: true });

createRoot(document.getElementById("root")!).render(
  <StrictMode>
    <App />
  </StrictMode>,
);

