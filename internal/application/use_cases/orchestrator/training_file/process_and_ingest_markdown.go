package trainingfile

import (
	"errors"
	"strings"
	"unicode"
)

func parseLineBasedChunks(content string, cfg chunkingRuntimeConfig) ([]pipelineChunk, chunkingStats) {
	lines := strings.Split(content, "\n")
	tokens := make([]lineToken, 0, len(lines))
	stats := chunkingStats{}

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}

		imageMatches := inlineImagePattern.FindAllStringSubmatch(trimmed, -1)
		if len(imageMatches) > 0 {
			for _, m := range imageMatches {
				if len(m) == 2 {
					tokens = append(tokens, lineToken{Type: lineTokenImage, Value: strings.TrimSpace(m[1])})
				}
			}
			trimmed = inlineImagePattern.ReplaceAllString(trimmed, "")
			trimmed = strings.TrimSpace(trimmed)
		}

		if trimmed == "" {
			continue
		}

		rawSentences := splitSentences(trimmed)
		stableSentences, lineStats := stabilizeSentenceUnits(rawSentences, cfg)
		stats.RawSentenceCount += lineStats.RawSentenceCount
		stats.KeptSentenceCount += lineStats.KeptSentenceCount
		stats.DroppedHeader += lineStats.DroppedHeader
		stats.DroppedNumeric += lineStats.DroppedNumeric
		stats.DroppedNoise += lineStats.DroppedNoise
		stats.MergedShort += lineStats.MergedShort

		for _, sentence := range stableSentences {
			tokens = append(tokens, lineToken{Type: lineTokenText, Value: sentence})
		}
	}

	chunks := make([]pipelineChunk, 0, len(tokens))
	for i := 0; i < len(tokens); i++ {
		token := tokens[i]
		if token.Type == lineTokenText {
			chunk := pipelineChunk{
				Text:       token.Value,
				ImagePaths: []string{},
			}
			for i+1 < len(tokens) && tokens[i+1].Type == lineTokenImage {
				chunk.ImagePaths = append(chunk.ImagePaths, tokens[i+1].Value)
				i++
			}
			chunks = append(chunks, chunk)
			continue
		}

		if len(chunks) > 0 {
			chunks[len(chunks)-1].ImagePaths = append(chunks[len(chunks)-1].ImagePaths, token.Value)
			continue
		}
		chunks = append(chunks, pipelineChunk{
			Text:       "",
			ImagePaths: []string{token.Value},
		})
	}

	out := make([]pipelineChunk, 0, len(chunks))
	for _, c := range chunks {
		if strings.TrimSpace(c.Text) == "" {
			continue
		}
		out = append(out, c)
	}
	return out, stats
}

func splitSentences(input string) []string {
	text := normalizeWhitespace(input)
	if text == "" {
		return nil
	}

	matches := sentenceSplitPattern.FindAllStringSubmatch(text, -1)
	if len(matches) == 0 {
		return []string{text}
	}

	out := make([]string, 0, len(matches))
	for _, m := range matches {
		if len(m) < 2 {
			continue
		}
		s := normalizeWhitespace(m[1])
		if s != "" {
			out = append(out, s)
		}
	}
	if len(out) == 0 {
		return []string{text}
	}
	return out
}

func stabilizeSentenceUnits(raw []string, cfg chunkingRuntimeConfig) ([]string, chunkingStats) {
	stats := chunkingStats{RawSentenceCount: len(raw)}
	if len(raw) == 0 {
		return nil, stats
	}

	kept := make([]string, 0, len(raw))
	for _, item := range raw {
		s := normalizeWhitespace(item)
		if s == "" {
			continue
		}
		if isHeaderLike(s) {
			stats.DroppedHeader++
			continue
		}
		if isNumericOnlySentence(s) {
			stats.DroppedNumeric++
			continue
		}
		if shouldAttachToPreviousSentence(s, cfg) && len(kept) > 0 {
			kept[len(kept)-1] = normalizeWhitespace(kept[len(kept)-1] + " " + s)
			stats.MergedShort++
			continue
		}
		if isVeryShortNoise(s, cfg) {
			stats.DroppedNoise++
			continue
		}
		kept = append(kept, s)
	}

	if len(kept) > 1 && isShortSentence(kept[0], cfg) {
		kept[1] = normalizeWhitespace(kept[0] + " " + kept[1])
		kept = kept[1:]
		stats.MergedShort++
	}

	merged := make([]string, 0, len(kept))
	for _, s := range kept {
		if len(merged) > 0 && isShortSentence(s, cfg) {
			merged[len(merged)-1] = normalizeWhitespace(merged[len(merged)-1] + " " + s)
			stats.MergedShort++
			continue
		}
		merged = append(merged, s)
	}

	stats.KeptSentenceCount = len(merged)
	return merged, stats
}

