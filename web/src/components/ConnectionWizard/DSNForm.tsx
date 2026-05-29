// DSN tab (primary, §13.3). The user pastes a connection URI. We auto-detect the
// engine from the scheme as they type, dry-run validate on blur via POST /test,
// and submit via POST /connections (handled by the parent's onSubmit).
import { useCallback, useMemo, useState } from 'react';
import { useForm } from 'react-hook-form';
import { zodResolver } from '@hookform/resolvers/zod';
import { z } from 'zod';

import * as connectionsApi from '@/api/connections';
import { APIError } from '@/api/client';
import type { ConnectionConfig } from '@/types/graph';

import { detectEngineFromDSN, engineLabel } from './engine';
import {
  FieldError,
  FieldLabel,
  FormErrorBanner,
  SubmitButton,
  TestResult,
  TextField,
} from './fields';

const schema = z.object({
  dsn: z
    .string()
    .trim()
    .min(1, 'Enter a connection string')
    .refine((v) => detectEngineFromDSN(v) !== null, {
      message: 'Unrecognized scheme — expected postgres://, mongodb://, or a .db file path',
    }),
  label: z.string().trim().optional(),
});

type FormValues = z.infer<typeof schema>;

type TestState =
  | { kind: 'idle' }
  | { kind: 'testing' }
  | { kind: 'ok'; engine: string }
  | { kind: 'error'; message: string };

export function DSNForm({
  onSubmit,
  busy,
  formError,
}: {
  onSubmit: (config: ConnectionConfig) => void | Promise<void>;
  busy?: boolean;
  formError?: string | null;
}) {
  const {
    register,
    handleSubmit,
    watch,
    formState: { errors },
  } = useForm<FormValues>({
    resolver: zodResolver(schema),
    mode: 'onBlur',
    defaultValues: { dsn: '', label: '' },
  });

  const [testState, setTestState] = useState<TestState>({ kind: 'idle' });

  const dsnValue = watch('dsn');
  const detected = useMemo(() => detectEngineFromDSN(dsnValue ?? ''), [dsnValue]);

  // Dry-run validation on blur (§13.3): only fires when a known scheme is present.
  const runTest = useCallback(async () => {
    const engine = detectEngineFromDSN(dsnValue ?? '');
    if (engine === null) {
      setTestState({ kind: 'idle' });
      return;
    }
    setTestState({ kind: 'testing' });
    try {
      const res = await connectionsApi.test({ engine, dsn: dsnValue.trim() });
      setTestState({ kind: 'ok', engine: res.engine });
    } catch (err) {
      const message =
        err instanceof APIError
          ? err.hint
            ? `${err.message} — ${err.hint}`
            : err.message
          : 'Connection test failed';
      setTestState({ kind: 'error', message });
    }
  }, [dsnValue]);

  const dsnField = register('dsn', {
    onBlur: () => {
      void runTest();
    },
    onChange: () => {
      // Invalidate any previous test result once the user edits the DSN.
      setTestState({ kind: 'idle' });
    },
  });

  const submit = handleSubmit((values) => {
    const engine = detectEngineFromDSN(values.dsn);
    if (engine === null) return;
    void onSubmit({
      engine,
      dsn: values.dsn.trim(),
      label: values.label?.trim() || undefined,
    });
  });

  return (
    <form onSubmit={submit} className="space-y-4">
      <div>
        <FieldLabel>Connection string</FieldLabel>
        <TextField
          {...dsnField}
          autoFocus
          spellCheck={false}
          autoComplete="off"
          placeholder="postgres://reader:••••@localhost:5432/mydb"
          aria-invalid={errors.dsn ? 'true' : 'false'}
          className="font-mono"
        />
        <FieldError message={errors.dsn?.message} />
        {detected !== null && !errors.dsn && (
          <p className="mt-1.5 text-xs text-neutral-500">
            Detected engine: <span className="font-medium">{engineLabel(detected)}</span>
          </p>
        )}
        <TestResult state={testState} />
      </div>

      <div>
        <FieldLabel>Label (optional)</FieldLabel>
        <TextField
          {...register('label')}
          placeholder="e.g. Staging Postgres"
          autoComplete="off"
        />
      </div>

      <FormErrorBanner message={formError} />

      <div className="flex justify-end">
        <SubmitButton busy={busy}>Connect</SubmitButton>
      </div>
    </form>
  );
}
