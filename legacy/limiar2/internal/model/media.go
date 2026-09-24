package model

// PhotoMetadata contém os campos MTProto necessários para download de imagem
// sob demanda via upload.GetFile (ADR 011).
type PhotoMetadata struct {
	ID            int64 // processed_messages.id (PK); 0 em metadados vindos de MTProto (renew/refetch)
	MsgID         int64
	ChannelID     int64
	PhotoID       int64
	AccessHash    int64
	FileReference string // base64
	DCID          int
}

// PhotoStats resume a cobertura de metadados MTProto em processed_messages para
// diagnóstico do subsistema de mídia (smoke test, ADR 011). Permite validar, em
// produção, se o backfill da Fase A produziu metadados utilizáveis pelo
// MediaResolver na Wave 4.
type PhotoStats struct {
	TotalProcessed  int64 // total de mensagens processadas
	WithPhoto       int64 // mensagens com photo_id > 0
	CompleteMTProto int64 // com foto E todos os campos MTProto (access_hash, file_ref, dcid)
}
