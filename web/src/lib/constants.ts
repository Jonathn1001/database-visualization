// Shared tuning constants: D3 force params, perf-mode thresholds (§16.3) and
// the ConnectionStatus ping interval (§19). Feature agents should read these
// rather than hard-coding magic numbers so behavior stays consistent.

// --- D3 force-simulation defaults (§13.1, §14.4) ---
export const FORCE = {
  // d3-force defaults are alphaDecay 0.0228 / velocityDecay 0.4. Perf mode bumps
  // these (see PERF below).
  alphaDecay: 0.0228,
  velocityDecay: 0.4,
  chargeStrength: -240, // forceManyBody
  linkDistance: 90, // forceLink
  collideRadiusPadding: 6, // forceCollide = nodeRadius + padding
  centerStrength: 0.05,
  nodeRadiusMin: 10,
  nodeRadiusMax: 34,
} as const;

// --- Frontend perf mode thresholds (§16.3) ---
export const PERF = {
  // When nodeCount exceeds this, settle faster + reduce visual noise.
  perfModeNodeThreshold: 200,
  // When nodeCount exceeds this, stop continuous ticking after settle and only
  // render link labels on hover.
  highNodeThreshold: 350,
  // Perf-mode force overrides.
  perfAlphaDecay: 0.05,
  perfVelocityDecay: 0.6,
  // Link opacity: normal vs perf mode (§16.3).
  linkOpacityNormal: 0.55,
  linkOpacityPerf: 0.3,
  // Stop the simulation once alpha drops below this (§16.3, high node mode).
  settleAlpha: 0.01,
} as const;

// Soft warning threshold mirrored from backend (§16.1) — surfaced in UI copy.
export const SOFT_NODE_WARNING_THRESHOLD = 200;

// --- Connection health polling (§19.1) ---
// ConnectionStatus polls POST /api/connections/:id/ping on this interval.
export const PING_INTERVAL_MS = 15_000;

// --- Desktop-only gate (§13.5) ---
export const DESKTOP_MIN_WIDTH = 1024;
export const DESKTOP_MEDIA_QUERY = '(min-width: 1024px)';

// --- Sample data defaults (§6.2) ---
export const SAMPLE_DEFAULT_LIMIT = 5;
