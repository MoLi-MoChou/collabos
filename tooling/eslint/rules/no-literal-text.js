// Views must not hard-code user-facing text (conventions §2.4): JSX text and
// text-bearing attributes have to come from i18n.
const LETTER = /\p{L}/u;
const ATTRS = new Set(["placeholder", "title", "alt", "aria-label", "label"]);

export default {
  meta: {
    type: "problem",
    docs: { description: "Disallow hard-coded UI text in views" },
    schema: [],
    messages: { text: "Hard-coded UI text {{text}}; use t('…') from i18n." },
  },
  create(context) {
    return {
      JSXText(node) {
        const text = node.value.trim();
        if (text && LETTER.test(text)) context.report({ node, messageId: "text", data: { text: JSON.stringify(text.slice(0, 30)) } });
      },
      JSXAttribute(node) {
        const name = node.name && node.name.name;
        if (!ATTRS.has(name) || !node.value || node.value.type !== "Literal") return;
        if (typeof node.value.value === "string" && LETTER.test(node.value.value)) {
          context.report({ node, messageId: "text", data: { text: JSON.stringify(node.value.value.slice(0, 30)) } });
        }
      },
    };
  },
};
