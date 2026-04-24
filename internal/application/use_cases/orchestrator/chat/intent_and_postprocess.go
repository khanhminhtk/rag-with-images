package chat

import (
	"strings"

	portsOrchestrator "rag_imagetotext_texttoimage/internal/application/ports/orchestrator"
)

func shouldSkipRetrieval(query string, imagePath string) bool {
	if strings.TrimSpace(imagePath) != "" {
		return false
	}
	q := normalizeForOverlap(query)
	if q == "" {
		return true
	}
	// Keep LLM flow but avoid pulling unrelated RAG context for casual greetings.
	greetingPatterns := []string{
		"xin chao", "chao ban", "hello", "hi", "hey", "alo", "good morning", "good afternoon",
	}
	for _, pat := range greetingPatterns {
		if strings.Contains(q, pat) {
			return true
		}
	}
	return false
}

func memoryTopK(v int) int {
	if v <= 0 {
		return 5
	}
	return v
}

func normalizeIntentText(in string) string {
	in = strings.ToLower(strings.TrimSpace(in))
	replacer := strings.NewReplacer(
		"?", " ",
		"!", " ",
		".", " ",
		",", " ",
		";", " ",
		":", " ",
		"(", " ",
		")", " ",
		"[", " ",
		"]", " ",
		"{", " ",
		"}", " ",
		"*", " ",
		"#", " ",
		"\n", " ",
		"\t", " ",
	)
	in = replacer.Replace(in)
	return strings.Join(strings.Fields(in), " ")
}

func latestAssistantMessage(history []portsOrchestrator.ChatMessage, topK int) string {
	if topK <= 0 {
		topK = 5
	}
	count := 0
	for i := len(history) - 1; i >= 0; i-- {
		if strings.ToLower(strings.TrimSpace(history[i].Role)) != "assistant" {
			continue
		}
		if strings.TrimSpace(history[i].Content) == "" {
			continue
		}
		count++
		if count == 1 {
			return strings.TrimSpace(history[i].Content)
		}
		if count >= topK {
			break
		}
	}
	return ""
}

func shouldAcceptPostprocess(rawAnswer, postAnswer, lastAssistant string) (bool, string) {
	raw := strings.TrimSpace(rawAnswer)
	post := strings.TrimSpace(postAnswer)
	if post == "" {
		return false, "empty_postprocess"
	}
	if len(strings.Fields(post)) < 4 {
		return false, "too_short"
	}

	genericPatterns := []string{
		"rat vui duoc ho tro",
		"moi ban dat cau hoi",
		"xin chao",
		"moi ban cung cap noi dung",
	}
	normalizedPost := normalizeForOverlap(post)
	for _, pat := range genericPatterns {
		if strings.Contains(normalizedPost, pat) {
			return false, "generic_reply"
		}
	}

	if strings.TrimSpace(lastAssistant) != "" && strings.EqualFold(strings.TrimSpace(lastAssistant), post) {
		return false, "duplicate_last_assistant"
	}

	if overlapRatio(raw, post) < 0.08 {
		return false, "low_overlap_with_raw_answer"
	}
	return true, "accepted"
}

func overlapRatio(a string, b string) float64 {
	aSet := tokenSet(a)
	bSet := tokenSet(b)
	if len(aSet) == 0 || len(bSet) == 0 {
		return 0
	}
	common := 0
	for token := range aSet {
		if _, ok := bSet[token]; ok {
			common++
		}
	}
	minBase := len(aSet)
	if len(bSet) < minBase {
		minBase = len(bSet)
	}
	if minBase == 0 {
		return 0
	}
	return float64(common) / float64(minBase)
}

func tokenSet(s string) map[string]struct{} {
	out := make(map[string]struct{})
	normalized := normalizeForOverlap(s)
	for _, tok := range strings.Fields(normalized) {
		if len(tok) <= 1 {
			continue
		}
		out[tok] = struct{}{}
	}
	return out
}

func normalizeForOverlap(s string) string {
	s = normalizeIntentText(s)
	return stripVietnameseAccents(s)
}

func stripVietnameseAccents(s string) string {
	replacer := strings.NewReplacer(
		"á", "a", "à", "a", "ả", "a", "ã", "a", "ạ", "a",
		"ă", "a", "ắ", "a", "ằ", "a", "ẳ", "a", "ẵ", "a", "ặ", "a",
		"â", "a", "ấ", "a", "ầ", "a", "ẩ", "a", "ẫ", "a", "ậ", "a",
		"é", "e", "è", "e", "ẻ", "e", "ẽ", "e", "ẹ", "e",
		"ê", "e", "ế", "e", "ề", "e", "ể", "e", "ễ", "e", "ệ", "e",
		"í", "i", "ì", "i", "ỉ", "i", "ĩ", "i", "ị", "i",
		"ó", "o", "ò", "o", "ỏ", "o", "õ", "o", "ọ", "o",
		"ô", "o", "ố", "o", "ồ", "o", "ổ", "o", "ỗ", "o", "ộ", "o",
		"ơ", "o", "ớ", "o", "ờ", "o", "ở", "o", "ỡ", "o", "ợ", "o",
		"ú", "u", "ù", "u", "ủ", "u", "ũ", "u", "ụ", "u",
		"ư", "u", "ứ", "u", "ừ", "u", "ử", "u", "ữ", "u", "ự", "u",
		"ý", "y", "ỳ", "y", "ỷ", "y", "ỹ", "y", "ỵ", "y",
		"đ", "d",
	)
	return replacer.Replace(s)
}
