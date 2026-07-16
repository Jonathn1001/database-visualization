import { describe, expect, it } from 'vitest';

import {
  impliedLinks,
  mergeImpliedLinks,
  severityByNode,
  severityRank,
  sortFindings,
} from './insights';
import type { Finding, GraphModel, InsightsResult, Link } from '@/types/graph';

const finding = (over: Partial<Finding>): Finding => ({
  category: 'gaps',
  severity: 'info',
  nodeId: 'public.a',
  title: 't',
  detail: 'd',
  ...over,
});

const link = (over: Partial<Link>): Link => ({
  source: 'public.a',
  target: 'public.b',
  type: '1:N',
  cascade: false,
  viaColumn: 'b_id',
  inferred: true,
  confidence: 0.75,
  ...over,
});

const result: InsightsResult = {
  categories: [
    {
      category: 'index',
      status: 'ok',
      findings: [finding({ category: 'index', severity: 'warn', nodeId: 'public.b' })],
    },
    {
      category: 'health',
      status: 'unsupported',
      findings: [finding({ category: 'health', severity: 'critical', nodeId: 'public.c' })],
    },
    {
      category: 'gaps',
      status: 'ok',
      findings: [finding({ severity: 'critical', nodeId: 'public.b' })],
      impliedLinks: [link({})],
    },
  ],
};

describe('severityRank / sortFindings', () => {
  it('orders critical < warn < info', () => {
    expect(severityRank('critical')).toBeLessThan(severityRank('warn'));
    expect(severityRank('warn')).toBeLessThan(severityRank('info'));
  });

  it('sorts by severity then node, or node then severity', () => {
    const fs = [
      finding({ severity: 'info', nodeId: 'public.a' }),
      finding({ severity: 'critical', nodeId: 'public.z' }),
    ];
    expect(sortFindings(fs, 'severity')[0].severity).toBe('critical');
    expect(sortFindings(fs, 'table')[0].nodeId).toBe('public.a');
  });

  it('table mode sorts by node then breaks ties by severity', () => {
    const fs = [
      finding({ severity: 'critical', nodeId: 'public.z' }),
      finding({ severity: 'info', nodeId: 'public.a' }),
      finding({ severity: 'warn', nodeId: 'public.a' }),
    ];
    const sorted = sortFindings(fs, 'table');
    expect(sorted.map((f) => f.nodeId)).toEqual(['public.a', 'public.a', 'public.z']);
    expect(sorted.map((f) => f.severity)).toEqual(['warn', 'info', 'critical']);
  });
});

describe('impliedLinks', () => {
  it('extracts the gaps category links', () => {
    expect(impliedLinks(result)).toHaveLength(1);
    expect(impliedLinks(null)).toHaveLength(0);
  });
});

describe('mergeImpliedLinks', () => {
  const graph: GraphModel = {
    engine: 'postgres',
    database: 'db',
    schemas: ['public'],
    nodes: [
      { id: 'public.a', schema: 'public', label: 'a', kind: 'table', columns: [], rowCount: 0, sizeBytes: 0 },
      { id: 'public.b', schema: 'public', label: 'b', kind: 'table', columns: [], rowCount: 0, sizeBytes: 0 },
    ],
    links: [link({ inferred: false })],
    stats: { nodeCount: 2, linkCount: 1, cascadeChainDepth: 0, columnCount: 0 },
  };

  it('appends implied links with known endpoints', () => {
    const merged = mergeImpliedLinks(graph, [link({ viaColumn: 'other_id' })]);
    expect(merged.links).toHaveLength(2);
    expect(graph.links).toHaveLength(1); // input untouched
  });

  it('drops duplicates of real links and unknown endpoints', () => {
    const merged = mergeImpliedLinks(graph, [
      link({}), // same source/target/viaColumn as the real link -> dropped
      link({ target: 'public.ghost' }), // unknown endpoint -> dropped
    ]);
    expect(merged.links).toHaveLength(1);
  });

  it('returns input graph unchanged when nothing merges', () => {
    expect(mergeImpliedLinks(graph, [])).toBe(graph);
  });
});

describe('severityByNode', () => {
  it('keeps the highest severity per node and skips unsupported categories', () => {
    const map = severityByNode(result);
    expect(map.get('public.b')).toBe('critical'); // gaps critical beats index warn
    expect(map.has('public.c')).toBe(false); // unsupported category ignored
  });
});
