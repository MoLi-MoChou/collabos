import tseslint from "typescript-eslint";
import reactHooks from "eslint-plugin-react-hooks";
import noRawStyle from "./rules/no-raw-style.js";
import noLiteralText from "./rules/no-literal-text.js";

export const plugin = { rules: { "no-raw-style": noRawStyle, "no-literal-text": noLiteralText } };

/** Shared flat config. Pass { i18n: true } for packages/views. */
export function config({ i18n = false } = {}) {
  return tseslint.config(
    { ignores: ["dist/**", "**/*.gen.ts", "node_modules/**"] },
    ...tseslint.configs.recommended,
    {
      files: ["**/*.{ts,tsx}"],
      plugins: { collabos: plugin, "react-hooks": reactHooks },
      rules: {
        "collabos/no-raw-style": "error",
        "react-hooks/rules-of-hooks": "error",
        "react-hooks/exhaustive-deps": "warn",
        "@typescript-eslint/no-unused-vars": ["error", { argsIgnorePattern: "^_", varsIgnorePattern: "^_" }],
        "@typescript-eslint/consistent-type-assertions": ["error", { assertionStyle: "never" }],
        ...(i18n ? { "collabos/no-literal-text": "error" } : {}),
      },
    },
    {
      files: ["**/*.test.{ts,tsx}"],
      rules: { "collabos/no-literal-text": "off", "@typescript-eslint/consistent-type-assertions": "off" },
    },
  );
}
