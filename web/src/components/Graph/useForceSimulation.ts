// D3 force-directed layout for the schema graph (DESIGN_PLAN §13.1, §14.2, §16.3).
//
// This module owns ALL D3 state. It never triggers a React render: node
// positions live in D3's internal datum state and are written straight to the
// SVG on each tick. `GraphCanvas` builds it once per database and soft-updates
// it on schema refresh.
//
// D3 import discipline (§14.2): never `import * as d3`; only granular packages.
import {
  forceCenter,
  forceCollide,
  forceLink,
  forceManyBody,
  forceSimulation,
  type Simulation,
  type SimulationLinkDatum,
  type SimulationNodeDatum,
} from 'd3-force';
import { drag, type D3DragEvent } from 'd3-drag';
import { select, type Selection } from 'd3-selection';
import { zoom, zoomIdentity, type ZoomBehavior } from 'd3-zoom';

import type { GraphModel, Link, Node } from '@/types/graph';
import { FORCE, PERF } from '@/lib/constants';
import { renderNodes, nodeRadius } from './nodeRenderer';
import { renderLinks } from './linkRenderer';

// A node in the simulation: the model node plus D3's mutable x/y/vx/vy fields.
export interface SimNode extends Node, SimulationNodeDatum {
  /** Cached radius so renderers + collide force agree without recomputing. */
  radius: number;
}

// A link in the simulation. d3-force replaces the string source/target ids with
// the resolved SimNode objects in place, so the resolved fields are unioned in.
export interface SimLink extends Omit<Link, 'source' | 'target'>, SimulationLinkDatum<SimNode> {
  source: string | SimNode;
  target: string | SimNode;
}

/**
 * SimRefs is the imperative handle held by GraphCanvas (in a ref). It bundles
 * the live D3 simulation, the four layered selections and the lookup maps the
 * animation primitives need. It does NOT cause React updates.
 */
export interface SimRefs {
  simulation: Simulation<SimNode, SimLink>;
  /** Root <g> that the zoom/pan transform is applied to. */
  root: Selection<SVGGElement, unknown, null, undefined>;
  /** <g> holding the link <path> elements. */
  linkLayer: Selection<SVGGElement, unknown, null, undefined>;
  /** <g> holding the link-label <text> elements (rendered, toggled by perf mode). */
  linkLabelLayer: Selection<SVGGElement, unknown, null, undefined>;
  /** <g> holding the node <g> groups. */
  nodeLayer: Selection<SVGGElement, unknown, null, undefined>;
  /** <g> holding transient particle/highlight overlays. */
  overlayLayer: Selection<SVGGElement, unknown, null, undefined>;
  /** id -> SimNode for O(1) lookup by the animation primitives. */
  nodeById: Map<string, SimNode>;
  /** Current resolved links (source/target swapped to SimNode by d3-force). */
  links: SimLink[];
  /** Adjacency: nodeId -> set of neighbour nodeIds (both directions) for hover fade. */
  neighbors: Map<string, Set<string>>;
  /** True once node count crosses the perf threshold (§16.3). */
  perfMode: boolean;
  /** True once node count crosses the high threshold (§16.3). */
  highMode: boolean;
  /** Dark color scheme active (drives SVG inline border colors). */
  dark: boolean;
  /** Whether the simulation is currently "stopped after settle" (high mode). */
  settled: boolean;
  /** Tears down the simulation, zoom handlers and any running animations. */
  stop: () => void;
}

type DragEvt = D3DragEvent<SVGGElement, SimNode, SimNode>;

function isDark(): boolean {
  return (
    typeof window !== 'undefined' &&
    typeof window.matchMedia === 'function' &&
    window.matchMedia('(prefers-color-scheme: dark)').matches
  );
}

function buildNeighbors(links: SimLink[]): Map<string, Set<string>> {
  const map = new Map<string, Set<string>>();
  const add = (a: string, b: string) => {
    let set = map.get(a);
    if (!set) {
      set = new Set<string>();
      map.set(a, set);
    }
    set.add(b);
  };
  for (const l of links) {
    const s = typeof l.source === 'string' ? l.source : l.source.id;
    const t = typeof l.target === 'string' ? l.target : l.target.id;
    add(s, t);
    add(t, s);
  }
  return map;
}

