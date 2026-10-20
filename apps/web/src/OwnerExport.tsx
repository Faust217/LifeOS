import { useEffect, useRef, useState } from "react";
import { SessionExpiredError } from "./api/auth";
import { downloadOwnerExport, type ExportFormat } from "./api/export";

export function OwnerExport({ csrfToken, onSessionExpired }: { csrfToken: string; onSessionExpired: () => void }) {
  const [pending, setPending] = useState(false);
  const [message, setMessage] = useState("");
  const [error, setError] = useState("");
  const [download, setDownload] = useState<{ url: string; filename: string } | null>(null);
  const controller = useRef<AbortController | null>(null);
  const objectURL = useRef<string | null>(null);
  const expiry = useRef<ReturnType<typeof setTimeout> | null>(null);
  function release() {
    if (objectURL.current) URL.revokeObjectURL(objectURL.current);
    objectURL.current = null;
    if (expiry.current) clearTimeout(expiry.current);
    expiry.current = null;
  }
  useEffect(() => () => { controller.current?.abort(); release(); }, []);
  function cancel() {
    controller.current?.abort(); release(); setDownload(null); setMessage("Download cancelled. You can try again.");
  }
  async function downloadExport(format: ExportFormat) {
    if (controller.current) return;
    release(); setDownload(null); setError(""); setMessage("Preparing your private export…"); setPending(true);
    const request = new AbortController(); controller.current = request;
    try {
      const result = await downloadOwnerExport(csrfToken, format, request.signal);
      if (request.signal.aborted) return;
      const url = URL.createObjectURL(result.blob); objectURL.current = url;
      setDownload({ url, filename: result.filename }); setMessage("Download ready. Use Save file if it did not start.");
      const link = document.createElement("a"); link.href = url; link.download = result.filename; link.hidden = true;
      document.body.appendChild(link); link.click(); link.remove();
      expiry.current = setTimeout(() => { release(); setDownload(null); setMessage("Download link expired. Generate another export when needed."); }, 5 * 60 * 1000);
    } catch (cause) {
      if (request.signal.aborted) return;
      if (cause instanceof SessionExpiredError) onSessionExpired();
      else { setMessage(""); setError(cause instanceof Error ? cause.message : "Download failed. Please retry."); }
    } finally { if (controller.current === request) { controller.current = null; setPending(false); } }
  }
  return <section className="me-card export-section" aria-labelledby="export-title">
    <h2 id="export-title">Export my data</h2>
    <p id="export-privacy">This file contains private financial and Goal data. Store it somewhere safe. Deleted transactions are excluded.</p>
    <div className="me-actions" aria-describedby="export-privacy">
      <button className="button button-primary" type="button" disabled={pending} onClick={() => void downloadExport("json")}>Download JSON</button>
      <button className="button button-outline" type="button" disabled={pending} onClick={() => void downloadExport("finance_csv")}>Download Finance CSV (ZIP)</button>
      {pending && <button className="button button-outline" type="button" onClick={cancel}>Cancel download</button>}
    </div>
    <p className="me-help">JSON includes Money, Goals and preferences. The CSV ZIP contains Finance spreadsheet files.</p>
    {message && <p role="status" aria-live="polite">{message}</p>}
    {error && <p className="inline-error" role="alert">{error}</p>}
    {download && <a className="button button-outline export-save" href={download.url} download={download.filename}>Save file</a>}
  </section>;
}
