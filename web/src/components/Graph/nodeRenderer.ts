// Node rendering for the schema graph (DESIGN_PLAN §13.4).
//
// Nodes are soft circles: muted earthy fills, subtle 1px translucent borders,
// no neon / glow / gradient. Radius scales with row count (primary) nudged by
// column count (secondary). PK / FK / Index presence is shown with thin rings
// + small badges so the shape stays readable at a glance. Labels use Inter,
// 11-13px, clamped onto the graph.
import { select, type Selection } from 'd3-selection';

import type { Node } from '@/types/graph';
import { FORCE } from '@/lib/constants';
import { nodeFill, palette, severityColor } from '@/lib/colors';
import type { SimNode, SimRefs } from './useForceSimulation';

const { nodeRadiusMin, nodeRadiusMax } = FORCE;

// Ring accents (still muted — no neon).
const RING_PK = palette.accentUpdate; // gold-ish: primary key present
const RING_FK = palette.accentQuery; // blue: foreign key present
const BADGE_INDEX = palette.nodeIndexPattern; // mauve dot: has secondary indexes

/**
 * nodeRadius scales a node's circle from row count (log-compressed so a 10M-row
 * table is not 1000x a 10-row one) with a small bump for wide tables. Pure +
 * deterministic so the collide force and the renderer agree.
 */
export function nodeRadius(n: Node): number {
  const rows = Math.max(0, n.rowCount ?? 0);
  // log scale: 0 rows -> 0, ~1k -> ~0.5, ~1M -> ~1.0 of the row component.
  const rowScale = rows > 0 ? Math.log10(rows + 1) / 6 : 0;
  const colCount = n.columns?.length ?? 0;
  const colScale = Math.min(colCount / 40, 1); // wide tables nudge size up
  const t = Math.min(1, rowScale * 0.8 + colScale * 0.2);
  return nodeRadiusMin + t * (nodeRadiusMax - nodeRadiusMin);
}

function hasPk(n: Node): boolean {
  return (n.columns ?? []).some((c) => c.isPk);
}
function hasFk(n: Node): boolean {
  return (n.columns ?? []).some((c) => c.isFk);
}
function hasIndex(n: Node): boolean {
  return (n.columns ?? []).some((c) => (c.indexes?.length ?? 0) > 0);
}

/** Truncates a label so it stays compact under the circle. */
function clampLabel(label: string, max = 22): string {
  if (label.length <= max) return label;
  return `${label.slice(0, max - 1)}…`;
}

function borderColor(dark: boolean): string {
  return dark ? palette.nodeBorderDark : palette.nodeBorder;
}
function labelColor(dark: boolean): string {
  return dark ? 'rgba(232,232,232,0.92)' : 'rgba(26,26,26,0.88)';
}

/**
 * renderNodes binds the current SimNodes to <g class="dbviz-node"> groups and
 * (re)draws their inner shapes via a general-update join. Each group holds:
 *   - circle.dbviz-node-fill   (soft body)
 *   - circle.dbviz-node-pk      (gold ring, if PK)
 *   - circle.dbviz-node-fk      (blue ring, if FK)
 *   - circle.dbviz-node-idx     (mauve badge dot, if indexed)
 *   - text.dbviz-node-label     (Inter label below the circle)
 * The enter/update split keeps the join idempotent across soft updates.
 */
