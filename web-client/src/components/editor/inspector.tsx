"use client";

import {
  FIELDS,
  bool,
  items,
  str,
  strings,
  type Block,
  type BlockProps,
  type Field,
  type PropValue,
} from "@/lib/site-blocks";

/* Small shared bits ------------------------------------------------- */

const inputClass =
  "w-full rounded-md border border-edge bg-[#08080a] px-2.5 py-1.5 text-sm text-bone outline-none transition-colors placeholder:text-slate-deep focus:border-ember";

const miniButton =
  "rounded border border-edge px-1.5 py-0.5 font-mono text-[11px] text-slate transition-colors hover:border-edge-bright hover:text-bone disabled:opacity-30";

function Label({
  children,
  hint,
}: {
  children: string;
  hint?: string;
}) {
  return (
    <div className="mb-1.5">
      <span className="font-mono text-[11px] tracking-[0.16em] text-slate uppercase">
        {children}
      </span>
      {hint && <p className="mt-1 text-[11px] leading-snug text-slate-deep">{hint}</p>}
    </div>
  );
}

/* Field editors ----------------------------------------------------- */

function StringsEditor({
  value,
  itemLabel = "item",
  onChange,
}: {
  value: string[];
  itemLabel?: string;
  onChange: (next: string[]) => void;
}) {
  const move = (i: number, by: number) => {
    const next = [...value];
    const j = i + by;
    if (j < 0 || j >= next.length) return;
    [next[i], next[j]] = [next[j], next[i]];
    onChange(next);
  };

  return (
    <div className="flex flex-col gap-1.5">
      {value.map((item, i) => (
        <div key={i} className="flex items-center gap-1">
          <input
            className={inputClass}
            value={item}
            onChange={(e) => {
              const next = [...value];
              next[i] = e.target.value;
              onChange(next);
            }}
          />
          <button
            type="button"
            className={miniButton}
            onClick={() => move(i, -1)}
            disabled={i === 0}
            aria-label="move up"
          >
            ↑
          </button>
          <button
            type="button"
            className={miniButton}
            onClick={() => move(i, 1)}
            disabled={i === value.length - 1}
            aria-label="move down"
          >
            ↓
          </button>
          <button
            type="button"
            className={miniButton}
            onClick={() => onChange(value.filter((_, j) => j !== i))}
            aria-label="remove"
          >
            ✕
          </button>
        </div>
      ))}
      <button
        type="button"
        className="mt-1 self-start rounded border border-dashed border-edge-bright px-2 py-1 text-xs text-slate transition-colors hover:text-bone"
        onClick={() => onChange([...value, ""])}
      >
        + add {itemLabel}
      </button>
    </div>
  );
}

function ItemsEditor({
  value,
  subfields,
  itemLabel = "item",
  onChange,
}: {
  value: Record<string, string>[];
  subfields: { key: string; label: string; multiline?: boolean }[];
  itemLabel?: string;
  onChange: (next: Record<string, string>[]) => void;
}) {
  const move = (i: number, by: number) => {
    const next = [...value];
    const j = i + by;
    if (j < 0 || j >= next.length) return;
    [next[i], next[j]] = [next[j], next[i]];
    onChange(next);
  };

  return (
    <div className="flex flex-col gap-2.5">
      {value.map((item, i) => (
        <div key={i} className="rounded-md border border-edge bg-void-2 p-2.5">
          <div className="mb-2 flex items-center justify-between">
            <span className="font-mono text-[10px] tracking-[0.18em] text-slate-deep uppercase">
              {itemLabel} {i + 1}
            </span>
            <div className="flex gap-1">
              <button
                type="button"
                className={miniButton}
                onClick={() => move(i, -1)}
                disabled={i === 0}
                aria-label="move up"
              >
                ↑
              </button>
              <button
                type="button"
                className={miniButton}
                onClick={() => move(i, 1)}
                disabled={i === value.length - 1}
                aria-label="move down"
              >
                ↓
              </button>
              <button
                type="button"
                className={miniButton}
                onClick={() => onChange(value.filter((_, j) => j !== i))}
                aria-label="remove"
              >
                ✕
              </button>
            </div>
          </div>
          <div className="flex flex-col gap-2">
            {subfields.map((sub) => (
              <div key={sub.key}>
                <span className="mb-1 block text-[11px] text-slate-deep">
                  {sub.label}
                </span>
                {sub.multiline ? (
                  <textarea
                    className={`${inputClass} resize-y`}
                    rows={3}
                    value={item[sub.key] ?? ""}
                    onChange={(e) => {
                      const next = [...value];
                      next[i] = { ...next[i], [sub.key]: e.target.value };
                      onChange(next);
                    }}
                  />
                ) : (
                  <input
                    className={inputClass}
                    value={item[sub.key] ?? ""}
                    onChange={(e) => {
                      const next = [...value];
                      next[i] = { ...next[i], [sub.key]: e.target.value };
                      onChange(next);
                    }}
                  />
                )}
              </div>
            ))}
          </div>
        </div>
      ))}
      <button
        type="button"
        className="self-start rounded border border-dashed border-edge-bright px-2 py-1 text-xs text-slate transition-colors hover:text-bone"
        onClick={() =>
          onChange([
            ...value,
            Object.fromEntries(subfields.map((s) => [s.key, ""])) as Record<
              string,
              string
            >,
          ])
        }
      >
        + add {itemLabel}
      </button>
    </div>
  );
}

