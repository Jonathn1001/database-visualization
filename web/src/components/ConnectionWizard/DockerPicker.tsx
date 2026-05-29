// Docker tab (§13.3). Lists running DB-like containers from GET /api/docker/containers
// (react-query). Selecting one prefills connection fields (host 127.0.0.1, the
// container's detected engine, and the default port for that engine). The user can
// fill in credentials / database before connecting.
import { useEffect, useMemo } from 'react';
import { useQuery } from '@tanstack/react-query';
import { useForm } from 'react-hook-form';
import { zodResolver } from '@hookform/resolvers/zod';
import { z } from 'zod';
import { Container, LoaderCircle, RefreshCw } from 'lucide-react';

import * as connectionsApi from '@/api/connections';
import type { ConnectionConfig, DockerContainer, Engine } from '@/types/graph';

import { DEFAULT_PORTS } from './engine';
import {
  FieldError,
  FieldLabel,
  FormErrorBanner,
  SubmitButton,
  TextField,
  inputClass,
} from './fields';

const KNOWN_ENGINES: Engine[] = ['postgres', 'mongodb', 'sqlite'];

function isKnownEngine(e: string): e is Engine {
  return (KNOWN_ENGINES as string[]).includes(e);
}

const schema = z.object({
  containerId: z.string().min(1, 'Select a container'),
  host: z.string().trim().min(1, 'Host is required'),
  port: z.coerce.number().int().min(1, 'Port is required').max(65535, 'Invalid port'),
  database: z.string().trim().optional(),
  user: z.string().trim().optional(),
  password: z.string().optional(),
});

type FormValues = z.input<typeof schema>;

export function DockerPicker({
  onSubmit,
  busy,
  formError,
}: {
  onSubmit: (config: ConnectionConfig) => void | Promise<void>;
  busy?: boolean;
  formError?: string | null;
}) {
  const containersQuery = useQuery({
    queryKey: ['docker', 'containers'],
    queryFn: connectionsApi.listDockerContainers,
    staleTime: 10_000,
    retry: 0,
  });

  const containersData = containersQuery.data;
  const containers = useMemo(() => containersData ?? [], [containersData]);

  const {
    register,
    handleSubmit,
    watch,
    setValue,
    formState: { errors },
  } = useForm<FormValues>({
    resolver: zodResolver(schema),
    defaultValues: { containerId: '', host: '127.0.0.1', port: 5432, database: '', user: '', password: '' },
  });

  const selectedId = watch('containerId');
  const selected = useMemo(
    () => containers.find((c) => c.id === selectedId) ?? null,
    [containers, selectedId],
  );

  // When the selected container changes, prefill engine-derived defaults (§13.3).
  useEffect(() => {
    if (!selected) return;
    const engine = isKnownEngine(selected.engine) ? selected.engine : null;
    setValue('host', '127.0.0.1');
    if (engine) {
      setValue('port', DEFAULT_PORTS[engine], { shouldValidate: true });
    }
  }, [selected, setValue]);

  const submit = handleSubmit((values) => {
    if (!selected) return;
    const engine: Engine | '' = isKnownEngine(selected.engine) ? selected.engine : '';
    const config: ConnectionConfig = {
      engine,
      host: values.host.trim(),
      port: Number(values.port),
      database: values.database?.trim() || undefined,
      user: values.user?.trim() || undefined,
      password: values.password || undefined,
      label: selected.name,
    };
    void onSubmit(config);
  });

  if (containersQuery.isLoading) {
    return (
      <div className="flex items-center justify-center gap-2 py-10 text-sm text-neutral-500">
        <LoaderCircle size={16} className="animate-spin" aria-hidden />
        Scanning Docker containers…
      </div>
    );
  }

  if (containersQuery.isError) {
    return (
      <div className="space-y-3 py-6 text-center">
        <p className="text-sm text-neutral-500">
          Couldn’t reach Docker. Is the Docker daemon running?
        </p>
        <button
          type="button"
          onClick={() => containersQuery.refetch()}
          className="inline-flex items-center gap-1.5 rounded-md border border-black/15 px-3 py-1.5 text-sm hover:bg-black/5 dark:border-white/15 dark:hover:bg-white/5"
        >
          <RefreshCw size={14} aria-hidden />
          Retry
        </button>
      </div>
    );
  }

  if (containers.length === 0) {
    return (
      <div className="space-y-3 py-6 text-center">
        <Container size={28} className="mx-auto text-neutral-400" aria-hidden />
        <p className="text-sm text-neutral-500">No database containers found (is Docker running?)</p>
        <button
          type="button"
          onClick={() => containersQuery.refetch()}
          className="inline-flex items-center gap-1.5 rounded-md border border-black/15 px-3 py-1.5 text-sm hover:bg-black/5 dark:border-white/15 dark:hover:bg-white/5"
        >
          <RefreshCw size={14} aria-hidden />
          Rescan
        </button>
      </div>
    );
  }

  return (
    <form onSubmit={submit} className="space-y-4">
      <div>
        <FieldLabel>Container</FieldLabel>
        <select {...register('containerId')} className={inputClass} aria-invalid={errors.containerId ? 'true' : 'false'}>
          <option value="">Select a container…</option>
          {containers.map((c) => (
            <option key={c.id} value={c.id}>
              {dockerOptionLabel(c)}
            </option>
          ))}
        </select>
        <FieldError message={errors.containerId?.message} />
      </div>

      {selected && (
        <>
          <div className="grid grid-cols-2 gap-3">
            <div>
              <FieldLabel>Host</FieldLabel>
              <TextField {...register('host')} autoComplete="off" aria-invalid={errors.host ? 'true' : 'false'} />
              <FieldError message={errors.host?.message} />
            </div>
            <div>
              <FieldLabel>Port</FieldLabel>
              <TextField type="number" {...register('port')} autoComplete="off" aria-invalid={errors.port ? 'true' : 'false'} />
              <FieldError message={errors.port?.message} />
            </div>
          </div>

          <div className="grid grid-cols-2 gap-3">
            <div>
              <FieldLabel>User</FieldLabel>
              <TextField {...register('user')} autoComplete="off" placeholder="reader" />
            </div>
            <div>
              <FieldLabel>Password</FieldLabel>
              <TextField type="password" {...register('password')} autoComplete="off" />
            </div>
          </div>

          <div>
            <FieldLabel>Database</FieldLabel>
            <TextField {...register('database')} autoComplete="off" placeholder="mydb" />
          </div>
        </>
      )}

      <FormErrorBanner message={formError} />

      <div className="flex justify-end">
        <SubmitButton busy={busy} disabled={!selected}>
          Connect
        </SubmitButton>
      </div>
    </form>
  );
}

function dockerOptionLabel(c: DockerContainer): string {
  const ports = c.ports.length > 0 ? ` (${c.ports.join(', ')})` : '';
  return `${c.name} — ${c.image}${ports}`;
}
