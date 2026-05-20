#!/usr/bin/env bash
# Generate a markdown benchmark report and detect regressions.
# Outputs: report to OUTPUT_FILE, "regression=true/false" to stdout for CI.
set -uo pipefail

OLD_FILE="${1:-/tmp/old.txt}"
NEW_FILE="${2:-/tmp/new.txt}"
OUTPUT_FILE="${3:-/tmp/report.md}"
REGRESSION_THRESHOLD="${4:-20}"

# Parse benchmark file: outputs "name|time|allocs" per line
parse_bench() {
    grep -E "^Benchmark.*allocs/op" "$1" 2>/dev/null | awk '{
        name=$1; gsub(/-[0-9]+$/,"",name); gsub(/^Benchmark/,"",name)
        for(i=1;i<=NF;i++) {
            if($(i+1)=="ns/op"||$(i+1)=="µs/op"||$(i+1)=="ms/op") time=$i" "$(i+1)
            if($(i+1)=="allocs/op") allocs=$i
        }
        if(name&&allocs) print name"|"time"|"allocs
    }'
}

# Cache parsed results
OLD_DATA=$(parse_bench "$OLD_FILE" || echo "")
NEW_DATA=$(parse_bench "$NEW_FILE" || echo "")

# Track regression state
REGRESSION="false"

# Build report and detect regressions
{
    echo "## Benchmark Results"
    echo ""
    echo "<details open>"
    echo "<summary>Comparison: main vs PR</summary>"
    echo ""
    echo "| Benchmark | main (Time) | PR (Time) | main (Allocs) | PR (Allocs) | Change |"
    echo "|-----------|-------------|-----------|---------------|-------------|--------|"

    echo "$NEW_DATA" | while IFS='|' read -r name new_time new_allocs; do
        [ -z "$name" ] && continue
        old=$(echo "$OLD_DATA" | grep "^${name}|" | head -1 || echo "")
        if [ -n "$old" ]; then
            old_time=$(echo "$old" | cut -d'|' -f2)
            old_allocs=$(echo "$old" | cut -d'|' -f3)
            if [ "$old_allocs" -gt 0 ] 2>/dev/null; then
                pct=$(awk "BEGIN{printf \"%.1f\",(($new_allocs-$old_allocs)/$old_allocs)*100}")
                pct_int="${pct%.*}"
                if [ "$pct_int" -ge "$REGRESSION_THRESHOLD" ] 2>/dev/null; then
                    change=":red_circle: +${pct}%"
                    echo "REGRESSION_DETECTED" >&2
                elif [ "$pct_int" -gt 0 ] 2>/dev/null; then
                    change=":small_red_triangle: +${pct}%"
                elif [ "$pct_int" -lt -5 ] 2>/dev/null; then
                    change=":green_circle: ${pct}%"
                else
                    change="${pct}%"
                fi
            else
                change="N/A"
            fi
        else
            old_time="-"; old_allocs="-"; change="new"
        fi
        echo "| $name | $old_time | $new_time | $old_allocs | $new_allocs | $change |"
    done

    echo "</details>"
    echo ""
} > "$OUTPUT_FILE" 2>/tmp/regression_flag

# Check if regression was detected during report generation
if grep -q "REGRESSION_DETECTED" /tmp/regression_flag 2>/dev/null; then
    REGRESSION="true"
    echo ":warning: **Warning:** Significant allocation regression detected (>$REGRESSION_THRESHOLD%)" >> "$OUTPUT_FILE"
else
    echo ":white_check_mark: No significant performance regression detected" >> "$OUTPUT_FILE"
fi

# Output regression status for CI to capture
echo "regression=$REGRESSION"
