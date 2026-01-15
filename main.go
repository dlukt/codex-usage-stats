package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

const copilotLimit = 1500

var (
	codexDir    = filepath.Join(os.Getenv("HOME"), ".codex")
	historyFile = filepath.Join(codexDir, "history.jsonl")
	sessionsDir = filepath.Join(codexDir, "sessions")
	outputFile  = filepath.Join(codexDir, "usage-statistics.md")
)

type HistoryEntry struct {
	SessionID string `json:"session_id"`
	Timestamp int64  `json:"ts"`
	Text      string `json:"text"`
}

type TokenUsage struct {
	InputTokens           int64 `json:"input_tokens"`
	CachedInputTokens     int64 `json:"cached_input_tokens"`
	OutputTokens          int64 `json:"output_tokens"`
	ReasoningOutputTokens int64 `json:"reasoning_output_tokens"`
	TotalTokens           int64 `json:"total_tokens"`
}

type TokenInfo struct {
	TotalTokenUsage TokenUsage `json:"total_token_usage"`
}

type TokenPayload struct {
	Type string     `json:"type"`
	Info *TokenInfo `json:"info"`
}

type EventMsg struct {
	Type    string       `json:"type"`
	Payload TokenPayload `json:"payload"`
}

type MonthlyTokens struct {
	InputTokens           int64
	CachedInputTokens     int64
	OutputTokens          int64
	ReasoningOutputTokens int64
	TotalTokens           int64
	SessionCount          int
}

type Stats struct {
	TotalMessages   int
	ActiveDays      int
	DateRange       [2]string
	MonthlyMessages map[string]int
	TopDays         [][2]interface{} // [date string, count int]
	PeakDay         [2]interface{}
	PeakMonth       [2]interface{}
	AvgPerDay       float64
	DailyCounts     map[string]int
}

