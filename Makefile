# Build the ChatLM PWA into dist/.
# TinyGo 0.41+ and Go 1.26+ are required for the chat program.

BUILD_DIR ?= dist
SRC_DIR := src

.PHONY: all wasm assets dist clean serve

all: dist

wasm:
	tinygo build -target wasm -opt=2 -no-debug -stack-size=128KB -o $(SRC_DIR)/wasm/yzma.wasm .
	cp "$(shell tinygo env TINYGOROOT)/targets/wasm_exec.js" $(SRC_DIR)/wasm/wasm_exec.js

assets:
	mkdir -p $(BUILD_DIR)/wasm $(BUILD_DIR)/icons
	cp $(SRC_DIR)/index.html $(SRC_DIR)/styles.css $(SRC_DIR)/app.js $(SRC_DIR)/worker.js $(SRC_DIR)/sw.js $(SRC_DIR)/manifest.json $(BUILD_DIR)/
	cp $(SRC_DIR)/wasm/yzma.wasm $(SRC_DIR)/wasm/yzma_wasm.wasm $(SRC_DIR)/wasm/yzma_wasm.js $(SRC_DIR)/wasm/wasm_exec.js $(BUILD_DIR)/wasm/
	cp $(SRC_DIR)/icons/* $(BUILD_DIR)/icons/

dist: wasm assets

serve: dist
	python3 -m http.server 4173 --directory $(BUILD_DIR) --bind 127.0.0.1

clean:
	rm -rf $(BUILD_DIR)
