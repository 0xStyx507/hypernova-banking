import { useEffect, useState } from "react";

type ApiStatus = "loading" | "ready" | "offline";

// Docker serves the web app behind /api; Vite can override this for local
// development without changing the component code.
const apiBaseUrl = import.meta.env.VITE_API_BASE_URL ?? "/api";

export default function App() {
  const [apiStatus, setApiStatus] = useState<ApiStatus>("loading");

  useEffect(() => {
    const controller = new AbortController();
    fetch(`${apiBaseUrl}/v1/health`, { signal: controller.signal })
      .then((response) => {
        if (!response.ok) throw new Error("api unavailable");
        setApiStatus("ready");
      })
      .catch(() => setApiStatus("offline"));

    // Prevent a late response from updating state after the view unmounts.
    return () => controller.abort();
  }, []);

  return (
    <main className="min-h-screen bg-cream px-5 py-6 text-ink sm:px-10 sm:py-10">
      <div className="mx-auto flex max-w-6xl flex-col gap-12">
        <header className="flex flex-col gap-5 sm:flex-row sm:items-center sm:justify-between">
          <div>
            <p className="text-sm font-semibold uppercase tracking-[0.25em] text-slate-500">Hypernova</p>
            <h1 className="mt-2 text-4xl font-semibold tracking-tight sm:text-6xl">Tu dinero, claro.</h1>
          </div>
          <span className="w-fit rounded-full bg-white px-4 py-2 text-sm shadow-sm">
            API: {apiStatus === "loading" ? "conectando…" : apiStatus === "ready" ? "operativa" : "sin conexión"}
          </span>
        </header>

        <section className="grid gap-5 md:grid-cols-[1.3fr_0.7fr]">
          <div className="rounded-3xl bg-ink p-7 text-white shadow-xl sm:p-10">
            <p className="text-sm text-slate-300">Balance disponible</p>
            <p className="mt-5 text-5xl font-semibold tracking-tight">$0.00</p>
            <p className="mt-3 text-sm text-slate-300">Tu cuenta estará lista cuando completes el registro.</p>
            <button className="mt-8 rounded-full bg-mint px-5 py-3 text-sm font-semibold text-ink transition hover:brightness-95">
              Crear cuenta
            </button>
          </div>

          <div className="rounded-3xl border border-slate-200 bg-white p-7 shadow-sm sm:p-10">
            <p className="text-sm font-semibold uppercase tracking-[0.2em] text-slate-400">Próximamente</p>
            <h2 className="mt-4 text-2xl font-semibold">Una banca sencilla y verificable.</h2>
            <p className="mt-4 text-slate-600">Transferencias con confirmación explícita, historial transparente y soporte inteligente.</p>
          </div>
        </section>

        <footer className="border-t border-slate-200 pt-5 text-sm text-slate-500">
          Fase 0 · Entorno demostrable en construcción
        </footer>
      </div>
    </main>
  );
}
