package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"reflect"
	"sort"
	"strings"

	_ "turso.tech/database/tursogo"
)

type RawMessage struct {
	ID            int64
	ChannelID     int64
	MessageID     int64
	Payload       string
	ReceivedAt    string
	SchemaVersion int
}

type Channel struct {
	ID              int64
	Username        string
	Title           string
	Active          bool
	AddedAt         string
	LastMessageID   int64
	LastCollectedAt sql.NullString
}

func main() {
	db, err := sql.Open("turso", "./limiar.db")
	if err != nil {
		log.Fatal(err)
	}
	defer func() { _ = db.Close() }()

	// Verifica quais tabelas existem
	fmt.Printf("=== TABELAS NO BANCO DE DADOS ===\n")
	tableRows, err := db.Query("SELECT name FROM sqlite_master WHERE type='table' ORDER BY name")
	if err != nil {
		log.Fatal(err)
	}
	for tableRows.Next() {
		var name string
		err = tableRows.Scan(&name)
		if err != nil {
			log.Fatal(err)
		}
		_ = name
		fmt.Printf("  %s\n", name)
	}
	_ = tableRows.Close()
	fmt.Printf("\n")

	// Obtém estatísticas básicas
	var totalMessages int
	err = db.QueryRow("SELECT COUNT(*) FROM raw_messages").Scan(&totalMessages)
	if err != nil {
		log.Fatal(err)
	}

	var totalChannels int
	err = db.QueryRow("SELECT COUNT(*) FROM channels").Scan(&totalChannels)
	if err != nil {
		log.Fatal(err)
	}

	fmt.Printf("=== ESTATÍSTICAS DO BANCO DE DADOS ===\n")
	fmt.Printf("Total de mensagens brutas (raw): %d\n", totalMessages)
	fmt.Printf("Total de canais: %d\n", totalChannels)
	fmt.Printf("\n")

	// Obtém a distribuição por canal
	fmt.Printf("=== DISTRIBUIÇÃO POR CANAL ===\n")
	rows, err := db.Query(`
		SELECT c.username, c.title, COUNT(rm.id) as msg_count
		FROM channels c
		LEFT JOIN raw_messages rm ON c.id = rm.channel_id
		GROUP BY c.id
		ORDER BY msg_count DESC
	`)
	if err != nil {
		log.Fatal(err)
	}
	defer func() { _ = rows.Close() }()

	for rows.Next() {
		var username, title string
		var count int
		err = rows.Scan(&username, &title, &count)
		if err != nil {
			log.Fatal(err)
		}
		fmt.Printf("  @%s (%s): %d mensagens\n", username, title, count)
	}
	fmt.Printf("\n")

	// Extrai todos os payloads para análise
	fmt.Printf("=== ANÁLISE DE PAYLOAD ===\n")
	msgRows, err := db.Query("SELECT id, channel_id, message_id, payload, received_at, schema_version FROM raw_messages ORDER BY id")
	if err != nil {
		log.Fatal(err)
	}
	defer func() { _ = msgRows.Close() }()

	var messages []RawMessage
	for msgRows.Next() {
		var msg RawMessage
		err = msgRows.Scan(&msg.ID, &msg.ChannelID, &msg.MessageID, &msg.Payload, &msg.ReceivedAt, &msg.SchemaVersion)
		if err != nil {
			log.Fatal(err)
		}
		messages = append(messages, msg)
	}

	// Analisa a estrutura do payload
	analyzePayloads(messages)

	// Exporta todos os payloads para um arquivo JSON para análise com jq
	exportPayloads(messages)
}

