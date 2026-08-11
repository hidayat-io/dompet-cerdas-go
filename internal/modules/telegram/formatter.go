package telegram

import (
	"fmt"
	"math"
	"regexp"
	"strings"
	"time"

	"github.com/mthidayat/dompet-cerdas-go/internal/domain"
	"github.com/mthidayat/dompet-cerdas-go/internal/modules/transaction"
	"github.com/mthidayat/dompet-cerdas-go/internal/shared/money"
)

// mdEscaper escapes for Telegram's legacy Markdown (V1), the parse_mode every
// send call in this codebase uses. V1 only treats '_', '*', '`', '[' as
// escapable; anything else preceded by a backslash renders as a literal
// backslash instead of being consumed, so escaping a wider set (as
// MarkdownV2 requires) leaks backslashes into the message. A literal
// backslash in the input is also escaped, so it can't combine with a
// delimiter injected right after it (e.g. the closing '*' of a *bold* span).
var mdEscaper = regexp.MustCompile(`([_*\[` + "`" + `\\])`)

func EscapeMarkdown(v interface{}) string {
	if v == nil {
		return ""
	}
	var str string
	switch val := v.(type) {
	case string:
		str = val
	case int, int32, int64, float32, float64:
		str = fmt.Sprintf("%v", val)
	default:
		str = fmt.Sprintf("%v", val)
	}
	if str == "" {
		return ""
	}
	return mdEscaper.ReplaceAllString(str, `\$1`)
}

func WithAccountHeader(message string, accountName string) string {
	if accountName == "" {
		return message
	}
	return fmt.Sprintf("📁 *Akun: %s*\n\n%s", EscapeMarkdown(accountName), message)
}

// FormatDate renders a "YYYY-MM-DD" date as "15 Jan - Kamis".
//
// The weekday suffix is part of the legacy output (responseFormatter.ts:308)
// even though the doc comment there says only "27 Jan"; the parity fixture
// telegram_transaction_details.json is the authority.
//
// The legacy version parses with new Date(), which reads the string as UTC
// midnight and then reports local calendar fields. In Asia/Jakarta that never
// shifts the day, so reading the date parts directly is equivalent for the
// deployment timezone and avoids the shift a negative UTC offset would cause.
func FormatDate(dateStr string) string {
	if dateStr == "" {
		return ""
	}
	t, err := time.Parse("2006-01-02", dateStr)
	if err != nil {
		return EscapeMarkdown(dateStr)
	}
	months := []string{"Jan", "Feb", "Mar", "Apr", "Mei", "Jun", "Jul", "Agt", "Sep", "Okt", "Nov", "Des"}
	days := []string{"Minggu", "Senin", "Selasa", "Rabu", "Kamis", "Jumat", "Sabtu"}
	return fmt.Sprintf("%d %s - %s", t.Day(), months[t.Month()-1], days[int(t.Weekday())])
}

// FormatTimeRange renders a time range identifier as the Indonesian phrase used
// inside replies, mirroring formatTimeRange (queryService.ts:621). An
// unrecognized identifier is echoed back unchanged.
func FormatTimeRange(timeRange string) string {
	switch timeRange {
	case TimeRangeToday:
		return "hari ini"
	case TimeRangeYesterday:
		return "kemarin"
	case TimeRangeThisWeek:
		return "minggu ini"
	case TimeRangeLastWeek:
		return "7 hari terakhir"
	case TimeRangeThisMonth:
		return "bulan ini"
	case TimeRangeLastMonth:
		return "bulan lalu"
	case TimeRangeAllTime:
		return "semua waktu"
	default:
		return timeRange
	}
}

func FormatExpenseResponse(total int64, count int, timeRange string) string {
	var average int64 = 0
	if count > 0 {
		average = total / int64(count)
	}
	return fmt.Sprintf("💰 *Pengeluaran %s*: %s\n\n📊 Detail:\n• %d transaksi\n• Rata-rata: %s/transaksi",
		EscapeMarkdown(timeRange),
		EscapeMarkdown(money.FormatExact(total)),
		count,
		EscapeMarkdown(money.FormatCompact(average)))
}

func FormatIncomeResponse(total int64, count int, timeRange string) string {
	return fmt.Sprintf("💵 *Pemasukan %s*: %s\n\n📊 Detail:\n• %d transaksi",
		EscapeMarkdown(timeRange),
		EscapeMarkdown(money.FormatExact(total)),
		count)
}

