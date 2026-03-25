package qdrant

const qdrantSourcePrefix = "internal.infra.qdrant"

func qdrantSource(funcName string) string {
	return qdrantSourcePrefix + "." + funcName
}
