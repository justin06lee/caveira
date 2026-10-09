import { StrictMode } from "react";
import { createRoot } from "react-dom/client";
import { App } from "./App";
import { mac } from "./lib/platform";
// Poppins ships with the app, so no text waits on the network for it.
import "@fontsource/poppins/400.css";
import "@fontsource/poppins/400-italic.css";
import "@fontsource/poppins/500.css";
import "@fontsource/poppins/600.css";
import "@fontsource/poppins/700.css";
import "./styles.css";

// The Mac window leaves room for the traffic lights; see styles.css.
document.documentElement.dataset.platform = mac ? "mac" : "other";

createRoot(document.getElementById("root")!).render(
  <StrictMode>
    <App />
  </StrictMode>,
);
