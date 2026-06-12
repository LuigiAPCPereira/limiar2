import os

def replace_in_file(path, replacements):
    with open(path, 'r') as f:
        content = f.read()
    for old, new in replacements:
        content = content.replace(old, new)
    with open(path, 'w') as f:
        f.write(content)

replace_in_file("internal/processor/repository.go", [
    ("defer rows.Close()", "defer func() { _ = rows.Close() }()")
])

replace_in_file("internal/telegram/auth.go", [
    ("fmt.Fprintln(a.out, \"  Informe o número de telefone da conta (formato internacional).\")", "_, _ = fmt.Fprintln(a.out, \"  Informe o número de telefone da conta (formato internacional).\")"),
    ("fmt.Fprintln(a.out)", "_, _ = fmt.Fprintln(a.out)"),
    ("fmt.Fprintln(a.out, \"  🔒 Verificação em duas etapas ativada.\")", "_, _ = fmt.Fprintln(a.out, \"  🔒 Verificação em duas etapas ativada.\")")
])

replace_in_file("internal/logger/pretty_test.go", [
    ("""hasDiamond := strings.HasPrefix(trimmed, "◆")
hasPipe := strings.HasPrefix(trimmed, "├")
hasCorner := strings.HasPrefix(trimmed, "└")
if !hasDiamond && !hasPipe && !hasCorner {""", """//nolint:staticcheck
if !strings.HasPrefix(trimmed, "◆") && !strings.HasPrefix(trimmed, "├") && !strings.HasPrefix(trimmed, "└") {""")
])

replace_in_file("internal/logger/error.go", [
    ("// errorAttr is intentionally unused for now\nfunc errorAttr(err error)", "//nolint:unused\nfunc errorAttr(err error)")
])

replace_in_file("internal/telegram/dispatcher_test.go", [
    ("// errHandler is intentionally unused for now\ntype errHandler", "//nolint:unused\ntype errHandler"),
    ("func (h *errHandler) HandleUpdate", "//nolint:unused\nfunc (h *errHandler) HandleUpdate")
])

