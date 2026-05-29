// The D3 force-directed schema graph (DESIGN_PLAN §13.1).
//
// GraphCanvas owns the <svg> and does NOT re-render on D3 tick — all node
// positions live in D3's internal state and are written straight to the DOM by
// useForceSimulation. React only runs for the two structural effects below:
//   - build the simulation once per [graph.engine, graph.database]
//   - soft-update it on [nodes.length, links.length]
//
// On mount it builds a GraphAnimator from the particle primitives + SimRefs and
// registers it via useGraphStore().setAnimator(handle); it clears it on unmount.
//
// Hover fades unrelated nodes/edges to 0.15 within 100ms and writes the hovered
// id to the selection store; click selects. Drag + zoom/pan come from the sim.
import { useEffect, useRef } from 'react';

import type {
  CascadeResult,
  CrudFlowResult,
  GraphAnimator,
  GraphModel,
  QueryPlan,
} from '@/types/graph';
import { useGraphStore } from '@/store/graph';
import { useSelectionStore } from '@/store/selection';
import { palette, crudColor, HOVER_FADE_OPACITY } from '@/lib/colors';

import {
  buildSimulation,
  updateGraphData,
  type SimLink,
  type SimNode,
  type SimRefs,
} from './useForceSimulation';
import { animateParticle, pulseNode, highlightPath, clearOverlay } from './particles';

const HOVER_FADE_MS = 100;
const PARTICLE_MS = 600;
const PULSE_MS = 500;

