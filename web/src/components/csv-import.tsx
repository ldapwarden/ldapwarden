import { useMemo, useRef, useState } from 'react'
import { useRouter } from '@tanstack/react-router'
import { useQueryClient } from '@tanstack/react-query'
import Papa from 'papaparse'
import { toast } from 'sonner'
import type { ImportResult, ImportRow } from '@/lib/api'
import { Button } from '@/components/ui/button'
import { Select } from '@/components/ui/select'
import { Card, CardContent, CardHeader, CardTitle, CardDescription } from '@/components/ui/card'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import { ArrowLeft, Upload, CheckCircle2, XCircle, Download } from 'lucide-react'

export type FieldType = 'string' | 'number' | 'stringArray'

export interface ImportField {
  key: string
  label: string
  type: FieldType
  required?: boolean
  /** Extra header names (besides key/label) that auto-map to this field. */
  aliases?: string[]
  /** Optional charset check applied client-side (e.g. RDN keys). */
  pattern?: RegExp
  patternHint?: string
  /** Sample value for the downloadable template. */
  example?: string
}

const IGNORE = '__ignore__'
/** Multi-value CSV cells are split on ';' (comma is the CSV delimiter). */
const MULTI_SEP = ';'

interface ParsedRow {
  row: ImportRow
  errors: string[]
}

/**
 * Reusable CSV import workflow (upload → map columns → preview/validate →
 * submit → per-row report), shared by the users and groups import routes. The
 * parent supplies the target field spec and the submit function; the backend
 * remains authoritative for validation, this only catches obvious mistakes
 * early.
 */
