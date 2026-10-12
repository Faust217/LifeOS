import { useCallback, useEffect, useRef, useState, type FormEvent } from "react";

import {
  addGoalItem, archiveGoal, createGoal, GoalApiError, listGoals, removeGoalItem, restoreGoal,
  renameGoal, renameGoalItem, setGoalCompletion, setGoalItemCompletion, type Goal,
} from "./api/goals";
import { Icon } from "./Icons";
import "./goals.css";

type View = { status: "loading" } | { status: "error"; message: string } | { status: "ready"; goals: Goal[] };

function errorMessage(error: unknown): string {
  return error instanceof Error ? error.message : "LifeOS could not complete that action. Check the list before trying again.";
}

function titleValue(value: string): string {
  const title = value.trim();
  if (!title || [...title].length > 160) throw new Error("Enter a title of up to 160 characters.");
  return title;
}

export function GoalsPage({ goalID = null, csrfToken, onSessionExpired }: { goalID?: string | null; csrfToken: string; onSessionExpired: () => void }) {
  const [view, setView] = useState<View>({ status: "loading" });
  const [title, setTitle] = useState("");
  const [createError, setCreateError] = useState("");
  const [readError, setReadError] = useState("");
  const [notice, setNotice] = useState("");
  const [pending, setPending] = useState(false);
  const [refreshing, setRefreshing] = useState(false);
  const pendingRef = useRef(false);
  const requestSequence = useRef(0);

  const refresh = useCallback(async (): Promise<boolean> => {
    const request = ++requestSequence.current;
    setRefreshing(true);
    try {
      const response = await listGoals();
      if (request !== requestSequence.current) return false;
      setView({ status: "ready", goals: response.items });
      setReadError("");
      setRefreshing(false);
      return true;
    } catch (error) {
      if (request !== requestSequence.current) return false;
      if (error instanceof GoalApiError && error.status === 401) onSessionExpired();
      setView((current) => current.status === "ready" ? current : { status: "error", message: errorMessage(error) });
      setReadError(errorMessage(error));
      setRefreshing(false);
      return false;
    }
  }, [onSessionExpired]);

  useEffect(() => {
    void refresh();
    return () => { requestSequence.current += 1; };
  }, [refresh]);

  async function mutate(work: () => Promise<Goal>, message: string): Promise<void> {
    if (pendingRef.current) return;
    pendingRef.current = true;
    setPending(true);
    setNotice("");
    try {
      const changed = await work();
      setView((current) => current.status === "ready"
        ? { status: "ready", goals: [changed, ...current.goals.filter((goal) => goal.id !== changed.id)] }
        : current);
      const reloaded = await refresh();
      setNotice(reloaded ? message : "Saved, but the latest Goals could not load. Refresh the list before another change.");
    } catch (error) {
      if (error instanceof GoalApiError && error.status === 401) onSessionExpired();
      throw error;
    } finally {
      pendingRef.current = false;
      setPending(false);
    }
  }

  async function submitGoal(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    setCreateError("");
    try {
      const clean = titleValue(title);
      await mutate(() => createGoal(csrfToken, clean), "Goal created. Add a first step whenever you are ready.");
      setTitle("");
    } catch (error) { setCreateError(errorMessage(error)); }
  }

  const goals = view.status === "ready" ? view.goals : null;
  const active = goals?.filter((goal) => !goal.archived_at) ?? [];
  const archived = goals?.filter((goal) => goal.archived_at) ?? [];
  const selected = goalID ? goals?.find((goal) => goal.id.toLowerCase() === goalID.toLowerCase()) : undefined;
  return <div className="page-stack goals-page">
    {goalID && <a className="goal-back-link" href="/plans">← All Goals</a>}
    <header className="page-heading"><p className="eyebrow">PLANS · YOUR NEXT STEPS</p><h1>{selected ? selected.title : "Give your goals room to grow."}</h1><p>{goalID ? "Keep the next steps together and see your progress." : "Start with one intention, then make it practical with a simple checklist. Goals never change your Money balance."}</p></header>
    {view.status === "loading" && <section className="goals-state" role="status" aria-live="polite"><span className="loading-orb" aria-hidden="true" /><h2>Loading your Goals…</h2><p>Checking your latest steps.</p></section>}
    {view.status === "error" && <section className="goals-state" role="alert"><Icon name="shield" /><h2>Goals could not load.</h2><p>{view.message}</p><button className="button button-primary" type="button" onClick={() => void refresh()}>Try again</button></section>}
    {goals && goalID && !selected && <section className="goals-state" role="alert"><h2>Goal not found.</h2><p>It may have been removed or belongs to another account.</p><a className="button button-outline" href="/plans">Return to Goals</a></section>}
    {goals && goalID && selected && <>
      {notice && <p className="goal-notice" role="status">{notice}</p>}
      {readError && <div className="goal-read-error"><p role="alert">{readError}</p><button type="button" className="button button-outline" onClick={() => void refresh()}>Refresh Goal</button></div>}
      {refreshing && <p className="goal-refreshing" role="status">Refreshing Goal…</p>}
      <GoalCard goal={selected} csrfToken={csrfToken} pending={pending} mutate={mutate} />
    </>}
    {goals && !goalID && <>
      <section className="goal-create-card" aria-labelledby="goal-create-title"><div><p className="eyebrow">A FRESH START</p><h2 id="goal-create-title">What matters next?</h2><p>Give your Goal a short name. You can add steps, rename it, or archive it later.</p></div>
        <form onSubmit={(event) => void submitGoal(event)}><label htmlFor="goal-new-title">Goal title</label><div className="goal-create-row"><input id="goal-new-title" value={title} onChange={(event) => { setTitle(event.target.value); setCreateError(""); }} maxLength={160} required placeholder="Build a reading habit" aria-invalid={!!createError} aria-describedby={createError ? "goal-create-error" : undefined} disabled={pending} /><button className="button button-primary" type="submit" disabled={pending}>{pending ? "Saving…" : "Create Goal"}</button></div>{createError && <p id="goal-create-error" className="goal-error" role="alert">{createError}</p>}</form>
      </section>
      {notice && <p className="goal-notice" role="status">{notice}</p>}
      {readError && <div className="goal-read-error"><p role="alert">{readError}</p><button type="button" className="button button-outline" onClick={() => void refresh()}>Refresh Goals</button></div>}
      {refreshing && <p className="goal-refreshing" role="status">Refreshing Goals…</p>}
      <section className="goal-list-section" aria-labelledby="goal-list-title"><div className="section-heading"><div><p className="eyebrow">YOUR GOALS</p><h2 id="goal-list-title">Small steps add up.</h2></div></div>
        {active.length === 0 ? <div className="goals-empty"><Icon name="plans" /><h3>No active Goals yet.</h3><p>Create one above to begin with a single step. Your checklist stays private to you.</p></div>
          : <div className="goal-list goal-overview-list">{active.map((goal) => <GoalSummary key={goal.id} goal={goal} />)}</div>}
      </section>
      {archived.length > 0 && <details className="goal-archive-list"><summary>Archived Goals ({archived.length})</summary><div className="goal-list goal-overview-list">{archived.map((goal) => <GoalSummary key={goal.id} goal={goal} />)}</div></details>}
    </>}
  </div>;
}

