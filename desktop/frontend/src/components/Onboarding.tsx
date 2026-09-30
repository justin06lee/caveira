import { useEffect, useState } from "react";
import { api } from "../lib/bridge";
import { thisComputer } from "../lib/platform";
import { getState, setState, toast } from "../lib/store";
import type { Workspace } from "../lib/types";
import { ImportPanel } from "./ImportPanel";
import { Logo } from "./Logo";
import { PathPicker } from "./PathPicker";

// Onboarding is the first run: bring chats over from other agents, then
// say where projects live. Both can be skipped, and both are in Settings
// afterwards.
export function Onboarding() {
  const [step, setStep] = useState<"import" | "workspace">("import");
  const [suggested, setSuggested] = useState<Workspace | null>(null);

  useEffect(() => {
    if (step !== "workspace") return;
    api
      .suggestWorkspace()
      .then(setSuggested)
      .catch(() => setSuggested({ path: "", short: "" }));
  }, [step]);

  const finish = async (dir?: string) => {
    try {
      const workspace = dir === undefined ? getState().workspace : await api.setWorkspace(dir);
      await api.finishOnboarding();
      setState(() => ({ onboarded: true, workspace }));
    } catch (e) {
      toast(e);
    }
  };

  return (
    <>
      <header className="header drag" />
      <div className="onboard">
        <div className="onboard-inner" key={step}>
          <div className="steps" aria-label={`Step ${step === "import" ? 1 : 2} of 2`}>
            <span className={step === "import" ? "on" : ""} />
            <span className={step === "workspace" ? "on" : ""} />
          </div>
          {step === "import" ? (
            <>
              <Logo size={40} />
              <h1>Bring your chats along</h1>
              <p className="sub">
                caveira can copy the chats and projects other coding agents left on {thisComputer}, so you can carry on with
                them here. The originals stay where they are.
              </p>
              <ImportPanel skipLabel="Skip" onDone={() => setStep("workspace")} />
            </>
          ) : (
            <>
              <h1>Where do your projects live?</h1>
              <p className="sub">
                {suggested?.short
                  ? "Most of the projects caveira knows about are in this folder. Enter to keep it, or type another."
                  : "Type the folder that holds them. Opening a project starts there."}
              </p>
              {suggested && (
                <PathPicker
                  base=""
                  mode="workspace"
                  placeholder="type the folder"
                  initial={suggested.short ? suggested.short + "/" : ""}
                  onPick={(path) => finish(path)}
                  aside={
                    <button className="btn quiet" onClick={() => finish()}>
                      Skip
                    </button>
                  }
                />
              )}
            </>
          )}
        </div>
      </div>
    </>
  );
}
