package telegram

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"

	"github.com/mthidayat/dompet-cerdas-go/internal/domain"
	"github.com/mthidayat/dompet-cerdas-go/internal/modules/account"
	"github.com/mthidayat/dompet-cerdas-go/internal/modules/advisor"
	"github.com/mthidayat/dompet-cerdas-go/internal/modules/transaction"
	"github.com/mthidayat/dompet-cerdas-go/internal/shared/datetime"
)

// ProcessBotUpdate is the main entry point for routing an inbound Telegram update.
func (h *Handler) ProcessBotUpdate(ctx context.Context, update map[string]interface{}) error {
	// 1. Check callback query
	if callbackQuery, ok := update["callback_query"].(map[string]interface{}); ok {
		return h.handleCallbackQuery(ctx, callbackQuery)
	}

	// 2. Check message
	if msg, ok := update["message"].(map[string]interface{}); ok {
		return h.handleMessage(ctx, msg)
	}

	return nil
}

func (h *Handler) handleCallbackQuery(ctx context.Context, query map[string]interface{}) error {
	id, _ := query["id"].(string)
	data, _ := query["data"].(string)

	if id != "" {
		_ = h.bot.AnswerCallbackQuery(ctx, id, "")
	}

	if data == "" {
		return nil
	}

	telegramID := callbackTelegramID(query)
	if telegramID == 0 {
		slog.Warn("telegram callback: cannot resolve chat id", "data", data)
		return nil
	}

	messageID := callbackMessageID(query)

	switch {
	case strings.HasPrefix(data, callbackDraftSave):
		return h.handleDraftSave(ctx, telegramID, messageID, strings.TrimPrefix(data, callbackDraftSave))
	case strings.HasPrefix(data, callbackDraftCancel):
		return h.handleDraftCancel(ctx, telegramID, messageID, strings.TrimPrefix(data, callbackDraftCancel))
	case strings.HasPrefix(data, callbackDraftRemove):
		return h.handleDraftRemove(ctx, telegramID, messageID, strings.TrimPrefix(data, callbackDraftRemove))
	case strings.HasPrefix(data, callbackSwitchAccount):
		return h.handleSwitchAccount(ctx, telegramID, messageID, strings.TrimPrefix(data, callbackSwitchAccount))
	case data == callbackConfirmUnlink:
		return h.handleConfirmUnlink(ctx, telegramID, messageID)
	case data == callbackCancelUnlink:
		return h.handleCancelUnlink(ctx, telegramID, messageID)
	}

	// An unrecognised payload can only come from a keyboard this build does not
	// know about. Say so rather than leaving the press unanswered.
	slog.Info("telegram callback with unknown payload", "data", data)
	return h.bot.SendMessage(ctx, telegramID, notPortedMessage("Tombol ini"), "Markdown")
}

// callbackMessageID locates the message the button is attached to, so replies
// can edit it in place instead of appending a new message under the draft.
func callbackMessageID(query map[string]interface{}) int {
	msg, ok := query["message"].(map[string]interface{})
	if !ok {
		return 0
	}
	switch v := msg["message_id"].(type) {
	case float64:
		return int(v)
	case int:
		return v
	case int64:
		return int(v)
	}
	return 0
}

// callbackTelegramID pulls the chat id out of a callback query, preferring the
// originating message's chat over the pressing user so that the reply lands in
// the same conversation.
func callbackTelegramID(query map[string]interface{}) int64 {
	if msg, ok := query["message"].(map[string]interface{}); ok {
		if chat, ok := msg["chat"].(map[string]interface{}); ok {
			if id := extractTelegramID(chat); id != 0 {
				return id
			}
		}
	}
	if from, ok := query["from"].(map[string]interface{}); ok {
		return extractTelegramID(from)
	}
	return 0
}