export function renderNodes(refs: SimRefs): void {
  const data = refs.simulation.nodes();
  const dark = refs.dark;

  const groups = refs.nodeLayer
    .selectAll<SVGGElement, SimNode>('g.dbviz-node')
    .data(data, (d) => d.id);

  groups.exit().remove();

  const enter = groups
    .enter()
    .append('g')
    .attr('class', 'dbviz-node')
    .attr('data-node-id', (d) => d.id)
    .style('cursor', 'pointer');

  // Soft body circle.
  enter
    .append('circle')
    .attr('class', 'dbviz-node-fill')
    .attr('stroke-width', 1);

  // PK ring (outermost thin ring).
  enter
    .append('circle')
    .attr('class', 'dbviz-node-pk')
    .attr('fill', 'none')
    .attr('stroke', RING_PK)
    .attr('stroke-width', 1.5)
    .attr('opacity', 0.85);

  // FK ring (dashed, sits just inside the PK ring).
  enter
    .append('circle')
    .attr('class', 'dbviz-node-fk')
    .attr('fill', 'none')
    .attr('stroke', RING_FK)
    .attr('stroke-width', 1.25)
    .attr('stroke-dasharray', '3 2')
    .attr('opacity', 0.8);

  // Index badge dot (top-right).
  enter
    .append('circle')
    .attr('class', 'dbviz-node-idx')
    .attr('fill', BADGE_INDEX)
    .attr('stroke', 'none')
    .attr('opacity', 0.85);

  // Label below the circle.
  enter
    .append('text')
    .attr('class', 'dbviz-node-label')
    .attr('text-anchor', 'middle')
    .attr('font-family', 'Inter, system-ui, sans-serif')
    .attr('font-weight', 500)
    .style('pointer-events', 'none')
    .style('user-select', 'none');

  const merged = enter.merge(groups);

  // --- Update every join cycle so soft updates reflect changed metadata. ---
  merged
    .select<SVGCircleElement>('circle.dbviz-node-fill')
    .attr('r', (d) => d.radius)
    .attr('fill', (d) => nodeFill(d.kind))
    .attr('stroke', borderColor(dark))
    .attr('fill-opacity', 0.9);

  merged
    .select<SVGCircleElement>('circle.dbviz-node-pk')
    .attr('r', (d) => d.radius + 3)
    .attr('display', (d) => (hasPk(d) ? null : 'none'));

  merged
    .select<SVGCircleElement>('circle.dbviz-node-fk')
    .attr('r', (d) => d.radius + 6)
    .attr('display', (d) => (hasFk(d) ? null : 'none'));

  merged
    .select<SVGCircleElement>('circle.dbviz-node-idx')
    .attr('r', 2.6)
    .attr('cx', (d) => d.radius * 0.7)
    .attr('cy', (d) => -d.radius * 0.7)
    .attr('display', (d) => (hasIndex(d) ? null : 'none'));

  merged
    .select<SVGTextElement>('text.dbviz-node-label')
    .attr('y', (d) => d.radius + 14)
    .attr('font-size', (d) => labelFontSize(d))
    .attr('fill', labelColor(dark))
    .text((d) => clampLabel(d.label));
}

/** Label size stays within the 11-13px graph range, larger circles get 13px. */
function labelFontSize(d: SimNode): number {
  if (d.radius >= 24) return 13;
  if (d.radius >= 16) return 12;
  return 11;
}

/**
 * setNodeOpacity fades a node-group selection's opacity using a CSS transition
 * (we avoid d3-transition, which is not in the dependency set). Setting the
 * `transition` style lets the browser animate the `opacity` change for free.
 */
export function setNodeOpacity(
  group: Selection<SVGGElement, SimNode, SVGGElement, unknown>,
  opacity: number,
  durationMs = 0,
): void {
  group.style('transition', `opacity ${Math.max(0, durationMs)}ms ease`);
  group.style('opacity', opacity);
}

/**
 * renderSeverityBadges draws (or removes) a small severity dot at the
 * top-left of each node circle — the insights overlay. Badges live inside
 * the node <g>, so they track the node on every tick for free. Idempotent:
 * calling with an empty map removes all badges.
 */
export function renderSeverityBadges(
  refs: SimRefs,
  severityByNode: Map<string, 'info' | 'warn' | 'critical'>,
): void {
  refs.nodeLayer
    .selectAll<SVGGElement, SimNode>('g.dbviz-node')
    .each(function (d) {
      const group = select(this);
      const severity = severityByNode.get(d.id);
      const existing = group.select<SVGCircleElement>('circle.dbviz-node-sev');
      if (!severity) {
        existing.remove();
        return;
      }
      const badge = existing.empty()
        ? group.append('circle').attr('class', 'dbviz-node-sev')
        : existing;
      badge
        .attr('r', 3.4)
        .attr('cx', (-d.radius) * 0.7)
        .attr('cy', (-d.radius) * 0.7)
        .attr('fill', severityColor(severity))
        .attr('stroke', 'none')
        .attr('opacity', 0.95);
    });
}

// Re-exported so other modules can grab the raw d3 select if needed without a
// second import line.
export { select };
