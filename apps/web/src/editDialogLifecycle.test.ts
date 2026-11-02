import { afterEach, describe, expect, it, vi } from "vitest";
import { openEditDialog } from "./editDialogLifecycle";

class InlineStyle {
  private values = new Map<string, [string, string]>();
  setProperty(name: string, value: string, priority = "") { this.values.set(name, [value, priority]); }
  getPropertyValue(name: string) { return this.values.get(name)?.[0] ?? ""; }
  getPropertyPriority(name: string) { return this.values.get(name)?.[1] ?? ""; }
  removeProperty(name: string) { const old = this.getPropertyValue(name); this.values.delete(name); return old; }
}

function environment(width = 320, gutter = 0) {
  const root = new InlineStyle();
  const body = new InlineStyle();
  const scrollTo = vi.fn();
  const focus = vi.fn();
  const dialog = { showModal: vi.fn(), close: vi.fn() };
  vi.stubGlobal("document", { documentElement: { style: root, clientWidth: width - gutter }, body: { style: body } });
  vi.stubGlobal("window", { scrollX: 7, scrollY: 640, innerWidth: width, scrollTo });
  return { root, body, scrollTo, focus, dialog, open: () => openEditDialog(dialog as unknown as HTMLDialogElement, { focus, isConnected: true } as unknown as HTMLElement) };
}

afterEach(() => vi.unstubAllGlobals());

describe("Edit dialog page scroll and focus lifecycle", () => {
  it.each([[320, 0], [390, 0], [1280, 15]])("locks the background at %ipx without consuming the sheet's scroll area", (width, gutter) => {
    const e = environment(width, gutter);
    const close = e.open();
    expect(e.root.getPropertyValue("overflow")).toBe("hidden");
    expect(e.body.getPropertyValue("position")).toBe("fixed");
    expect(e.body.getPropertyValue("top")).toBe("-640px");
    expect(e.body.getPropertyValue("left")).toBe("-7px");
    expect(e.body.getPropertyValue("width")).toBe(`calc(100% - ${gutter}px)`);
    expect(e.dialog.showModal).toHaveBeenCalledOnce();
    close();
    expect(e.dialog.close).toHaveBeenCalledOnce();
    expect(e.focus).toHaveBeenCalledWith({ preventScroll: true });
    expect(e.scrollTo).toHaveBeenCalledWith({ left: 7, top: 640, behavior: "instant" });
    expect(e.root.getPropertyValue("overflow")).toBe("");
    expect(e.body.getPropertyValue("position")).toBe("");
    close();
    expect(e.scrollTo).toHaveBeenCalledOnce();
  });

  it("restores prior inline styles and priorities without discarding unrelated styles", () => {
    const e = environment();
    e.root.setProperty("overflow", "clip", "important");
    e.root.setProperty("scroll-behavior", "smooth");
    e.body.setProperty("position", "relative");
    e.body.setProperty("width", "95%");
    const close = e.open();
    e.body.setProperty("color", "green");
    close();
    expect(e.root.getPropertyValue("overflow")).toBe("clip");
    expect(e.root.getPropertyPriority("overflow")).toBe("important");
    expect(e.root.getPropertyValue("scroll-behavior")).toBe("smooth");
    expect(e.body.getPropertyValue("position")).toBe("relative");
    expect(e.body.getPropertyValue("width")).toBe("95%");
    expect(e.body.getPropertyValue("color")).toBe("green");
  });

  it("releases the page lock if opening the dialog fails", () => {
    const e = environment();
    e.dialog.showModal.mockImplementation(() => { throw new Error("Unavailable dialog"); });
    expect(e.open).toThrow("Unavailable dialog");
    expect(e.body.getPropertyValue("position")).toBe("");
    expect(e.root.getPropertyValue("overflow")).toBe("");
    expect(e.scrollTo).toHaveBeenCalledOnce();
  });

  it("ignores a missing dialog without touching the document", () => {
    expect(() => openEditDialog(null, null)()).not.toThrow();
  });
});
