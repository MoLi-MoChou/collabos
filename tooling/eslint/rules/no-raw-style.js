// Enforces the tokens-only styling rule (conventions §2.5): no raw colors,
// no Tailwind arbitrary values, no default palette or size-scale classes.
const HEX = /(^|[^\w&/])#(?:[0-9a-fA-F]{3,4}|[0-9a-fA-F]{6}|[0-9a-fA-F]{8})\b/;
const FUNC = /\b(?:rgba?|hsla?|oklch|oklab|lab|lch|color-mix)\(/;
const ARBITRARY = /(?:^|\s|:)!?-?[a-z][a-z0-9-]*-\[[^\]]+\](?!:)/;
const PALETTE =
  /(?:^|\s|:)(?:bg|text|border|ring|fill|stroke|from|to|via|outline|divide|shadow|accent|caret|decoration|placeholder)-(?:slate|gray|zinc|neutral|stone|red|orange|amber|yellow|lime|green|emerald|teal|cyan|sky|blue|indigo|violet|purple|fuchsia|pink|rose|black|white)(?:-\d{2,3})?(?:\/\d+)?(?=\s|$)/;
const SIZE = /(?:^|\s|:)text-(?:xs|sm|base|lg|xl|[2-9]xl)(?=\s|$)/;

export const checks = [
  [HEX, "raw hex color"],
  [FUNC, "raw color function"],
  [ARBITRARY, "Tailwind arbitrary value"],
  [PALETTE, "default palette class"],
  [SIZE, "size-scale text class (use text-body / text-caption ...)"],
];

export function violation(value) {
  for (const [re, what] of checks) if (re.test(value)) return what;
  return null;
}

export default {
  meta: {
    type: "problem",
    docs: { description: "Only semantic design tokens may be used for styling" },
    schema: [],
    messages: { raw: "{{what}} is not allowed; use a semantic token (see packages/ui/themes)." },
  },
  create(context) {
    const check = (node, value) => {
      if (typeof value !== "string") return;
      const what = violation(value);
      if (what) context.report({ node, messageId: "raw", data: { what } });
    };
    return {
      Literal(node) {
        check(node, node.value);
      },
      TemplateElement(node) {
        check(node, node.value.cooked);
      },
    };
  },
};
