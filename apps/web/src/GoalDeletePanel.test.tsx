import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it, vi } from "vitest";
import { GoalApiError, type Goal } from "./api/goals";
import { GoalDeletePanel } from "./GoalDeletePanel";
import { GoalCard } from "./GoalsPage";
import { beginGoalDelete, goalDeleteChecked, goalDeleteFailure } from "./goalDeleteState";

const goal: Goal = {id:"goal-a",title:"A very long Goal title ".repeat(5),created_at:"2026-11-04T00:00:00Z",updated_at:"2026-11-04T00:00:00Z",completed_count:0,total_count:1,items:[{id:"step",title:"Retained step",created_at:"2026-11-04T00:00:00Z",updated_at:"2026-11-04T00:00:00Z"}]};
const panel=(attempt=beginGoalDelete(goal))=>renderToStaticMarkup(<GoalDeletePanel attempt={attempt} onCancel={()=>undefined} onDelete={()=>undefined} onCheck={()=>undefined}/>);
describe("Goal Delete confirmation and recovery",()=>{
  it.each(["active","completed","archived"])("keeps Delete secondary and available for %s Goals",state=>{
    const record={...goal,...(state==="completed"?{completed_at:goal.created_at}:{}),...(state==="archived"?{archived_at:goal.created_at}:{})};
    const html=renderToStaticMarkup(<GoalCard goal={record} csrfToken="test" pending={false} mutate={async()=>undefined} onDelete={()=>undefined}/>);
    expect(html).toContain('class="goal-link-action goal-delete-action"');expect(html).toContain(">Delete Goal</button>");expect(html).toContain("Retained step");expect(html).not.toContain("Trash");
  });
  it("labels confirmation, keeps a cancel target and explains retained internal data",()=>{
    const html=panel();expect(html).toContain('aria-labelledby="goal-delete-title"');expect(html).toContain('id="goal-delete-keep"');expect(html).toContain("Keep Goal");expect(html).toContain("backups or earlier exports");expect(html).not.toContain('role="dialog"');
  });
  it("retains the exact target after lost response, protection failures and a still-visible GET",()=>{
    const start=beginGoalDelete(goal);const uncertain=goalDeleteFailure(start,new GoalApiError(0,"network_error"));
    expect(uncertain).toMatchObject({targetGoalID:"goal-a",phase:"uncertain",uncertain:true});
    expect(goalDeleteFailure(uncertain,new GoalApiError(403,"invalid_csrf"))).toMatchObject({targetGoalID:"goal-a",phase:"uncertain"});
    expect(goalDeleteChecked(uncertain)).toMatchObject({targetGoalID:"goal-a",phase:"uncertain"});
    const html=panel(uncertain);expect(html).toContain("Retry deletion");expect(html).toContain("Check Goal");expect(html).not.toContain("Keep Goal");expect(html).toContain("does not undo a delete already sent");
    expect(goalDeleteFailure(start,new GoalApiError(403,"invalid_csrf")).phase).toBe("error");
    expect(goalDeleteFailure(start,new GoalApiError(200,"internal_error")).phase).toBe("uncertain");
  });
  it("announces confirmed success separately from generic unavailability",()=>{
    expect(panel({...beginGoalDelete(goal),phase:"done"})).toContain("Goal deleted.");
    const html=panel({...beginGoalDelete(goal),phase:"unavailable"});expect(html).toContain("no longer available");expect(html).not.toContain("Goal deleted.");
  });
  it("only lets Escape cancel before sending or an uncertain outcome",()=>{
    const cancel=vi.fn();
    for(const phase of ["confirm","error","sending","checking","uncertain"] as const){
      const attempt={...beginGoalDelete(goal),phase,uncertain:phase==="uncertain"};
      const element=GoalDeletePanel({attempt,onCancel:cancel,onDelete:()=>undefined,onCheck:()=>undefined});
      const preventDefault=vi.fn();
      element.props.onKeyDown({key:"Escape",preventDefault});
      expect(preventDefault).toHaveBeenCalledTimes(phase==="confirm"||phase==="error"?1:0);
    }
    expect(cancel).toHaveBeenCalledTimes(2);
    const html=panel(goalDeleteFailure(beginGoalDelete(goal),new GoalApiError(0,"network_error")));
    expect(html).toContain('id="goal-delete-error"');expect(html).toContain('tabindex="-1"');
  });
});
