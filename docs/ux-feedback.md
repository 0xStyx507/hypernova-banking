# Politica de avisos de interfaz

Los avisos deben ayudar a decidir el siguiente paso sin interrumpir innecesariamente al usuario.

## Niveles

- `error`: la accion no se completo o el usuario debe corregir algo. Usa `role="alert"` y lenguaje accionable.
- `success`: la accion se completo. Usa `role="status"`; no debe comportarse como una interrupcion urgente.
- `warning`: existe una condicion que requiere atencion antes de continuar, como una operacion pendiente.
- `info`: contexto operativo, instrucciones o estado de carga.

## Reglas

1. Los errores de un campo deben aparecer junto al campo y conservar el foco en el control que necesita correccion.
2. Los resultados de transacciones deben indicar resultado, siguiente accion e impacto visible en historial o saldo.
3. La expiracion del PIN es un error accionable: informar que vencio y enlazar o dirigir a Configuraciones.
4. No usar `role="alert"` para mensajes exitosos o textos informativos.
5. No mostrar mensajes tecnicos, tokens, hashes ni detalles internos del API.
6. Los mensajes globales deben permanecer el tiempo suficiente para ser leidos y no reemplazar una confirmacion permanente cuando exista un comprobante.

Los componentes compartidos viven en `web/src/components/feedback` y `mobile/src/components/feedback`.