func (h *Handler) handleMessage(ctx context.Context, msg map[string]interface{}) error {
	from, ok := msg["from"].(map[string]interface{})
	if !ok {
		return nil
	}
	telegramID := extractTelegramID(from)
	if telegramID == 0 {
		return nil
	}

	// 1. Update last interaction
	_ = h.linkService.UpdateLastInteraction(ctx, telegramID)

	// 2. Check commands or text
	if text, ok := msg["text"].(string); ok && text != "" {
		if text == "/start" || text == "/link" || text == "/hubungkan" {
			return h.handleStartCommand(ctx, telegramID, from)
		}
		if text == "/help" || text == "/bantuan" {
			return h.handleHelpCommand(ctx, telegramID)
		}
		if text == "/akun" || text == "/account" {
			return h.handleAccountCommand(ctx, telegramID)
		}
		if text == "/unlink" || text == "/disconnect" {
			return h.handleUnlinkCommand(ctx, telegramID)
		}
		return h.handleTextMessage(ctx, telegramID, text)
	}

	// A photographed receipt, or an image sent uncompressed as a document.
	if fileID := largestPhotoFileID(msg); fileID != "" {
		caption, _ := msg["caption"].(string)
		return h.handleReceiptPhoto(ctx, telegramID, fileID, caption)
	}
	if fileID := documentFileID(msg); fileID != "" {
		caption, _ := msg["caption"].(string)
		return h.handleReceiptPhoto(ctx, telegramID, fileID, caption)
	}

	if voice, ok := extractVoice(msg); ok {
		return h.handleVoiceMessage(ctx, telegramID, voice)
	}

	// Video and anything else the bot has never handled. Reply rather than
	// dropping the message on the floor.
	if hasNonTextPayload(msg) {
		return h.bot.SendMessage(ctx, telegramID, notPortedMessage("Kiriman ini"), "Markdown")
	}

	return nil
}

func hasNonTextPayload(msg map[string]interface{}) bool {
	for _, key := range []string{"photo", "voice", "audio", "document", "video", "video_note"} {
		if _, ok := msg[key]; ok {
			return true
		}
	}
	return false
}

// notPortedMessage is the reply for a capability that exists in the legacy bot
// but not yet in this service. It names what was skipped so the user is not left
// guessing whether the bot is down.
func notPortedMessage(subject string) string {
	return fmt.Sprintf("🚧 %s belum tersedia di versi baru bot ini.\n\nSaat ini yang sudah bisa: catat transaksi lewat chat, cek pengeluaran, pemasukan, saldo, detail transaksi, breakdown kategori, dan daftar kategori.\n\nKetik /help untuk panduan.", subject)
}

func (h *Handler) handleStartCommand(ctx context.Context, telegramID int64, from map[string]interface{}) error {
	username, _ := from["username"].(string)
	firstName, _ := from["first_name"].(string)
	lastName, _ := from["last_name"].(string)

	token, err := h.linkService.GenerateLinkToken(ctx, telegramID, username, firstName, lastName)
	if err != nil {
		return err
	}

	msg := "Selamat datang di *Dompet Cerdas*! 🤖\n\n" +
		"Untuk menghubungkan akun Telegram Anda dengan Web App, silakan klik tautan di bawah ini (berlaku 5 menit):\n\n" +
		"🔗 " + EscapeMarkdown(token)
	return h.bot.SendMessage(ctx, telegramID, msg, "Markdown")
}

func (h *Handler) handleHelpCommand(ctx context.Context, telegramID int64) error {
	msg := "💡 *Panduan Penggunaan Dompet Cerdas*\n\n" +
		"• *Catat Transaksi*: `makan 25rb`, `kopi 18rb, parkir 5rb`\n" +
		"• *Cek Pengeluaran*: `berapa pengeluaran hari ini?`\n" +
		"• *Cek Saldo*: `saldo`\n" +
		"• *Kelola Akun*: /akun\n\n" +
		"Ketik pesan secara alami, AI kami siap membantu!"
	return h.bot.SendMessage(ctx, telegramID, msg, "Markdown")
}

// replyContext carries everything a reply needs: who to answer, whose data to
// read, and the account label stamped on top of every message.
type replyContext struct {
	telegramID  int64
	userID      string
	accountName string
	ac          *account.Context
}

