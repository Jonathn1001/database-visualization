// File tab (§13.3). A SQLite database lives in a file; the user types its path.
// Browsers can't expose absolute filesystem paths from a file picker, and the Go
// binary opens the file server-side, so this is a plain text path input gated to
// the SQLite file extensions (.db / .sqlite / .sqlite3) — engine is always sqlite.
import { useForm } from 'react-hook-form';
import { zodResolver } from '@hookform/resolvers/zod';
import { z } from 'zod';
import { HardDrive } from 'lucide-react';

import type { ConnectionConfig } from '@/types/graph';

import { isSqliteFilePath } from './engine';
import { FieldError, FieldLabel, FormErrorBanner, SubmitButton, TextField } from './fields';

const schema = z.object({
  filePath: z
    .string()
    .trim()
    .min(1, 'Enter a file path')
    .refine((v) => isSqliteFilePath(v), {
      message: 'Must be a .db, .sqlite, or .sqlite3 file',
    }),
  label: z.string().trim().optional(),
});

type FormValues = z.infer<typeof schema>;

export function FileForm({
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
    formState: { errors },
  } = useForm<FormValues>({
    resolver: zodResolver(schema),
    mode: 'onBlur',
    defaultValues: { filePath: '', label: '' },
  });

  const submit = handleSubmit((values) => {
    void onSubmit({
      engine: 'sqlite',
      filePath: values.filePath.trim(),
      label: values.label?.trim() || undefined,
    });
  });

  return (
    <form onSubmit={submit} className="space-y-4">
      <div>
        <FieldLabel>SQLite file path</FieldLabel>
        <div className="relative">
          <HardDrive
            size={15}
            className="pointer-events-none absolute left-3 top-1/2 -translate-y-1/2 text-neutral-400"
            aria-hidden
          />
          <TextField
            {...register('filePath')}
            autoFocus
            spellCheck={false}
            autoComplete="off"
            placeholder="/path/to/database.sqlite"
            aria-invalid={errors.filePath ? 'true' : 'false'}
            className="pl-9 font-mono"
          />
        </div>
        <FieldError message={errors.filePath?.message} />
        <p className="mt-1.5 text-xs text-neutral-500">
          Path on the machine running dbviz. Accepts .db, .sqlite, .sqlite3.
        </p>
      </div>

      <div>
        <FieldLabel>Label (optional)</FieldLabel>
        <TextField {...register('label')} placeholder="e.g. Local test DB" autoComplete="off" />
      </div>

      <FormErrorBanner message={formError} />

      <div className="flex justify-end">
        <SubmitButton busy={busy}>Connect</SubmitButton>
      </div>
    </form>
  );
}
