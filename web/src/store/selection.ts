// Tracks which node is selected (for the inspector) and which is hovered (for
// graph fade highlighting). Kept out of React render path for the graph itself.
import { create } from 'zustand';

interface SelectionState {
  selectedNodeId: string | null;
  hoverNodeId: string | null;
  select: (nodeId: string | null) => void;
  setHover: (nodeId: string | null) => void;
  clear: () => void;
}

export const useSelectionStore = create<SelectionState>((set) => ({
  selectedNodeId: null,
  hoverNodeId: null,
  select: (nodeId) => set({ selectedNodeId: nodeId }),
  setHover: (nodeId) => set({ hoverNodeId: nodeId }),
  clear: () => set({ selectedNodeId: null, hoverNodeId: null }),
}));