function FieldEditor({
  field,
  props,
  onChange,
}: {
  field: Field;
  props: BlockProps;
  onChange: (key: string, value: PropValue) => void;
}) {
  switch (field.kind) {
    case "text":
      return (
        <div>
          <Label hint={field.hint}>{field.label}</Label>
          <input
            className={inputClass}
            value={str(props, field.key)}
            onChange={(e) => onChange(field.key, e.target.value)}
          />
        </div>
      );
    case "textarea":
      return (
        <div>
          <Label hint={field.hint}>{field.label}</Label>
          <textarea
            className={`${inputClass} resize-y leading-relaxed`}
            rows={field.rows ?? 3}
            value={str(props, field.key)}
            onChange={(e) => onChange(field.key, e.target.value)}
          />
        </div>
      );
    case "toggle":
      return (
        <label className="flex cursor-pointer items-start gap-2.5">
          <input
            type="checkbox"
            className="mt-0.5 h-3.5 w-3.5 accent-[color:var(--color-ember)]"
            checked={bool(props, field.key)}
            onChange={(e) => onChange(field.key, e.target.checked)}
          />
          <span>
            <span className="font-mono text-[11px] tracking-[0.16em] text-slate uppercase">
              {field.label}
            </span>
            {field.hint && (
              <span className="mt-0.5 block text-[11px] leading-snug text-slate-deep">
                {field.hint}
              </span>
            )}
          </span>
        </label>
      );
    case "select":
      return (
        <div>
          <Label hint={field.hint}>{field.label}</Label>
          <select
            className={inputClass}
            value={str(props, field.key, field.options[0]?.value)}
            onChange={(e) => onChange(field.key, e.target.value)}
          >
            {field.options.map((opt) => (
              <option key={opt.value} value={opt.value}>
                {opt.label}
              </option>
            ))}
          </select>
        </div>
      );
    case "strings":
      return (
        <div>
          <Label hint={field.hint}>{field.label}</Label>
          <StringsEditor
            value={strings(props, field.key)}
            itemLabel={field.itemLabel}
            onChange={(next) => onChange(field.key, next)}
          />
        </div>
      );
    case "items":
      return (
        <div>
          <Label hint={field.hint}>{field.label}</Label>
          <ItemsEditor
            value={items(props, field.key)}
            subfields={field.subfields}
            itemLabel={field.itemLabel}
            onChange={(next) => onChange(field.key, next)}
          />
        </div>
      );
  }
}

/* The panel --------------------------------------------------------- */

export function Inspector({
  block,
  onChange,
}: {
  block: Block;
  onChange: (props: BlockProps) => void;
}) {
  const set = (key: string, value: PropValue) =>
    onChange({ ...block.props, [key]: value });

  return (
    <div className="flex flex-col gap-5">
      {FIELDS[block.type].map((field) => (
        <FieldEditor key={field.key} field={field} props={block.props} onChange={set} />
      ))}
    </div>
  );
}
