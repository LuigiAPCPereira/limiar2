import os

def replace_in_file(path, replacements):
    with open(path, 'r') as f:
        content = f.read()
    for old, new in replacements:
        content = content.replace(old, new)
    with open(path, 'w') as f:
        f.write(content)

replace_in_file("internal/cli/channels.go", [
    ("fmt.Fprintln(out)", "_, _ = fmt.Fprintln(out)")
])

replace_in_file("internal/telegram/auth.go", [
    ("fmt.Fprintln(a.out, \"  Telegram enviou um código para o seu aplicativo ou SMS.\")", "_, _ = fmt.Fprintln(a.out, \"  Telegram enviou um código para o seu aplicativo ou SMS.\")")
])

replace_in_file("internal/telegram/client.go", [
    ("fmt.Fprintln(os.Stdout, \"\\n  �� Conectando ao Telegram...\")", "_, _ = fmt.Fprintln(os.Stdout, \"\\n  📡 Conectando ao Telegram...\")")
])

replace_in_file("internal/logger/pretty_test.go", [
    ("""//nolint:staticcheck
if !strings.HasPrefix(trimmed, "◆") && !strings.HasPrefix(trimmed, "├") && !strings.HasPrefix(trimmed, "└") {
Could be a continuation — just check no garbled output
tinue
"""hasPrefix := false
_, prefix := range []string{"◆", "├", "└"} {
strings.HasPrefix(trimmed, prefix) {
= true
!hasPrefix {
tinue
])