export function CsvImport({
  title,
  noun,
  fields,
  backTo,
  submit,
}: {
  title: string
  noun: string
  fields: ImportField[]
  backTo: string
  submit: (rows: ImportRow[]) => Promise<ImportResult>
}) {
  const router = useRouter()
  const queryClient = useQueryClient()
  const fileRef = useRef<HTMLInputElement>(null)
  const [headers, setHeaders] = useState<string[]>([])
  const [csvRows, setCsvRows] = useState<Record<string, string>[]>([])
  const [mapping, setMapping] = useState<Record<string, string>>({})
  const [step, setStep] = useState<'upload' | 'map' | 'preview' | 'result'>('upload')
  const [submitting, setSubmitting] = useState(false)
  const [result, setResult] = useState<ImportResult | null>(null)

  function handleFile(e: React.ChangeEvent<HTMLInputElement>) {
    const file = e.target.files?.[0]
    e.target.value = ''
    if (!file) return
    Papa.parse<Record<string, string>>(file, {
      header: true,
      skipEmptyLines: true,
      transformHeader: (h) => h.trim(),
      complete: (res) => {
        const cols = (res.meta.fields ?? []).filter(Boolean)
        if (cols.length === 0 || res.data.length === 0) {
          toast.error('The file has no header row or no data rows.')
          return
        }
        setHeaders(cols)
        setCsvRows(res.data)
        setMapping(autoMap(fields, cols))
        setStep('map')
      },
      error: () => toast.error('Could not parse the CSV file.'),
    })
  }

  const requiredUnmapped = fields.filter((f) => f.required && !mapping[f.key])

  const parsedRows: ParsedRow[] = useMemo(() => {
    if (step !== 'preview' && step !== 'result') return []
    return csvRows.map((csvRow) => buildRow(csvRow, fields, mapping))
  }, [csvRows, fields, mapping, step])

  const validRows = parsedRows.filter((r) => r.errors.length === 0)
  const invalidCount = parsedRows.length - validRows.length

  async function handleSubmit() {
    setSubmitting(true)
    try {
      const res = await submit(validRows.map((r) => r.row))
      setResult(res)
      setStep('result')
      // An import creates entries wholesale — users, and the groups a user row
      // can name — so nothing cached describes the directory any more. Without
      // this, "Done" returned to a list that still showed the old contents.
      queryClient.invalidateQueries()
    } catch (e) {
      toast.error(e instanceof Error ? e.message : 'Import failed')
    } finally {
      setSubmitting(false)
    }
  }

  return (
    <div className="space-y-4">
      <div className="flex items-center gap-4">
        <Button variant="ghost" size="icon" aria-label="Back" onClick={() => router.navigate({ to: backTo })}>
          <ArrowLeft className="h-4 w-4" />
        </Button>
        <h1 className="text-2xl font-bold">{title}</h1>
      </div>

      <Steps current={step} />

      {step === 'upload' && (
        <Card>
          <CardHeader>
            <CardTitle>Choose a CSV file</CardTitle>
            <CardDescription>
              The first row must be a header. You'll map its columns to fields on the next step.
              Multi-value cells (e.g. groups) are separated by "{MULTI_SEP}".
            </CardDescription>
          </CardHeader>
          <CardContent className="space-y-3">
            <input ref={fileRef} type="file" accept=".csv,text/csv" className="hidden" onChange={handleFile} />
            <div className="flex flex-wrap gap-2">
              <Button onClick={() => fileRef.current?.click()}>
                <Upload className="h-4 w-4 mr-1" />
                Select CSV file
              </Button>
              {/* The column names, the required ones and the ';' convention were
                  only discoverable by uploading something first and reading the
                  mapping screen. A template states them up front. */}
              <Button variant="outline" onClick={() => downloadTemplate(fields, noun)}>
                <Download className="h-4 w-4 mr-1" />
                Download template
              </Button>
            </div>
            <p className="text-xs text-muted-foreground">
              Required column{fields.filter((f) => f.required).length === 1 ? '' : 's'}:{' '}
              {fields.filter((f) => f.required).map((f) => f.label).join(', ')}. Any other column is
              optional, and columns your file does not have can be left unmapped.
            </p>
          </CardContent>
        </Card>
      )}

      {step === 'map' && (
        <Card>
          <CardHeader>
            <CardTitle>Map columns</CardTitle>
            <CardDescription>
              Match each field to a column from your file. {csvRows.length} row{csvRows.length === 1 ? '' : 's'} detected.
            </CardDescription>
          </CardHeader>
          <CardContent className="space-y-3">
            {fields.map((f) => (
              <div key={f.key} className="flex items-center gap-3">
                <label className="w-48 text-sm">
                  {f.label}
                  {f.required && <span className="text-destructive"> *</span>}
                </label>
                <Select
                  className="max-w-xs"
                  value={mapping[f.key] ?? IGNORE}
                  onChange={(e) => setMapping((m) => ({ ...m, [f.key]: e.target.value }))}
                  options={[
                    { value: IGNORE, label: '— ignore —' },
                    ...headers.map((h) => ({ value: h, label: h })),
                  ]}
                />
              </div>
            ))}
            {requiredUnmapped.length > 0 && (
              <p className="text-sm text-destructive">
                Map the required field{requiredUnmapped.length === 1 ? '' : 's'}:{' '}
                {requiredUnmapped.map((f) => f.label).join(', ')}.
              </p>
            )}
            <div className="flex gap-2 pt-2">
              <Button variant="outline" onClick={() => setStep('upload')}>
                Back
              </Button>
              <Button disabled={requiredUnmapped.length > 0} onClick={() => setStep('preview')}>
                Preview
              </Button>
            </div>
          </CardContent>
        </Card>
      )}

      {step === 'preview' && (
        <Card>
          <CardHeader>
            <CardTitle>Preview</CardTitle>
            <CardDescription>
              {validRows.length} valid, {invalidCount} invalid. Invalid rows are skipped on import.
            </CardDescription>
          </CardHeader>
          <CardContent className="space-y-3">
            <div className="border rounded-lg max-h-96 overflow-auto">
              <Table>
                <TableHeader>
                  <TableRow>
                    <TableHead className="w-10">#</TableHead>
                    {fields.map((f) => (
                      <TableHead key={f.key}>{f.label}</TableHead>
                    ))}
                    <TableHead>Status</TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {parsedRows.map((pr, i) => (
                    <TableRow key={i} data-state={pr.errors.length ? 'selected' : undefined}>
                      <TableCell className="text-muted-foreground">{i + 1}</TableCell>
                      {fields.map((f) => (
                        <TableCell key={f.key}>{formatCell(pr.row[f.key])}</TableCell>
                      ))}
                      <TableCell>
                        {pr.errors.length === 0 ? (
                          <span className="text-success">OK</span>
                        ) : (
                          // All of them: the rest used to live in a `title`,
                          // where a row with three problems looked like a row
                          // with one and took three round trips to fix.
                          <ul className="space-y-0.5 text-destructive">
                            {pr.errors.map((error) => (
                              <li key={error}>{error}</li>
                            ))}
                          </ul>
                        )}
                      </TableCell>
                    </TableRow>
                  ))}
                </TableBody>
              </Table>
            </div>
            <div className="flex gap-2">
              <Button variant="outline" onClick={() => setStep('map')}>
                Back
              </Button>
              <Button disabled={validRows.length === 0 || submitting} onClick={handleSubmit}>
                {submitting ? 'Importing...' : `Import ${validRows.length} ${noun}`}
              </Button>
            </div>
          </CardContent>
        </Card>
      )}

      {step === 'result' && result && (
        <Card>
          <CardHeader>
            <CardTitle className="flex items-center gap-2">
              {result.failed === 0 ? (
                <CheckCircle2 className="h-5 w-5 text-success" />
              ) : (
                <XCircle className="h-5 w-5 text-destructive" />
              )}
              Import complete
            </CardTitle>
            <CardDescription>
              {result.created} created, {result.failed} failed.
            </CardDescription>
          </CardHeader>
          <CardContent className="space-y-3">
            {result.failed > 0 && (
              <div className="border rounded-lg max-h-80 overflow-auto">
                <Table>
                  <TableHeader>
                    <TableRow>
                      <TableHead className="w-10">#</TableHead>
                      <TableHead>Key</TableHead>
                      <TableHead>Error</TableHead>
                    </TableRow>
                  </TableHeader>
                  <TableBody>
                    {result.results
                      .filter((r) => r.status === 'error')
                      .map((r) => (
                        <TableRow key={r.index}>
                          <TableCell className="text-muted-foreground">{r.index + 1}</TableCell>
                          <TableCell className="font-medium">{r.key}</TableCell>
                          <TableCell className="text-destructive">{r.error}</TableCell>
                        </TableRow>
                      ))}
                  </TableBody>
                </Table>
              </div>
            )}
            <div className="flex gap-2">
              <Button variant="outline" onClick={() => downloadReport(result, noun)}>
                Download report
              </Button>
              <Button onClick={() => router.navigate({ to: backTo })}>Done</Button>
            </div>
          </CardContent>
        </Card>
      )}
    </div>
  )
}