func mergeChunksBySemantic(chunks []pipelineChunk, embeddings [][]float32, cfg chunkingRuntimeConfig) ([]pipelineChunk, [][]float32, error) {
	if len(chunks) == 0 || len(embeddings) == 0 || len(chunks) != len(embeddings) {
		return nil, nil, errors.New("invalid input for semantic merge")
	}

	threshold := cfg.SemanticSimThreshold
	mergedChunks := make([]pipelineChunk, 0, len(chunks))
	mergedVectors := make([][]float32, 0, len(chunks))

	currentChunk := chunks[0]
	currentVectors := [][]float32{embeddings[0]}

	for i := 1; i < len(chunks); i++ {
		sim, err := consineSimilarity(embeddings[i-1], embeddings[i])
		if err != nil {
			return nil, nil, err
		}

		candidateText := normalizeWhitespace(currentChunk.Text + " " + chunks[i].Text)
		candidateTokens := countTextTokens(candidateText)
		canMergeByLength := cfg.MaxChunkTokens <= 0 || candidateTokens <= cfg.MaxChunkTokens
		if sim >= threshold && canMergeByLength {
			currentChunk.Text = candidateText
			currentChunk.ImagePaths = append(currentChunk.ImagePaths, chunks[i].ImagePaths...)
			currentVectors = append(currentVectors, embeddings[i])
			continue
		}

		mergedChunks = append(mergedChunks, currentChunk)
		mergedVectors = append(mergedVectors, averageVectors(currentVectors))

		currentChunk = chunks[i]
		currentVectors = [][]float32{embeddings[i]}
	}

	mergedChunks = append(mergedChunks, currentChunk)
	mergedVectors = append(mergedVectors, averageVectors(currentVectors))

	mergedChunks, mergedVectors = repairShortChunks(mergedChunks, mergedVectors, cfg)
	mergedChunks, mergedVectors = splitOverlongChunks(mergedChunks, mergedVectors, cfg)
	mergedChunks = applySentenceOverlap(mergedChunks, cfg.ChunkOverlapSentences)

	return mergedChunks, mergedVectors, nil
}

func repairShortChunks(chunks []pipelineChunk, vectors [][]float32, cfg chunkingRuntimeConfig) ([]pipelineChunk, [][]float32) {
	if len(chunks) <= 1 {
		return chunks, vectors
	}

	for i := 0; i < len(chunks); i++ {
		if !isShortChunk(chunks[i], cfg) || len(chunks) <= 1 {
			continue
		}

		if i == 0 {
			chunks[1].Text = normalizeWhitespace(chunks[0].Text + " " + chunks[1].Text)
			chunks[1].ImagePaths = append(chunks[0].ImagePaths, chunks[1].ImagePaths...)
			vectors[1] = averageVectors([][]float32{vectors[0], vectors[1]})
			chunks = chunks[1:]
			vectors = vectors[1:]
			i = -1
			continue
		}

		chunks[i-1].Text = normalizeWhitespace(chunks[i-1].Text + " " + chunks[i].Text)
		chunks[i-1].ImagePaths = append(chunks[i-1].ImagePaths, chunks[i].ImagePaths...)
		vectors[i-1] = averageVectors([][]float32{vectors[i-1], vectors[i]})
		chunks = append(chunks[:i], chunks[i+1:]...)
		vectors = append(vectors[:i], vectors[i+1:]...)
		i--
	}

	return chunks, vectors
}

func splitOverlongChunks(chunks []pipelineChunk, vectors [][]float32, cfg chunkingRuntimeConfig) ([]pipelineChunk, [][]float32) {
	if cfg.MaxChunkTokens <= 0 {
		return chunks, vectors
	}

	finalChunks := make([]pipelineChunk, 0, len(chunks))
	finalVectors := make([][]float32, 0, len(vectors))

	for i := 0; i < len(chunks); i++ {
		chunk := chunks[i]
		vec := vectors[i]
		if countTextTokens(chunk.Text) <= cfg.MaxChunkTokens {
			finalChunks = append(finalChunks, chunk)
			finalVectors = append(finalVectors, vec)
			continue
		}

		sentences := splitSentences(chunk.Text)
		if len(sentences) <= 1 {
			finalChunks = append(finalChunks, chunk)
			finalVectors = append(finalVectors, vec)
			continue
		}

		buffer := ""
		for _, sentence := range sentences {
			candidate := normalizeWhitespace(strings.TrimSpace(buffer + " " + sentence))
			if buffer != "" && countTextTokens(candidate) > cfg.MaxChunkTokens {
				finalChunks = append(finalChunks, pipelineChunk{Text: buffer, ImagePaths: []string{}})
				finalVectors = append(finalVectors, vec)
				buffer = sentence
				continue
			}
			buffer = candidate
		}
		if buffer != "" {
			finalChunks = append(finalChunks, pipelineChunk{Text: buffer, ImagePaths: []string{}})
			finalVectors = append(finalVectors, vec)
		}

		if len(finalChunks) > 0 {
			finalChunks[len(finalChunks)-1].ImagePaths = append(finalChunks[len(finalChunks)-1].ImagePaths, chunk.ImagePaths...)
		}
	}

	return finalChunks, finalVectors
}

