"use client";

import Link from "next/link";
import { useCallback, useEffect, useRef, useState } from "react";

import { Inspector } from "@/components/editor/inspector";
import { RenderBlock } from "@/components/site/block-renderer";
import { DEFAULT_LAYOUT } from "@/lib/default-layout";
import {
  BLOCK_META,
  BLOCK_ORDER,
  blockSummary,
  newBlock,
  type Block,
  type BlockProps,
  type BlockType,
  type SiteLayout,
} from "@/lib/site-blocks";

type Status = "loading" | "idle" | "saving" | "saved" | "error";

export function EditorShell() {
  const [blocks, setBlocks] = useState<Block[]>([]);
  const [selectedId, setSelectedId] = useState<string | null>(null);
  const [status, setStatus] = useState<Status>("loading");
  const [previewOnly, setPreviewOnly] = useState(false);

  // Autosave must not fire on the load that populated the editor, or an empty
  // first render would overwrite a good file on disk.
  const loaded = useRef(false);
  const timer = useRef<ReturnType<typeof setTimeout> | null>(null);
  const blockRefs = useRef(new Map<string, HTMLDivElement>());

  // Picking a block in the outline should take you to it. `nearest` means a
  // block already on screen — the one you just clicked in the preview — stays
  // exactly where it is.
  useEffect(() => {
    if (!selectedId) return;
    blockRefs.current
      .get(selectedId)
      ?.scrollIntoView({ block: "nearest", behavior: "smooth" });
  }, [selectedId]);

  useEffect(() => {
    (async () => {
      try {
        const res = await fetch("/api/editor/layout", { cache: "no-store" });
        const layout = (await res.json()) as SiteLayout;
        setBlocks(layout.blocks ?? []);
        setSelectedId(layout.blocks?.[0]?.id ?? null);
        setStatus("idle");
      } catch {
        setBlocks(DEFAULT_LAYOUT.blocks);
        setStatus("error");
      } finally {
        loaded.current = true;
      }
    })();
  }, []);

  const save = useCallback(async (next: Block[]) => {
    setStatus("saving");
    try {
      const res = await fetch("/api/editor/layout", {
        method: "POST",
        headers: { "content-type": "application/json" },
        body: JSON.stringify({ blocks: next }),
      });
      setStatus(res.ok ? "saved" : "error");
    } catch {
      setStatus("error");
    }
  }, []);

  // Debounced autosave: type freely, the file catches up when you pause.
  useEffect(() => {
    if (!loaded.current) return;
    if (timer.current) clearTimeout(timer.current);
    timer.current = setTimeout(() => void save(blocks), 700);
    return () => {
      if (timer.current) clearTimeout(timer.current);
    };
  }, [blocks, save]);

  /* -- mutations --------------------------------------------------- */

  const update = (id: string, props: BlockProps) =>
    setBlocks((bs) => bs.map((b) => (b.id === id ? { ...b, props } : b)));

  const move = (id: string, by: number) =>
    setBlocks((bs) => {
      const i = bs.findIndex((b) => b.id === id);
      const j = i + by;
      if (i < 0 || j < 0 || j >= bs.length) return bs;
      const next = [...bs];
      [next[i], next[j]] = [next[j], next[i]];
      return next;
    });

  const remove = (id: string) => {
    const i = blocks.findIndex((b) => b.id === id);
    const next = blocks.filter((b) => b.id !== id);
    setBlocks(next);
    // Land on whatever took its place, so deleting a run of blocks keeps the
    // selection where you are looking instead of snapping back to the top.
    if (selectedId === id) setSelectedId((next[i] ?? next[i - 1])?.id ?? null);
  };

  const duplicate = (id: string) => {
    const i = blocks.findIndex((b) => b.id === id);
    if (i < 0) return;
    const copy: Block = {
      ...newBlock(blocks[i].type),
      props: JSON.parse(JSON.stringify(blocks[i].props)) as BlockProps,
    };
    const next = [...blocks];
    next.splice(i + 1, 0, copy);
    setBlocks(next);
    setSelectedId(copy.id);
  };

  const add = (type: BlockType) => {
    const block = newBlock(type);
    // Drop it after whatever is selected, so building top-down just works.
    const i = blocks.findIndex((b) => b.id === selectedId);
    const next = [...blocks];
    next.splice(i < 0 ? blocks.length : i + 1, 0, block);
    setBlocks(next);
    setSelectedId(block.id);
  };

  const reset = () => {
    if (!confirm("Replace the whole layout with the page as it ships today?")) return;
    setBlocks(JSON.parse(JSON.stringify(DEFAULT_LAYOUT.blocks)) as Block[]);
    setSelectedId(DEFAULT_LAYOUT.blocks[0]?.id ?? null);
  };

  const selected = blocks.find((b) => b.id === selectedId) ?? null;

  /* -- chrome ------------------------------------------------------- */

  const statusText: Record<Status, string> = {
    loading: "loading…",
    idle: "ready",
    saving: "saving…",
    saved: "saved to site-layout.json",
    error: "save failed",
  };

  return (
    <div className="flex h-screen flex-col overflow-hidden bg-void">
      <header className="flex shrink-0 items-center justify-between gap-4 border-b border-edge bg-void-2 px-4 py-2.5">
        <div className="flex items-center gap-3">
          <span className="grid h-5 w-5 place-items-center border border-ember/70 bg-ember/10 font-mono text-[11px] text-ember">
            c
          </span>
          <span className="font-mono text-xs tracking-[0.28em] text-bone uppercase">
            site editor
          </span>
          <span className="font-mono text-[11px] text-slate-deep">
            {blocks.length} blocks
          </span>
        </div>

        <div className="flex items-center gap-3">
          <span
            className={`font-mono text-[11px] ${
              status === "error"
                ? "text-ember"
                : status === "saved"
                  ? "text-acid"
                  : "text-slate"
            }`}
          >
            {statusText[status]}
          </span>
          <button
            type="button"
            onClick={() => setPreviewOnly((v) => !v)}
            className="rounded border border-edge px-2.5 py-1 font-mono text-[11px] text-slate transition-colors hover:border-edge-bright hover:text-bone"
          >
            {previewOnly ? "show panels" : "preview only"}
          </button>
          <button
            type="button"
            onClick={reset}
            className="rounded border border-edge px-2.5 py-1 font-mono text-[11px] text-slate transition-colors hover:border-edge-bright hover:text-bone"
          >
            reset
          </button>
          <button
            type="button"
            onClick={() => void save(blocks)}
            className="rounded border border-ember/60 bg-ember/15 px-2.5 py-1 font-mono text-[11px] text-ember-bright transition-colors hover:bg-ember/25"
          >
            save now
          </button>
          <Link
            href="/"
            className="rounded border border-edge px-2.5 py-1 font-mono text-[11px] text-slate transition-colors hover:border-edge-bright hover:text-bone"
          >
            live site
          </Link>
        </div>
      </header>

      <div className="flex min-h-0 flex-1">
        {/* -- outline + palette ------------------------------------- */}
        {!previewOnly && (
          <aside className="flex w-64 shrink-0 flex-col overflow-y-auto border-r border-edge bg-void-2">
            <div className="p-3">
              <p className="mb-2 font-mono text-[10px] tracking-[0.2em] text-slate-deep uppercase">
                outline
              </p>
              <ol className="flex flex-col gap-1">
                {blocks.map((block, i) => {
                  const active = block.id === selectedId;
                  return (
                    <li key={block.id}>
                      <div
                        className={`group rounded border px-2 py-1.5 transition-colors ${
                          active
                            ? "border-ember/60 bg-ember/10"
                            : "border-transparent hover:border-edge hover:bg-panel"
                        }`}
                      >
                        <button
                          type="button"
                          onClick={() => setSelectedId(block.id)}
                          className="block w-full text-left"
                        >
                          <span
                            className={`block font-mono text-[10px] tracking-[0.16em] uppercase ${
                              active ? "text-ember-bright" : "text-slate-deep"
                            }`}
                          >
                            {String(i + 1).padStart(2, "0")} ·{" "}
                            {BLOCK_META[block.type].label}
                          </span>
                          <span className="mt-0.5 block truncate text-xs text-bone-dim">
                            {blockSummary(block)}
                          </span>
                        </button>
                        <div className="mt-1.5 flex gap-1 opacity-0 transition-opacity group-hover:opacity-100">
                          {[
                            { label: "↑", fn: () => move(block.id, -1), title: "up" },
                            { label: "↓", fn: () => move(block.id, 1), title: "down" },
                            { label: "⧉", fn: () => duplicate(block.id), title: "duplicate" },
                            { label: "✕", fn: () => remove(block.id), title: "delete" },
                          ].map((b) => (
                            <button
                              key={b.title}
                              type="button"
                              title={b.title}
                              onClick={b.fn}
                              className="rounded border border-edge px-1.5 py-0.5 font-mono text-[10px] text-slate transition-colors hover:border-edge-bright hover:text-bone"
                            >
                              {b.label}
                            </button>
                          ))}
                        </div>
                      </div>
                    </li>
                  );
                })}
              </ol>
            </div>

            <div className="mt-auto border-t border-edge p-3">
              <p className="mb-2 font-mono text-[10px] tracking-[0.2em] text-slate-deep uppercase">
                add block
              </p>
              <div className="flex flex-col gap-1">
                {BLOCK_ORDER.map((type) => (
                  <button
                    key={type}
                    type="button"
                    onClick={() => add(type)}
                    className="rounded border border-edge bg-panel px-2 py-1.5 text-left transition-colors hover:border-ember/50"
                  >
                    <span className="block text-xs text-bone">
                      {BLOCK_META[type].label}
                    </span>
                    <span className="block text-[10px] leading-snug text-slate-deep">
                      {BLOCK_META[type].blurb}
                    </span>
                  </button>
                ))}
              </div>
            </div>
          </aside>
        )}

        {/* -- preview ------------------------------------------------ */}
        <main className="min-w-0 flex-1 overflow-y-auto bg-void">
          {blocks.length === 0 && status !== "loading" && (
            <div className="grid h-full place-items-center p-10 text-center">
              <div>
                <p className="text-sm text-slate">
                  Nothing here yet. Add a block from the left.
                </p>
              </div>
            </div>
          )}
          {blocks.map((block) => {
            const active = block.id === selectedId;
            return (
              <div
                key={block.id}
                ref={(el) => {
                  if (el) blockRefs.current.set(block.id, el);
                  else blockRefs.current.delete(block.id);
                }}
                onClick={() => setSelectedId(block.id)}
                className={`relative cursor-pointer transition-shadow ${
                  active && !previewOnly
                    ? "outline outline-2 -outline-offset-2 outline-ember/70"
                    : "hover:outline hover:outline-1 hover:-outline-offset-1 hover:outline-edge-bright"
                }`}
              >
                {!previewOnly && (
                  <span
                    className={`pointer-events-none absolute left-0 top-0 z-20 px-1.5 py-0.5 font-mono text-[10px] tracking-[0.16em] uppercase ${
                      active
                        ? "bg-ember text-[#150807]"
                        : "bg-edge/80 text-slate opacity-0 transition-opacity hover:opacity-100"
                    }`}
                  >
                    {BLOCK_META[block.type].label}
                  </span>
                )}
                <RenderBlock block={block} />
              </div>
            );
          })}
        </main>

        {/* -- inspector ---------------------------------------------- */}
        {!previewOnly && (
          <aside className="w-80 shrink-0 overflow-y-auto border-l border-edge bg-void-2 p-4">
            {selected ? (
              <>
                <div className="mb-4 flex items-baseline justify-between gap-2">
                  <span className="font-mono text-[11px] tracking-[0.2em] text-bone uppercase">
                    {BLOCK_META[selected.type].label}
                  </span>
                  <span className="font-mono text-[10px] text-slate-deep">
                    {selected.type}
                  </span>
                </div>
                <Inspector
                  block={selected}
                  onChange={(props) => update(selected.id, props)}
                />
              </>
            ) : (
              <p className="text-xs text-slate">
                Select a block to edit it, or add one from the left.
              </p>
            )}
          </aside>
        )}
      </div>
    </div>
  );
}
