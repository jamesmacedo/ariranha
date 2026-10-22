APP := ariranha
DIST := dist
RAYLIB_VERSION := 5.5
RAYLIB_DLL_URL := https://github.com/raysan5/raylib/releases/download/$(RAYLIB_VERSION)/raylib-$(RAYLIB_VERSION)_win64_mingw-w64.zip

ORT_VERSION := 1.29.0
ORT_URL := https://github.com/microsoft/onnxruntime/releases/download/v$(ORT_VERSION)

MODEL_URL := https://huggingface.co/pulsejet/siglip-base-patch16-256-multilingual-onnx/resolve/main
MODELS := models/vision_model_quantized.onnx models/text_model_quantized.onnx models/tokenizer.json

BUFFALO_URL := https://github.com/deepinsight/insightface/releases/download/v0.7/buffalo_l.zip
FACE_MODELS := models/det_10g.onnx models/w600k_r50.onnx

WINDOWS_CGO ?= 0

ORT_LINUX := lib/linux-amd64/libonnxruntime.so
ORT_WINDOWS := lib/windows-amd64/onnxruntime.dll
ORT_DARWIN := lib/darwin-arm64/libonnxruntime.dylib

PACKAGED_MODELS := $(MODELS) $(FACE_MODELS)

define layout
	mkdir -p $(1)/storage $(1)/system/models $(1)/system/runtime $(1)/system/db $(1)/system/cache $(1)/system/logs $(1)/system/trash
	for f in $(PACKAGED_MODELS); do cp -lf $$f $(1)/system/models/ 2>/dev/null || cp -f $$f $(1)/system/models/; done
	cp -f $(2) $(1)/system/runtime/
	rm -f $(1)/libonnxruntime.so $(1)/onnxruntime.dll $(1)/libonnxruntime.dylib
endef

.PHONY: run setup models linux windows darwin-arm64 all clean

run: linux
	cd $(DIST)/linux-amd64 && ./$(APP)

setup: models $(ORT_LINUX)

models: $(PACKAGED_MODELS)

$(FACE_MODELS):
	mkdir -p models/tmp
	curl -fL --progress-bar -o models/tmp/buffalo_l.zip $(BUFFALO_URL)
	unzip -o -j models/tmp/buffalo_l.zip det_10g.onnx w600k_r50.onnx -d models
	rm -rf models/tmp

models/%.onnx:
	mkdir -p models
	curl -fL --progress-bar -o $@.part $(MODEL_URL)/onnx/$*.onnx
	mv $@.part $@

models/tokenizer.json:
	mkdir -p models
	curl -fsSL -o $@.part $(MODEL_URL)/tokenizer.json
	mv $@.part $@

$(ORT_LINUX):
	mkdir -p lib/linux-amd64 lib/tmp
	curl -fsSL $(ORT_URL)/onnxruntime-linux-x64-$(ORT_VERSION).tgz | tar -xz -C lib/tmp
	cp lib/tmp/onnxruntime-linux-x64-$(ORT_VERSION)/lib/libonnxruntime.so.$(ORT_VERSION) $@
	rm -rf lib/tmp

$(ORT_WINDOWS):
	mkdir -p lib/windows-amd64 lib/tmp
	curl -fsSL -o lib/tmp/ort.zip $(ORT_URL)/onnxruntime-win-x64-$(ORT_VERSION).zip
	unzip -o -j lib/tmp/ort.zip '*/lib/onnxruntime.dll' -d lib/windows-amd64
	rm -rf lib/tmp

$(ORT_DARWIN):
	mkdir -p lib/darwin-arm64 lib/tmp
	curl -fsSL $(ORT_URL)/onnxruntime-osx-arm64-$(ORT_VERSION).tgz | tar -xz -C lib/tmp
	cp lib/tmp/onnxruntime-osx-arm64-$(ORT_VERSION)/lib/libonnxruntime.$(ORT_VERSION).dylib $@
	rm -rf lib/tmp

linux: $(ORT_LINUX) $(PACKAGED_MODELS)
	CGO_ENABLED=1 GOOS=linux GOARCH=amd64 go build -ldflags="-s -w" -o $(DIST)/linux-amd64/$(APP) .
	$(call layout,$(DIST)/linux-amd64,$(ORT_LINUX))

windows: $(DIST)/windows-amd64/raylib.dll $(ORT_WINDOWS) $(PACKAGED_MODELS)
	CGO_ENABLED=$(WINDOWS_CGO) GOOS=windows GOARCH=amd64 go build -ldflags="-s -w -H=windowsgui" -o $(DIST)/windows-amd64/$(APP).exe .
	$(call layout,$(DIST)/windows-amd64,$(ORT_WINDOWS))

$(DIST)/windows-amd64/raylib.dll:
	mkdir -p $(DIST)/windows-amd64 $(DIST)/tmp
	curl -sSL -o $(DIST)/tmp/raylib.zip $(RAYLIB_DLL_URL)
	unzip -o -j $(DIST)/tmp/raylib.zip '*/lib/raylib.dll' -d $(DIST)/windows-amd64
	rm -rf $(DIST)/tmp

darwin-arm64: $(ORT_DARWIN) $(PACKAGED_MODELS)
	CGO_ENABLED=1 GOOS=darwin GOARCH=arm64 go build -ldflags="-s -w" -o $(DIST)/darwin-arm64/$(APP) .
	$(call layout,$(DIST)/darwin-arm64,$(ORT_DARWIN))

all: linux windows

clean:
	find $(DIST) -mindepth 2 -maxdepth 2 ! -name storage -exec rm -rf {} +
