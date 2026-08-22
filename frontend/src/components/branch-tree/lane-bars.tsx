import type { Branch, MergeEdge } from "../../../bindings/kombu";
import { MIN_BAR_LENGTH, laneY } from "./geometry";
import type { TimeScale } from "./time-scale";

/**
 * The lane bars themselves: one horizontal line per branch, spanning only
 * between timestamps actually known for it (its tip, and any merge it took
 * part in) — never drawn back to a fabricated start, since fork points
 * aren't inferred yet (see AGENTS.md). Deliberately no per-commit markers:
 * the tree shows branches and merges, not commits.
 */
export function LaneBars({
  lanes,
  merges,
  scale,
}: {
  lanes: Branch[];
  merges: MergeEdge[];
  scale: TimeScale;
}) {
  return (
    <>
      {lanes.map((branch, index) => {
        const y = laneY(index);
        const touchXs = [scale.toX(branch.committerDate)];
        for (const edge of merges) {
          if (edge.into === branch.name || edge.from === branch.name) {
            touchXs.push(scale.toX(edge.when));
          }
        }

        let startX = Math.min(...touchXs);
        const endX = Math.max(...touchXs);
        if (endX - startX < MIN_BAR_LENGTH) {
          startX = Math.max(0, endX - MIN_BAR_LENGTH);
        }

        const strokeWidth = branch.isDefault ? 4 : branch.mergedToDefault ? 2 : 3;
        const lineClassName = branch.mergedToDefault
          ? "stroke-muted-foreground opacity-70"
          : "stroke-foreground";

        return (
          <line
            key={branch.name}
            x1={startX}
            x2={endX}
            y1={y}
            y2={y}
            strokeWidth={strokeWidth}
            strokeLinecap="round"
            className={lineClassName}
          >
            <title>{`${branch.name} — last commit ${new Date(branch.committerDate).toLocaleString()}`}</title>
          </line>
        );
      })}
    </>
  );
}
