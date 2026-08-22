import { RULER_HEIGHT } from "./geometry";
import type { TimeScale } from "./time-scale";

const formatTick = new Intl.DateTimeFormat(undefined, {
  month: "short",
  day: "numeric",
});

/** The sticky time axis above the plot: tick marks at "nice" calendar points across the shared time scale. */
export function TimeRuler({ scale }: { scale: TimeScale }) {
  return (
    <svg
      width={scale.width}
      height={RULER_HEIGHT}
      className="block"
      role="presentation"
    >
      {scale.ticks.map((tick) => {
        const x = scale.toX(tick);
        return (
          <g key={tick.toISOString()}>
            <line
              x1={x}
              x2={x}
              y1={RULER_HEIGHT - 6}
              y2={RULER_HEIGHT}
              strokeWidth={1}
              className="stroke-border"
            />
            <text
              x={x}
              y={RULER_HEIGHT - 12}
              textAnchor="middle"
              className="fill-muted-foreground text-[10px]"
            >
              {formatTick.format(tick)}
            </text>
          </g>
        );
      })}
    </svg>
  );
}