// send applies the account header and delivers the reply.
func (h *Handler) send(ctx context.Context, rc replyContext, body string) error {
	return h.bot.SendMessage(ctx, rc.telegramID, WithAccountHeader(body, rc.accountName), "Markdown")
}

// handleTextMessage classifies a free-text message and answers it.
//
// Every path sends exactly one reply, including the failure paths. An earlier
// revision of this function detected the intent and then returned nil without
// sending anything, which read to users as the bot being offline.
//
// Every branch either answers with data or says plainly why it cannot, so no
// input can reach this function and produce nothing.
func (h *Handler) handleTextMessage(ctx context.Context, telegramID int64, text string) error {
	userID, link, err := h.linkService.GetTelegramLinkContext(ctx, telegramID)
	if err != nil {
		slog.Error("telegram: failed to resolve link context", "telegramId", telegramID, "error", err)
		return h.bot.SendMessage(ctx, telegramID, "❌ Terjadi kesalahan. Silakan coba lagi.", "Markdown")
	}
	if userID == "" {
		return h.bot.SendMessage(ctx, telegramID,
			"🔗 Akun Telegram ini belum terhubung.\n\nKetik /start untuk menghubungkannya dengan akun DompetCerdas kamu.", "Markdown")
	}

	preferredAccountID, _ := link["defaultAccountId"].(string)
	accountName, _ := link["defaultAccountName"].(string)

	ac, err := h.accountService.GetAccountContext(ctx, userID, preferredAccountID)
	if err != nil {
		slog.Error("telegram: failed to resolve account context", "userId", userID, "error", err)
		return h.bot.SendMessage(ctx, telegramID, "❌ Terjadi kesalahan. Silakan coba lagi.", "Markdown")
	}

	rc := replyContext{telegramID: telegramID, userID: userID, accountName: accountName, ac: ac}

	// A message that parses as a transaction must not fall through to intent
	// classification, or "makan 25rb" would be answered as an unknown query.
	if transaction.ShouldAttemptTransactionParsing(text) {
		if parsed := transaction.ParseLocally(text); parsed != nil && len(parsed.Items) > 0 {
			return h.sendDraft(ctx, rc, parsed, text, domain.SessionSourceText)
		}
	}

	intent := DetectSimpleIntent(text)
	if intent == nil {
		return h.send(ctx, rc, FormatUnknownIntent())
	}

	// ShouldPreferAIIntentParsing marks messages the legacy bot re-parsed with
	// the LLM. That path is unported, so the locally-detected intent is used as
	// is. This is safe in the read direction: the risk it guarded against was a
	// typo'd query being saved as a transaction, and saving is not reachable
	// here. Logged so the gap stays visible.
	if ShouldPreferAIIntentParsing(text, intent) {
		slog.Info("telegram: LLM re-parse unavailable, using local intent",
			"intent", intent.Intent, "message", text)
	}

	switch intent.Intent {
	case IntentQueryExpenses:
		return h.replyExpenses(ctx, rc, intent.Parameters)
	case IntentQueryIncome:
		return h.replyIncome(ctx, rc, intent.Parameters)
	case IntentQueryBalance:
		return h.replyBalance(ctx, rc, intent.Parameters)
	case IntentQueryDetails:
		return h.replyDetails(ctx, rc, intent.Parameters)
	case IntentCategoryBreakdown:
		return h.replyBreakdown(ctx, rc, intent.Parameters)
	case IntentListCategories:
		return h.replyCategoryList(ctx, rc)
	case IntentAddTransaction:
		return h.replyAddTransaction(ctx, rc, intent.Parameters, text)
	case IntentFinancialAdvice:
		return h.replyAdvice(ctx, rc, advisor.ModeHealth)
	case IntentSavingsStrategy:
		return h.replyAdvice(ctx, rc, advisor.ModeSavings)
	case IntentExpenseAnalysis:
		return h.replyAdvice(ctx, rc, advisor.ModeSpending)
	default:
		return h.send(ctx, rc, FormatUnknownIntent())
	}
}

