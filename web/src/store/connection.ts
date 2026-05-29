// Holds the active connection and the schemas currently loaded for it.
import { create } from 'zustand';
import type { ConnMeta } from '@/types/graph';

interface ConnectionState {
  conn: ConnMeta | null;
  schemas: string[];
  setConn: (conn: ConnMeta) => void;
  clearConn: () => void;
  setSchemas: (schemas: string[]) => void;
}

export const useConnectionStore = create<ConnectionState>((set) => ({
  conn: null,
  schemas: [],
  setConn: (conn) => set({ conn }),
  clearConn: () => set({ conn: null, schemas: [] }),
  setSchemas: (schemas) => set({ schemas }),
}));
