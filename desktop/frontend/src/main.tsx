import { StrictMode } from "react";
import { createRoot } from "react-dom/client";
import { App } from "./App";
import { mac } from "./lib/platform";
import "./styles.css";

// The Mac window leaves room for the traffic lights; see styles.css.
document.documentElement.dataset.platform = mac ? "mac" : "other";

createRoot(document.getElementById("root")!).render(
  <StrictMode>
    <App />
  </StrictMode>,
);