func applySentenceOverlap(chunks []pipelineChunk, overlapSentences int) []pipelineChunk {
	if overlapSentences <= 0 || len(chunks) <= 1 {
		return chunks
	}

	out := make([]pipelineChunk, len(chunks))
	copy(out, chunks)

	for i := 1; i < len(out); i++ {
		tail := tailSentences(out[i-1].Text, overlapSentences)
		if len(tail) == 0 {
			continue
		}
		prefix := normalizeWhitespace(strings.Join(tail, " "))
		if prefix == "" {
			continue
		}
		out[i].Text = normalizeWhitespace(prefix + " " + out[i].Text)
	}

	return out
}

func averageVectors(vectors [][]float32) []float32 {
	if len(vectors) == 0 {
		return nil
	}
	dim := len(vectors[0])
	out := make([]float32, dim)
	for _, v := range vectors {
		for i := 0; i < dim && i < len(v); i++ {
			out[i] += v[i]
		}
	}
	for i := range out {
		out[i] = out[i] / float32(len(vectors))
	}
	return out
}

func filterValidChunksAndEmbeddings(chunks []pipelineChunk, embeddings [][]float32) ([]pipelineChunk, [][]float32, int, int, error) {
	if len(chunks) != len(embeddings) {
		return nil, nil, 0, 0, errors.New("chunk and embedding size mismatch")
	}

	filteredChunks := make([]pipelineChunk, 0, len(chunks))
	filteredEmbeddings := make([][]float32, 0, len(embeddings))
	skippedZeroNorm := 0
	skippedDimMismatch := 0
	expectedDim := 0

	for i := 0; i < len(chunks); i++ {
		vec := embeddings[i]
		if isZeroNormVector(vec) {
			skippedZeroNorm++
			continue
		}

		if expectedDim == 0 {
			expectedDim = len(vec)
		}
		if len(vec) != expectedDim {
			skippedDimMismatch++
			continue
		}

		filteredChunks = append(filteredChunks, chunks[i])
		filteredEmbeddings = append(filteredEmbeddings, vec)
	}

	if len(filteredChunks) == 0 {
		return nil, nil, skippedZeroNorm, skippedDimMismatch, errors.New("all chunks were filtered out due to invalid embeddings")
	}

	return filteredChunks, filteredEmbeddings, skippedZeroNorm, skippedDimMismatch, nil
}

func normalizeWhitespace(text string) string {
	return strings.Join(strings.Fields(strings.TrimSpace(text)), " ")
}

func countTextTokens(text string) int {
	return len(strings.Fields(strings.TrimSpace(text)))
}

func isHeaderLike(text string) bool {
	t := strings.TrimSpace(text)
	if t == "" {
		return false
	}
	if strings.HasPrefix(t, "#") {
		return true
	}
	for _, r := range t {
		if r != '#' && r != '-' && !unicode.IsSpace(r) {
			return false
		}
	}
	return true
}

func isNumericOnlySentence(text string) bool {
	t := strings.TrimSpace(text)
	if t == "" {
		return false
	}
	hasDigit := false
	for _, r := range t {
		if unicode.IsDigit(r) {
			hasDigit = true
			continue
		}
		if unicode.IsSpace(r) {
			continue
		}
		switch r {
		case '.', ',', ':', ';', '%', '-', '+', '/', '(', ')':
			continue
		default:
			return false
		}
	}
	return hasDigit
}

func isVeryShortNoise(text string, cfg chunkingRuntimeConfig) bool {
	if !isShortSentence(text, cfg) {
		return false
	}
	for _, r := range text {
		if unicode.IsLetter(r) {
			return false
		}
	}
	return true
}

func isShortSentence(text string, cfg chunkingRuntimeConfig) bool {
	chars := len([]rune(strings.TrimSpace(text)))
	tokens := countTextTokens(text)
	return chars < cfg.MinChunkChars || tokens < cfg.MinChunkTokens
}

func shouldAttachToPreviousSentence(text string, cfg chunkingRuntimeConfig) bool {
	if isNumericOnlySentence(text) {
		return true
	}
	chars := len([]rune(strings.TrimSpace(text)))
	tokens := countTextTokens(text)
	if tokens <= 3 {
		return true
	}
	return chars < cfg.MinChunkChars/2
}

func isShortChunk(chunk pipelineChunk, cfg chunkingRuntimeConfig) bool {
	return isShortSentence(chunk.Text, cfg)
}

func tailSentences(text string, n int) []string {
	if n <= 0 {
		return nil
	}
	sentences := splitSentences(text)
	if len(sentences) == 0 {
		return nil
	}
	if len(sentences) <= n {
		return sentences
	}
	return sentences[len(sentences)-n:]
}
