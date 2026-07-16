// Holds the latest insights result + the graph-overlay toggle. The panel
// writes the result; the ActionBar toggles the overlay; GraphCanvas and the
// Workspace read both to render badges and implied edges.
import { create } from 'zustand';
import type { InsightsResult } from '@/types/graph';

interface InsightsState {
  result: InsightsResult | null;
  overlay: boolean;
  setResult: (result: InsightsResult | null) => void;
  setOverlay: (overlay: boolean) => void;
  clear: () => void;
}

export const useInsightsStore = create<InsightsState>((set) => ({
  result: null,
  overlay: false,
  setResult: (result) => set({ result }),
  setOverlay: (overlay) => set({ overlay }),
  clear: () => set({ result: null, overlay: false }),
}));
