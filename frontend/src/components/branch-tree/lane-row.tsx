import { GripVertical } from "lucide-react";
import { useSortable } from "@dnd-kit/sortable";
import { CSS } from "@dnd-kit/utilities";
import type { Branch } from "../../../bindings/kombu";
import { LANE_HEIGHT } from "./geometry";
import { cn } from "@/lib/utils";

/**
 * One row of the label gutter: drag handle, branch name, and its status
 * badges. Purely HTML/CSS — the SVG plot only reads lane order from this
 * list, it never renders its own drag affordance.
 */
export function LaneRow({ branch }: { branch: Branch }) {
  const { attributes, listeners, setNodeRef, transform, transition, isDragging } =
    useSortable({ id: branch.name });

  return (
    <div
      ref={setNodeRef}
      style={{
        height: LANE_HEIGHT,
        transform: CSS.Transform.toString(transform),
        transition,
      }}
      className={cn(
        "flex items-center gap-1.5 border-b border-border/60 px-2 text-sm",
        isDragging && "relative z-10 bg-background shadow-sm",
      )}
    >
      <button
        type="button"
        aria-label={`Reorder ${branch.name}`}
        className="grid size-5 shrink-0 cursor-grab touch-none place-items-center rounded text-muted-foreground/60 outline-none hover:text-muted-foreground focus-visible:ring-2 focus-visible:ring-ring/50 active:cursor-grabbing"
        {...attributes}
        {...listeners}
      >
        <GripVertical className="size-3.5" aria-hidden="true" />
      </button>
      <span
        className={cn(
          "truncate",
          branch.mergedToDefault ? "text-muted-foreground" : "text-foreground",
        )}
        title={branch.name}
      >
        {branch.name}
      </span>
      {branch.isDefault && (
        <span className="shrink-0 rounded-full bg-foreground/10 px-1.5 py-0.5 text-[10px] text-foreground">
          default
        </span>
      )}
      {branch.isCurrent && (
        <span className="shrink-0 rounded-full bg-foreground/10 px-1.5 py-0.5 text-[10px] text-foreground">
          current
        </span>
      )}
    </div>
  );
}