func analyzePayloads(messages []RawMessage) {
	typeCounts := make(map[string]int)
	topLevelKeys := make(map[string]int)
	nestedStructures := make(map[string]map[string]int)
	invalidJSON := 0
	emptyPayloads := 0

	fieldTypes := make(map[string]map[string]bool)

	for _, msg := range messages {
		if msg.Payload == "" || msg.Payload == "null" {
			emptyPayloads++
			continue
		}

		var payload any
		err := json.Unmarshal([]byte(msg.Payload), &payload)
		if err != nil {
			invalidJSON++
			continue
		}

		// Determina o tipo de nível superior (top-level type)
		typeName := reflect.TypeOf(payload).String()
		typeCounts[typeName]++

		// Analisa a estrutura se for um mapa (map)
		if m, ok := payload.(map[string]any); ok {
			for k, v := range m {
				topLevelKeys[k]++

				// Rastreia os tipos de campo (field types)
				if fieldTypes[k] == nil {
					fieldTypes[k] = make(map[string]bool)
				}
				typeName := "null"
				if v != nil {
					typeName = reflect.TypeOf(v).String()
				}
				fieldTypes[k][typeName] = true

				// Analisa objetos aninhados (nested objects)
				if nested, ok := v.(map[string]any); ok {
					if nestedStructures[k] == nil {
						nestedStructures[k] = make(map[string]int)
					}
					for nk := range nested {
						nestedStructures[k][nk]++
					}
				}
			}
		}
	}

	fmt.Printf("Tipos de nível superior (Top-level types):\n")
	for t, count := range typeCounts {
		fmt.Printf("  %s: %d\n", t, count)
	}
	fmt.Printf("\n")

	fmt.Printf("Payloads JSON inválidos: %d\n", invalidJSON)
	fmt.Printf("Payloads vazios/nulos: %d\n", emptyPayloads)
	fmt.Printf("Payloads válidos: %d\n", len(messages)-invalidJSON-emptyPayloads)
	fmt.Printf("\n")

	fmt.Printf("Chaves de nível superior (frequência):\n")
	sortedKeys := make([]string, 0, len(topLevelKeys))
	for k := range topLevelKeys {
		sortedKeys = append(sortedKeys, k)
	}
	sort.Slice(sortedKeys, func(i, j int) bool {
		return topLevelKeys[sortedKeys[i]] > topLevelKeys[sortedKeys[j]]
	})
	for _, k := range sortedKeys {
		fmt.Printf("  %s: %d (%.1f%%)\n", k, topLevelKeys[k], float64(topLevelKeys[k])/float64(len(messages))*100)
	}
	fmt.Printf("\n")

	fmt.Printf("Variações de tipo de campo:\n")
	for field, types := range fieldTypes {
		if len(types) > 1 {
			typeList := make([]string, 0, len(types))
			for t := range types {
				typeList = append(typeList, t)
			}
			sort.Strings(typeList)
			fmt.Printf("  %s: %s\n", field, strings.Join(typeList, ", "))
		}
	}
	fmt.Printf("\n")

	fmt.Printf("Estruturas aninhadas:\n")
	for parent, children := range nestedStructures {
		fmt.Printf("  %s:\n", parent)
		childKeys := make([]string, 0, len(children))
		for k := range children {
			childKeys = append(childKeys, k)
		}
		sort.Slice(childKeys, func(i, j int) bool {
			return children[childKeys[i]] > children[childKeys[j]]
		})
		for _, k := range childKeys {
			fmt.Printf("    %s: %d\n", k, children[k])
		}
	}
}

func exportPayloads(messages []RawMessage) {
	type ExportRecord struct {
		ID            int64           `json:"id"`
		ChannelID     int64           `json:"channel_id"`
		MessageID     int64           `json:"message_id"`
		ReceivedAt    string          `json:"received_at"`
		SchemaVersion int             `json:"schema_version"`
		Payload       json.RawMessage `json:"payload"`
	}

	var records []ExportRecord
	for _, msg := range messages {
		var payload json.RawMessage
		if msg.Payload != "" && msg.Payload != "null" {
			payload = json.RawMessage(msg.Payload)
		} else {
			payload = json.RawMessage("null")
		}

		records = append(records, ExportRecord{
			ID:            msg.ID,
			ChannelID:     msg.ChannelID,
			MessageID:     msg.MessageID,
			ReceivedAt:    msg.ReceivedAt,
			SchemaVersion: msg.SchemaVersion,
			Payload:       payload,
		})
	}

	f, err := os.OpenFile("payloads_export.json", os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0600)
	if err != nil {
		log.Fatal(err)
	}
	defer func() { _ = f.Close() }()

	encoder := json.NewEncoder(f)
	encoder.SetIndent("", "  ")
	err = encoder.Encode(records)
	if err != nil {
		log.Fatal(err)
	}

	fmt.Printf("\n=== EXPORTAÇÃO ===\n")
	fmt.Printf("Todos os payloads exportados para: payloads_export.json\n")
	fmt.Printf("Use com jq: jq '.[] | .payload' payloads_export.json\n")
}
