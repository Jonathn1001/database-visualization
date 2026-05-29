// Animation primitives for the schema graph (DESIGN_PLAN §13.2).
//
// PURE functions — no React state. The cascade / query-path / crud animations
// in the GraphAnimator (GraphCanvas) compose these three primitives:
//   animateParticle(refs, sourceId, targetId, color, ms): Promise<void>
//   pulseNode(refs, nodeId, color, ms): void
//   highlightPath(refs, nodeIds, color): { stop() }
//
// They draw onto the dedicated overlay layer so they never disturb the node /
// link selections the simulation maintains. Animation uses requestAnimationFrame
// (NOT d3-transition, which is not in the dependency set) so we stay within the
// granular d3 packages allowed by §14.2.
import { interpolate } from 'd3-interpolate';
import { select } from 'd3-selection';

import type { SimNode, SimRefs } from './useForceSimulation';

function center(refs: SimRefs, nodeId: string): { x: number; y: number } | null {
  const n = refs.nodeById.get(nodeId);
  if (!n || n.x == null || n.y == null) return null;
  return { x: n.x, y: n.y };
}

function radiusOf(refs: SimRefs, nodeId: string): number {
  return refs.nodeById.get(nodeId)?.radius ?? 12;
}

const easeOut = (t: number): number => t * (2 - t);

/**
 * runFrames drives an rAF loop from 0..1 over durationMs, calling onFrame each
 * tick and onDone (with done=true if it ran to completion) at the end. Returns
 * a cancel() that stops the loop and reports done=false.
 */
function runFrames(
  durationMs: number,
  onFrame: (t: number) => void,
  onDone: (completed: boolean) => void,
): () => void {
  const start =
    typeof performance !== 'undefined' ? performance.now() : Date.now();
  let raf = 0;
  let cancelled = false;

  const step = () => {
    if (cancelled) return;
    const now =
      typeof performance !== 'undefined' ? performance.now() : Date.now();
    const t = durationMs <= 0 ? 1 : Math.min(1, (now - start) / durationMs);
    onFrame(t);
    if (t >= 1) {
      onDone(true);
      return;
    }
    raf = requestAnimationFrame(step);
  };
  raf = requestAnimationFrame(step);

  return () => {
    if (cancelled) return;
    cancelled = true;
    cancelAnimationFrame(raf);
    onDone(false);
  };
}

/**
 * animateParticle sends a small dot from the source node to the target node
 * along a straight interpolated path. Resolves when the travel finishes (the
 * dot removes itself). In perf mode the caller may choose to skip unrelated
 * particles (§16.3) — this primitive always runs when invoked.
 */
export function animateParticle(
  refs: SimRefs,
  sourceId: string,
  targetId: string,
  color: string,
  durationMs: number,
): Promise<void> {
  const from = center(refs, sourceId);
  const to = center(refs, targetId);
  if (!from || !to) return Promise.resolve();

  const interp = interpolate(from, to);
  const dot = refs.overlayLayer
    .append('circle')
    .attr('class', 'dbviz-particle')
    .attr('r', 3.2)
    .attr('fill', color)
    .attr('opacity', 0.9)
    .attr('cx', from.x)
    .attr('cy', from.y)
    .style('pointer-events', 'none');

  return new Promise<void>((resolve) => {
    runFrames(
      durationMs,
      (t) => {
        const p = interp(t);
        dot.attr('cx', p.x).attr('cy', p.y);
      },
      () => {
        dot.remove();
        resolve();
      },
    );
  });
}

/**
 * pulseNode briefly expands + fades a ring around a node to flag it as touched
 * (insert/update/delete/check or a standalone pulse). Fire-and-forget; the ring
 * removes itself when the animation completes.
 */
export function pulseNode(
  refs: SimRefs,
  nodeId: string,
  color: string,
  durationMs: number,
): void {
  const c = center(refs, nodeId);
  if (!c) return;
  const r0 = radiusOf(refs, nodeId);

  const ring = refs.overlayLayer
    .append('circle')
    .attr('class', 'dbviz-pulse')
    .attr('cx', c.x)
    .attr('cy', c.y)
    .attr('r', r0)
    .attr('fill', 'none')
    .attr('stroke', color)
    .attr('stroke-width', 2)
    .attr('opacity', 0.75)
    .style('pointer-events', 'none');

  runFrames(
    durationMs,
    (t) => {
      const e = easeOut(t);
      ring
        .attr('r', r0 + 18 * e)
        .attr('stroke-width', 2 - 1.5 * e)
        .attr('opacity', 0.75 * (1 - e));
    },
    () => ring.remove(),
  );
}

/**
 * highlightPath draws a persistent halo around each node in the path and a
 * connecting overlay line between consecutive nodes, then returns a stop()
 * handle that removes the overlay. Used to keep a cascade / query path visible
 * while the step-through animation runs.
 */
export function highlightPath(
  refs: SimRefs,
  nodeIds: string[],
  color: string,
): { stop: () => void } {
  const group = refs.overlayLayer.append('g').attr('class', 'dbviz-path-highlight');

  // Connecting segments between consecutive resolvable nodes.
  for (let i = 0; i < nodeIds.length - 1; i += 1) {
    const a = center(refs, nodeIds[i]);
    const b = center(refs, nodeIds[i + 1]);
    if (!a || !b) continue;
    group
      .append('line')
      .attr('x1', a.x)
      .attr('y1', a.y)
      .attr('x2', b.x)
      .attr('y2', b.y)
      .attr('stroke', color)
      .attr('stroke-width', 2)
      .attr('stroke-opacity', 0.5)
      .attr('stroke-linecap', 'round')
      .style('pointer-events', 'none');
  }

  // Halo rings around each node.
  for (const id of nodeIds) {
    const c = center(refs, id);
    if (!c) continue;
    group
      .append('circle')
      .attr('cx', c.x)
      .attr('cy', c.y)
      .attr('r', radiusOf(refs, id) + 4)
      .attr('fill', 'none')
      .attr('stroke', color)
      .attr('stroke-width', 2)
      .attr('stroke-opacity', 0.7)
      .style('pointer-events', 'none');
  }

  return {
    stop: () => {
      group.remove();
    },
  };
}

/** Removes every transient overlay (particles, pulses, highlights). */
export function clearOverlay(refs: SimRefs): void {
  refs.overlayLayer.selectAll('*').remove();
}

// Re-export d3 select + the SimNode type for downstream callers that compose
// these primitives without re-importing d3 directly.
export { select };
export type { SimNode };
