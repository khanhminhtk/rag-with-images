#ifndef BRIDGE_H
#define BRIDGE_H

#ifdef __cplusplus
extern "C" {
#endif

#include <stdint.h>

// Handle representing the Inference Engine (DIContainer)
typedef void* JinaHandle;

// Initialization and Release
JinaHandle jina_init(const char* config_path);
void jina_release(JinaHandle handle);
const char* jina_last_error(void);

/**
 * 1. Embed Single Text
 * @param handle: Initialization handle
 * @param text: C-string input
 * @param out_data: Pointer to pre-allocated float array (min size 768)
 * @return: 0 on success, negative on error
 */
int jina_embed_text(JinaHandle handle, const char* text, float* out_data);

/**
 * 2. Embed Single Image (Raw RGB data)
 * @param img_data: Flat buffer of RGB pixels
 * @param width, height, channels: Image dimensions
 * @param out_data: Pointer to pre-allocated float array (min size 768)
 * @return: 0 on success
 */
int jina_embed_image(JinaHandle handle, const uint8_t* img_data, int width, int height, int channels, float* out_data);

/**
 * 3. Embed Batch Text
 * @param texts: Array of C-strings
 * @param count: Number of strings
 * @param out_data: Flat float array (size count * 768)
 */
int jina_embed_batch_text(JinaHandle handle, const char** texts, int count, float* out_data);

/**
 * 4. Embed Batch Image
 * @param imgs_data: Flat buffer representing all pixels of all images concatenated
 * @param count: Number of images
 */
int jina_embed_batch_image(JinaHandle handle, const uint8_t* imgs_data, int count, int width, int height, int channels, float* out_data);

#ifdef __cplusplus
}
#endif

#endif
