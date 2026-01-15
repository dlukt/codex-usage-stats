#!/usr/bin/env python3
"""Generate Codex CLI usage statistics markdown file."""

import json
import re
from datetime import datetime
from collections import defaultdict
from pathlib import Path

CODEX_DIR = Path.home() / ".codex"
HISTORY_FILE = CODEX_DIR / "history.jsonl"
SESSIONS_DIR = CODEX_DIR / "sessions"
OUTPUT_FILE = CODEX_DIR / "usage-statistics.md"
COPILOT_LIMIT = 1500


def parse_history():
    """Parse history.jsonl and return message counts by date."""
    daily_counts = defaultdict(int)

    if not HISTORY_FILE.exists():
        return daily_counts

    with open(HISTORY_FILE, "r") as f:
        for line in f:
            try:
                entry = json.loads(line.strip())
                ts = entry.get("ts")
                if ts:
                    date = datetime.fromtimestamp(ts).strftime("%Y-%m-%d")
                    daily_counts[date] += 1
            except (json.JSONDecodeError, KeyError):
                continue

    return daily_counts


def parse_session_tokens(session_file):
    """Parse a session file and return the final token counts."""
    tokens = {
        "input_tokens": 0,
        "output_tokens": 0,
        "cached_input_tokens": 0,
        "reasoning_output_tokens": 0,
        "total_tokens": 0,
    }

    try:
        with open(session_file, "r") as f:
            for line in f:
                try:
                    entry = json.loads(line.strip())
                    # Look for token_count in event_msg payload
                    if entry.get("type") == "event_msg":
                        payload = entry.get("payload", {})
                        if payload.get("type") == "token_count":
                            info = payload.get("info")
                            if info and "total_token_usage" in info:
                                usage = info["total_token_usage"]
                                # Update with latest values (cumulative)
                                tokens["input_tokens"] = usage.get("input_tokens", 0)
                                tokens["output_tokens"] = usage.get("output_tokens", 0)
                                tokens["cached_input_tokens"] = usage.get("cached_input_tokens", 0)
                                tokens["reasoning_output_tokens"] = usage.get("reasoning_output_tokens", 0)
                                tokens["total_tokens"] = usage.get("total_tokens", 0)
                except json.JSONDecodeError:
                    continue
    except Exception:
        pass

    return tokens


def collect_token_stats():
    """Collect token statistics from all session files."""
    monthly_tokens = defaultdict(lambda: {
        "input_tokens": 0,
        "output_tokens": 0,
        "cached_input_tokens": 0,
        "reasoning_output_tokens": 0,
        "total_tokens": 0,
        "session_count": 0,
    })

    if not SESSIONS_DIR.exists():
        return monthly_tokens

    for year_dir in SESSIONS_DIR.iterdir():
        if not year_dir.is_dir():
            continue
        year = year_dir.name

        for month_dir in year_dir.iterdir():
            if not month_dir.is_dir():
                continue
            month = month_dir.name
            month_key = f"{year}-{month}"

            for day_dir in month_dir.iterdir():
                if not day_dir.is_dir():
                    continue

                for session_file in day_dir.glob("*.jsonl"):
                    tokens = parse_session_tokens(session_file)
                    monthly_tokens[month_key]["input_tokens"] += tokens["input_tokens"]
                    monthly_tokens[month_key]["output_tokens"] += tokens["output_tokens"]
                    monthly_tokens[month_key]["cached_input_tokens"] += tokens["cached_input_tokens"]
                    monthly_tokens[month_key]["reasoning_output_tokens"] += tokens["reasoning_output_tokens"]
                    monthly_tokens[month_key]["total_tokens"] += tokens["total_tokens"]
                    monthly_tokens[month_key]["session_count"] += 1

    return monthly_tokens


def count_sessions():
    """Count session files per month."""
    monthly_sessions = defaultdict(int)

    if not SESSIONS_DIR.exists():
        return monthly_sessions

    for year_dir in SESSIONS_DIR.iterdir():
        if not year_dir.is_dir():
            continue
        year = year_dir.name

        for month_dir in year_dir.iterdir():
            if not month_dir.is_dir():
                continue
            month = month_dir.name
            month_key = f"{year}-{month}"

            for day_dir in month_dir.iterdir():
                if day_dir.is_dir():
                    monthly_sessions[month_key] += len(list(day_dir.glob("*.jsonl")))

    return monthly_sessions


