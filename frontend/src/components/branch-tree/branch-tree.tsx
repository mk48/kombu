import { useMemo } from "react";
import {
  DndContext,
  KeyboardSensor,
  PointerSensor,
  useSensor,
  useSensors,
  type DragEndEvent,
} from "@dnd-kit/core";
import {
  SortableContext,
  arrayMove,
  sortableKeyboardCoordinates,
  verticalListSortingStrategy,
} from "@dnd-kit/sortable";
import type { Branch, ForkEdge, MergeEdge } from "../../../bindings/kombu";
import { LaneRow } from "./lane-row";
import { TimeRuler } from "./time-ruler";
import { LaneBars } from "./lane-bars";
import { MergeConnectors } from "./merge-connectors";
import { ForkConnectors } from "./fork-connectors";
import { createTimeScale } from "./time-scale";
import {
  LABEL_GUTTER_WIDTH,
  LANE_HEIGHT,
  PLOT_WIDTH,
  RULER_HEIGHT,
} from "./geometry";

/**
 * The horizontal branch lane tree: one fixed lane per branch, reorderable by
 * dragging its label, with merge connectors drawn between lanes. See
 * AGENTS.md for the rules this view exists to uphold — one lane per branch
 * for its whole life, X = time, Y = order only.
 *
 * Geometry: a single shared SVG holds every lane's bar and every merge
 * connector, addressed by one `laneY(index)` scheme, inside the same
 * scrolling container as the HTML label gutter — so a connector between two
 * far-apart lanes, one of them off-screen, is correct by construction, no
 * manual scroll sync needed. Dragging only re-renders the HTML gutter until
 * drop; the SVG plot catches up once the reorder is confirmed.
 */
export function BranchTree({
  lanes,
  merges,
  forks,
  onReorder,
}: {
  lanes: Branch[];
  merges: MergeEdge[];
  forks: ForkEdge[];
  onReorder: (order: string[]) => void;
}) {
  const laneIndex = useMemo(
    () => new Map(lanes.map((branch, index) => [branch.name, index])),
    [lanes],
  );

  const scale = useMemo(() => {
    const dates = lanes.map((branch) => new Date(branch.committerDate));
    for (const edge of merges) dates.push(new Date(edge.when));
    for (const fork of forks) dates.push(new Date(fork.at));
    return createTimeScale(dates, PLOT_WIDTH);
  }, [lanes, merges, forks]);

  const sensors = useSensors(
    useSensor(PointerSensor, { activationConstraint: { distance: 4 } }),
    useSensor(KeyboardSensor, {
      coordinateGetter: sortableKeyboardCoordinates,
    }),
  );

  function handleDragEnd(event: DragEndEvent) {
    const { active, over } = event;
    if (!over || active.id === over.id) return;
    const oldIndex = lanes.findIndex((branch) => branch.name === active.id);
    const newIndex = lanes.findIndex((branch) => branch.name === over.id);
    if (oldIndex === -1 || newIndex === -1) return;
    onReorder(arrayMove(lanes, oldIndex, newIndex).map((branch) => branch.name));
  }

  if (lanes.length === 0) return null;

  const plotHeight = lanes.length * LANE_HEIGHT;

  return (
    <div className="min-h-0 flex-1 overflow-auto">
      <div
        className="grid"
        style={{ gridTemplateColumns: `${LABEL_GUTTER_WIDTH}px 1fr` }}
      >
        <div
          className="sticky top-0 left-0 z-20 border-b border-border bg-background"
          style={{ height: RULER_HEIGHT }}
        />
        <div
          className="sticky top-0 z-10 border-b border-border bg-background"
          style={{ height: RULER_HEIGHT }}
        >
          <TimeRuler scale={scale} />
        </div>

        <div className="sticky left-0 z-10 bg-background">
          <DndContext sensors={sensors} onDragEnd={handleDragEnd}>
            <SortableContext
              items={lanes.map((branch) => branch.name)}
              strategy={verticalListSortingStrategy}
            >
              {lanes.map((branch) => (
                <LaneRow key={branch.name} branch={branch} />
              ))}
            </SortableContext>
          </DndContext>
        </div>

        <div style={{ width: scale.width, height: plotHeight }}>
          <svg width={scale.width} height={plotHeight} className="block">
            <LaneBars lanes={lanes} merges={merges} forks={forks} scale={scale} />
            <ForkConnectors forks={forks} laneIndex={laneIndex} scale={scale} />
            <MergeConnectors merges={merges} laneIndex={laneIndex} scale={scale} />
          </svg>
        </div>
      </div>
    </div>
  );
}
