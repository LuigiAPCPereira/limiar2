#!/usr/bin/env python3
import sqlite3
import json
import sys
from collections import defaultdict, Counter

def analyze_database():
    db_path = '/home/projetos/Projetos/Limiar2/limiar.db'
    print(f"Usando banco de dados: {db_path}")
    conn = sqlite3.connect(db_path)
    conn.row_factory = sqlite3.Row
    cursor = conn.cursor()

    # Get tables
    print("=== TABELAS NO BANCO DE DADOS ===")
    cursor.execute("SELECT name FROM sqlite_master WHERE type='table' ORDER BY name")
    tables = [row[0] for row in cursor.fetchall()]
    for table in tables:
        print(f"  {table}")
    print()

    # Get basic stats
    cursor.execute("SELECT COUNT(*) FROM raw_messages")
    total_messages = cursor.fetchone()[0]

    cursor.execute("SELECT COUNT(*) FROM channels")
    total_channels = cursor.fetchone()[0]

    print("=== ESTATÍSTICAS DO BANCO DE DADOS ===")
    print(f"Total de mensagens brutas (raw): {total_messages}")
    print(f"Total de canais: {total_channels}")
    print()

    # Channel distribution
    print("=== DISTRIBUIÇÃO POR CANAL ===")
    cursor.execute("""
        SELECT c.username, c.title, COUNT(rm.id) as msg_count
        FROM channels c
        LEFT JOIN raw_messages rm ON c.id = rm.channel_id
        GROUP BY c.id
        ORDER BY msg_count DESC
    """)
    for row in cursor.fetchall():
        print(f"  @{row['username']} ({row['title']}): {row['msg_count']} mensagens")
    print()

    # Extract all messages
    cursor.execute("""
        SELECT id, channel_id, message_id, payload, received_at, schema_version
        FROM raw_messages
        ORDER BY id
    """)
    messages = cursor.fetchall()

    # Analyze payloads
    analyze_payloads(messages)

    # Export to JSON
    export_payloads(messages)

    conn.close()

def analyze_payloads(messages):
    print("=== ANÁLISE DE PAYLOAD ===")

    invalid_json = 0
    empty_payloads = 0
    type_counts = Counter()
    top_level_keys = Counter()
    field_types = defaultdict(set)
    nested_structures = defaultdict(Counter)

    for msg in messages:
        payload_str = msg['payload']

        if not payload_str or payload_str == 'null':
            empty_payloads += 1
            continue

        try:
            payload = json.loads(payload_str)
        except json.JSONDecodeError:
            invalid_json += 1
            continue

        # Top-level type
        type_name = type(payload).__name__
        type_counts[type_name] += 1

        # Analyze structure if dict
        if isinstance(payload, dict):
            for key, value in payload.items():
                top_level_keys[key] += 1
                field_types[key].add(type(value).__name__)

                # Nested structures
                if isinstance(value, dict):
                    for nested_key in value.keys():
                        nested_structures[key][nested_key] += 1

    print("Tipos de nível superior (Top-level types):")
    for t, count in type_counts.most_common():
        print(f"  {t}: {count}")
    print()

    print(f"Payloads JSON inválidos: {invalid_json}")
    print(f"Payloads vazios/nulos: {empty_payloads}")
    print(f"Payloads válidos: {len(messages) - invalid_json - empty_payloads}")
    print()

    print("Chaves de nível superior (frequência):")
    for key, count in top_level_keys.most_common():
        pct = (count / len(messages)) * 100
        print(f"  {key}: {count} ({pct:.1f}%)")
    print()

    print("Variações de tipo de campo:")
    for field, types in sorted(field_types.items()):
        if len(types) > 1:
            print(f"  {field}: {', '.join(sorted(types))}")
    print()

    print("Estruturas aninhadas:")
    for parent, children in sorted(nested_structures.items()):
        print(f"  {parent}:")
        for child, count in children.most_common():
            print(f"    {child}: {count}")
    print()

def export_payloads(messages):
    records = []
    for msg in messages:
        record = {
            'id': msg['id'],
            'channel_id': msg['channel_id'],
            'message_id': msg['message_id'],
            'received_at': msg['received_at'],
            'schema_version': msg['schema_version'],
        }

        payload_str = msg['payload']
        if payload_str and payload_str != 'null':
            try:
                record['payload'] = json.loads(payload_str)
            except json.JSONDecodeError:
                record['payload'] = None
                record['payload_invalid'] = True
        else:
            record['payload'] = None

        records.append(record)

    with open('payloads_export.json', 'w', encoding='utf-8') as f:
        json.dump(records, f, indent=2, ensure_ascii=False)

    print("=== EXPORTAÇÃO ===")
    print(f"Todos os payloads exportados para: payloads_export.json")
    print(f"Use com jq: jq '.[] | .payload' payloads_export.json")

if __name__ == '__main__':
    analyze_database()
