package cgo

/*
#cgo CFLAGS: -I${SRCDIR}/../../../third_party/onnx_c++/src/api
#cgo LDFLAGS: -L${SRCDIR}/../../../third_party/onnx_c++/build-debug/src -ledge_sentinel_lib -lonnxruntime -lopencv_core -lopencv_imgproc -lopencv_imgcodecs -lstdc++ -lm
#include "bridge.h"
#include <stdlib.h>
*/
import "C"
import (
	"errors"
	"fmt"
	"unsafe"

	"rag_imagetotext_texttoimage/internal/application/ports"
	"rag_imagetotext_texttoimage/internal/util"
)

var _ ports.Inference = (*JinaAdapter)(nil)

type JinaAdapter struct {
	handle C.JinaHandle
	logger util.Logger
}

func NewJinaAdapter(configPath string, appLogger util.Logger) (*JinaAdapter, error) {
	cConfigPath := C.CString(configPath)
	defer C.free(unsafe.Pointer(cConfigPath))

	h := C.jina_init(cConfigPath)
	if h == nil {
		appLogger.Error("[internal.infra.cgo.JinaAdapter.NewJinaAdapter] failed due to: ", errors.New("cgo initialization error"))
		return nil, errors.New("cgo initialization error")
	}

	appLogger.Info("[internal.infra.cgo.JinaAdapter.NewJinaAdapter] Jina adapter initialized successfully with config: ", configPath)

	return &JinaAdapter{handle: h, logger: appLogger}, nil
}

func (a *JinaAdapter) Close() {
	if a.handle != nil {
		C.jina_release(a.handle)
		a.handle = nil
	}
	a.logger.Info("[internal.infra.cgo.JinaAdapter.Close] Jina adapter resources released")
}

func (a *JinaAdapter) EmbedText(text string) ([]float32, error) {
	cText := C.CString(text)
	defer C.free(unsafe.Pointer(cText))

	embedding := make([]float32, 768)
	a.logger.Info("[internal.infra.cgo.JinaAdapter.EmbedText] Embedding text: ", text)
	res := C.jina_embed_text(a.handle, cText, (*C.float)(unsafe.Pointer(&embedding[0])))
	if res != 0 {
		a.logger.Error("[internal.infra.cgo.JinaAdapter.EmbedText] failed due to: ", fmt.Errorf("cgo error in EmbedText: %d", res))
		return nil, fmt.Errorf("cgo error in EmbedText: %d", res)
	}
	a.logger.Info("[internal.infra.cgo.JinaAdapter.EmbedText] Successfully embedded text: ", text)

	return embedding, nil
}

func (a *JinaAdapter) EmbedBatchText(texts []string) ([][]float32, error) {
	count := len(texts)
	if count == 0 {
		return nil, nil
	}

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
	res := C.jina_embed_batch_text(a.handle, (**C.char)(cArray), C.int(count), (*C.float)(unsafe.Pointer(&flatResults[0])))
	if res != 0 {
		a.logger.Error("[internal.infra.cgo.JinaAdapter.EmbedBatchText] failed due to: ", fmt.Errorf("cgo error in EmbedBatchText: %d", res))
		return nil, fmt.Errorf("cgo error in EmbedBatchText: %d", res)
	}
	a.logger.Info("[internal.infra.cgo.JinaAdapter.EmbedBatchText] Successfully embedded batch text")

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

	embedding := make([]float32, 768)
	res := C.jina_embed_image(
		a.handle,
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
		a.handle,
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
