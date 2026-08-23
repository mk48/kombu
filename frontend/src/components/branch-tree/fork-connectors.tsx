import { line, curveBumpX } from "d3-shape";
import type { ForkEdge } from "../../../bindings/kombu";
import { FORK_CONNECTOR_PULLBACK, laneY } from "./geometry";
import type { TimeScale } from "./time-scale";

const connectorPath = line<{ x: number; y: number }>()
  .x((d) => d.x)
  .y((d) => d.y)
  .curve(curveBumpX);

/**
 * Fork connectors: "this branch was cut from that one." Git records no
 * parent-branch pointer, so every edge here is a best guess (see
 * inferForkEdges in git.go) — dashed, in a lighter weight than a merge
 * connector's solid line, and with a hollow (not filled) arrowhead, so a
 * fork never reads as visually equivalent to an observed merge commit. A
 * branch with no confident candidate parent simply has no edge; nothing is
 * drawn rather than guessing.
 */
export function ForkConnectors({
  forks,
  laneIndex,
  scale,
}: {
  forks: ForkEdge[];
  laneIndex: Map<string, number>;
  scale: TimeScale;
}) {
  return (
    <>
      <defs>
        <marker
          id="fork-arrow"
          viewBox="0 0 8 8"
          refX="7"
          refY="4"
          markerWidth="6"
          markerHeight="6"
          // Fixed for the same reason as the merge arrow: the source point
          // sits only FORK_CONNECTOR_PULLBACK px left of the target, so the
          // curve's true end tangent is nearly vertical and not a reliable
          // thing to rotate an arrowhead from.
          orient="0"
        >
          <path
            d="M0.5,1 L7,4 L0.5,7"
            fill="none"
            strokeWidth={1.2}
            className="stroke-muted-foreground"
          />
        </marker>
      </defs>
      {forks.map((fork) => {
        const childIndex = laneIndex.get(fork.branch);
        const parentIndex = laneIndex.get(fork.from);
        if (childIndex === undefined || parentIndex === undefined) return null;

        const forkX = scale.toX(fork.at);
        const parentY = laneY(parentIndex);
        const childY = laneY(childIndex);
        const d = connectorPath([
          { x: Math.max(0, forkX - FORK_CONNECTOR_PULLBACK), y: parentY },
          { x: forkX, y: childY },
        ]);

        return (
          <path
            key={fork.branch}
            d={d ?? undefined}
            fill="none"
            strokeWidth={1.25}
            // strokeDasharray="4 3"
            markerEnd="url(#triangle)"
            className="stroke-muted-foreground/70"
          >
            <title>{`${fork.branch} branched from ${fork.from} (inferred) around ${new Date(fork.at).toLocaleString()}`}</title>
          </path>
        );
      })}
    </>
  );
}