const STEPS = [
  { key: 'upload', label: 'Choose file' },
  { key: 'map', label: 'Map columns' },
  { key: 'preview', label: 'Check' },
  { key: 'result', label: 'Import' },
] as const

/**
 * Where you are in the import.
 *
 * The four stages already existed as state; nothing showed them, so the screen
 * changed under the operator with no sense of how many steps were left or that
 * the mapping they were about to confirm was not the import itself. Numbered
 * because this genuinely is a sequence — the order carries meaning.
 */
function Steps({ current }: { current: (typeof STEPS)[number]['key'] }) {
  const currentIndex = STEPS.findIndex((s) => s.key === current)
  return (
    <ol className="flex flex-wrap items-center gap-x-2 gap-y-1 text-sm">
      {STEPS.map((step, index) => {
        const done = index < currentIndex
        const active = index === currentIndex
        return (
          <li key={step.key} className="flex items-center gap-2">
            {index > 0 && <span className="text-muted-foreground" aria-hidden="true">→</span>}
            <span
              className={
                active
                  ? 'font-medium text-foreground'
                  : done
                    ? 'text-muted-foreground'
                    : 'text-muted-foreground/60'
              }
              aria-current={active ? 'step' : undefined}
            >
              <span className="tabular-nums">{index + 1}.</span> {step.label}
            </span>
          </li>
        )
      })}
    </ol>
  )
}

/**
 * A CSV with the canonical column names, and a sample row where a field
 * declares one. Required columns are marked in the header so the file is
 * self-describing once it leaves the app.
 */
function downloadTemplate(fields: ImportField[], noun: string) {
  const header = fields.map((f) => f.key)
  const sample = fields.map((f) => f.example ?? '')
  const csv = Papa.unparse([header, sample])
  downloadCsv(csv, `${noun}-import-template.csv`)
}

/** Auto-match target fields to CSV headers by key/label/alias, case-insensitive. */
function autoMap(fields: ImportField[], headers: string[]): Record<string, string> {
  const byLower = new Map(headers.map((h) => [h.toLowerCase(), h]))
  const mapping: Record<string, string> = {}
  for (const f of fields) {
    const candidates = [f.key, f.label, ...(f.aliases ?? [])].map((c) => c.toLowerCase())
    for (const c of candidates) {
      const match = byLower.get(c)
      if (match) {
        mapping[f.key] = match
        break
      }
    }
  }
  return mapping
}

function buildRow(csvRow: Record<string, string>, fields: ImportField[], mapping: Record<string, string>): ParsedRow {
  const row: ImportRow = {}
  const errors: string[] = []
  for (const f of fields) {
    const header = mapping[f.key]
    if (!header || header === IGNORE) {
      if (f.required) errors.push(`${f.label} is required`)
      continue
    }
    const raw = (csvRow[header] ?? '').trim()
    if (raw === '') {
      if (f.required) errors.push(`${f.label} is required`)
      continue
    }
    if (f.type === 'number') {
      const n = Number(raw)
      if (!Number.isInteger(n)) {
        errors.push(`${f.label} must be a whole number`)
        continue
      }
      row[f.key] = n
    } else if (f.type === 'stringArray') {
      row[f.key] = raw.split(MULTI_SEP).map((s) => s.trim()).filter(Boolean)
    } else {
      if (f.pattern && !f.pattern.test(raw)) {
        errors.push(`${f.label} ${f.patternHint ?? 'has invalid characters'}`)
        continue
      }
      row[f.key] = raw
    }
  }
  return { row, errors }
}

function formatCell(value: unknown): string {
  if (Array.isArray(value)) return value.join(', ')
  if (value === undefined || value === null) return ''
  return String(value)
}

function downloadReport(result: ImportResult, noun: string) {
  const csv = Papa.unparse(
    result.results.map((r) => ({ row: r.index + 1, key: r.key, status: r.status, error: r.error ?? '' })),
  )
  downloadCsv(csv, `${noun}-import-report.csv`)
}

function downloadCsv(csv: string, filename: string) {
  const blob = new Blob([csv], { type: 'text/csv' })
  const url = URL.createObjectURL(blob)
  const a = document.createElement('a')
  a.href = url
  a.download = filename
  a.click()
  URL.revokeObjectURL(url)
}