/** Converts model nodes into SimNodes (carrying a cached radius). */
function toSimNodes(graph: GraphModel, prev?: Map<string, SimNode>): SimNode[] {
  return graph.nodes.map((n) => {
    const existing = prev?.get(n.id);
    const sim: SimNode = {
      ...n,
      radius: nodeRadius(n),
      // Preserve position across soft updates so the layout does not jump.
      x: existing?.x,
      y: existing?.y,
      vx: existing?.vx,
      vy: existing?.vy,
    };
    return sim;
  });
}

/** Fresh copies of the model links (source/target as ids; d3 resolves them). */
function toSimLinks(graph: GraphModel): SimLink[] {
  return graph.links.map((l) => ({ ...l }));
}

/**
 * buildSimulation wires the SVG structure, the force simulation, drag and
 * zoom/pan, then starts ticking. It returns the SimRefs handle.
 */
export function buildSimulation(svgEl: SVGSVGElement, graph: GraphModel): SimRefs {
  const dark = isDark();
  const nodeCount = graph.stats?.nodeCount ?? graph.nodes.length;
  const perfMode = nodeCount > PERF.perfModeNodeThreshold;
  const highMode = nodeCount > PERF.highNodeThreshold;

  const svg = select(svgEl);
  // Clear any prior content (in case of a rebuild on the same element).
  svg.selectAll('*').remove();

  const { width, height } = svgEl.getBoundingClientRect();
  const w = width || 800;
  const h = height || 600;

  // Layer order: links under labels under nodes under overlay.
  const root = svg.append('g').attr('class', 'dbviz-root') as Selection<
    SVGGElement,
    unknown,
    null,
    undefined
  >;
  const linkLayer = root.append('g').attr('class', 'dbviz-links') as Selection<
    SVGGElement,
    unknown,
    null,
    undefined
  >;
  const linkLabelLayer = root.append('g').attr('class', 'dbviz-link-labels') as Selection<
    SVGGElement,
    unknown,
    null,
    undefined
  >;
  const nodeLayer = root.append('g').attr('class', 'dbviz-nodes') as Selection<
    SVGGElement,
    unknown,
    null,
    undefined
  >;
  const overlayLayer = root.append('g').attr('class', 'dbviz-overlay') as Selection<
    SVGGElement,
    unknown,
    null,
    undefined
  >;

  const nodes = toSimNodes(graph);
  const links = toSimLinks(graph);
  const nodeById = new Map(nodes.map((n) => [n.id, n]));

  const simulation = forceSimulation<SimNode, SimLink>(nodes)
    .force(
      'link',
      forceLink<SimNode, SimLink>(links)
        .id((d) => d.id)
        .distance(FORCE.linkDistance),
    )
    .force('charge', forceManyBody<SimNode>().strength(FORCE.chargeStrength))
    .force('center', forceCenter<SimNode>(w / 2, h / 2).strength(FORCE.centerStrength))
    .force(
      'collide',
      forceCollide<SimNode>().radius((d) => d.radius + FORCE.collideRadiusPadding),
    )
    .alphaDecay(perfMode ? PERF.perfAlphaDecay : FORCE.alphaDecay)
    .velocityDecay(perfMode ? PERF.perfVelocityDecay : FORCE.velocityDecay);

  const refs: SimRefs = {
    simulation,
    root,
    linkLayer,
    linkLabelLayer,
    nodeLayer,
    overlayLayer,
    nodeById,
    links,
    neighbors: buildNeighbors(links),
    perfMode,
    highMode,
    dark,
    settled: false,
    stop: () => {},
  };

  // --- Zoom / pan (§13.1) ---
  const zoomBehavior: ZoomBehavior<SVGSVGElement, unknown> = zoom<SVGSVGElement, unknown>()
    .scaleExtent([0.15, 4])
    .on('zoom', (event) => {
      root.attr('transform', event.transform.toString());
    });
  svg.call(zoomBehavior);
  // Start at identity so the centered layout is visible.
  svg.call(zoomBehavior.transform, zoomIdentity);

  // --- Initial paint of links + nodes (selections bound by renderers) ---
  paint(refs);

  // --- Tick: write positions straight to the DOM, no React (§13.1) ---
  simulation.on('tick', () => {
    tick(refs);
    if (refs.highMode && simulation.alpha() < PERF.settleAlpha && !refs.settled) {
      simulation.stop();
      refs.settled = true;
    }
  });

  // --- Drag (§13.1) — bind after nodes exist; rebound on each paint ---
  attachDrag(refs);

  refs.stop = () => {
    simulation.on('tick', null);
    simulation.stop();
    svg.on('.zoom', null);
    nodeLayer.selectAll<SVGGElement, SimNode>('g.dbviz-node').on('.drag', null);
    // Drop any transient overlay (particles / pulses / highlights).
    overlayLayer.selectAll('*').remove();
  };

  return refs;
}

