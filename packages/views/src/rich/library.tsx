import { createContext, useContext, useState, type ReactNode } from "react";
import { createLibrary, defineComponent } from "@openuidev/react-lang";
import { z } from "zod/v4";
import { Button as UIButton, Input } from "@collabos/ui";

/**
 * CollabOS OpenUI whitelist (PRD 7.1.5). Keep the names in sync with
 * server/internal/service/richmsg.go RichComponents: the server refuses
 * anything else, and this library cannot render anything else. Components
 * never navigate, fetch or run platform operations; interactions only go
 * through onAction, which posts a user_input reply on behalf of the clicker.
 */
export interface RichActions {
  onAction: (actionId: string, value?: unknown) => void;
  disabled: boolean;
}
export const RichActionContext = createContext<RichActions>({ onAction: () => undefined, disabled: true });
const useRich = () => useContext(RichActionContext);

const FormCtx = createContext<{ set: (k: string, v: string) => void } | null>(null);

const tone = z.enum(["info", "success", "warning", "danger"]).optional();
const toneClass: Record<string, string> = {
  info: "bg-tag text-fg",
  success: "bg-success-soft text-success",
  warning: "bg-warn-soft text-warn",
  danger: "bg-danger-soft text-danger",
};

const Stack = defineComponent({
  name: "Stack",
  description: "Vertical or horizontal layout of child components",
  props: z.object({ children: z.array(z.any()), direction: z.enum(["vertical", "horizontal"]).optional() }),
  component: ({ props, renderNode }) => (
    <div className={props.direction === "horizontal" ? "flex flex-wrap items-center gap-2" : "flex flex-col gap-2"}>{renderNode(props.children)}</div>
  ),
});

const Text = defineComponent({
  name: "Text",
  description: "A paragraph of plain text",
  props: z.object({ content: z.string(), weight: z.enum(["normal", "bold"]).optional() }),
  component: ({ props }) => <p className={"whitespace-pre-wrap text-body " + (props.weight === "bold" ? "font-semibold" : "")}>{props.content}</p>,
});

const Callout = defineComponent({
  name: "Callout",
  description: "Highlighted note with a tone",
  props: z.object({ tone: tone, text: z.string() }),
  component: ({ props }) => <div className={"rounded-control px-3 py-2 text-body " + toneClass[props.tone ?? "info"]}>{props.text}</div>,
});

const CodeBlock = defineComponent({
  name: "CodeBlock",
  description: "Monospaced code or log output",
  props: z.object({ code: z.string(), language: z.string().optional() }),
  component: ({ props }) => <pre className="overflow-x-auto rounded-control bg-term-bg px-3 py-2 font-mono text-mono text-term-fg">{props.code}</pre>,
});

const Table = defineComponent({
  name: "Table",
  description: "A table with a header row",
  props: z.object({ columns: z.array(z.string()), rows: z.array(z.array(z.union([z.string(), z.number()]))) }),
  component: ({ props }) => (
    <div className="overflow-x-auto">
      <table className="w-full text-caption">
        <thead>
          <tr>{props.columns.map((c, i) => <th key={i} className="border-b border-border px-2 py-1 text-left font-medium text-fg-muted">{c}</th>)}</tr>
        </thead>
        <tbody>
          {props.rows.map((r, i) => (
            <tr key={i}>{r.map((v, j) => <td key={j} className="border-b border-border px-2 py-1">{String(v)}</td>)}</tr>
          ))}
        </tbody>
      </table>
    </div>
  ),
});

const Chart = defineComponent({
  name: "Chart",
  description: "Horizontal bar chart of labelled values",
  props: z.object({ labels: z.array(z.string()), values: z.array(z.number()), title: z.string().optional() }),
  component: ({ props }) => {
    const max = Math.max(1, ...props.values);
    return (
      <figure className="flex flex-col gap-1" role="img" aria-label={props.title ?? "chart"}>
        {props.title && <figcaption className="text-caption font-medium">{props.title}</figcaption>}
        {props.labels.map((l, i) => (
          <div key={i} className="flex items-center gap-2 text-caption">
            <span className="w-24 truncate text-fg-muted">{l}</span>
            <span className="h-2 rounded-pill bg-accent" style={{ width: `${(100 * (props.values[i] ?? 0)) / max}%` }} />
            <span className="tabular-nums">{props.values[i] ?? 0}</span>
          </div>
        ))}
      </figure>
    );
  },
});

