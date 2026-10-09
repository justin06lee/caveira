import { useCallback, useEffect, useRef, useState, type ReactNode } from "react";
import { Check, ChevronDown } from "lucide-react";
import { useDismiss } from "../lib/hooks";

export interface SelectOption {
  value: string;
  label: ReactNode;
  // sub is a quiet second line in the menu; aside, quiet words after the
  // label on the button.
  sub?: string;
  aside?: string;
  disabled?: boolean;
}

// Select is the app's own dropdown, drawn like its other menus: a field
// that opens a list of options under it, with the chosen one ticked. The
// arrow keys move through the list, Enter picks, Escape closes.
export function Select(props: {
  value: string;
  options: SelectOption[];
  onChange: (value: string) => void;
  placeholder?: string;
  label?: string;
}) {
  const { value, options, onChange } = props;
  const [open, setOpen] = useState(false);
  const [active, setActive] = useState(-1);
  const close = useCallback(() => setOpen(false), []);
  const ref = useDismiss<HTMLDivElement>(open, close);
  const button = useRef<HTMLButtonElement>(null);
  const list = useRef<HTMLDivElement>(null);

  const chosen = options.find((o) => o.value === value);
  const usable = (i: number) => i >= 0 && i < options.length && !options[i].disabled;

  const show = () => {
    setActive(options.findIndex((o) => o.value === value));
    setOpen(true);
  };
  const pick = (o: SelectOption) => {
    setOpen(false);
    button.current?.focus();
    if (o.value !== value) onChange(o.value);
  };
  const step = (by: number) => {
    let i = active;
    for (let n = 0; n < options.length; n++) {
      i = (i + by + options.length) % options.length;
      if (usable(i)) break;
    }
    setActive(i);
  };

  useEffect(() => {
    if (open) list.current?.querySelector<HTMLElement>(`[data-i="${active}"]`)?.scrollIntoView({ block: "nearest" });
  }, [open, active]);

  return (
    <div ref={ref} className="select">
      <button
        ref={button}
        type="button"
        className={`select-btn${open ? " open" : ""}`}
        aria-haspopup="listbox"
        aria-expanded={open}
        aria-label={props.label}
        onClick={() => (open ? close() : show())}
        onKeyDown={(e) => {
          if (!open) {
            if (e.key === "ArrowDown" || e.key === "ArrowUp") {
              e.preventDefault();
              show();
            }
            return;
          }
          if (e.key === "ArrowDown" || e.key === "ArrowUp") {
            e.preventDefault();
            step(e.key === "ArrowDown" ? 1 : -1);
          } else if (e.key === "Enter" || e.key === " ") {
            e.preventDefault();
            if (usable(active)) pick(options[active]);
          } else if (e.key === "Tab") {
            close();
          }
        }}
      >
        <span className="select-value">
          {chosen ? (
            <>
              {chosen.label}
              {chosen.aside && <span className="select-aside">{chosen.aside}</span>}
            </>
          ) : (
            <span className="select-aside">{props.placeholder}</span>
          )}
        </span>
        <ChevronDown size={14} className="select-chev" />
      </button>
      {open && (
        <div ref={list} className="menu select-menu" role="listbox">
          {options.map((o, i) => (
            <button
              key={o.value}
              type="button"
              data-i={i}
              role="option"
              aria-selected={o.value === value}
              className={`menu-item${i === active ? " active" : ""}`}
              disabled={o.disabled}
              onMouseEnter={() => !o.disabled && setActive(i)}
              onClick={() => pick(o)}
            >
              <span className="grow">
                <span className="select-label">{o.label}</span>
                {o.sub && <span className="sub">{o.sub}</span>}
              </span>
              {o.value === value && <Check size={14} className="check" />}
            </button>
          ))}
          {!options.length && <div className="side-empty">{props.placeholder}</div>}
        </div>
      )}
    </div>
  );
}
