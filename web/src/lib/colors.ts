// Visual palette + helpers shared by the graph and panels (DESIGN_PLAN §13.4).
// Style rules: soft circles with subtle borders, thin translucent edges, no
// neon / glow / gradients. Background #f0ece4 light / #1a1a1a dark.

export const palette = {
  canvasLight: '#f0ece4',
  canvasDark: '#1a1a1a',

  // Node fills, kept muted and earthy to match the reference style.
  nodeTable: '#7c93b0', // tables — soft slate blue
  nodeView: '#8fae8b', // views — muted sage
  nodeCollection: '#b0937c', // collections — warm taupe
  nodeIndexPattern: '#a890b0', // index patterns — muted mauve
  nodeDefault: '#9a9a9a',

  nodeBorder: 'rgba(0, 0, 0, 0.18)',
  nodeBorderDark: 'rgba(255, 255, 255, 0.18)',

  // Edges: thin and translucent.
  linkCascade: 'rgba(80, 80, 80, 0.55)', // solid (real FK / cascade)
  linkInferred: 'rgba(120, 120, 120, 0.45)', // dashed (heuristic FK)

  // Animation accents (still muted — no neon).
  accentCascade: '#c2683f', // deletes ripple
  accentQuery: '#3f78c2', // query path
  accentInsert: '#5a9e6f', // crud insert
  accentUpdate: '#c2a23f', // crud update
  accentDelete: '#c2683f', // crud delete
  accentPulse: '#c2a23f',
} as const;

/** Opacity applied to unrelated nodes/edges while hovering (§13.4). */
export const HOVER_FADE_OPACITY = 0.15;

/** Returns the fill color for a node based on its kind. */
export function nodeFill(kind: string): string {
  switch (kind) {
    case 'table':
      return palette.nodeTable;
    case 'view':
      return palette.nodeView;
    case 'collection':
      return palette.nodeCollection;
    case 'index_pattern':
      return palette.nodeIndexPattern;
    default:
      return palette.nodeDefault;
  }
}

/** Returns the stroke color for a link: solid for real/cascade FKs, lighter for inferred. */
export function linkColor(inferred: boolean): string {
  return inferred ? palette.linkInferred : palette.linkCascade;
}

/** Returns the SVG dash array for a link, or undefined for a solid line. */
export function linkDash(inferred: boolean): string | undefined {
  return inferred ? '4 3' : undefined;
}

/** Maps a CRUD action to its accent color. */
export function crudColor(action: 'insert' | 'update' | 'delete' | 'check'): string {
  switch (action) {
    case 'insert':
      return palette.accentInsert;
    case 'update':
      return palette.accentUpdate;
    case 'delete':
      return palette.accentDelete;
    default:
      return palette.accentQuery;
  }
}

/** Maps an insight severity to its badge color (muted accents, no neon). */
export function severityColor(severity: 'info' | 'warn' | 'critical'): string {
  switch (severity) {
    case 'critical':
      return palette.accentDelete;
    case 'warn':
      return palette.accentUpdate;
    default:
      return palette.accentQuery;
  }
}
