APP := ariranha
DIST := dist
RAYLIB_VERSION := 5.5
RAYLIB_DLL_URL := https://github.com/raysan5/raylib/releases/download/$(RAYLIB_VERSION)/raylib-$(RAYLIB_VERSION)_win64_mingw-w64.zip

ORT_VERSION := 1.29.0
ORT_URL := https://github.com/microsoft/onnxruntime/releases/download/v$(ORT_VERSION)

MODEL_URL := https://huggingface.co/pulsejet/siglip-base-patch16-256-multilingual-onnx/resolve/main
MODELS := models/vision_model_quantized.onnx models/text_model_quantized.onnx models/tokenizer.json

FFMPEG_VERSION := 8.1
FFMPEG_URL := https://github.com/BtbN/FFmpeg-Builds/releases/download/latest
FFMPEG_LINUX := lib/linux-amd64/ffmpeg lib/linux-amd64/ffprobe
FFMPEG_WINDOWS := lib/windows-amd64/ffmpeg.exe lib/windows-amd64/ffprobe.exe

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

.PHONY: run setup models ffmpeg linux windows darwin-arm64 all clean

run: linux
	cd $(DIST)/linux-amd64 && ./$(APP)

setup: models $(ORT_LINUX) $(FFMPEG_LINUX)

ffmpeg: $(FFMPEG_LINUX) $(FFMPEG_WINDOWS)

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

$(FFMPEG_LINUX) &:
	mkdir -p lib/linux-amd64 lib/tmp
	curl -fL --progress-bar $(FFMPEG_URL)/ffmpeg-n$(FFMPEG_VERSION)-latest-linux64-lgpl-$(FFMPEG_VERSION).tar.xz | tar -xJ -C lib/tmp --wildcards '*/bin/ffmpeg' '*/bin/ffprobe'
	cp lib/tmp/*/bin/ffmpeg lib/tmp/*/bin/ffprobe lib/linux-amd64/
	rm -rf lib/tmp

$(FFMPEG_WINDOWS) &:
	mkdir -p lib/windows-amd64 lib/tmp
	curl -fL --progress-bar -o lib/tmp/ffmpeg.zip $(FFMPEG_URL)/ffmpeg-n$(FFMPEG_VERSION)-latest-win64-lgpl-$(FFMPEG_VERSION).zip
	unzip -o -j lib/tmp/ffmpeg.zip '*/bin/ffmpeg.exe' '*/bin/ffprobe.exe' -d lib/windows-amd64
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

linux: $(ORT_LINUX) $(FFMPEG_LINUX) $(PACKAGED_MODELS)
	CGO_ENABLED=1 GOOS=linux GOARCH=amd64 go build -ldflags="-s -w" -o $(DIST)/linux-amd64/$(APP) .
	$(call layout,$(DIST)/linux-amd64,$(ORT_LINUX) $(FFMPEG_LINUX))

windows: $(DIST)/windows-amd64/raylib.dll $(ORT_WINDOWS) $(FFMPEG_WINDOWS) $(PACKAGED_MODELS)
	CGO_ENABLED=$(WINDOWS_CGO) GOOS=windows GOARCH=amd64 go build -ldflags="-s -w -H=windowsgui" -o $(DIST)/windows-amd64/$(APP).exe .
	$(call layout,$(DIST)/windows-amd64,$(ORT_WINDOWS) $(FFMPEG_WINDOWS))

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
