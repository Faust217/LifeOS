import type { GoalDeleteAttempt } from "./goalDeleteState";

export function GoalDeletePanel({ attempt, onCancel, onDelete, onCheck }: { attempt: GoalDeleteAttempt; onCancel: () => void; onDelete: () => void; onCheck: () => void }) {
  if (attempt.phase === "done" || attempt.phase === "unavailable") return <section className="goal-notice" role="status" tabIndex={-1} id="goal-delete-result"><p>{attempt.phase === "done" ? "Goal deleted." : "This Goal is no longer available."}</p><a className="button button-outline" href="/plans">Return to Goals</a></section>;
  const busy = attempt.phase === "sending" || attempt.phase === "checking";
  return <section className="goal-remove-confirm goal-delete-confirm" role="group" aria-labelledby="goal-delete-title" aria-describedby="goal-delete-description" aria-busy={busy} onKeyDown={(event) => { if (event.key === "Escape" && !busy && !attempt.uncertain) { event.preventDefault(); onCancel(); } }}>
    <h3 id="goal-delete-title">{attempt.uncertain ? "Check your Goal deletion" : "Delete Goal?"}</h3>
    <p id="goal-delete-description">{attempt.uncertain ? "The delete may have completed. Check this Goal or retry safely." : <>Delete “{attempt.title}” and its checklist from LifeOS? You cannot undo this in the app. Data may remain in internal storage, backups or earlier exports.</>}</p>
    {attempt.error && <p id="goal-delete-error" className="goal-error" role="alert" tabIndex={-1}>{attempt.error}</p>}
    <div className="goal-button-row">
      {!attempt.uncertain && <button id="goal-delete-keep" type="button" className="button button-outline" disabled={busy} onClick={onCancel}>Keep Goal</button>}
      {attempt.uncertain && <button type="button" className="button button-outline" disabled={busy} onClick={onCheck}>Check Goal</button>}
      <button type="button" className="button button-danger" disabled={busy} onClick={onDelete}>{attempt.phase === "sending" ? "Deleting…" : attempt.phase === "checking" ? "Checking…" : attempt.uncertain ? "Retry deletion" : "Delete Goal"}</button>
    </div>
    {attempt.uncertain && <p className="goal-soft-note">Leaving this page does not undo a delete already sent. <a href="/plans">Return to Goals</a></p>}
    {busy && <span className="goal-screen-reader-only" role="status">{attempt.phase === "sending" ? "Deleting Goal." : "Checking Goal."}</span>}
  </section>;
}
