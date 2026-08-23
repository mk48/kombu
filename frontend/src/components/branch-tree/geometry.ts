/** Pixel height of a single lane row — shared by the HTML label gutter and the SVG plot so they stay aligned. */
export const LANE_HEIGHT = 32;

/** Fixed width of the label column (branch name + badges + drag handle). */
export const LABEL_GUTTER_WIDTH = 220;

/** Height of the sticky time-ruler header above the plot. */
export const RULER_HEIGHT = 28;

/** Width of the plot area's time scale. The scroll container handles overflow, so this only needs to be "wide enough to read" rather than tied to the window. */
export const PLOT_WIDTH = 960;

/** Minimum on-screen length of a branch bar, so a branch with only one known timestamp (no merge activity yet) still reads as a lane rather than a dot. */
export const MIN_BAR_LENGTH = 40;

/** Horizontal pull-back of a merge connector's source point from the merge's exact time, so the curve has room to bend before landing on the target lane. */
export const CONNECTOR_PULLBACK = 28;

/** Length of the dashed stub drawn for a merge whose source branch no longer exists. */
export const UNKNOWN_SOURCE_STUB_LENGTH = 36;

/** Horizontal pull-back of a fork connector's parent-side point from the fork point, mirroring CONNECTOR_PULLBACK but tunable independently. */
export const FORK_CONNECTOR_PULLBACK = 28;

/** Vertical center of lane index `i`, in the plot's local coordinate space. */
export function laneY(index: number): number {
  return index * LANE_HEIGHT + LANE_HEIGHT / 2;
}
