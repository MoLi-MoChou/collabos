import { splitFences } from "@collabos/core";
import { DiffBlock } from "../cards/DiffBlock";

/** Plain text with ``` fenced blocks rendered as code (diff fences get +/- colouring). */
export function Fenced({ body }: { body: string }) {
  return (
    <>
      {splitFences(body).map((p, i) =>
        p.code === null ? (
          <span key={i}>{p.text}</span>
        ) : p.lang === "diff" ? (
          <DiffBlock key={i} diff={p.code} />
        ) : (
          <pre key={i} className="my-1 overflow-x-auto rounded-control bg-term-bg px-3 py-2 font-mono text-mono text-term-fg" data-lang={p.lang}>
            {p.code}
          </pre>
        ),
      )}
    </>
  );
}
