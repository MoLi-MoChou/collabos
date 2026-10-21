import { RuleTester } from "eslint";
import { describe, it } from "vitest";
import noRawStyle from "./rules/no-raw-style.js";
import noLiteralText from "./rules/no-literal-text.js";

RuleTester.describe = describe;
RuleTester.it = it;
const tester = new RuleTester({ languageOptions: { parserOptions: { ecmaFeatures: { jsx: true } } } });

tester.run("no-raw-style", noRawStyle, {
  valid: [`const a = "bg-surface text-fg-muted rounded-card p-3"`, `const h = "#main"`, `const x = "text-body"`, `const v = "data-[state=active]:border-accent"`],
  invalid: [
    { code: `const a = "bg-[#fff]"`, errors: 1 },
    { code: `const a = "color: #ff0000"`, errors: 1 },
    { code: "const a = `p-[13px]`", errors: 1 },
    { code: `const a = "bg-zinc-900"`, errors: 1 },
    { code: `const a = "hover:text-blue-500"`, errors: 1 },
    { code: `const a = "rgb(1,2,3)"`, errors: 1 },
    { code: `const a = "text-sm"`, errors: 1 },
  ],
});

tester.run("no-literal-text", noLiteralText, {
  valid: [`const a = <div>{t("x")}</div>`, `const a = <div> · </div>`, `const a = <input placeholder={t("p")} />`],
  invalid: [
    { code: `const a = <div>Hello</div>`, errors: 1 },
    { code: `const a = <div>你好</div>`, errors: 1 },
    { code: `const a = <input placeholder="Type" />`, errors: 1 },
  ],
});
