package cgo

/*
#cgo CFLAGS: -I${SRCDIR}/../../../third_party/onnx_c++/src/api
#cgo LDFLAGS: -L${SRCDIR}/../../../third_party/onnx_c++/build/src -L${SRCDIR}/../../../third_party/onnx_c++/build/_deps/onnxruntime_prebuilt-src/lib -L${SRCDIR}/../../../third_party/onnx_c++/build/_deps/yaml-cpp-build -L${SRCDIR}/../../../third_party/onnx_c++/build-debug/src -L${SRCDIR}/../../../third_party/onnx_c++/build-debug/_deps/onnxruntime_prebuilt-src/lib -L${SRCDIR}/../../../third_party/onnx_c++/build-debug/_deps/yaml-cpp-build -Wl,-rpath,${SRCDIR}/../../../third_party/onnx_c++/build/_deps/onnxruntime_prebuilt-src/lib -Wl,-rpath,${SRCDIR}/../../../third_party/onnx_c++/build-debug/_deps/onnxruntime_prebuilt-src/lib -ledge_sentinel_lib -lyaml-cpp -lonnxruntime -lopencv_core -lopencv_imgproc -lopencv_imgcodecs -lstdc++ -lm
#include "bridge.h"
#include <stdlib.h>
*/
import "C"
import (
	"errors"
	"fmt"
	"strings"
	"sync"
	"unsafe"

	"rag_imagetotext_texttoimage/internal/application/ports"
	"rag_imagetotext_texttoimage/internal/util"
)

var _ ports.Inference = (*JinaAdapter)(nil)

type JinaAdapter struct {
	handle C.JinaHandle
	logger util.Logger
	mu     sync.RWMutex
	closed bool
}

func NewJinaAdapter(configPath string, appLogger util.Logger) (*JinaAdapter, error) {
	cConfigPath := C.CString(configPath)
	defer C.free(unsafe.Pointer(cConfigPath))

	h := C.jina_init(cConfigPath)
	if h == nil {
		lastErr := strings.TrimSpace(C.GoString(C.jina_last_error()))
		if lastErr == "" {
			lastErr = "unknown native initialization failure"
		}
		err := fmt.Errorf("cgo initialization error: %s", lastErr)
		appLogger.Error("jina adapter initialization failed", err, "config_path", configPath)
		return nil, err
	}

	appLogger.Info("jina adapter initialized successfully", "config_path", configPath)

	return &JinaAdapter{handle: h, logger: appLogger}, nil
}

func (a *JinaAdapter) Close() {
	a.mu.Lock()
	defer a.mu.Unlock()

	if a.closed {
		return
	}
	if a.handle != nil {
		C.jina_release(a.handle)
		a.handle = nil
	}
	a.closed = true
	if a.logger != nil {
		a.logger.Info("jina adapter resources released")
	}
}

func (a *JinaAdapter) EmbedText(text string) ([]float32, error) {
	a.mu.RLock()
	if a.closed || a.handle == nil {
		a.mu.RUnlock()
		return nil, errors.New("jina adapter is closed")
	}
	handle := a.handle
	defer a.mu.RUnlock()

	cText := C.CString(text)
	defer C.free(unsafe.Pointer(cText))

	embedding := make([]float32, 768)
	a.logger.Debug("embedding text started", "text_len", len(text))
	res := C.jina_embed_text(handle, cText, (*C.float)(unsafe.Pointer(&embedding[0])))
	if res != 0 {
		a.logger.Error("embedding text failed", fmt.Errorf("cgo error in embed_text: %d", res), "text_len", len(text))
		return nil, fmt.Errorf("cgo error in embed_text: %d", res)
	}
	a.logger.Debug("embedding text success", "text_len", len(text), "dimension", len(embedding))

	return embedding, nil
}

