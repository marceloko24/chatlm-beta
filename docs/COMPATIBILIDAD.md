# ChatLM — Compatibilidad, límites y qué está verificado

Informe actualizado el 25 de noviembre de 2026, hora de Uruguay (UTC-3).

## Cómo está armada

- Motor: yzma v1.33.0 + llama.cpp v1.0.0 en WebAssembly, **solo** el build de CPU de un hilo (sin WebGPU y sin hilos / SharedArrayBuffer). El wasm de llama.cpp **ya incluye SIMD** (opcodes `v128` / `f32x4`); no hizo falta recompilarlo. Safari soporta wasm SIMD desde iOS 16.4.
- Programa de chat: Go compilado con TinyGo 0.44, `-opt=2 -no-debug`, contexto 512, lote 512, hasta 128 tokens por respuesta. Entre tokens se cede el event loop con `time.Sleep` (camino `sleepTicks` / `go_scheduler` de TinyGo). No se usa `setTimeout` + `js.FuncOf` + `resume()`, porque eso choca con el scheduler **asyncify** y termina en «Unreachable code should not be executed (evaluating 'this._inst.exports.resume()')».
- Memoria WASM de llama.cpp: inicial 128 MB, máximo declarado **1 GB** (no 4 GB), con crecimiento.
- El modelo vive en OPFS y se copia al MEMFS del motor en tiras de 4 MB, sin meter el archivo entero en un solo ArrayBuffer.
- Si el runtime de Go se traba, el worker se recrea y, si hay un modelo guardado, se vuelve a cargar. El mensaje es en español; no se muestra el texto crudo de `resume()`.
- La etiqueta «qwen2 1B» venía de `llama_model_desc` (un balde de llama.cpp, no un parseo nuestro). Ahora se usa `ModelNParams` y se muestra **qwen2 0.5B**.
- El sitio no pide encabezados COOP/COEP. Un service worker guarda la cáscara para usar la app sin red, una vez instalada.
- No hay llamadas a APIs de IA, ni analítica, ni CDN de fuentes o scripts.

## Pruebas en el iPhone SE 2020 (Safari, PWA desde Inicio)

Fecha: 25 de noviembre de 2026, sobre https://marceloko24.github.io/chatlm/ **antes** de esta corrección.

- Verificado: importar el GGUF, persistir en OPFS (al reabrir aparecía «Hay un modelo guardado… Qwen2.5-0.5B-Instruct-Q4_K_M.gguf (379.4 MB)»), primera respuesta en español (22 tokens, 0,5 tokens/s), encabezado del motor.
- Error reproducido en el teléfono: al reabrir / usar «Cargar el modelo guardado» o en una segunda generación, el estado quedaba en rojo con el texto crudo de `resume()`. A veces igual aparecía una respuesta.
- **La corrección de ese error todavía no se reconfirmó en el iPhone.**

## Pruebas en esta computadora (Debian 13, Playwright 1.65.1)

Modelo: `Qwen2.5-0.5B-Instruct-Q4_K_M.gguf` (397,8 MB). No se publicó el GGUF.

### Chromium 159 — verificado (después de la corrección)

- Importación, OPFS, recarga, «Cargar el modelo guardado» dos veces, dos generaciones seguidas, Nueva charla, Parar.
- Etiqueta: `qwen2 0.5B Q4_K - Medium, CPU, 1 hilo, contexto 512`.
- Velocidad (misma máquina, mismo prompt «Hola, respondé en español. ¿Qué podés hacer?»):
  - Antes del cambio: 31 tokens, **1,4 tokens/s** (primera respuesta, ~10:33 hora de Uruguay).
  - Después: 71 tokens, **2,1 tokens/s** en una corrida; otras corridas más cortas dieron ~0,8–1,4 tokens/s porque el temporizador incluye el prefill del prompt. No hay una mejora estable y aislada atribuible a `-opt=2` o al lote 512.
- Mensaje amigable + recarga automática del modelo ante una trampa `resume()` simulada. El error de memoria simulado no congela la UI.

### Playwright WebKit 27.2 (Linux) — parcial

- No es Safari de iOS. OPFS sigue sin funcionar en este entorno; la app carga el GGUF solo para la sesión.
- Verificado: dos generaciones, Parar, etiqueta 0.5B, mensaje amigable ante la trampa simulada. Sin auto-recuperación porque no hay modelo persistido.

## Funciones, una por una

| Función | Estado |
| --- | --- |
| Interfaz oscura en español, nombre ChatLM | Verificado en Chromium, WebKit Linux e iPhone SE 2020 |
| Importar GGUF (selector de archivos, sin MIME restrictivo) | Verificado en Chromium, WebKit Linux y en el iPhone (selector de iOS) |
| Compatible con Qwen2.5-0.5B-Instruct-Q4_K_M.gguf | Verificado (carga y genera) en Chromium, WebKit Linux y iPhone (primera respuesta) |
| Guardar en OPFS por bloques | Verificado en Chromium y en el iPhone SE 2020. En WebKit Linux, no usable |
| Cargar el modelo desde el almacenamiento local | Verificado en Chromium y en el iPhone (el modelo persistió al reabrir). La corrección del error al recargar **no** está reconfirmada en el iPhone |
| Inferencia solo en el dispositivo | Verificado en el diseño y en las pruebas; el hosting solo sirve la app |
| Indicador de carga, respuesta en vivo, tokens/s | Verificado en Chromium, WebKit Linux e iPhone (primera respuesta: 0,5 tokens/s) |
| Botón Parar | Verificado en Chromium y WebKit Linux. En el iPhone, no se informó una prueba de Parar |
| Segunda generación / recargar modelo sin trampa `resume()` | Verificado en Chromium y WebKit Linux después de la corrección. En el iPhone, **no reconfirmado** (ahí se vio el bug en la versión anterior) |
| Etiqueta del modelo 0.5B (no «1B») | Verificado en Chromium y WebKit Linux. En el iPhone, la versión previa mostraba «qwen2 1B» (texto de llama.cpp) |
| PWA, HTTPS, offline en escritorio | Verificado en la URL pública (Chromium y WebKit). Offline real en el iPhone: no se detalló en la prueba física |
| Manejo de error de memoria / trampa del motor sin congelar la UI | Verificado en escritorio (simulado). Un corte real de pestaña en iOS no se puede simular acá |
| Safari de iOS / iPhone SE 2020 | Parcial: importar, OPFS y la primera respuesta sí. Recarga / segunda generación: pendiente de reconfirmar con esta corrección |

## Riesgos en el iPhone SE 2020

- 3 GB de RAM. Safari puede cerrar la pestaña al cargar un GGUF de ~400 MB más el motor.
- Si no se agrega a Inicio, Safari puede borrar el modelo a los 7 días sin uso.
- El modelo de 0,5B a veces responde corto o se va de tema.
- En el teléfono se midió ~0,5 tokens/s; en escritorio, alrededor de 1–2 tokens/s en la primera respuesta. La segunda suele ser más lenta porque el temporizador incluye el prompt acumulado.