const Checklist = defineComponent({
  name: "Checklist",
  description: "Read-only list of items with done state",
  props: z.object({ items: z.array(z.string()), done: z.array(z.boolean()).optional() }),
  component: ({ props }) => (
    <ul className="flex flex-col gap-1 text-body">
      {props.items.map((it, i) => (
        <li key={i} className="flex items-center gap-2">
          <span aria-hidden>{props.done?.[i] ? "☑" : "☐"}</span>
          <span className={props.done?.[i] ? "text-fg-muted line-through" : ""}>{it}</span>
        </li>
      ))}
    </ul>
  ),
});

const Choices = defineComponent({
  name: "Choices",
  description: "A question with option buttons; the pick is sent as the action value",
  props: z.object({ question: z.string(), options: z.array(z.string()), actionId: z.string() }),
  component: ({ props }) => <ChoicesView {...props} />,
});
function ChoicesView(p: { question: string; options: string[]; actionId: string }) {
  const r = useRich();
  return (
    <div className="flex flex-col gap-2">
      <div className="text-body">{p.question}</div>
      <div className="flex flex-wrap gap-1.5">
        {p.options.map((o) => (
          <UIButton key={o} size="sm" variant="secondary" disabled={r.disabled} onClick={() => r.onAction(p.actionId, { choice: o })} data-testid="rich-choice">
            {o}
          </UIButton>
        ))}
      </div>
    </div>
  );
}

const Field = defineComponent({
  name: "Field",
  description: "A text input inside a Form",
  props: z.object({ name: z.string(), label: z.string(), placeholder: z.string().optional() }),
  component: ({ props }) => <FieldView {...props} />,
});
function FieldView(p: { name: string; label: string; placeholder?: string }) {
  const f = useContext(FormCtx);
  return (
    <label className="flex flex-col gap-1 text-caption">
      <span className="text-fg-muted">{p.label}</span>
      <Input placeholder={p.placeholder} onChange={(e) => f?.set(p.name, e.target.value)} data-testid="rich-field" />
    </label>
  );
}

const Form = defineComponent({
  name: "Form",
  description: "Fields plus a submit button; values are sent as the action value",
  props: z.object({ fields: z.array(z.any()), submitLabel: z.string(), actionId: z.string() }),
  component: ({ props, renderNode }) => <FormView submitLabel={props.submitLabel} actionId={props.actionId}>{renderNode(props.fields)}</FormView>,
});
function FormView(p: { submitLabel: string; actionId: string; children: ReactNode }) {
  const r = useRich();
  const [vals, setVals] = useState<Record<string, string>>({});
  return (
    <FormCtx.Provider value={{ set: (k, v) => setVals((o) => ({ ...o, [k]: v })) }}>
      <form
        className="flex flex-col gap-2"
        onSubmit={(e) => {
          e.preventDefault();
          r.onAction(p.actionId, vals);
        }}
      >
        {p.children}
        <div>
          <UIButton size="sm" variant="primary" type="submit" disabled={r.disabled} data-testid="rich-submit">
            {p.submitLabel}
          </UIButton>
        </div>
      </form>
    </FormCtx.Provider>
  );
}

const Button = defineComponent({
  name: "Button",
  description: "A button that sends actionId back to the bot as user input",
  props: z.object({ label: z.string(), actionId: z.string() }),
  component: ({ props }) => <ButtonView {...props} />,
});
function ButtonView(p: { label: string; actionId: string }) {
  const r = useRich();
  return (
    <UIButton size="sm" variant="secondary" disabled={r.disabled} onClick={() => r.onAction(p.actionId)} data-testid="rich-button">
      {p.label}
    </UIButton>
  );
}

export const richComponents = [Stack, Text, Callout, CodeBlock, Table, Chart, Checklist, Choices, Form, Field, Button];
export const RICH_WHITELIST = richComponents.map((c) => c.name);
export const richLibrary = createLibrary({ components: richComponents, root: "Stack" });