func parseHistory() map[string]int {
	dailyCounts := make(map[string]int)

	file, err := os.Open(historyFile)
	if err != nil {
		return dailyCounts
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	// Increase buffer size for long lines
	buf := make([]byte, 0, 1024*1024)
	scanner.Buffer(buf, 10*1024*1024)

	for scanner.Scan() {
		var entry HistoryEntry
		if err := json.Unmarshal(scanner.Bytes(), &entry); err != nil {
			continue
		}
		if entry.Timestamp > 0 {
			date := time.Unix(entry.Timestamp, 0).Format("2006-01-02")
			dailyCounts[date]++
		}
	}

	return dailyCounts
}

func parseSessionTokens(sessionFile string) TokenUsage {
	var tokens TokenUsage

	file, err := os.Open(sessionFile)
	if err != nil {
		return tokens
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	buf := make([]byte, 0, 1024*1024)
	scanner.Buffer(buf, 10*1024*1024)

	for scanner.Scan() {
		var event EventMsg
		if err := json.Unmarshal(scanner.Bytes(), &event); err != nil {
			continue
		}
		if event.Type == "event_msg" && event.Payload.Type == "token_count" {
			if event.Payload.Info != nil {
				usage := event.Payload.Info.TotalTokenUsage
				tokens.InputTokens = usage.InputTokens
				tokens.CachedInputTokens = usage.CachedInputTokens
				tokens.OutputTokens = usage.OutputTokens
				tokens.ReasoningOutputTokens = usage.ReasoningOutputTokens
				tokens.TotalTokens = usage.TotalTokens
			}
		}
	}

	return tokens
}

func collectTokenStats() map[string]*MonthlyTokens {
	monthlyTokens := make(map[string]*MonthlyTokens)

	years, err := os.ReadDir(sessionsDir)
	if err != nil {
		return monthlyTokens
	}

	for _, yearEntry := range years {
		if !yearEntry.IsDir() {
			continue
		}
		year := yearEntry.Name()
		yearPath := filepath.Join(sessionsDir, year)

		months, err := os.ReadDir(yearPath)
		if err != nil {
			continue
		}

		for _, monthEntry := range months {
			if !monthEntry.IsDir() {
				continue
			}
			month := monthEntry.Name()
			monthKey := fmt.Sprintf("%s-%s", year, month)
			monthPath := filepath.Join(yearPath, month)

			if monthlyTokens[monthKey] == nil {
				monthlyTokens[monthKey] = &MonthlyTokens{}
			}

			days, err := os.ReadDir(monthPath)
			if err != nil {
				continue
			}

			for _, dayEntry := range days {
				if !dayEntry.IsDir() {
					continue
				}
				dayPath := filepath.Join(monthPath, dayEntry.Name())

				files, err := os.ReadDir(dayPath)
				if err != nil {
					continue
				}

				for _, fileEntry := range files {
					if !strings.HasSuffix(fileEntry.Name(), ".jsonl") {
						continue
					}
					sessionFile := filepath.Join(dayPath, fileEntry.Name())
					tokens := parseSessionTokens(sessionFile)

					monthlyTokens[monthKey].InputTokens += tokens.InputTokens
					monthlyTokens[monthKey].CachedInputTokens += tokens.CachedInputTokens
					monthlyTokens[monthKey].OutputTokens += tokens.OutputTokens
					monthlyTokens[monthKey].ReasoningOutputTokens += tokens.ReasoningOutputTokens
					monthlyTokens[monthKey].TotalTokens += tokens.TotalTokens
					monthlyTokens[monthKey].SessionCount++
				}
			}
		}
	}

	return monthlyTokens
}

func countSessions() map[string]int {
	monthlySessions := make(map[string]int)

	years, err := os.ReadDir(sessionsDir)
	if err != nil {
		return monthlySessions
	}

	for _, yearEntry := range years {
		if !yearEntry.IsDir() {
			continue
		}
		year := yearEntry.Name()
		yearPath := filepath.Join(sessionsDir, year)

		months, err := os.ReadDir(yearPath)
		if err != nil {
			continue
		}

		for _, monthEntry := range months {
			if !monthEntry.IsDir() {
				continue
			}
			month := monthEntry.Name()
			monthKey := fmt.Sprintf("%s-%s", year, month)
			monthPath := filepath.Join(yearPath, month)

			days, err := os.ReadDir(monthPath)
			if err != nil {
				continue
			}

			for _, dayEntry := range days {
				if !dayEntry.IsDir() {
					continue
				}
				dayPath := filepath.Join(monthPath, dayEntry.Name())

				files, err := os.ReadDir(dayPath)
				if err != nil {
					continue
				}

				for _, f := range files {
					if strings.HasSuffix(f.Name(), ".jsonl") {
						monthlySessions[monthKey]++
					}
				}
			}
		}
	}

	return monthlySessions
}

func calculateStats(dailyCounts map[string]int) Stats {
	var stats Stats

	if len(dailyCounts) == 0 {
		return stats
	}

	// Get sorted dates
	dates := make([]string, 0, len(dailyCounts))
	for date := range dailyCounts {
		dates = append(dates, date)
	}
	sort.Strings(dates)

	// Calculate totals
	totalMessages := 0
	for _, count := range dailyCounts {
		totalMessages += count
	}

	// Monthly aggregation
	monthlyMessages := make(map[string]int)
	for date, count := range dailyCounts {
		monthKey := date[:7]
		monthlyMessages[monthKey] += count
	}

	// Top days
	type dayCount struct {
		date  string
		count int
	}
	dayCounts := make([]dayCount, 0, len(dailyCounts))
	for date, count := range dailyCounts {
		dayCounts = append(dayCounts, dayCount{date, count})
	}
	sort.Slice(dayCounts, func(i, j int) bool {
		return dayCounts[i].count > dayCounts[j].count
	})

	topDays := make([][2]interface{}, 0, 10)
	for i := 0; i < len(dayCounts) && i < 10; i++ {
		topDays = append(topDays, [2]interface{}{dayCounts[i].date, dayCounts[i].count})
	}

	// Peak stats
	peakDay := [2]interface{}{dayCounts[0].date, dayCounts[0].count}

	var peakMonth [2]interface{}
	maxMonthCount := 0
	for month, count := range monthlyMessages {
		if count > maxMonthCount {
			maxMonthCount = count
			peakMonth = [2]interface{}{month, count}
		}
	}

	stats.TotalMessages = totalMessages
	stats.ActiveDays = len(dailyCounts)
	stats.DateRange = [2]string{dates[0], dates[len(dates)-1]}
	stats.MonthlyMessages = monthlyMessages
	stats.TopDays = topDays
	stats.PeakDay = peakDay
	stats.PeakMonth = peakMonth
	stats.AvgPerDay = float64(totalMessages) / float64(len(dailyCounts))
	stats.DailyCounts = dailyCounts

	return stats
}

func getDayOfWeek(dateStr string) string {
	t, err := time.Parse("2006-01-02", dateStr)
	if err != nil {
		return ""
	}
	return t.Weekday().String()
}

func formatMonthName(monthKey string) string {
	t, err := time.Parse("2006-01", monthKey)
	if err != nil {
		return monthKey
	}
	return t.Format("Jan 2006")
}

func formatTokens(n int64) string {
	if n >= 1_000_000 {
		return fmt.Sprintf("%.1fM", float64(n)/1_000_000)
	} else if n >= 1_000 {
		return fmt.Sprintf("%.1fK", float64(n)/1_000)
	}
	return fmt.Sprintf("%d", n)
}

func generateMarkdown(stats Stats, monthlySessions map[string]int, monthlyTokens map[string]*MonthlyTokens) string {
	var sb strings.Builder
	now := time.Now().Format("January 02, 2006")

	// Calculate total tokens
	var totalInput, totalOutput, totalCached, totalReasoning, totalTokens int64
	for _, t := range monthlyTokens {
		totalInput += t.InputTokens
		totalOutput += t.OutputTokens
		totalCached += t.CachedInputTokens
		totalReasoning += t.ReasoningOutputTokens
		totalTokens += t.TotalTokens
	}

	sb.WriteString("# Codex CLI Usage Statistics\n\n")
	sb.WriteString(fmt.Sprintf("**Generated**: %s\n", now))
	sb.WriteString("**Data Source**: `~/.codex/history.jsonl` and `~/.codex/sessions/`\n\n")
	sb.WriteString("---\n\n")

	// Summary
	sb.WriteString("## Summary\n\n")
	sb.WriteString("| Metric | Value |\n")
	sb.WriteString("|--------|-------|\n")
	sb.WriteString(fmt.Sprintf("| **Total Messages** | %d |\n", stats.TotalMessages))
	sb.WriteString(fmt.Sprintf("| **Active Days** | %d |\n", stats.ActiveDays))
	sb.WriteString(fmt.Sprintf("| **Date Range** | %s - %s |\n", stats.DateRange[0], stats.DateRange[1]))
	sb.WriteString(fmt.Sprintf("| **Average per Active Day** | ~%.0f messages |\n", stats.AvgPerDay))
	sb.WriteString(fmt.Sprintf("| **Peak Day** | %s (%d messages) |\n", stats.PeakDay[0], stats.PeakDay[1]))
	sb.WriteString("\n---\n\n")

	// Token Usage Summary
	sb.WriteString("## Token Usage Summary\n\n")
	sb.WriteString("| Metric | Tokens |\n")
	sb.WriteString("|--------|--------|\n")
	sb.WriteString(fmt.Sprintf("| **Total Tokens** | %d (%s) |\n", totalTokens, formatTokens(totalTokens)))
	sb.WriteString(fmt.Sprintf("| **Input Tokens** | %d (%s) |\n", totalInput, formatTokens(totalInput)))
	sb.WriteString(fmt.Sprintf("| **Output Tokens** | %d (%s) |\n", totalOutput, formatTokens(totalOutput)))
	sb.WriteString(fmt.Sprintf("| **Cached Input** | %d (%s) |\n", totalCached, formatTokens(totalCached)))
	sb.WriteString(fmt.Sprintf("| **Reasoning Output** | %d (%s) |\n", totalReasoning, formatTokens(totalReasoning)))
	sb.WriteString("\n---\n\n")

	// Monthly Breakdown
	sb.WriteString("## Monthly Breakdown\n\n")
	sb.WriteString("| Month | Messages | Sessions | Copilot Pro % |\n")
	sb.WriteString("|-------|----------|----------|---------------|\n")

	months := make([]string, 0, len(stats.MonthlyMessages))
	for m := range stats.MonthlyMessages {
		months = append(months, m)
	}
	sort.Strings(months)

	totalSessions := 0
	for _, monthKey := range months {
		msgCount := stats.MonthlyMessages[monthKey]
		sessionCount := monthlySessions[monthKey]
		totalSessions += sessionCount
		quotaPct := float64(msgCount) / float64(copilotLimit) * 100
		monthName := formatMonthName(monthKey)
		sb.WriteString(fmt.Sprintf("| %s | %d | %d | %.0f%% |\n", monthName, msgCount, sessionCount, quotaPct))
	}
	sb.WriteString(fmt.Sprintf("| **Total** | **%d** | **%d** | — |\n", stats.TotalMessages, totalSessions))
	sb.WriteString(fmt.Sprintf("\n*Copilot Pro allows %d messages/month*\n\n", copilotLimit))
	sb.WriteString("---\n\n")

	// Monthly Token Usage
	sb.WriteString("## Monthly Token Usage\n\n")
	sb.WriteString("| Month | Total | Input | Output | Cached | Reasoning |\n")
	sb.WriteString("|-------|-------|-------|--------|--------|-----------|\n")

	tokenMonths := make([]string, 0, len(monthlyTokens))
	for m := range monthlyTokens {
		tokenMonths = append(tokenMonths, m)
	}
	sort.Strings(tokenMonths)

	for _, monthKey := range tokenMonths {
		t := monthlyTokens[monthKey]
		monthName := formatMonthName(monthKey)
		sb.WriteString(fmt.Sprintf("| %s | %s | %s | %s | %s | %s |\n",
			monthName,
			formatTokens(t.TotalTokens),
			formatTokens(t.InputTokens),
			formatTokens(t.OutputTokens),
			formatTokens(t.CachedInputTokens),
			formatTokens(t.ReasoningOutputTokens)))
	}
	sb.WriteString(fmt.Sprintf("| **Total** | **%s** | **%s** | **%s** | **%s** | **%s** |\n",
		formatTokens(totalTokens),
		formatTokens(totalInput),
		formatTokens(totalOutput),
		formatTokens(totalCached),
		formatTokens(totalReasoning)))
	sb.WriteString("\n---\n\n")

	// Top 10 Busiest Days
	sb.WriteString("## Top 10 Busiest Days\n\n")
	sb.WriteString("| Rank | Date | Messages | Day of Week |\n")
	sb.WriteString("|------|------|----------|-------------|\n")

	for i, day := range stats.TopDays {
		date := day[0].(string)
		count := day[1].(int)
		dow := getDayOfWeek(date)
		sb.WriteString(fmt.Sprintf("| %d | %s | %d | %s |\n", i+1, date, count, dow))
	}
	sb.WriteString("\n---\n\n")

	// Daily Breakdown
	sb.WriteString("## Daily Breakdown (All Active Days)\n\n")

	dailyByMonth := make(map[string][][2]interface{})
	for date, count := range stats.DailyCounts {
		monthKey := date[:7]
		dailyByMonth[monthKey] = append(dailyByMonth[monthKey], [2]interface{}{date, count})
	}

	for _, monthKey := range months {
		days := dailyByMonth[monthKey]
		sort.Slice(days, func(i, j int) bool {
			return days[i][0].(string) < days[j][0].(string)
		})

		monthTotal := 0
		for _, d := range days {
			monthTotal += d[1].(int)
		}
		monthName := formatMonthName(monthKey)

		sb.WriteString(fmt.Sprintf("### %s (%d messages, %d days)\n", monthName, monthTotal, len(days)))
		sb.WriteString("| Date | Messages |\n")
		sb.WriteString("|------|----------|\n")

		for _, d := range days {
			date := d[0].(string)
			count := d[1].(int)
			t, _ := time.Parse("2006-01-02", date)
			sb.WriteString(fmt.Sprintf("| %s | %d |\n", t.Format("Jan 02"), count))
		}
		sb.WriteString("\n")
	}

	// Analysis section
	sb.WriteString("---\n\n")
	sb.WriteString("## Usage Pattern Analysis\n\n")
	sb.WriteString("### Copilot Pro Comparison\n\n")
	sb.WriteString("| Metric | Your Usage | Copilot Pro Limit | Assessment |\n")
	sb.WriteString("|--------|------------|-------------------|------------|\n")

	peakMonthCount := stats.PeakMonth[1].(int)
	peakMonthName := formatMonthName(stats.PeakMonth[0].(string))
	avgMonth := float64(stats.TotalMessages) / float64(len(stats.MonthlyMessages))
	peakDayCount := stats.PeakDay[1].(int)

	peakAssess := "Pass"
	if peakMonthCount > copilotLimit {
		peakAssess = "Exceed"
	}
	avgAssess := "Pass"
	if avgMonth > copilotLimit {
		avgAssess = "Exceed"
	}
	peakDayAssess := "Pass"
	if peakDayCount*30 > copilotLimit {
		peakDayAssess = "Would exceed"
	}

	sb.WriteString(fmt.Sprintf("| Peak Month | %d (%s) | %d | %s (%.0f%% of limit) |\n",
		peakMonthCount, peakMonthName, copilotLimit, peakAssess, float64(peakMonthCount)/float64(copilotLimit)*100))
	sb.WriteString(fmt.Sprintf("| Average Month | ~%.0f | %d | %s (%.0f%% of limit) |\n",
		avgMonth, copilotLimit, avgAssess, avgMonth/float64(copilotLimit)*100))
	sb.WriteString(fmt.Sprintf("| Peak Day x 30 | %d (theoretical) | %d | %s if every day was peak |\n",
		peakDayCount*30, copilotLimit, peakDayAssess))

	sb.WriteString("\n---\n\n")
	sb.WriteString("*Data extracted from ~/.codex/history.jsonl and ~/.codex/sessions/*\n")

	return sb.String()
}

func main() {
	fmt.Println("Parsing history.jsonl...")
	dailyCounts := parseHistory()

	if len(dailyCounts) == 0 {
		fmt.Println("No history data found.")
		return
	}

	totalMessages := 0
	for _, c := range dailyCounts {
		totalMessages += c
	}
	fmt.Printf("Found %d messages across %d days\n", totalMessages, len(dailyCounts))

	fmt.Println("Counting session files...")
	monthlySessions := countSessions()
	totalSessions := 0
	for _, c := range monthlySessions {
		totalSessions += c
	}
	fmt.Printf("Found %d session files\n", totalSessions)

	fmt.Println("Collecting token statistics from sessions (this may take a moment)...")
	monthlyTokens := collectTokenStats()
	var totalTokens int64
	for _, t := range monthlyTokens {
		totalTokens += t.TotalTokens
	}
	fmt.Printf("Found %d total tokens across all sessions\n", totalTokens)

	fmt.Println("Calculating statistics...")
	stats := calculateStats(dailyCounts)

	fmt.Println("Generating markdown...")
	markdown := generateMarkdown(stats, monthlySessions, monthlyTokens)

	fmt.Printf("Writing to %s...\n", outputFile)
	if err := os.WriteFile(outputFile, []byte(markdown), 0644); err != nil {
		fmt.Printf("Error writing file: %v\n", err)
		return
	}

	fmt.Printf("Done! Statistics written to %s\n", outputFile)
}