def calculate_stats(daily_counts):
    """Calculate various statistics from daily counts."""
    if not daily_counts:
        return {}

    dates = sorted(daily_counts.keys())
    total_messages = sum(daily_counts.values())
    active_days = len(daily_counts)

    monthly_messages = defaultdict(int)
    for date, count in daily_counts.items():
        month_key = date[:7]
        monthly_messages[month_key] += count

    top_days = sorted(daily_counts.items(), key=lambda x: x[1], reverse=True)[:10]
    peak_day = max(daily_counts.items(), key=lambda x: x[1])
    peak_month = max(monthly_messages.items(), key=lambda x: x[1])
    avg_per_day = total_messages / active_days if active_days > 0 else 0

    return {
        "total_messages": total_messages,
        "active_days": active_days,
        "date_range": (dates[0], dates[-1]),
        "monthly_messages": dict(sorted(monthly_messages.items())),
        "top_days": top_days,
        "peak_day": peak_day,
        "peak_month": peak_month,
        "avg_per_day": avg_per_day,
        "daily_counts": dict(sorted(daily_counts.items())),
    }


def get_day_of_week(date_str):
    """Get day of week name from YYYY-MM-DD string."""
    dt = datetime.strptime(date_str, "%Y-%m-%d")
    return dt.strftime("%A")


def format_month_name(month_key):
    """Convert YYYY-MM to 'Mon YYYY' format."""
    dt = datetime.strptime(month_key + "-01", "%Y-%m-%d")
    return dt.strftime("%b %Y")


def format_tokens(n):
    """Format token count with K/M suffix."""
    if n >= 1_000_000:
        return f"{n/1_000_000:.1f}M"
    elif n >= 1_000:
        return f"{n/1_000:.1f}K"
    return str(n)