export function GraphCanvas({ graph }: { graph: GraphModel }) {
  const svgRef = useRef<SVGSVGElement>(null);
  const simRef = useRef<SimRefs | null>(null);
  const animatorRef = useRef<GraphAnimator | null>(null);
  // Holds the stop() handle of the currently displayed cascade/query highlight.
  const activeHighlight = useRef<{ stop: () => void } | null>(null);

  // --- Build once per database (§13.1). ---
  useEffect(() => {
    const svg = svgRef.current;
    if (!svg) return;

    const refs = buildSimulation(svg, graph);
    simRef.current = refs;

    wireInteractions(refs);

    const animator = makeAnimator(refs, activeHighlight);
    animatorRef.current = animator;
    useGraphStore.getState().setAnimator(animator);

    return () => {
      activeHighlight.current?.stop();
      activeHighlight.current = null;
      // Only clear the store if it still points at our handle.
      if (useGraphStore.getState().animator === animatorRef.current) {
        useGraphStore.getState().setAnimator(null);
      }
      animatorRef.current = null;
      refs.stop();
      simRef.current = null;
    };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [graph.engine, graph.database]);

  // --- Soft update on schema refresh (§13.1). ---
  useEffect(() => {
    const refs = simRef.current;
    if (!refs) return;
    updateGraphData(refs, graph);
    wireInteractions(refs);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [graph.nodes.length, graph.links.length]);

  // --- Hover fade driven by the selection store, imperatively (no re-render). ---
  useEffect(() => {
    // Apply current hover immediately, then subscribe to future changes.
    applyHoverFade(simRef.current, useSelectionStore.getState().hoverNodeId);
    const unsub = useSelectionStore.subscribe((state, prev) => {
      if (state.hoverNodeId !== prev.hoverNodeId) {
        applyHoverFade(simRef.current, state.hoverNodeId);
      }
    });
    return unsub;
  }, []);

  return (
    <svg
      ref={svgRef}
      className="h-full w-full"
      role="img"
      aria-label="Database schema graph"
    />
  );
}

/**
 * wireInteractions (re)binds hover + click handlers onto the node groups. Called
 * after every (re)paint so freshly-entered nodes get handlers. Hover writes the
 * selection store's hoverNodeId (which drives the fade via the subscription);
 * click writes selectedNodeId.
 */
function wireInteractions(refs: SimRefs): void {
  const { setHover, select } = useSelectionStore.getState();
  refs.nodeLayer
    .selectAll<SVGGElement, SimNode>('g.dbviz-node')
    .on('mouseenter', (_event, d) => setHover(d.id))
    .on('mouseleave', () => setHover(null))
    .on('click', (event, d) => {
      event.stopPropagation();
      select(d.id);
    });
}

/** Returns the set of node ids "related" to the hovered node (it + neighbours). */
function relatedSet(refs: SimRefs, hoverId: string): Set<string> {
  const set = new Set<string>([hoverId]);
  const neigh = refs.neighbors.get(hoverId);
  if (neigh) for (const n of neigh) set.add(n);
  return set;
}

/**
 * applyHoverFade fades unrelated nodes + links to HOVER_FADE_OPACITY (0.15)
 * within HOVER_FADE_MS, or restores full opacity when nothing is hovered. Uses a
 * CSS opacity transition (we avoid d3-transition, which is not a dependency).
 */
function applyHoverFade(refs: SimRefs | null, hoverId: string | null): void {
  if (!refs) return;
  const fade = `opacity ${HOVER_FADE_MS}ms ease`;
  const nodes = refs.nodeLayer
    .selectAll<SVGGElement, SimNode>('g.dbviz-node')
    .style('transition', fade);
  const links = refs.linkLayer
    .selectAll<SVGPathElement, SimLink>('path.dbviz-link')
    .style('transition', fade);
  const labels = refs.linkLabelLayer
    .selectAll<SVGTextElement, SimLink>('text.dbviz-link-label')
    .style('transition', fade);

  if (!hoverId) {
    nodes.style('opacity', 1);
    links.style('opacity', 1);
    labels.style('opacity', 1);
    return;
  }

  const related = relatedSet(refs, hoverId);
  const endpointId = (v: string | SimNode) => (typeof v === 'string' ? v : v.id);
  const touches = (l: SimLink) =>
    endpointId(l.source) === hoverId || endpointId(l.target) === hoverId;

  nodes.style('opacity', (d) => (related.has(d.id) ? 1 : HOVER_FADE_OPACITY));
  links.style('opacity', (l) => (touches(l) ? 1 : HOVER_FADE_OPACITY));
  labels.style('opacity', (l) => (touches(l) ? 1 : HOVER_FADE_OPACITY));
}

/**
 * makeAnimator builds the imperative GraphAnimator handle the ActionBar drives.
 * It composes the pure particle primitives:
 *   - animateCascade: walks affected steps grouped by depth, pulsing the root
 *     then, per step, sending a particle via->nodeId and pulsing the target.
 *   - animateQueryPath: pulses + connects plan.nodes in scan order.
 *   - animateCrud: steps through ordered crud actions, color-coded per action.
 *   - pulse: a single standalone pulse.
 *   - clearHighlights: tears down any persistent highlight + transient overlay.
 */
function makeAnimator(
  refs: SimRefs,
  activeHighlight: React.MutableRefObject<{ stop: () => void } | null>,
): GraphAnimator {
  const clearHighlights = () => {
    activeHighlight.current?.stop();
    activeHighlight.current = null;
    clearOverlay(refs);
  };

  const animateCascade = (r: CascadeResult) => {
    clearHighlights();
    const pathIds = [r.root, ...r.affected.map((s) => s.nodeId)];
    activeHighlight.current = highlightPath(refs, pathIds, palette.accentCascade);
    pulseNode(refs, r.root, palette.accentCascade, PULSE_MS);

    // Sort by depth so the ripple radiates outward; chain promises per depth.
    const steps = [...r.affected].sort((a, b) => a.depth - b.depth);
    let chain = Promise.resolve();
    for (const step of steps) {
      chain = chain.then(async () => {
        await animateParticle(
          refs,
          step.via || r.root,
          step.nodeId,
          palette.accentCascade,
          PARTICLE_MS,
        );
        pulseNode(refs, step.nodeId, palette.accentCascade, PULSE_MS);
      });
    }
  };

  const animateQueryPath = (p: QueryPlan) => {
    clearHighlights();
    const ids = p.nodes.map((n) => qualifiedNodeId(refs, n.table)).filter(Boolean) as string[];
    if (ids.length > 0) {
      activeHighlight.current = highlightPath(refs, ids, palette.accentQuery);
    }
    let chain = Promise.resolve();
    for (let i = 0; i < ids.length; i += 1) {
      const id = ids[i];
      const prev = i > 0 ? ids[i - 1] : null;
      chain = chain.then(async () => {
        if (prev) {
          await animateParticle(refs, prev, id, palette.accentQuery, PARTICLE_MS);
        }
        pulseNode(refs, id, palette.accentQuery, PULSE_MS);
      });
    }
  };

  const animateCrud = (r: CrudFlowResult) => {
    clearHighlights();
    const steps = [...r.steps].sort((a, b) => a.order - b.order);
    const ids = steps.map((s) => s.nodeId);
    activeHighlight.current = highlightPath(refs, [r.root, ...ids], crudColor(r.operation));

    let chain = Promise.resolve();
    let prevId: string = r.root;
    for (const step of steps) {
      const color = crudColor(step.action);
      const fromId = step.via || prevId;
      const toId = step.nodeId;
      chain = chain.then(async () => {
        if (fromId !== toId) {
          await animateParticle(refs, fromId, toId, color, PARTICLE_MS);
        }
        pulseNode(refs, toId, color, PULSE_MS);
      });
      prevId = step.nodeId;
    }
  };

  const pulse = (nodeId: string) => {
    pulseNode(refs, nodeId, palette.accentPulse, PULSE_MS);
  };

  return { animateCascade, animateQueryPath, animateCrud, pulse, clearHighlights };
}

/**
 * qualifiedNodeId resolves an EXPLAIN node's table name to a graph node id. The
 * planner may return either the fully-qualified id ("public.users") or a bare
 * table name ("users"); try the exact id first, then a suffix match.
 */
function qualifiedNodeId(refs: SimRefs, table: string): string | null {
  if (refs.nodeById.has(table)) return table;
  for (const id of refs.nodeById.keys()) {
    if (id === table || id.endsWith(`.${table}`)) return id;
  }
  return null;
}
