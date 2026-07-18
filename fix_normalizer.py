import re

with open("internal/processor/normalizer.go", "r") as f:
    lines = f.readlines()

new_lines = []
skip = False
for i, line in enumerate(lines):
    if line.strip() == "//nolint:unused":
        continue
    if "reCoupon   = regexp.MustCompile" in line:
        continue
    if "rePix        = regexp.MustCompile" in line:
        continue
    if "reCorre    = regexp.MustCompile" in line:
        continue
    if "reUltima   = regexp.MustCompile" in line:
        continue
    if "reNacional = regexp.MustCompile" in line:
        continue
    new_lines.append(line)

with open("internal/processor/normalizer.go", "w") as f:
    f.writelines(new_lines)