function GoalSummary({ goal }: { goal: Goal }) {
  const status = goal.archived_at ? "Archived" : goal.completed_at ? "Completed" : "In progress";
  return <a className="goal-summary" href={`/plans/goals/${goal.id}`} aria-label={`Open ${goal.title}, ${status}, ${goal.completed_count} of ${goal.total_count} steps complete`}>
    <span className="goal-status">{status}</span>
    <strong>{goal.title}</strong>
    <span>{goal.total_count === 0 ? "No steps yet" : `${goal.completed_count} of ${goal.total_count} steps`}</span>
    <progress className="goal-progress" value={goal.completed_count} max={goal.total_count || 1} aria-label={`${goal.completed_count} of ${goal.total_count} steps complete`} />
    <span className="goal-summary-link">View Goal →</span>
  </a>;
}

function GoalCard({ goal, csrfToken, pending, mutate }: { goal: Goal; csrfToken: string; pending: boolean; mutate: (work: () => Promise<Goal>, message: string) => Promise<void> }) {
  const [renameOpen, setRenameOpen] = useState(false);
  const [titleDraft, setTitleDraft] = useState(goal.title);
  const [itemDraft, setItemDraft] = useState("");
  const [editingID, setEditingID] = useState<string | null>(null);
  const [editingDraft, setEditingDraft] = useState("");
  const [removingID, setRemovingID] = useState<string | null>(null);
  const [archiveConfirm, setArchiveConfirm] = useState(false);
  const [error, setError] = useState("");
  const archived = !!goal.archived_at;
  const completed = !!goal.completed_at;

  async function perform(work: () => Promise<Goal>, message: string, after?: () => void) {
    setError("");
    try { await mutate(work, message); after?.(); }
    catch (cause) { setError(errorMessage(cause)); }
  }

  return <article className={`goal-card${archived ? " goal-card-archived" : ""}`} aria-labelledby={`goal-title-${goal.id}`}>
    <div className="goal-card-head"><div><span className="goal-status">{archived ? "Archived" : completed ? "Completed" : "In progress"}</span><h2 id={`goal-title-${goal.id}`}>{goal.title}</h2></div><span className="goal-progress-text">{goal.total_count === 0 ? "No steps yet" : `${goal.completed_count} of ${goal.total_count} steps`}</span></div>
    <progress className="goal-progress" value={goal.completed_count} max={goal.total_count || 1} aria-label={`${goal.title}: ${goal.completed_count} of ${goal.total_count} checklist steps complete`} />
    {!archived && <>
      {renameOpen ? <form className="goal-inline-form" onSubmit={(event) => { event.preventDefault(); let clean: string; try { clean = titleValue(titleDraft); } catch (cause) { setError(errorMessage(cause)); return; } void perform(() => renameGoal(csrfToken, goal.id, clean), "Goal renamed.", () => setRenameOpen(false)); }}><label htmlFor={`goal-rename-${goal.id}`}>Rename Goal</label><input id={`goal-rename-${goal.id}`} value={titleDraft} maxLength={160} onChange={(event) => setTitleDraft(event.target.value)} aria-invalid={!!error} disabled={pending} /><div className="goal-button-row"><button className="button button-primary" type="submit" disabled={pending}>Save name</button><button className="button button-outline" type="button" onClick={() => { setRenameOpen(false); setTitleDraft(goal.title); setError(""); }} disabled={pending}>Cancel</button></div></form> : <button type="button" className="goal-link-action" onClick={() => { setTitleDraft(goal.title); setRenameOpen(true); }} disabled={pending}>Rename Goal</button>}
    </>}
    <div className="goal-steps"><h3>Checklist</h3>{goal.items.length === 0 && <p className="goal-soft-note">No steps yet. Add one small, clear action.</p>}
      <ul>{goal.items.map((item) => <li key={item.id} className="goal-step">
        <label className="goal-step-check"><input type="checkbox" checked={!!item.completed_at} disabled={pending || archived || completed} onChange={(event) => void perform(() => setGoalItemCompletion(csrfToken, goal.id, item.id, event.target.checked), event.target.checked ? "Step completed." : "Step reopened.")} /><span>{item.title}</span></label>
        {!archived && !completed && <div className="goal-step-actions">
          {editingID === item.id ? <form className="goal-inline-form" onSubmit={(event) => { event.preventDefault(); let clean: string; try { clean = titleValue(editingDraft); } catch (cause) { setError(errorMessage(cause)); return; } void perform(() => renameGoalItem(csrfToken, goal.id, item.id, clean), "Step renamed.", () => setEditingID(null)); }}><label htmlFor={`goal-item-edit-${item.id}`}>Edit step</label><input id={`goal-item-edit-${item.id}`} value={editingDraft} maxLength={160} onChange={(event) => setEditingDraft(event.target.value)} aria-invalid={!!error} disabled={pending} /><div className="goal-button-row"><button className="button button-primary" type="submit" disabled={pending}>Save</button><button className="button button-outline" type="button" onClick={() => { setEditingID(null); setError(""); }} disabled={pending}>Cancel</button></div></form> : <button type="button" className="goal-link-action" onClick={() => { setEditingID(item.id); setEditingDraft(item.title); }} disabled={pending}>Edit</button>}
          {removingID === item.id ? <div className="goal-remove-confirm"><p>Remove this step? This cannot be undone.</p><div className="goal-button-row"><button className="button button-danger" type="button" onClick={() => void perform(() => removeGoalItem(csrfToken, goal.id, item.id), "Step removed.", () => setRemovingID(null))} disabled={pending}>Confirm remove</button><button className="button button-outline" type="button" onClick={() => setRemovingID(null)} disabled={pending}>Keep step</button></div></div> : <button type="button" className="goal-link-action" onClick={() => setRemovingID(item.id)} disabled={pending}>Remove</button>}
        </div>}
      </li>)}</ul>
    </div>
    {!archived && !completed && <form className="goal-inline-form goal-add-step" onSubmit={(event) => { event.preventDefault(); let clean: string; try { clean = titleValue(itemDraft); } catch (cause) { setError(errorMessage(cause)); return; } void perform(() => addGoalItem(csrfToken, goal.id, clean), "Step added.", () => setItemDraft("")); }}><label htmlFor={`goal-new-item-${goal.id}`}>Add a step</label><div className="goal-create-row"><input id={`goal-new-item-${goal.id}`} value={itemDraft} maxLength={160} placeholder="One action I can take" onChange={(event) => setItemDraft(event.target.value)} aria-invalid={!!error} disabled={pending} /><button type="submit" className="button button-outline" disabled={pending}>Add step</button></div></form>}
    {!archived && <div className="goal-footer"><div className="goal-button-row">{completed ? <button type="button" className="button button-outline" onClick={() => void perform(() => setGoalCompletion(csrfToken, goal.id, false), "Goal reopened.")} disabled={pending}>Reopen Goal</button> : <button type="button" className="button button-primary" onClick={() => void perform(() => setGoalCompletion(csrfToken, goal.id, true), "Goal completed.")} disabled={pending || goal.total_count === 0 || goal.completed_count !== goal.total_count}>Complete Goal</button>}{!completed && (goal.total_count === 0 || goal.completed_count !== goal.total_count) && <span className="goal-soft-note">Finish every step first.</span>}</div>
      {archiveConfirm ? <div className="goal-remove-confirm"><p>Archive this Goal? Its checklist stays available in Archived Goals.</p><div className="goal-button-row"><button className="button button-danger" type="button" onClick={() => void perform(() => archiveGoal(csrfToken, goal.id), "Goal archived.", () => setArchiveConfirm(false))} disabled={pending}>Confirm archive</button><button className="button button-outline" type="button" onClick={() => setArchiveConfirm(false)} disabled={pending}>Keep Goal</button></div></div> : <button type="button" className="goal-link-action" onClick={() => setArchiveConfirm(true)} disabled={pending}>Archive Goal</button>}</div>}
    {archived && <div className="goal-footer"><p className="goal-soft-note">This Goal and its checklist are preserved.</p><button type="button" className="button button-outline" disabled={pending} onClick={() => void perform(() => restoreGoal(csrfToken, goal.id), "Goal restored.")}>Restore Goal</button></div>}
    {error && <p className="goal-error" role="alert">{error}</p>}
  </article>;
}
