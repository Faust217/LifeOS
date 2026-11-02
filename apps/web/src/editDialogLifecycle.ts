// A modal dialog makes the background inert, but does not itself lock page scrolling.
export function openEditDialog(dialog: HTMLDialogElement | null, returnFocus: HTMLElement | null) {
  if (!dialog) return () => undefined;
  const root = document.documentElement;
  const body = document.body;
  const x = window.scrollX;
  const y = window.scrollY;
  const gutter = Math.max(0, window.innerWidth - root.clientWidth);
  function preserve(style: CSSStyleDeclaration, names: string[]) {
    const previous = names.map((name) => [name, style.getPropertyValue(name), style.getPropertyPriority(name)]);
    return () => previous.forEach(([name, value, priority]) => {
      if (value) style.setProperty(name, value, priority);
      else style.removeProperty(name);
    });
  }
  const restoreRoot = preserve(root.style, ["overflow", "scroll-behavior"]);
  const restoreBody = preserve(body.style, ["position", "top", "left", "width", "overflow"]);
  root.style.setProperty("overflow", "hidden");
  root.style.setProperty("scroll-behavior", "auto");
  body.style.setProperty("position", "fixed");
  body.style.setProperty("top", `${-y}px`);
  body.style.setProperty("left", `${-x}px`);
  body.style.setProperty("width", `calc(100% - ${gutter}px)`);
  body.style.setProperty("overflow", "hidden");
  let active = true;
  function restore() {
    if (!active) return;
    active = false;
    dialog?.close();
    restoreBody();
    if (returnFocus?.isConnected) returnFocus.focus({ preventScroll: true });
    window.scrollTo({ left: x, top: y, behavior: "instant" });
    restoreRoot();
  }
  try { dialog.showModal(); }
  catch (error) { restore(); throw error; }
  return restore;
}