def generate_markdown(stats, monthly_sessions, monthly_tokens):
    """Generate the markdown report."""
    now = datetime.now().strftime("%B %d, %Y")

    # Calculate total tokens
    total_input = sum(t["input_tokens"] for t in monthly_tokens.values())
    total_output = sum(t["output_tokens"] for t in monthly_tokens.values())
    total_cached = sum(t["cached_input_tokens"] for t in monthly_tokens.values())
    total_reasoning = sum(t["reasoning_output_tokens"] for t in monthly_tokens.values())
    total_tokens = sum(t["total_tokens"] for t in monthly_tokens.values())

    lines = [
        "# Codex CLI Usage Statistics",
        "",
        f"**Generated**: {now}",
        "**Data Source**: `~/.codex/history.jsonl` and `~/.codex/sessions/`",
        "",
        "---",
        "",
        "## Summary",
        "",
        "| Metric | Value |",
        "|--------|-------|",
        f"| **Total Messages** | {stats['total_messages']:,} |",
        f"| **Active Days** | {stats['active_days']} |",
        f"| **Date Range** | {stats['date_range'][0]} - {stats['date_range'][1]} |",
        f"| **Average per Active Day** | ~{stats['avg_per_day']:.0f} messages |",
        f"| **Peak Day** | {stats['peak_day'][0]} ({stats['peak_day'][1]} messages) |",
        "",
        "---",
        "",
        "## Token Usage Summary",
        "",
        "| Metric | Tokens |",
        "|--------|--------|",
        f"| **Total Tokens** | {total_tokens:,} ({format_tokens(total_tokens)}) |",
        f"| **Input Tokens** | {total_input:,} ({format_tokens(total_input)}) |",
        f"| **Output Tokens** | {total_output:,} ({format_tokens(total_output)}) |",
        f"| **Cached Input** | {total_cached:,} ({format_tokens(total_cached)}) |",
        f"| **Reasoning Output** | {total_reasoning:,} ({format_tokens(total_reasoning)}) |",
        "",
        "---",
        "",
        "## Monthly Breakdown",
        "",
        "| Month | Messages | Sessions | Copilot Pro % |",
        "|-------|----------|----------|---------------|",
    ]

    total_sessions = 0
    for month_key, msg_count in stats["monthly_messages"].items():
        session_count = monthly_sessions.get(month_key, 0)
        total_sessions += session_count
        quota_pct = (msg_count / COPILOT_LIMIT) * 100
        month_name = format_month_name(month_key)
        lines.append(f"| {month_name} | {msg_count:,} | {session_count} | {quota_pct:.0f}% |")

    lines.extend([
        f"| **Total** | **{stats['total_messages']:,}** | **{total_sessions}** | — |",
        "",
        f"*Copilot Pro allows {COPILOT_LIMIT:,} messages/month*",
        "",
        "---",
        "",
        "## Monthly Token Usage",
        "",
        "| Month | Total | Input | Output | Cached | Reasoning |",
        "|-------|-------|-------|--------|--------|-----------|",
    ])

    for month_key in sorted(monthly_tokens.keys()):
        t = monthly_tokens[month_key]
        month_name = format_month_name(month_key)
        lines.append(
            f"| {month_name} | {format_tokens(t['total_tokens'])} | "
            f"{format_tokens(t['input_tokens'])} | {format_tokens(t['output_tokens'])} | "
            f"{format_tokens(t['cached_input_tokens'])} | {format_tokens(t['reasoning_output_tokens'])} |"
        )

    lines.extend([
        f"| **Total** | **{format_tokens(total_tokens)}** | **{format_tokens(total_input)}** | "
        f"**{format_tokens(total_output)}** | **{format_tokens(total_cached)}** | **{format_tokens(total_reasoning)}** |",
        "",
        "---",
        "",
        "## Top 10 Busiest Days",
        "",
        "| Rank | Date | Messages | Day of Week |",
        "|------|------|----------|-------------|",
    ])

    for i, (date, count) in enumerate(stats["top_days"], 1):
        dow = get_day_of_week(date)
        lines.append(f"| {i} | {date} | {count} | {dow} |")

    lines.extend([
        "",
        "---",
        "",
        "## Daily Breakdown (All Active Days)",
        "",
    ])

    daily_by_month = defaultdict(list)
    for date, count in stats["daily_counts"].items():
        month_key = date[:7]
        daily_by_month[month_key].append((date, count))

    for month_key in sorted(daily_by_month.keys()):
        days = daily_by_month[month_key]
        month_total = sum(c for _, c in days)
        month_name = format_month_name(month_key)

        lines.extend([
            f"### {month_name} ({month_total:,} messages, {len(days)} days)",
            "| Date | Messages |",
            "|------|----------|",
        ])

        for date, count in days:
            day_part = date[8:]
            month_abbr = datetime.strptime(date, "%Y-%m-%d").strftime("%b")
            lines.append(f"| {month_abbr} {int(day_part):02d} | {count} |")

        lines.append("")

    # Analysis section
    peak_month_name = format_month_name(stats["peak_month"][0])
    peak_month_count = stats["peak_month"][1]
    avg_month = stats["total_messages"] / len(stats["monthly_messages"]) if stats["monthly_messages"] else 0

    lines.extend([
        "---",
        "",
        "## Usage Pattern Analysis",
        "",
        "### Copilot Pro Comparison",
        "",
        "| Metric | Your Usage | Copilot Pro Limit | Assessment |",
        "|--------|------------|-------------------|------------|",
        f"| Peak Month | {peak_month_count:,} ({peak_month_name}) | {COPILOT_LIMIT:,} | {'Pass' if peak_month_count <= COPILOT_LIMIT else 'Exceed'} ({peak_month_count/COPILOT_LIMIT*100:.0f}% of limit) |",
        f"| Average Month | ~{avg_month:,.0f} | {COPILOT_LIMIT:,} | {'Pass' if avg_month <= COPILOT_LIMIT else 'Exceed'} ({avg_month/COPILOT_LIMIT*100:.0f}% of limit) |",
        f"| Peak Day x 30 | {stats['peak_day'][1] * 30:,} (theoretical) | {COPILOT_LIMIT:,} | {'Would exceed' if stats['peak_day'][1] * 30 > COPILOT_LIMIT else 'Pass'} if every day was peak |",
        "",
        "---",
        "",
        "*Data extracted from ~/.codex/history.jsonl and ~/.codex/sessions/*",
    ])

    return "\n".join(lines)


def main():
    print("Parsing history.jsonl...")
    daily_counts = parse_history()

    if not daily_counts:
        print("No history data found.")
        return

    print(f"Found {sum(daily_counts.values())} messages across {len(daily_counts)} days")

    print("Counting session files...")
    monthly_sessions = count_sessions()
    print(f"Found {sum(monthly_sessions.values())} session files")

    print("Collecting token statistics from sessions (this may take a moment)...")
    monthly_tokens = collect_token_stats()
    total_tokens = sum(t["total_tokens"] for t in monthly_tokens.values())
    print(f"Found {total_tokens:,} total tokens across all sessions")

    print("Calculating statistics...")
    stats = calculate_stats(daily_counts)

    print("Generating markdown...")
    markdown = generate_markdown(stats, monthly_sessions, monthly_tokens)

    print(f"Writing to {OUTPUT_FILE}...")
    with open(OUTPUT_FILE, "w") as f:
        f.write(markdown)

    print(f"Done! Statistics written to {OUTPUT_FILE}")


if __name__ == "__main__":
    main()
