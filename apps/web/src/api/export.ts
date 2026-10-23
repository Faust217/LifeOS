import { SessionExpiredError } from "./auth";
import type { components } from "./generated/schema";

export type ExportFormat = components["schemas"]["OwnerExportRequest"]["format"];
export const MAX_EXPORT_BYTES = 64 * 1024 * 1024;
export type ExportDownload = { blob: Blob; filename: string };

export async function downloadOwnerExport(csrfToken: string, format: ExportFormat, signal: AbortSignal, version: 1 | 2 = 1): Promise<ExportDownload> {
  const response = await fetch("/api/v1/me/export", {
    method: "POST", credentials: "same-origin", cache: "no-store", signal,
    headers: { "Content-Type": "application/json", "X-CSRF-Token": csrfToken }, body: JSON.stringify({ format, ...(version === 1 ? {} : { export_version: version }) }),
  });
  if (response.status === 401) throw new SessionExpiredError();
  if (!response.ok) {
    const messages: Record<number, string> = {
      409: "This data needs the latest export format. Refresh the page and try again.",
      403: "Download authorization failed. Refresh the page and try again.",
      422: "Your export exceeds the supported size. No partial file was generated.",
      429: "An export is already running. Wait a few seconds and retry.",
      503: "Export is temporarily unavailable or timed out. Please retry.",
    };
    throw new Error(messages[response.status] ?? "Export could not be completed. Please retry.");
  }
  const type = response.headers.get("Content-Type")?.split(";")[0].trim();
  const lengthText = response.headers.get("Content-Length");
  const length = lengthText && /^\d+$/.test(lengthText) ? Number(lengthText) : NaN;
  const expectedType = format === "json" ? "application/json" : "application/zip";
  const disposition = response.headers.get("Content-Disposition") ?? "";
  const filename = /^attachment; filename="(lifeos-(?:export-v[12]-\d{8}T\d{6}Z\.json|finance-csv-v[12]-\d{8}T\d{6}Z\.zip))"$/.exec(disposition)?.[1];
  const expectedPrefix = format === "json" ? `lifeos-export-v${version}-` : `lifeos-finance-csv-v${version}-`;
  if (type !== expectedType || !filename?.startsWith(expectedPrefix) || !Number.isSafeInteger(length) || length <= 0 || length > MAX_EXPORT_BYTES || !response.body) {
    await response.body?.cancel();
    throw new Error("The download response was incomplete or unsupported. Please retry.");
  }
  const reader = response.body.getReader();
  const chunks: Uint8Array<ArrayBuffer>[] = [];
  let received = 0;
  try {
    while (true) {
      if (signal.aborted) throw new DOMException("Download cancelled", "AbortError");
      const { done, value } = await reader.read();
      if (done) break;
      received += value.byteLength;
      if (received > MAX_EXPORT_BYTES || received > length) throw new Error("The download exceeded its expected size. Please retry.");
      chunks.push(new Uint8Array(value));
    }
    if (signal.aborted) throw new DOMException("Download cancelled", "AbortError");
    if (received !== length) throw new Error("The download was interrupted. Please retry.");
    return { blob: new Blob(chunks, { type: expectedType }), filename };
  } catch (error) { await reader.cancel().catch(() => undefined); throw error; }
  finally { reader.releaseLock(); }
}