// replyAddTransaction handles an explicit "tambah 50000 makan siang", where the
// intent classifier extracted the amount and description rather than the
// transaction parser (bot/index.ts:1396).
//
// It routes through the same draft path as a parsed message, so an explicitly
// phrased transaction and a shorthand one behave identically from here on.
func (h *Handler) replyAddTransaction(ctx context.Context, rc replyContext, p IntentParameters, rawMessage string) error {
	if p.Amount <= 0 || strings.TrimSpace(p.Description) == "" {
		return h.send(ctx, rc, "❌ Jumlah atau deskripsi tidak ditemukan.\n\nContoh: \"tambah 50000 makan siang\"")
	}

	parsed := &domain.HybridTransactionParseResult{
		Items: []domain.ParsedTransactionDraft{{
			Amount:       p.Amount,
			Description:  p.Description,
			CategoryHint: p.CategoryHint,
			SourceText:   rawMessage,
		}},
		// The legacy handler builds this draft with usedAI=false, which keeps it
		// eligible for auto-save when the hint resolves deterministically.
		UsedAI: false,
	}

	return h.sendDraft(ctx, rc, parsed, rawMessage, domain.SessionSourceText)
}

// allTimeStart is the lower bound the legacy backend uses for "all_time"
// (queryService.ts:105). It predates the app, so it behaves as "everything".
const allTimeStart = "2020-01-01"

// dateRangeFor resolves intent parameters into an inclusive date range.
//
// Precedence follows the legacy handlers: an explicit date wins over a
// custom month, which wins over a relative day offset, which wins over a named
// range. Callers pass the fallback the corresponding legacy branch used, since
// it differs per intent ("today" for details, "this_month" for the rest).
func dateRangeFor(p IntentParameters, fallbackRange string) (start, end string, err error) {
	now := datetime.Now()

	switch {
	case p.SpecificDate != "":
		return p.SpecificDate, p.SpecificDate, nil
	case p.CustomMonth != "":
		return datetime.ResolveRange("custom_month:"+p.CustomMonth, now)
	case p.DaysAgo > 0:
		return datetime.ResolveRange(fmt.Sprintf("days_ago:%d", p.DaysAgo), now)
	}

	spec := p.TimeRange
	if spec == "" {
		spec = fallbackRange
	}
	if spec == TimeRangeAllTime {
		return allTimeStart, datetime.TodayString(), nil
	}
	return datetime.ResolveRange(spec, now)
}

// timeRangeLabel renders the human phrase describing the range that was
// queried, mirroring the label each legacy branch builds.
func timeRangeLabel(p IntentParameters, fallbackRange string) string {
	switch {
	case p.CustomMonth != "":
		return customMonthLabel(p.CustomMonth)
	case p.DaysAgo > 0:
		return fmt.Sprintf("%d hari lalu", p.DaysAgo)
	}
	spec := p.TimeRange
	if spec == "" {
		spec = fallbackRange
	}
	return FormatTimeRange(spec)
}

var indonesianMonths = []string{
	"", "Januari", "Februari", "Maret", "April", "Mei", "Juni",
	"Juli", "Agustus", "September", "Oktober", "November", "Desember",
}

// customMonthLabel renders "2026-02" as "Februari 2026". A malformed value is
// echoed back rather than dropped, so a parser bug stays visible to the user
// instead of silently reading as the current month.
func customMonthLabel(customMonth string) string {
	parts := strings.SplitN(customMonth, "-", 2)
	if len(parts) != 2 {
		return customMonth
	}
	var month int
	if _, err := fmt.Sscanf(parts[1], "%d", &month); err != nil || month < 1 || month > 12 {
		return customMonth
	}
	return indonesianMonths[month] + " " + parts[0]
}

// queryRange reads every transaction in the resolved range, unfiltered. The
// aggregates need both types and all categories, so filtering is left to the
// caller.
func (h *Handler) queryRange(ctx context.Context, rc replyContext, start, end string) ([]transaction.Detail, error) {
	return transaction.QueryTransactions(ctx, h.accountService, h.accountRepo, rc.ac, transaction.QueryParams{
		StartDate: start,
		EndDate:   end,
	})
}

