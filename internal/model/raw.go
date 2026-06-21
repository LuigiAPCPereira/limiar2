package model

import "time"

// RawMessage é um payload de mensagem capturada do Telegram aguardando processamento
// posterior (downstream). Payload contém o update bruto do gotd/td serializado como JSON.
type RawMessage struct {
	ID            int64
	ChannelID     int64
	MessageID     int64
	Payload       []byte
	ReceivedAt    time.Time
	SchemaVersion int
}

// Channel é um canal monitorado do Telegram e seu cursor de coleta.
type Channel struct {
	ID              int64
	Username        string
	Title           string
	Active          bool
	AddedAt         time.Time
	LastMessageID   int64
	LastCollectedAt time.Time
}

// Peer é um peer do Telegram armazenado em cache (canal, usuário ou chat) com seu hash de acesso.
type Peer struct {
	ID         int64
	AccessHash int64
	Type       string
	Username   string
	UpdatedAt  time.Time
}

// ChannelStats mantém a contagem de mensagens para um único canal.
type ChannelStats struct {
	ChannelID    int64
	Username     string
	MessageCount int64
}
