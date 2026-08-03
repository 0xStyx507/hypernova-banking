interface HistoryPaginationProps {
  page: number;
  hasMore: boolean;
  busy: boolean;
  onPrevious: () => void;
  onNext: () => void;
}

/** Cursor pagination controls; it intentionally does not invent a total. */
export function HistoryPagination({ page, hasMore, busy, onPrevious, onNext }: HistoryPaginationProps) {
  return (
    <div className="pagination-controls" aria-label="Paginación del historial">
      <span>Página {page} · 5 movimientos por página</span>
      <div className="flex gap-2">
        <button className="secondary-button" disabled={page <= 1 || busy} onClick={onPrevious} type="button">Anterior</button>
        <button className="secondary-button" disabled={!hasMore || busy} onClick={onNext} type="button">{busy ? "Cargando…" : "Siguiente"}</button>
      </div>
    </div>
  );
}
