import os, re

def replace_in_file(path, replacements):
    with open(path, 'r') as f:
        content = f.read()
    for old, new in replacements:
        content = content.replace(old, new)
    with open(path, 'w') as f:
        f.write(content)

replace_in_file("cmd/spike_resolve/main.go", [
    ("defer db.Close()", "defer func() { _ = db.Close() }()"),
    ("defer rows.Close()", "defer func() { _ = rows.Close() }()"),
    ("defer resp.Body.Close()", "defer func() { _ = resp.Body.Close() }()")
])

replace_in_file("internal/cli/channels.go", [
    ("fmt.Fprintf(out, \"  • @%-30s  id:%-12d  %s\\n\", ch.Username, ch.ID, status)", "_, _ = fmt.Fprintf(out, \"  • @%-30s  id:%-12d  %s\\n\", ch.Username, ch.ID, status)"),
    ("fmt.Fprintf(out, \"  Total: %d canal(is)\\n\", len(channels))", "_, _ = fmt.Fprintf(out, \"  Total: %d canal(is)\\n\", len(channels))")
])

replace_in_file("internal/config/wizard_test.go", [
    ("wFile.Write([]byte(tt.input + \"\\n\"))", "_, _ = wFile.Write([]byte(tt.input + \"\\n\"))"),
    ("wFile.Close()", "_ = wFile.Close()"),
    ("os.Chdir(tmpDir)", "_ = os.Chdir(tmpDir)"),
    ("defer os.Chdir(oldWd)", "defer func() { _ = os.Chdir(oldWd) }()")
])

replace_in_file("internal/dashboard/server.go", [
    ("fmt.Fprint(w, indexHTML)", "_, _ = fmt.Fprint(w, indexHTML)"),
    ("fmt.Fprintf(w, \"event: %s\\ndata: %s\\n\\n\", event.Type, event.Data)", "_, _ = fmt.Fprintf(w, \"event: %s\\ndata: %s\\n\\n\", event.Type, event.Data)")
])

replace_in_file("internal/processor/repository.go", [
    ("defer tx.Rollback()", "defer func() { _ = tx.Rollback() }()")
])

replace_in_file("internal/storage/repository.go", [
    ("defer tx.Rollback()", "defer func() { _ = tx.Rollback() }()"),
    ("defer rows.Close()", "defer func() { _ = rows.Close() }()")
])

replace_in_file("internal/storage/repository_test.go", [
    ("defer db.Close()", "defer func() { _ = db.Close() }()"),
    ("defer db2.Close()", "defer func() { _ = db2.Close() }()")
])

replace_in_file("internal/telegram/auth.go", [
    ("fmt.Fprintln(out)", "_, _ = fmt.Fprintln(out)"),
    ("fmt.Fprintln(out, \"  📱 Etapa 2/2 · Login da conta Telegram\")", "_, _ = fmt.Fprintln(out, \"  📱 Etapa 2/2 · Login da conta Telegram\")"),
    ("fmt.Fprintln(out, \"  ──────────────────────────────────────\")", "_, _ = fmt.Fprintln(out, \"  ──────────────────────────────────────\")")
])

replace_in_file("tools/payload-analyzer/main.go", [
    ("defer db.Close()", "defer func() { _ = db.Close() }()"),
    ("tableRows.Close()", "_ = tableRows.Close()"),
    ("defer rows.Close()", "defer func() { _ = rows.Close() }()"),
    ("defer msgRows.Close()", "defer func() { _ = msgRows.Close() }()"),
    ("defer f.Close()", "defer func() { _ = f.Close() }()"),
    ("tables = append(tables, name)", "_ = name"),
    ("var fieldTypes map[string]map[string]bool = make(map[string]map[string]bool)", "fieldTypes := make(map[string]map[string]bool)")
])

replace_in_file("internal/telegram/client.go", [
    ("if !((c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || c == '_') {", "if (c < 'a' || c > 'z') && (c < 'A' || c > 'Z') && (c < '0' || c > '9') && c != '_' {")
])

replace_in_file("internal/logger/error.go", [
    ("func errorAttr(err error) (slog.Attr, bool) {", "// errorAttr is intentionally unused for now\nfunc errorAttr(err error) (slog.Attr, bool) {")
])

replace_in_file("internal/telegram/dispatcher_test.go", [
    ("type errHandler struct{ called atomic.Bool }", "// errHandler is intentionally unused for now\ntype errHandler struct{ called atomic.Bool }")
])

