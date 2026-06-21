package storage

import "github.com/limiar/collector/internal/model"

// Type aliases — preservam compatibilidade durante migração (ADR 013 Wave 1).

type RawMessage = model.RawMessage
type Channel = model.Channel
type Peer = model.Peer
type ChannelStats = model.ChannelStats
type ProcessedMessage = model.ProcessedMessage
type ProcessedTypeStats = model.ProcessedTypeStats
type PhotoMetadata = model.PhotoMetadata
type PhotoStats = model.PhotoStats

var ParseDBTime = model.ParseDBTime
var BoolToInt = model.BoolToInt

const DBTimeLayout = model.DBTimeLayout
