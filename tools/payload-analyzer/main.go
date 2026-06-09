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
	defer db.Close()

	// Check what tables exist
	fmt.Printf("=== TABLES IN DATABASE ===\n")
	tableRows, err := db.Query("SELECT name FROM sqlite_master WHERE type='table' ORDER BY name")
	if err != nil {
		log.Fatal(err)
	}
	var tables []string
	for tableRows.Next() {
		var name string
		err = tableRows.Scan(&name)
		if err != nil {
			log.Fatal(err)
		}
		tables = append(tables, name)
		fmt.Printf("  %s\n", name)
	}
	tableRows.Close()
	fmt.Printf("\n")

	// Get basic stats
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

	fmt.Printf("=== DATABASE STATISTICS ===\n")
	fmt.Printf("Total raw messages: %d\n", totalMessages)
	fmt.Printf("Total channels: %d\n", totalChannels)
	fmt.Printf("\n")

	// Get channel distribution
	fmt.Printf("=== CHANNEL DISTRIBUTION ===\n")
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
	defer rows.Close()

	for rows.Next() {
		var username, title string
		var count int
		err = rows.Scan(&username, &title, &count)
		if err != nil {
			log.Fatal(err)
		}
		fmt.Printf("  @%s (%s): %d messages\n", username, title, count)
	}
	fmt.Printf("\n")

	// Extract all payloads for analysis
	fmt.Printf("=== PAYLOAD ANALYSIS ===\n")
	msgRows, err := db.Query("SELECT id, channel_id, message_id, payload, received_at, schema_version FROM raw_messages ORDER BY id")
	if err != nil {
		log.Fatal(err)
	}
	defer msgRows.Close()

	var messages []RawMessage
	for msgRows.Next() {
		var msg RawMessage
		err = msgRows.Scan(&msg.ID, &msg.ChannelID, &msg.MessageID, &msg.Payload, &msg.ReceivedAt, &msg.SchemaVersion)
		if err != nil {
			log.Fatal(err)
		}
		messages = append(messages, msg)
	}

	// Analyze payload structure
	analyzePayloads(messages)

	// Export all payloads to JSON file for jq analysis
	exportPayloads(messages)
}

func analyzePayloads(messages []RawMessage) {
	typeCounts := make(map[string]int)
	topLevelKeys := make(map[string]int)
	nestedStructures := make(map[string]map[string]int)
	invalidJSON := 0
	emptyPayloads := 0

	var fieldTypes map[string]map[string]bool = make(map[string]map[string]bool)

	for _, msg := range messages {
		if msg.Payload == "" || msg.Payload == "null" {
			emptyPayloads++
			continue
		}

		var payload interface{}
		err := json.Unmarshal([]byte(msg.Payload), &payload)
		if err != nil {
			invalidJSON++
			continue
		}

		// Determine top-level type
		typeName := reflect.TypeOf(payload).String()
		typeCounts[typeName]++

		// Analyze structure if it's a map
		if m, ok := payload.(map[string]interface{}); ok {
			for k, v := range m {
				topLevelKeys[k]++

				// Track field types
				if fieldTypes[k] == nil {
					fieldTypes[k] = make(map[string]bool)
				}
				typeName := "null"
				if v != nil {
					typeName = reflect.TypeOf(v).String()
				}
				fieldTypes[k][typeName] = true

				// Analyze nested objects
				if nested, ok := v.(map[string]interface{}); ok {
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

	fmt.Printf("Top-level types:\n")
	for t, count := range typeCounts {
		fmt.Printf("  %s: %d\n", t, count)
	}
	fmt.Printf("\n")

	fmt.Printf("Invalid JSON payloads: %d\n", invalidJSON)
	fmt.Printf("Empty/null payloads: %d\n", emptyPayloads)
	fmt.Printf("Valid payloads: %d\n", len(messages)-invalidJSON-emptyPayloads)
	fmt.Printf("\n")

	fmt.Printf("Top-level keys (frequency):\n")
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

	fmt.Printf("Field type variations:\n")
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

	fmt.Printf("Nested structures:\n")
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

	f, err := os.Create("payloads_export.json")
	if err != nil {
		log.Fatal(err)
	}
	defer f.Close()

	encoder := json.NewEncoder(f)
	encoder.SetIndent("", "  ")
	err = encoder.Encode(records)
	if err != nil {
		log.Fatal(err)
	}

	fmt.Printf("\n=== EXPORT ===\n")
	fmt.Printf("All payloads exported to: payloads_export.json\n")
	fmt.Printf("Use with jq: jq '.[] | .payload' payloads_export.json\n")
}