// failQuery logs the cause and tells the user something went wrong, so a
// Firestore error never turns into silence.
func (h *Handler) failQuery(ctx context.Context, rc replyContext, op string, err error) error {
	slog.Error("telegram: query failed", "op", op, "userId", rc.userID, "error", err)
	return h.send(ctx, rc, "❌ Terjadi kesalahan. Silakan coba lagi.")
}

func (h *Handler) replyExpenses(ctx context.Context, rc replyContext, p IntentParameters) error {
	start, end, err := dateRangeFor(p, TimeRangeThisMonth)
	if err != nil {
		return h.failQuery(ctx, rc, "expenses.range", err)
	}

	details, err := h.queryRange(ctx, rc, start, end)
	if err != nil {
		return h.failQuery(ctx, rc, "expenses.query", err)
	}

	total, count := transaction.SumByType(details, domain.TransactionTypeExpense)
	return h.send(ctx, rc, FormatExpenseResponse(total, count, timeRangeLabel(p, TimeRangeThisMonth)))
}

func (h *Handler) replyIncome(ctx context.Context, rc replyContext, p IntentParameters) error {
	// The legacy income handler ignores days_ago and custom_month and uses the
	// named range only (bot/index.ts:1276-1278).
	rangeOnly := IntentParameters{TimeRange: p.TimeRange}

	start, end, err := dateRangeFor(rangeOnly, TimeRangeThisMonth)
	if err != nil {
		return h.failQuery(ctx, rc, "income.range", err)
	}

	details, err := h.queryRange(ctx, rc, start, end)
	if err != nil {
		return h.failQuery(ctx, rc, "income.query", err)
	}

	total, count := transaction.SumByType(details, domain.TransactionTypeIncome)
	return h.send(ctx, rc, FormatIncomeResponse(total, count, timeRangeLabel(rangeOnly, TimeRangeThisMonth)))
}

func (h *Handler) replyBalance(ctx context.Context, rc replyContext, p IntentParameters) error {
	// Balance is the one query with no default range: with no time parameters
	// at all the legacy handler omits the date filter entirely and sums the
	// whole history (queryService.ts:258-273).
	start, end := allTimeStart, datetime.TodayString()
	label := ""

	if p.CustomMonth != "" || p.DaysAgo > 0 || p.TimeRange != "" {
		var err error
		start, end, err = dateRangeFor(p, TimeRangeThisMonth)
		if err != nil {
			return h.failQuery(ctx, rc, "balance.range", err)
		}
		label = " (" + timeRangeLabel(p, TimeRangeThisMonth) + ")"
	}

	details, err := h.queryRange(ctx, rc, start, end)
	if err != nil {
		return h.failQuery(ctx, rc, "balance.query", err)
	}

	return h.send(ctx, rc, FormatBalanceResponse(transaction.NetBalance(details), label))
}

func (h *Handler) replyBreakdown(ctx context.Context, rc replyContext, p IntentParameters) error {
	// The legacy breakdown handler accepts days_ago and the named range only.
	noDate := IntentParameters{TimeRange: p.TimeRange, DaysAgo: p.DaysAgo}

	start, end, err := dateRangeFor(noDate, TimeRangeThisMonth)
	if err != nil {
		return h.failQuery(ctx, rc, "breakdown.range", err)
	}

	details, err := h.queryRange(ctx, rc, start, end)
	if err != nil {
		return h.failQuery(ctx, rc, "breakdown.query", err)
	}

	return h.send(ctx, rc, FormatCategoryBreakdown(
		transaction.BuildCategoryBreakdown(details),
		timeRangeLabel(noDate, TimeRangeThisMonth)))
}