func FormatBalanceResponse(balance int64, timeRangeText string) string {
	emoji := "ℹ️"
	status := "Saldo nol"
	if balance > 0 {
		emoji = "💰"
		status = "Saldo positif"
	} else if balance < 0 {
		emoji = "⚠️"
		status = "Saldo negatif"
	}

	escapedTimeRange := ""
	if timeRangeText != "" {
		escapedTimeRange = EscapeMarkdown(timeRangeText)
	}

	return fmt.Sprintf("%s *Saldo kamu%s*: %s\n\n%s",
		emoji,
		escapedTimeRange,
		EscapeMarkdown(money.FormatExact(balance)),
		EscapeMarkdown(status))
}

func FormatUnknownIntent() string {
	return `🤔 *Maaf, saya belum mengerti.*

Coba:
• "berapa pengeluaran minggu ini?"
• "tambah 50000 makan siang"
• "kategori paling boros bulan ini"

Atau ketik /help untuk panduan lengkap.`
}

// FormatClarification wraps a follow-up question from the parser.
func FormatClarification(clarification string) string {
	return "❓ " + clarification
}

// FormatFinancialAdvice, FormatSavingsStrategy, and FormatExpenseAnalysis wrap
// LLM prose in the advisor chrome. The prose is inserted raw, exactly as the
// legacy formatters do — the model is prompted to emit Telegram Markdown, so
// escaping it here would show the escape characters to the user.
func FormatFinancialAdvice(advice string) string {
	return "🤖 *AI Financial Advisor - DompetCerdas*\n\n" + advice +
		"\n\n---\n💬 *Mau tanya lebih lanjut?*\nContoh: \"tips hemat kategori Food\", \"kategori mana yang bisa dikurangi?\""
}

func FormatSavingsStrategy(strategy string) string {
	return "💰 *Strategi Hemat - DompetCerdas*\n\n" + strategy +
		"\n\n---\n💬 *Perlu analisa lebih detail?*\nKetik: \"analisa pengeluaranku\" atau \"gimana keuanganku?\""
}

func FormatExpenseAnalysis(analysis string) string {
	return "🔍 *Analisa Pengeluaran - DompetCerdas*\n\n" + analysis +
		"\n\n---\n💬 *Butuh strategi hemat?*\nKetik: \"tips hemat bulan depan\" atau \"saran biar hemat\""
}

// breakdownTopN is how many category rows a breakdown reply lists. The total
// line still covers every category, not just the listed ones.
const breakdownTopN = 5

// FormatCategoryBreakdown renders the per-category expense summary, mirroring
// formatCategoryBreakdown (responseFormatter.ts:359).
//
// timeRange is interpolated unescaped, matching the legacy template. Callers
// pass phrases produced by FormatTimeRange, never user input.
func FormatCategoryBreakdown(categories []transaction.CategoryBreakdown, timeRange string) string {
	if len(categories) == 0 {
		return "📊 Belum ada pengeluaran " + timeRange + "."
	}

	var total int64
	for _, cat := range categories {
		total += cat.Amount
	}

	shown := categories
	if len(shown) > breakdownTopN {
		shown = shown[:breakdownTopN]
	}

	lines := make([]string, 0, len(shown))
	for _, cat := range shown {
		lines = append(lines, fmt.Sprintf("%s %s: %s (%s%%)",
			EmojiFor(cat.Icon, cat.Category),
			EscapeMarkdown(cat.Category),
			money.FormatCompact(cat.Amount),
			formatPercent(cat.Percentage)))
	}

	return fmt.Sprintf("📊 *Pengeluaran per kategori (%s)*:\n\n%s\n\n💰 Total: %s",
		timeRange,
		strings.Join(lines, "\n"),
		money.FormatExact(total))
}

// formatPercent reproduces JavaScript's Number.prototype.toFixed(0), which
// rounds half away from zero. Go's %.0f rounds half to even, so 2.5 would print
// as "2" instead of "3"; math.Round restores the legacy behavior.
func formatPercent(percentage float64) string {
	return fmt.Sprintf("%.0f", math.Round(percentage))
}

// singleDayRanges are the time-range phrases that suppress date grouping in a
// details reply, matching the legacy check against the rendered phrase.
var singleDayRanges = map[string]bool{"hari ini": true, "kemarin": true}

