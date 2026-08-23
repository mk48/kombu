import type { Branch, ForkEdge, MergeEdge } from "../../../bindings/kombu";
import { MIN_BAR_LENGTH, laneY } from "./geometry";
import type { TimeScale } from "./time-scale";

/**
 * The lane bars themselves: one horizontal line per branch, spanning only
 * between timestamps actually known for it (its tip, any merge it took part
 * in, and its inferred fork point when one was found) — never drawn back to
 * a fabricated start when none of those are available. Deliberately no
 * per-commit markers: the tree shows branches and merges, not commits.
 */
export function LaneBars({
  lanes,
  merges,
  forks,
  scale,
}: {
  lanes: Branch[];
  merges: MergeEdge[];
  forks: ForkEdge[];
  scale: TimeScale;
}) {
  const forkByBranch = new Map(forks.map((fork) => [fork.branch, fork]));

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
        const fork = forkByBranch.get(branch.name);
        if (fork) touchXs.push(scale.toX(fork.at));

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
