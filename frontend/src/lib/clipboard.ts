/**
 * Copies text to the clipboard. The async Clipboard API only exists on HTTPS or
 * localhost, and MeshManager usually runs on plain http:// on the LAN, so fall back to
 * a hidden textarea and execCommand there.
 */
export async function copyText(text: string): Promise<boolean> {
  if (window.isSecureContext && navigator.clipboard) {
    try {
      await navigator.clipboard.writeText(text);
      return true;
    } catch {
      // fall through to the legacy path
    }
  }
  const previous = document.activeElement as HTMLElement | null;
  const ta = document.createElement("textarea");
  ta.value = text;
  ta.setAttribute("readonly", "");
  ta.style.position = "fixed";
  ta.style.opacity = "0";
  ta.style.fontSize = "16px"; // smaller text makes iOS zoom the page on focus
  document.body.appendChild(ta);
  ta.select();
  ta.setSelectionRange(0, text.length); // iOS ignores select() on a readonly field
  let ok = false;
  try {
    ok = ta.selectionEnd - ta.selectionStart === text.length && document.execCommand("copy");
  } catch {
    ok = false;
  }
  ta.remove();
  previous?.focus();
  return ok;
}