// FormatTransactionDetails renders a transaction listing, mirroring
// formatTransactionDetails (responseFormatter.ts:380).
//
// The summary counts every row of a type, including rows whose category no
// longer exists — those are typed as expenses by the query layer. This is why
// the summary here can disagree with a "berapa pengeluaran" reply, which uses
// SumByType and drops them. The behavior is inherited, not introduced.
//
// Multi-day output groups by date and renders descriptions plain; single-day
// output numbers the rows and bolds descriptions. Both branches come from the
// legacy formatter.
func FormatTransactionDetails(details []transaction.Detail, timeRange, notice string) string {
	if len(details) == 0 {
		return "📋 Belum ada transaksi " + timeRange + "."
	}

	var totalIncome, totalExpense int64
	var incomeCount, expenseCount int
	for _, d := range details {
		if d.Type == domain.TransactionTypeIncome {
			totalIncome += d.Amount
			incomeCount++
			continue
		}
		totalExpense += d.Amount
		expenseCount++
	}

	var b strings.Builder
	fmt.Fprintf(&b, "📋 *Detail transaksi %s*\n\n", timeRange)
	b.WriteString("💰 Ringkasan:\n")
	if incomeCount > 0 {
		fmt.Fprintf(&b, "  ➕ Pemasukan: %s (%dx)\n", money.FormatExact(totalIncome), incomeCount)
	}
	if expenseCount > 0 {
		fmt.Fprintf(&b, "  ➖ Pengeluaran: %s (%dx)\n", money.FormatExact(totalExpense), expenseCount)
	}
	if incomeCount > 0 && expenseCount > 0 {
		fmt.Fprintf(&b, "  💎 Saldo: %s\n", money.FormatExact(totalIncome-totalExpense))
	}
	b.WriteString("\n")

	if notice != "" {
		b.WriteString(notice + "\n\n")
	}

	if singleDayRanges[strings.ToLower(timeRange)] {
		for i, d := range details {
			fmt.Fprintf(&b, "\n%d. %s *%s*\n   💵 %s • %s %s",
				i+1,
				typeIndicator(d.Type),
				EscapeMarkdown(d.Description),
				money.FormatExact(d.Amount),
				EmojiFor(d.Icon, d.Category),
				EscapeMarkdown(d.Category))
		}
		return b.String()
	}

	order := make([]string, 0, len(details))
	grouped := make(map[string][]transaction.Detail, len(details))
	for _, d := range details {
		key := FormatDate(d.Date)
		if _, seen := grouped[key]; !seen {
			order = append(order, key)
		}
		grouped[key] = append(grouped[key], d)
	}

	sections := make([]string, 0, len(order))
	for _, key := range order {
		items := make([]string, 0, len(grouped[key]))
		for _, d := range grouped[key] {
			items = append(items, fmt.Sprintf("%s %s\n  💵 %s • %s %s",
				typeIndicator(d.Type),
				EscapeMarkdown(d.Description),
				money.FormatExact(d.Amount),
				EmojiFor(d.Icon, d.Category),
				EscapeMarkdown(d.Category)))
		}
		sections = append(sections, fmt.Sprintf("\n📅 *%s*\n%s", EscapeMarkdown(key), strings.Join(items, "\n")))
	}

	b.WriteString(strings.Join(sections, "\n"))
	return b.String()
}

func typeIndicator(txType domain.TransactionType) string {
	if txType == domain.TransactionTypeIncome {
		return "➕"
	}
	return "➖"
}

// DraftItem is one line of a transaction draft, as shown in the confirmation
// preview and in the saved confirmation.
type DraftItem struct {
	Amount       int64  `json:"amount"`
	Description  string `json:"description"`
	CategoryName string `json:"categoryName"`
}

// FormatTransactionDraftPreview renders the "check before saving" message that
// accompanies the inline keyboard, mirroring formatTransactionDraftPreview
// (responseFormatter.ts:470).
//
// The usedAI notice is user-visible on purpose: it tells the reader whether the
// numbers came from a deterministic parse or from the model, which is the cue
// for how carefully to check them.
func FormatTransactionDraftPreview(items []DraftItem, usedAI bool) string {
	saveLabel := "Simpan"
	if len(items) > 1 {
		saveLabel = "Simpan Semua"
	}

	header := fmt.Sprintf("🧾 *Cek Dulu Sebelum Disimpan*\n\nSaya menemukan *%d transaksi* dari pesan kamu.\n", len(items))

	parserNotice := "\n⚡ Diparse cepat tanpa AI karena formatnya sederhana.\n"
	if usedAI {
		parserNotice = "\n🤖 Dibantu AI karena format pesannya cukup bebas.\n"
	}

	lines := make([]string, 0, len(items))
	for i, item := range items {
		lines = append(lines, fmt.Sprintf("%d. *%s*\n   💰 %s\n   📁 %s",
			i+1,
			EscapeMarkdown(item.Description),
			money.FormatExact(item.Amount),
			EscapeMarkdown(item.CategoryName)))
	}

	editHint := ""
	if len(items) > 1 {
		editHint = "\nKalau ada item yang salah, klik tombol *Hapus 1 / Hapus 2 / ...* dulu.\n"
	}

	return fmt.Sprintf("%s%s\n%s\n%s\nKlik *%s* kalau sudah benar.",
		header, parserNotice, strings.Join(lines, "\n\n"), editHint, saveLabel)
}

