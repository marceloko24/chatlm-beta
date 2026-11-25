# ChatLM Beta

Versión de **prueba** de ChatLM, con Qwen3-0.6B y el modo sin pensamiento. Corre el modelo **en el dispositivo** (yzma + llama.cpp en WebAssembly).

**No es la versión estable.** La app estable sigue en [https://marceloko24.github.io/chatlm/](https://marceloko24.github.io/chatlm/) y el código estable en [github.com/marceloko24/chatlm](https://github.com/marceloko24/chatlm). Este repositorio no publica ni modifica esa dirección.

Las conversaciones no se envían a ningún servidor. El sitio solo entrega los archivos de la app. No hay APIs de IA externas, ni analítica, ni telemetría.

## Probarla en el iPhone (Safari)

**https://marceloko24.github.io/chatlm-beta/**

1. Abrila en **Safari** (no en una pestaña privada).
2. Tocá **Compartir** → **Agregar a inicio**. Tiene que aparecer como **ChatLM Beta**, distinta de la app estable.
3. Con wifi, descargá el modelo (378,3 MB):  
   https://huggingface.co/unsloth/Qwen3-0.6B-GGUF/resolve/main/Qwen3-0.6B-Q4_K_M.gguf
4. En ChatLM Beta, tocá **Elegí un archivo GGUF** e importá ese archivo desde Archivos. El modelo no se sube: queda en el teléfono, en una carpeta OPFS distinta (`chatlm-beta`), y **no pisa** el Qwen2.5-0.5B de la app estable.
5. Cuando termine de cargar, escribí un mensaje en español y anotá si anda, los tokens/s y si al cerrar y reabrir sigue el modelo.

El tope de memoria WASM sigue en 1 GiB. El archivo GGUF **no** está en este repositorio.

## Qué cambia respecto de la estable

- Rama de origen: `qwen3-nothink` de ChatLM (modo sin pensamiento para Qwen3).
- Nombre de la PWA, `id`, `start_url`, `scope` y caché del service worker: `ChatLM Beta` / `/chatlm-beta/` / `chatlm-beta-v1`.
- El service worker de la beta solo borra cachés que empiezan con `chatlm-beta-`; no toca la caché `chatlm-v3` de la app estable.
- OPFS: carpeta `chatlm-beta`, no `chatlm`.

## Desarrollo local

Necesitás Go 1.26+, TinyGo 0.41+ y Python 3.

```
python scripts/generate_icons.py
go install github.com/hybridgroup/yzma@v1.33.0
yzma install -lib src/wasm -os wasm -version v1.0.0
python scripts/patch_wasm_memory.py src/wasm/yzma_wasm.wasm
rm -f src/wasm/yzma_wasm_mt.* src/wasm/yzma_wasm_webgpu.*
make dist
python3 -m http.server 4175 --directory dist --bind 127.0.0.1
```

`scripts/patch_wasm_memory.py` deja el tope en 1 GiB. No subas modelos GGUF al repositorio.

## Licencia

Apache 2.0, igual que yzma. Ver `LICENSE`.