func (a *JinaAdapter) EmbedBatchText(texts []string) ([][]float32, error) {
	count := len(texts)
	if count == 0 {
		return nil, nil
	}

	a.mu.RLock()
	if a.closed || a.handle == nil {
		a.mu.RUnlock()
		return nil, errors.New("jina adapter is closed")
	}
	handle := a.handle
	defer a.mu.RUnlock()

	// 1. Prepare C array of strings
	cArray := C.malloc(C.size_t(uintptr(count) * unsafe.Sizeof(uintptr(0))))
	defer C.free(cArray)

	cPtrs := (*[1 << 28]*C.char)(cArray)
	for i, s := range texts {
		cStr := C.CString(s)
		defer C.free(unsafe.Pointer(cStr))
		cPtrs[i] = cStr
	}

	// 2. Prepare flat result buffer [count * 768]
	flatResults := make([]float32, count*768)

	// 3. Call Bridge
	res := C.jina_embed_batch_text(handle, (**C.char)(cArray), C.int(count), (*C.float)(unsafe.Pointer(&flatResults[0])))
	if res != 0 {
		a.logger.Error("embedding batch text failed", fmt.Errorf("cgo error in embed_batch_text: %d", res), "count", count)
		return nil, fmt.Errorf("cgo error in embed_batch_text: %d", res)
	}
	a.logger.Debug("embedding batch text success", "count", count, "dimension", 768)

	// 4. Reshape flat buffer to [][]float32
	results := make([][]float32, count)
	for i := 0; i < count; i++ {
		results[i] = make([]float32, 768)
		copy(results[i], flatResults[i*768:(i+1)*768])
	}

	return results, nil
}

func (a *JinaAdapter) EmbedImage(pixels []byte, width, height, channels int) ([]float32, error) {
	if len(pixels) == 0 {
		return nil, fmt.Errorf("empty image data")
	}

	a.mu.RLock()
	if a.closed || a.handle == nil {
		a.mu.RUnlock()
		return nil, errors.New("jina adapter is closed")
	}
	handle := a.handle
	defer a.mu.RUnlock()

	embedding := make([]float32, 768)
	res := C.jina_embed_image(
		handle,
		(*C.uint8_t)(unsafe.Pointer(&pixels[0])),
		C.int(width),
		C.int(height),
		C.int(channels),
		(*C.float)(unsafe.Pointer(&embedding[0])),
	)

	if res != 0 {
		return nil, fmt.Errorf("cgo error in EmbedImage: %d", res)
	}

	return embedding, nil
}

func (a *JinaAdapter) EmbedBatchImage(images [][]byte, width, height, channels int) ([][]float32, error) {
	count := len(images)
	if count == 0 {
		return nil, nil
	}

	a.mu.RLock()
	if a.closed || a.handle == nil {
		a.mu.RUnlock()
		return nil, errors.New("jina adapter is closed")
	}
	handle := a.handle
	defer a.mu.RUnlock()

	// 1. Prepare flat image buffer
	frameSize := width * height * channels
	flatImages := make([]byte, count*frameSize)
	for i, img := range images {
		if len(img) != frameSize {
			return nil, fmt.Errorf("image at index %d has wrong size: expected %d, got %d", i, frameSize, len(img))
		}
		copy(flatImages[i*frameSize:(i+1)*frameSize], img)
	}

	// 2. Prepare flat result buffer
	flatResults := make([]float32, count*768)

	// 3. Call Bridge
	res := C.jina_embed_batch_image(
		handle,
		(*C.uint8_t)(unsafe.Pointer(&flatImages[0])),
		C.int(count),
		C.int(width),
		C.int(height),
		C.int(channels),
		(*C.float)(unsafe.Pointer(&flatResults[0])),
	)

	if res != 0 {
		return nil, fmt.Errorf("cgo error in EmbedBatchImage: %d", res)
	}

	// 4. Reshape
	results := make([][]float32, count)
	for i := 0; i < count; i++ {
		results[i] = make([]float32, 768)
		copy(results[i], flatResults[i*768:(i+1)*768])
	}

	return results, nil
}
