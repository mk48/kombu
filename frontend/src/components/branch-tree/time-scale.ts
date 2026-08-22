import { scaleTime } from "d3-scale";

const DAY_MS = 24 * 60 * 60 * 1000;

export type TimeScale = {
  toX: (date: Date | string) => number;
  /** "Nice" tick dates for the ruler, already within the padded domain. */
  ticks: Date[];
  width: number;
};

/**
 * Builds a shared calendar-time scale across the whole diagram from every
 * timestamp actually known (branch tips + merge events) — literal time, not
 * ordinal sequence, so the ruler reads as "how long ago" at a glance. A
 * single-instant or empty input still produces a usable, padded domain
 * rather than a zero-width scale.
 */
export function createTimeScale(dates: Date[], width: number): TimeScale {
  const valid = dates.filter((d) => !Number.isNaN(d.getTime()));

  let min = valid.length
    ? new Date(Math.min(...valid.map((d) => d.getTime())))
    : new Date(Date.now() - DAY_MS);
  let max = valid.length
    ? new Date(Math.max(...valid.map((d) => d.getTime())))
    : new Date();

  if (min.getTime() === max.getTime()) {
    min = new Date(min.getTime() - DAY_MS);
    max = new Date(max.getTime() + DAY_MS);
  }

  const pad = (max.getTime() - min.getTime()) * 0.08;
  const scale = scaleTime()
    .domain([new Date(min.getTime() - pad), new Date(max.getTime() + pad)])
    .range([0, width]);

  return {
    toX: (date) => scale(typeof date === "string" ? new Date(date) : date),
    ticks: scale.ticks(6),
    width,
  };
}
