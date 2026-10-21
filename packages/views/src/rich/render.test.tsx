// @vitest-environment jsdom
import { describe, expect, it, vi } from "vitest";
import { fireEvent, render } from "@testing-library/react";
import { Renderer } from "@openuidev/react-lang";
import { RichActionContext, richLibrary } from "./library";

describe("OpenUI rendering", () => {
  it("renders whitelisted components and routes button clicks to onAction", () => {
    const onAction = vi.fn();
    const src = 'root = Stack([t, b])\nt = Text("p95 latency up 40%")\nb = Button("Acknowledge", "ack")';
    const r = render(
      <RichActionContext.Provider value={{ onAction, disabled: false }}>
        <Renderer response={src} library={richLibrary} isStreaming={false} />
      </RichActionContext.Provider>,
    );
    expect(r.container.textContent).toContain("p95 latency up 40%");
    fireEvent.click(r.getByTestId("rich-button"));
    expect(onAction).toHaveBeenCalledWith("ack");
  });
  it("does not render unknown components", () => {
    const r = render(<Renderer response={'root = Stack([x])\nx = Iframe("https://evil.example")'} library={richLibrary} isStreaming={false} />);
    expect(r.container.querySelector("iframe")).toBeNull();
    expect(r.container.textContent).not.toContain("evil");
  });
});
