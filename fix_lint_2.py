import os

path = "internal/logger/pretty_test.go"
with open(path, 'r') as f:
    content = f.read()
content = content.replace('if !strings.HasPrefix(trimmed, "◆") && !strings.HasPrefix(trimmed, "├") && !strings.HasPrefix(trimmed, "└") {',
'''hasDiamond := strings.HasPrefix(trimmed, "◆")
hasPipe := strings.HasPrefix(trimmed, "├")
hasCorner := strings.HasPrefix(trimmed, "└")
if !hasDiamond && !hasPipe && !hasCorner {''')
with open(path, 'w') as f:
    f.write(content)

path = "tools/payload-analyzer/main.go"
with open(path, 'r') as f:
    lines = f.readlines()
with open(path, 'w') as f:
    for line in lines:
        if "tables := []string{}" in line:
            continue
        if "var tables []string" in line:
            continue
        f.write(line)
