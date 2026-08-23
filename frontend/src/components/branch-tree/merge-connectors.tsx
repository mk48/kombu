import { line, curveBumpX } from "d3-shape";
import type { MergeEdge } from "../../../bindings/kombu";
import { CONNECTOR_PULLBACK, UNKNOWN_SOURCE_STUB_LENGTH, laneY } from "./geometry";
import type { TimeScale } from "./time-scale";

const connectorPath = line<{ x: number; y: number }>()
  .x((d) => d.x)
  .y((d) => d.y)
  .curve(curveBumpX);

/**
 * The merge connectors — the actual payload of the diagram. Each resolved
 * merge draws a curve from the source lane to the target lane, landing
 * exactly at the merge's timestamp with an arrowhead so direction is
 * unambiguous. A merge whose source branch no longer exists on origin
 * (`From: ""`) still needs to show that something landed here, without
 * fabricating a source lane — rendered as a short dashed stub instead.
 */
export function MergeConnectors({
  merges,
  laneIndex,
  scale,
}: {
  merges: MergeEdge[];
  laneIndex: Map<string, number>;
  scale: TimeScale;
}) {
  return (
    <>
      <defs>
        <marker
          id="merge-arrow"
          viewBox="0 0 8 8"
          refX="7"
          refY="4"
          markerWidth="6"
          markerHeight="6"
          // Fixed, not "auto": the connector's source point sits only
          // CONNECTOR_PULLBACK px left of its target, so the curve's true
          // tangent at the endpoint is nearly vertical and unreliable to
          // rotate an arrowhead from. Pointing right always reads correctly
          // here since the target point's x is always >= the source's.
          orient="0"
        >
          <path d="M0,0 L8,4 L0,8 Z" className="fill-chart-2" />
        </marker>
      </defs>
      {merges.map((edge, index) => {
        const intoIndex = laneIndex.get(edge.into);
        // The target branch isn't currently rendered (shouldn't normally
        // happen — every branch GetBranches returns gets a lane).
        if (intoIndex === undefined) return null;

        // An octopus merge (one commit, 3+ parents) produces multiple edges
        // sharing the same commit but different `from` — commit alone isn't
        // a unique key, and two unresolved sources on the same commit would
        // even share `from` (both ""), hence the index as a final tiebreak.
        const key = `${edge.commit}-${edge.from}-${index}`;
        const mergeX = scale.toX(edge.when);
        const intoY = laneY(intoIndex);
        const fromIndex = edge.from ? laneIndex.get(edge.from) : undefined;

        if (fromIndex === undefined) {
          const stubX = Math.max(0, mergeX - UNKNOWN_SOURCE_STUB_LENGTH);
          return (
            <g key={key} className="text-muted-foreground/60">
              <title>{`Merged from a branch no longer on origin — ${new Date(edge.when).toLocaleString()}`}</title>
              <line
                x1={stubX}
                x2={mergeX}
                y1={intoY}
                y2={intoY}
                strokeWidth={1.5}
                strokeDasharray="3 3"
                className="stroke-current"
              />
              <circle
                cx={stubX}
                cy={intoY}
                r={3}
                fill="none"
                strokeWidth={1.5}
                className="stroke-current"
              />
            </g>
          );
        }

        const fromY = laneY(fromIndex);
        const d = connectorPath([
          { x: Math.max(0, mergeX - CONNECTOR_PULLBACK), y: fromY },
          { x: mergeX, y: intoY },
        ]);

        return (
          <path
            key={key}
            d={d ?? undefined}
            fill="none"
            strokeWidth={1.5}
            markerEnd="url(#merge-arrow)"
            className="stroke-chart-2 opacity-60"
          >
            <title>{`${edge.from} → ${edge.into} at ${new Date(edge.when).toLocaleString()}`}</title>
          </path>
        );
      })}
    </>
  );
}
