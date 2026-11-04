import { GoalApiError, type Goal } from "./api/goals";

export type GoalDeleteAttempt = {
  targetGoalID: string;
  title: string;
  phase: "confirm" | "sending" | "checking" | "error" | "uncertain" | "done" | "unavailable";
  uncertain: boolean;
  error: string;
};

export function beginGoalDelete(goal: Pick<Goal, "id" | "title">): GoalDeleteAttempt {
  return { targetGoalID: goal.id, title: goal.title, phase: "confirm", uncertain: false, error: "" };
}

export function goalDeleteFailure(attempt: GoalDeleteAttempt, error: unknown): GoalDeleteAttempt {
  const uncertain = attempt.uncertain || !(error instanceof GoalApiError) || error.status === 0 || error.status >= 500 || (error.status >= 200 && error.status < 300);
  return { ...attempt, phase: uncertain ? "uncertain" : "error", uncertain, error: error instanceof Error ? error.message : "The delete could not be checked. Retry the same Goal." };
}

export function goalDeleteChecked(attempt: GoalDeleteAttempt): GoalDeleteAttempt {
  // A GET snapshot cannot prove that an earlier timed-out Delete won't commit.
  return { ...attempt, phase: "uncertain", uncertain: true, error: "This Goal is still visible. Retry deletion to settle the request safely." };
}
