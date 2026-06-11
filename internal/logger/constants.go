package logger

// Chaves de atributo estáveis usadas internamente pelo pacote logger.
// time e level são gerenciados automaticamente pelo slog e não possuem constante.
const (
	attrKeyComponent     = "component"
	attrKeyRunID         = "run_id"
	attrKeyError         = "erro"
	attrKeyService       = "service"
	attrKeyPipelineStage = "pipeline_stage"
)