func (h *Handler) replyCategoryList(ctx context.Context, rc replyContext) error {
	categories, err := h.accountRepo.GetUserCategories(ctx, rc.ac, false)
	if err != nil {
		slog.Error("telegram: failed to list categories", "userId", rc.userID, "error", err)
		return h.send(ctx, rc, "❌ Gagal mengambil daftar kategori.")
	}
	return h.send(ctx, rc, FormatCategoryList(categories))
}

func (h *Handler) replyDetails(ctx context.Context, rc replyContext, p IntentParameters) error {
	limit, clamped := transaction.ClampTelegramLimit(p.Limit)

	notice := ""
	if clamped {
		notice = "⚠️ Maksimal 30 transaksi. Menampilkan 30 transaksi terakhir."
	}

	// A limit with no time constraint searches all history, so that
	// "10 transaksi terakhir" is not silently confined to the current month
	// (bot/index.ts:1342-1344).
	effective := p
	if limit > 0 && p.TimeRange == "" && p.SpecificDate == "" && p.DaysAgo == 0 {
		effective.TimeRange = TimeRangeAllTime
	}

	start, end, err := dateRangeFor(effective, TimeRangeThisMonth)
	if err != nil {
		return h.failQuery(ctx, rc, "details.range", err)
	}

	categoryName, err := h.resolveCategoryFilter(ctx, rc, p.CategoryFilter)
	if err != nil {
		return h.failQuery(ctx, rc, "details.category", err)
	}

	details, err := transaction.QueryTransactions(ctx, h.accountService, h.accountRepo, rc.ac, transaction.QueryParams{
		StartDate:    start,
		EndDate:      end,
		CategoryName: categoryName,
		TypeFilter:   domain.TransactionType(p.TypeFilter),
		SortBy:       p.SortBy,
		Limit:        limit,
	})
	if err != nil {
		return h.failQuery(ctx, rc, "details.query", err)
	}

	label := detailsLabel(p, effective, limit)
	if p.CategoryFilter != "" {
		label += " kategori " + p.CategoryFilter
	}

	return h.send(ctx, rc, FormatTransactionDetails(details, label, notice))
}

// detailsLabel builds the phrase describing what was listed. A limit takes
// precedence over the time range, because "10 transaksi terakhir" describes the
// result better than "semua waktu" does.
func detailsLabel(p IntentParameters, effective IntentParameters, limit int) string {
	typeLabel := "transaksi"
	switch domain.TransactionType(p.TypeFilter) {
	case domain.TransactionTypeIncome:
		typeLabel = "pemasukan"
	case domain.TransactionTypeExpense:
		typeLabel = "pengeluaran"
	}

	switch {
	case limit > 0 && p.SortBy == transaction.SortByAmount:
		return fmt.Sprintf("%d %s tertinggi", limit, typeLabel)
	case limit > 0:
		return fmt.Sprintf("%d %s terakhir", limit, typeLabel)
	case p.SpecificDate != "":
		return "tanggal " + FormatDate(p.SpecificDate)
	case p.DaysAgo > 0:
		return fmt.Sprintf("%d hari lalu", p.DaysAgo)
	}
	return timeRangeLabel(effective, TimeRangeThisMonth)
}

// resolveCategoryFilter maps a user-typed category word onto a real category
// name: exact match first, then substring either way, mirroring the first two
// legs of matchCategoryFilter (queryService.ts:408-425).
//
// The legacy third leg asks Gemini for a semantic match ("food" → "FnB"). That
// leg is not ported, so an unrecognised word is passed through unchanged and
// simply matches nothing — the user gets "Belum ada transaksi ...", not a wrong
// category's transactions.
func (h *Handler) resolveCategoryFilter(ctx context.Context, rc replyContext, filter string) (string, error) {
	if filter == "" {
		return "", nil
	}

	categories, err := h.accountRepo.GetUserCategories(ctx, rc.ac, false)
	if err != nil {
		return "", err
	}

	lowered := strings.ToLower(filter)

	for _, cat := range categories {
		if strings.ToLower(cat.Name) == lowered {
			return cat.Name, nil
		}
	}
	for _, cat := range categories {
		name := strings.ToLower(cat.Name)
		if strings.Contains(name, lowered) || strings.Contains(lowered, name) {
			return cat.Name, nil
		}
	}

	slog.Info("telegram: category filter unmatched, semantic matching is unported", "filter", filter)
	return filter, nil
}