// batchAddedListLimit is how many saved rows the confirmation lists before
// collapsing the rest into a count.
const batchAddedListLimit = 5

// FormatTransactionBatchAdded confirms a saved batch, mirroring
// formatTransactionBatchAdded (responseFormatter.ts:493).
func FormatTransactionBatchAdded(items []DraftItem) string {
	var total int64
	for _, item := range items {
		total += item.Amount
	}

	shown := items
	if len(shown) > batchAddedListLimit {
		shown = shown[:batchAddedListLimit]
	}

	lines := make([]string, 0, len(shown))
	for i, item := range shown {
		lines = append(lines, fmt.Sprintf("%d. %s • %s • %s",
			i+1,
			EscapeMarkdown(item.Description),
			money.FormatExact(item.Amount),
			EscapeMarkdown(item.CategoryName)))
	}

	moreNotice := ""
	if len(items) > batchAddedListLimit {
		moreNotice = fmt.Sprintf("\n... dan %d transaksi lainnya", len(items)-batchAddedListLimit)
	}

	return fmt.Sprintf("✅ *Transaksi berhasil ditambahkan!*\n\n📦 Total transaksi: *%d*\n💰 Total nominal: *%s*\n\n%s%s",
		len(items), money.FormatExact(total), strings.Join(lines, "\n"), moreNotice)
}

// FormatAutoSavedTransaction confirms a transaction saved without asking,
// mirroring formatAutoSavedTransaction (responseFormatter.ts:511). The closing
// line matters: the user never got a chance to reject this one, so it has to say
// how to undo it.
func FormatAutoSavedTransaction(amount int64, description, categoryName string) string {
	return fmt.Sprintf("⚡ *Tersimpan otomatis!*\n\n💰 %s — %s\n🏷️ Kategori: %s\n\n_Ada yang salah? Hapus manual di aplikasi ya._",
		money.FormatExact(amount),
		EscapeMarkdown(description),
		EscapeMarkdown(categoryName))
}

// FormatVoiceTranscriptNote prefixes a draft preview with what the bot heard, so
// a mistranscription is visible before the user confirms (bot/index.ts:212).
func FormatVoiceTranscriptNote(rawMessage string) string {
	trimmed := strings.TrimSpace(rawMessage)
	if trimmed == "" {
		return ""
	}
	return "🎤 *Hasil suara:* _" + EscapeMarkdown(trimmed) + "_\n\n"
}

// FormatCategoryList renders the account's categories, mirroring
// formatCategoryList (responseFormatter.ts:563). A category with no type is
// counted as an expense, as in the legacy filter.
func FormatCategoryList(categories []domain.Category) string {
	if len(categories) == 0 {
		return "📋 Belum ada kategori yang dibuat.\n\n💡 Buat kategori baru di aplikasi web DompetCerdas."
	}

	var expense, income []domain.Category
	for _, cat := range categories {
		if cat.Type == domain.TransactionTypeIncome {
			income = append(income, cat)
			continue
		}
		expense = append(expense, cat)
	}

	var b strings.Builder
	b.WriteString("📋 *Daftar Kategori Tersedia*\n\n")

	if len(expense) > 0 {
		fmt.Fprintf(&b, "💸 *Pengeluaran* (%d kategori)\n", len(expense))
		for i, cat := range expense {
			fmt.Fprintf(&b, "   %d. %s\n", i+1, EscapeMarkdown(cat.Name))
		}
		b.WriteString("\n")
	}

	if len(income) > 0 {
		fmt.Fprintf(&b, "💰 *Pemasukan* (%d kategori)\n", len(income))
		for i, cat := range income {
			fmt.Fprintf(&b, "   %d. %s\n", i+1, EscapeMarkdown(cat.Name))
		}
		b.WriteString("\n")
	}

	b.WriteString("---\n💡 *Tips:*\n")
	b.WriteString("• Ketik \"breakdown bulan ini\" untuk lihat pengeluaran per kategori\n")
	b.WriteString("• Ketik \"detail kategori Food\" untuk lihat transaksi kategori tertentu")

	return b.String()
}