/** (Re)binds drag behavior to the current node selection. */
function attachDrag(refs: SimRefs): void {
  const { simulation } = refs;
  const dragBehavior = drag<SVGGElement, SimNode>()
    .on('start', (event: DragEvt, d) => {
      if (!event.active) simulation.alphaTarget(0.3).restart();
      refs.settled = false;
      d.fx = d.x;
      d.fy = d.y;
    })
    .on('drag', (event: DragEvt, d) => {
      d.fx = event.x;
      d.fy = event.y;
    })
    .on('end', (event: DragEvt, d) => {
      if (!event.active) simulation.alphaTarget(0);
      d.fx = null;
      d.fy = null;
    });
  refs.nodeLayer
    .selectAll<SVGGElement, SimNode>('g.dbviz-node')
    .call(dragBehavior as unknown as (sel: Selection<SVGGElement, SimNode, SVGGElement, unknown>) => void);
}

/** Binds + draws links and nodes via the dedicated renderers. */
function paint(refs: SimRefs): void {
  renderLinks(refs);
  renderNodes(refs);
}

/** Positions all bound elements from the current simulation state. */
function tick(refs: SimRefs): void {
  refs.linkLayer
    .selectAll<SVGPathElement, SimLink>('path.dbviz-link')
    .attr('d', (l) => linkPath(l));

  refs.linkLabelLayer
    .selectAll<SVGTextElement, SimLink>('text.dbviz-link-label')
    .attr('x', (l) => midX(l))
    .attr('y', (l) => midY(l));

  refs.nodeLayer
    .selectAll<SVGGElement, SimNode>('g.dbviz-node')
    .attr('transform', (d) => `translate(${d.x ?? 0},${d.y ?? 0})`);
}

function endpoints(l: SimLink): [number, number, number, number] {
  const s = l.source as SimNode;
  const t = l.target as SimNode;
  return [s.x ?? 0, s.y ?? 0, t.x ?? 0, t.y ?? 0];
}

/** A gently curved quadratic path between the two endpoints. */
export function linkPath(l: SimLink): string {
  const [x1, y1, x2, y2] = endpoints(l);
  const dx = x2 - x1;
  const dy = y2 - y1;
  // Slight curvature keeps reciprocal edges from overlapping; tiny offset.
  const curve = 0.12;
  const cx = (x1 + x2) / 2 - dy * curve;
  const cy = (y1 + y2) / 2 + dx * curve;
  return `M${x1},${y1}Q${cx},${cy} ${x2},${y2}`;
}

function midX(l: SimLink): number {
  const [x1, , x2] = endpoints(l);
  return (x1 + x2) / 2;
}
function midY(l: SimLink): number {
  const [, y1, , y2] = endpoints(l);
  return (y1 + y2) / 2;
}

/**
 * updateGraphData performs a soft update when the schema is refreshed: it
 * diffs nodes/links, preserves positions of surviving nodes, rebinds the
 * selections and re-heats the simulation. It does NOT rebuild the SVG or the
 * zoom/drag wiring.
 */
export function updateGraphData(refs: SimRefs, graph: GraphModel): void {
  const prev = refs.nodeById;
  const nodes = toSimNodes(graph, prev);
  const links = toSimLinks(graph);

  refs.nodeById = new Map(nodes.map((n) => [n.id, n]));
  refs.links = links;
  refs.neighbors = buildNeighbors(links);

  const nodeCount = graph.stats?.nodeCount ?? graph.nodes.length;
  refs.perfMode = nodeCount > PERF.perfModeNodeThreshold;
  refs.highMode = nodeCount > PERF.highNodeThreshold;

  refs.simulation.nodes(nodes);
  const linkForce = refs.simulation.force('link') as ReturnType<
    typeof forceLink<SimNode, SimLink>
  > | null;
  linkForce?.links(links);
  refs.simulation
    .alphaDecay(refs.perfMode ? PERF.perfAlphaDecay : FORCE.alphaDecay)
    .velocityDecay(refs.perfMode ? PERF.perfVelocityDecay : FORCE.velocityDecay);

  // Rebind + redraw, rebind drag, then re-heat.
  paint(refs);
  attachDrag(refs);
  refs.settled = false;
  refs.simulation.alpha(0.6).restart();
}