func extractTelegramID(from map[string]interface{}) int64 {
	raw, exists := from["id"]
	if !exists {
		return 0
	}
	switch v := raw.(type) {
	case float64:
		return int64(v)
	case int64:
		return v
	case int:
		return int64(v)
	}
	return 0
}

// replyContextFor rebuilds the reply context for a callback press, where the
// account is taken from the stored session rather than from the current link, so
// that a draft created before the user switched accounts still saves into the
// account it was previewed against.
func (h *Handler) replyContextFor(ctx context.Context, telegramID int64, accountID, accountName string) (replyContext, error) {
	userID, _, err := h.linkService.GetTelegramLinkContext(ctx, telegramID)
	if err != nil {
		return replyContext{}, err
	}
	if userID == "" {
		return replyContext{}, errors.New("telegram account is not linked")
	}

	ac, err := h.accountService.GetAccountContext(ctx, userID, accountID)
	if err != nil {
		return replyContext{}, err
	}

	return replyContext{telegramID: telegramID, userID: userID, accountName: accountName, ac: ac}, nil
}

// errNotLinked means the Telegram account has no active DompetCerdas link.
var errNotLinked = errors.New("telegram account is not linked")

// replyContextForLink resolves the reply context from the sender's current
// default account, for entry points that have no stored account of their own.
func (h *Handler) replyContextForLink(ctx context.Context, telegramID int64) (replyContext, error) {
	userID, link, err := h.linkService.GetTelegramLinkContext(ctx, telegramID)
	if err != nil {
		return replyContext{}, err
	}
	if userID == "" {
		return replyContext{}, errNotLinked
	}

	preferredAccountID, _ := link["defaultAccountId"].(string)
	accountName, _ := link["defaultAccountName"].(string)

	ac, err := h.accountService.GetAccountContext(ctx, userID, preferredAccountID)
	if err != nil {
		return replyContext{}, err
	}

	return replyContext{telegramID: telegramID, userID: userID, accountName: accountName, ac: ac}, nil
}

// replyAdvice runs an AI analysis and posts it.
//
// The analysis takes several seconds, so a progress notice goes out first;
// without it the bot looks unresponsive for exactly the kind of request users
// are least patient with.
func (h *Handler) replyAdvice(ctx context.Context, rc replyContext, mode advisor.Mode) error {
	if h.advisor == nil || !h.advisor.Available() {
		return h.send(ctx, rc, notPortedMessage("Analisa keuangan AI"))
	}

	if err := h.bot.SendMessage(ctx, rc.telegramID, "⏳ Menganalisis data keuanganmu...", "Markdown"); err != nil {
		slog.Warn("telegram advice: failed to send progress notice", "error", err)
	}

	result, err := h.advisor.Analyze(ctx, rc.userID, rc.ac.AccountID, mode)
	if err != nil {
		switch {
		case errors.Is(err, advisor.ErrCooldown), errors.Is(err, advisor.ErrQuotaExceeded):
			// The wrapped message already tells the user how long to wait.
			return h.send(ctx, rc, "⚠️ "+err.Error())
		case errors.Is(err, advisor.ErrUnavailable):
			return h.send(ctx, rc, notPortedMessage("Analisa keuangan AI"))
		default:
			slog.Error("telegram advice: analysis failed", "userId", rc.userID, "mode", mode, "error", err)
			return h.send(ctx, rc, "❌ Gagal menganalisis data keuangan. Coba lagi sebentar.")
		}
	}

	switch mode {
	case advisor.ModeSavings:
		return h.send(ctx, rc, FormatSavingsStrategy(result.Markdown))
	case advisor.ModeSpending:
		return h.send(ctx, rc, FormatExpenseAnalysis(result.Markdown))
	default:
		return h.send(ctx, rc, FormatFinancialAdvice(result.Markdown))
	}
}
