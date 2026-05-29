// Bridges the D3 GraphCanvas and the ActionBar. GraphCanvas registers an
// imperative GraphAnimator handle on mount; the ActionBar reads it to drive
// cascade / query-path / crud animations without prop-drilling the D3 sim.
import { create } from 'zustand';
import type { GraphAnimator } from '@/types/graph';

interface GraphState {
  animator: GraphAnimator | null;
  setAnimator: (animator: GraphAnimator | null) => void;
}

export const useGraphStore = create<GraphState>((set) => ({
  animator: null,
  setAnimator: (animator) => set({ animator }),
}));
