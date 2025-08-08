package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/joho/godotenv"
)

// Constants
const (
	CallbackPrefixMarkDone = "mark_done:"
	HeaderAPISecret        = "x-spendwise-secret"
	DefaultPort            = "8080"
	DefaultAPIURL          = "http://localhost:3000"
	ErrorSendMessage       = "Failed to send error message: %v"
	ErrorSendSuccess       = "Failed to send success message: %v"
)

// ---- Config Structures ----
type SpendWiseConfig struct {
	BotToken   string
	AllowedIDs map[string]bool
	APIUrl     string
	BotUrl     string
	APISecret  string
	Port       string
	UserNames  map[string]string // chatID -> userName mapping
}

// SecretConfig represents the JSON structure in Google Cloud Secret Manager
type SecretConfig struct {
	BotToken   string            `json:"botToken"`
	AllowedIDs []string          `json:"allowedIds"`
	APIUrl     string            `json:"apiUrl"`
	BotUrl     string            `json:"botUrl"`
	APISecret  string            `json:"apiSecret"`
	Port       string            `json:"port"`
	UserNames  map[string]string `json:"userNames"`
}

// ---- Data Models ----
type Reminder struct {
	ID        string `json:"id"`
	Type      string `json:"type"`
	SubType   string `json:"subType"`
	CreatedAt struct {
		Seconds     int64 `json:"seconds"`
		Nanoseconds int64 `json:"nanoseconds"`
	} `json:"createdAt"`
	PaidMonths      []string `json:"paidMonths"`
	DayOfMonthEnd   int      `json:"dayOfMonthEnd"`
	Amount          float64  `json:"amount"`
	Description     string   `json:"description"`
	DayOfMonthStart int      `json:"dayOfMonthStart"`
	UserID          string   `json:"userId"`
	MainType        string   `json:"mainType"`
	ActiveMonths    []string `json:"activeMonths"`
	DueDate         string   `json:"dueDate"`
}

type NotificationPayload struct {
	FcmTokens       []string   `json:"fcmTokens"`
	TelegramUserIds []string   `json:"telegramUserIds"`
	Reminders       []Reminder `json:"reminders"`
}

type ExpenseInput struct {
	Description    string  `json:"description"`
	Amount         float64 `json:"amount"`
	Date           string  `json:"date"`
	Source         string  `json:"source"`
	UserName       string  `json:"userName"`
	TelegramChatID string  `json:"telegramChatId"`
}

type SummaryResponse struct {
	Markdown string `json:"markdown"`
}

var config SpendWiseConfig
var bot *tgbotapi.BotAPI

func main() {
	log.Println("🚀 Starting SpendWise Telegram Bot")

	config = loadConfig()
	log.Printf("✅ Configuration loaded - Port: %s, API URL: %s", config.Port, config.APIUrl)

	var err error
	bot, err = tgbotapi.NewBotAPI(config.BotToken)
	if err != nil {
		log.Fatalf("❌ Failed to start bot: %v", err)
	}
	bot.Debug = false
	log.Println("✅ Bot initialized successfully")

	// Set Telegram webhook
	webhookURL := config.BotUrl + "/webhook"
	log.Printf("🔗 Setting webhook to: %s", webhookURL)
	webhookConfig, _ := tgbotapi.NewWebhook(webhookURL)
	_, err = bot.Request(webhookConfig)
	if err != nil {
		log.Fatalf("❌ Failed to set webhook: %v", err)
	}
	log.Printf("✅ Webhook set successfully to: %s", webhookURL)

	r := gin.Default()

	// Add request logging middleware
	r.Use(gin.LoggerWithFormatter(func(param gin.LogFormatterParams) string {
		return fmt.Sprintf("🌐 %s [%s] \"%s %s %s\" %d %s \"%s\" \"%s\" %s\n",
			param.ClientIP,
			param.TimeStamp.Format("2006/01/02 - 15:04:05"),
			param.Method,
			param.Path,
			param.Request.Proto,
			param.StatusCode,
			param.Latency,
			param.Request.UserAgent(),
			param.ErrorMessage,
			param.Request.Header.Get("Content-Length"),
		)
	}))

	r.POST("/webhook", func(c *gin.Context) {
		log.Printf("📥 Received webhook request from IP: %s", c.ClientIP())

		var update tgbotapi.Update
		if err := c.BindJSON(&update); err != nil {
			log.Printf("❌ Invalid webhook update received: %v", err)
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid update"})
			return
		}

		// Log update details
		if update.Message != nil {
			log.Printf("📩 Processing message update - ChatID: %d, MessageID: %d, Text: %s",
				update.Message.Chat.ID, update.Message.MessageID, update.Message.Text)
		} else if update.CallbackQuery != nil {
			log.Printf("🔘 Processing callback query - ChatID: %d, Data: %s",
				update.CallbackQuery.Message.Chat.ID, update.CallbackQuery.Data)
		} else {
			log.Printf("⚠️ Received unknown update type")
		}

		handleUpdate(update)
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})

	r.POST("/internal/send-message", func(c *gin.Context) {
		log.Printf("🔐 Internal send-message request from IP: %s", c.ClientIP())

		if c.GetHeader(HeaderAPISecret) != config.APISecret {
			log.Printf("❌ Unauthorized internal API request from IP: %s", c.ClientIP())
			c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
			return
		}

		var req struct {
			ChatID  int64                  `json:"chatId"`
			Message string                 `json:"message"`
			Options map[string]interface{} `json:"options"`
		}
		if err := c.BindJSON(&req); err != nil || req.ChatID == 0 || req.Message == "" {
			log.Printf("❌ Invalid send-message request: ChatID=%d, MessageEmpty=%t, Error=%v",
				req.ChatID, req.Message == "", err)
			c.JSON(http.StatusBadRequest, gin.H{"error": "missing fields"})
			return
		}

		log.Printf("📤 Sending internal message to ChatID: %d, Message: %s", req.ChatID, req.Message)

		msg := tgbotapi.NewMessage(req.ChatID, req.Message)
		if _, err := bot.Send(msg); err != nil {
			log.Printf("❌ Failed to send internal message to ChatID %d: %v", req.ChatID, err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to send message"})
			return
		}

		log.Printf("✅ Internal message sent successfully to ChatID: %d", req.ChatID)
		c.JSON(http.StatusOK, gin.H{"success": true})
	})

	r.GET("/health", func(c *gin.Context) {
		log.Printf("💚 Health check request from IP: %s", c.ClientIP())
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})

	log.Printf("🚀 Starting server on port %s", config.Port)
	log.Printf("📊 Configured for %d allowed users", len(config.AllowedIDs))
	log.Println("🔗 Server ready to accept requests")

	if err := r.Run(":" + config.Port); err != nil {
		log.Fatalf("❌ Failed to start server: %v", err)
	}
}

func handleUpdate(update tgbotapi.Update) {
	log.Printf("🔄 Processing update type: Message=%t, CallbackQuery=%t",
		update.Message != nil, update.CallbackQuery != nil)

	if update.Message != nil {
		handleMessage(update.Message)
	} else if update.CallbackQuery != nil {
		handleCallbackQuery(update.CallbackQuery)
	} else {
		log.Printf("⚠️ Received unsupported update type")
	}
}

func handleCallbackQuery(cb *tgbotapi.CallbackQuery) {
	chatID := cb.Message.Chat.ID
	log.Printf("🔘 Processing callback query - ChatID: %d, Data: %s, UserID: %d",
		chatID, cb.Data, cb.From.ID)

	if !config.AllowedIDs[strconv.FormatInt(chatID, 10)] {
		log.Printf("❌ Unauthorized callback query from ChatID: %d, UserID: %d", chatID, cb.From.ID)
		return
	}

	data := cb.Data
	if !strings.HasPrefix(data, CallbackPrefixMarkDone) {
		log.Printf("❌ Invalid callback action: %s", data)
		bot.Request(tgbotapi.NewCallback(cb.ID, "Invalid action."))
		return
	}

	parts := strings.Split(data, ":")
	if len(parts) != 3 {
		log.Printf("❌ Invalid callback format: %s", data)
		bot.Request(tgbotapi.NewCallback(cb.ID, "Invalid format."))
		return
	}

	reminderID := parts[1]
	reminderType := parts[2]
	userID := strconv.FormatInt(chatID, 10)

	log.Printf("📝 Marking reminder as done - ID: %s, Type: %s, UserID: %s",
		reminderID, reminderType, userID)

	bot.Request(tgbotapi.NewCallback(cb.ID, "Processing..."))

	body := map[string]string{
		"reminderId":   reminderID,
		"reminderType": reminderType,
		"userId":       userID,
	}

	respBody, err := apiCall("POST", "/api/reminders/mark-as-done", body)
	if err != nil {
		log.Printf("❌ Failed to mark reminder as done - ID: %s, Error: %v", reminderID, err)
		if _, sendErr := bot.Send(tgbotapi.NewEditMessageText(cb.Message.Chat.ID, cb.Message.MessageID, "❌ Error: "+err.Error())); sendErr != nil {
			log.Printf("Failed to send error message: %v", sendErr)
		}
		return
	}

	var resp struct {
		Message string `json:"message"`
	}
	if err := json.Unmarshal(respBody, &resp); err != nil || resp.Message == "" {
		log.Printf("✅ Reminder marked as done (default message) - ID: %s", reminderID)
		if _, sendErr := bot.Send(tgbotapi.NewEditMessageText(cb.Message.Chat.ID, cb.Message.MessageID, "✅ Marked as done.")); sendErr != nil {
			log.Printf(ErrorSendSuccess, sendErr)
		}
		return
	}

	log.Printf("✅ Reminder marked as done - ID: %s, Response: %s", reminderID, resp.Message)
	msg := tgbotapi.NewEditMessageText(cb.Message.Chat.ID, cb.Message.MessageID, "✅ "+resp.Message)
	msg.ParseMode = "Markdown"
	if _, err := bot.Send(msg); err != nil {
		log.Printf("Failed to send callback response: %v", err)
	}
}

func handleMessage(msg *tgbotapi.Message) {
	startTime := time.Now()
	chatID := msg.Chat.ID
	userID := msg.From.ID
	username := msg.From.UserName
	text := strings.TrimSpace(msg.Text)

	log.Printf("📨 Processing message - ChatID: %d, UserID: %d, Username: %s, Text: %s",
		chatID, userID, username, text)

	if !config.AllowedIDs[strconv.FormatInt(chatID, 10)] {
		log.Printf("❌ Unauthorized message from ChatID: %d, UserID: %d, Username: %s",
			chatID, userID, username)
		return
	}

	defer func() {
		duration := time.Since(startTime)
		log.Printf("⏱️ Message processing completed in %d ms (%.3f seconds) - Command: %s",
			duration.Milliseconds(), duration.Seconds(), text)
	}()

	// Handle different commands
	log.Printf("🔍 Analyzing command type for: %s", text)
	switch {
	case strings.HasPrefix(text, "/start"):
		log.Printf("▶️ Handling /start command")
		handleStartCommand(msg)
	case strings.HasPrefix(text, "/help"):
		log.Printf("❓ Handling /help command")
		handleHelpCommand(msg)
	case strings.HasPrefix(text, "/expense"):
		log.Printf("💰 Handling /expense command")
		handleExpenseCommand(msg)
	case strings.HasPrefix(text, "/reminders"):
		log.Printf("🔔 Handling /reminders command")
		handleRemindersCommand(msg)
	case strings.HasPrefix(text, "/summary"):
		log.Printf("📊 Handling /summary command")
		handleSummaryCommand(msg)
	case strings.HasPrefix(text, "/month"):
		log.Printf("📈 Handling /month command")
		handleMonthCommand(msg)
	default:
		// Try to parse as expense - check if it contains numbers (no currency symbols needed)
		if containsNumber(text) {
			log.Printf("💸 Detected quick expense input")
			handleQuickExpense(msg)
		} else {
			log.Printf("❓ Unknown command received")
			handleUnknownCommand(msg)
		}
	}
}

// containsNumber checks if text contains any numeric values
func containsNumber(text string) bool {
	parts := strings.Fields(text)
	for _, part := range parts {
		if _, err := strconv.ParseFloat(part, 64); err == nil {
			return true
		}
	}
	return false
}

func handleStartCommand(msg *tgbotapi.Message) {
	log.Printf("▶️ Sending welcome message to ChatID: %d", msg.Chat.ID)
	response := "Welcome to SpendWise Bot! Use /summary for today's expenses, or log expenses like 'Groceries 50'."

	reply := tgbotapi.NewMessage(msg.Chat.ID, response)
	if _, err := bot.Send(reply); err != nil {
		log.Printf("❌ Failed to send start message to ChatID %d: %v", msg.Chat.ID, err)
	} else {
		log.Printf("✅ Welcome message sent successfully to ChatID: %d", msg.Chat.ID)
	}
}

func handleHelpCommand(msg *tgbotapi.Message) {
	log.Printf("❓ Sending help message to ChatID: %d", msg.Chat.ID)
	response := "SpendWise Bot Help 📖\n\n" +
		"Commands:\n" +
		"• /start - Welcome message\n" +
		"• /expense - Add a new expense\n" +
		"• /reminders - View your reminders\n" +
		"• /summary - View today's expense summary\n" +
		"• /month - View this month's summary\n\n" +
		"Expense formats (both work):\n" +
		"• description amount\n" +
		"• amount description\n\n" +
		"Examples:\n" +
		"Coffee Tea 15.50\n" +
		"25 Lunch at restaurant\n\n" +
		"Batch example:\n" +
		"Coffee 5.50\n" +
		"12.25 Lunch\n" +
		"Gas bill 45"

	reply := tgbotapi.NewMessage(msg.Chat.ID, response)
	if _, err := bot.Send(reply); err != nil {
		log.Printf("❌ Failed to send help message to ChatID %d: %v", msg.Chat.ID, err)
	} else {
		log.Printf("✅ Help message sent successfully to ChatID: %d", msg.Chat.ID)
	}
}

func handleExpenseCommand(msg *tgbotapi.Message) {
	log.Printf("💰 Sending expense help to ChatID: %d", msg.Chat.ID)
	response := "To add expenses, use either format:\n\n" +
		"Format 1: description amount\n" +
		"Format 2: amount description\n\n" +
		"Examples:\n" +
		"• Coffee Tea 5.50\n" +
		"• 25.99 Groceries\n" +
		"• Gas bill 150\n" +
		"• 12 Lunch\n\n" +
		"Batch example:\n" +
		"Coffee 5.50\n" +
		"12 Lunch\n" +
		"Gas bill 45.75"

	reply := tgbotapi.NewMessage(msg.Chat.ID, response)
	if _, err := bot.Send(reply); err != nil {
		log.Printf("❌ Failed to send expense help to ChatID %d: %v", msg.Chat.ID, err)
	} else {
		log.Printf("✅ Expense help sent successfully to ChatID: %d", msg.Chat.ID)
	}
}

func handleRemindersCommand(msg *tgbotapi.Message) {
	startTime := time.Now()
	log.Printf("🔔 Starting reminders command processing")

	defer func() {
		duration := time.Since(startTime)
		log.Printf("⏱️ Reminders command completed in %d ms (%.3f seconds)",
			duration.Milliseconds(), duration.Seconds())
	}()

	respBody, err := apiCall("GET", "/api/reminders/get-payload", nil)
	if err != nil {
		reply := tgbotapi.NewMessage(msg.Chat.ID, "❌ Error fetching reminders: "+err.Error())
		if _, sendErr := bot.Send(reply); sendErr != nil {
			log.Printf(ErrorSendMessage, sendErr)
		}
		return
	}

	var payload NotificationPayload
	if err := json.Unmarshal(respBody, &payload); err != nil {
		reply := tgbotapi.NewMessage(msg.Chat.ID, "❌ Error parsing reminders")
		if _, sendErr := bot.Send(reply); sendErr != nil {
			log.Printf(ErrorSendMessage, sendErr)
		}
		return
	}

	if len(payload.Reminders) == 0 {
		log.Printf("📝 No reminders found for ChatID: %d", msg.Chat.ID)
		reply := tgbotapi.NewMessage(msg.Chat.ID, "No reminders found 📝")
		if _, err := bot.Send(reply); err != nil {
			log.Printf("❌ Failed to send 'no reminders' message to ChatID %d: %v", msg.Chat.ID, err)
		}
		return
	}

	log.Printf("📋 Found %d reminders for ChatID: %d", len(payload.Reminders), msg.Chat.ID)
	response := "🔔 Daily Reminders\n\n"
	for i, reminder := range payload.Reminders {
		formattedAmount := formatCurrency(reminder.Amount)
		dueDateText := formatDueDate(reminder)
		response += fmt.Sprintf("  • %s - %s (%s)\n",
			reminder.Description, formattedAmount, dueDateText)
		log.Printf("📌 Reminder %d: %s - %s (%s)", i+1, reminder.Description, formattedAmount, dueDateText)
	}

	response += "\nPlease check the app to take action."

	reply := tgbotapi.NewMessage(msg.Chat.ID, response)
	if _, err := bot.Send(reply); err != nil {
		log.Printf("❌ Failed to send reminders list to ChatID %d: %v", msg.Chat.ID, err)
	} else {
		log.Printf("✅ Reminders list sent successfully to ChatID: %d", msg.Chat.ID)
	}
}

// formatCurrency formats amount with Indian Rupee symbol and proper comma separation
func formatCurrency(amount float64) string {
	// Format with 2 decimal places
	amountStr := fmt.Sprintf("%.2f", amount)

	// Add commas for thousands separator (simple implementation for Indian format)
	if amount >= 1000 {
		// For amounts >= 1000, add comma separator
		parts := strings.Split(amountStr, ".")
		intPart := parts[0]
		decPart := parts[1]

		// Add comma before last 3 digits
		if len(intPart) > 3 {
			intPart = intPart[:len(intPart)-3] + "," + intPart[len(intPart)-3:]
		}
		amountStr = intPart + "." + decPart
	}

	return "₹" + amountStr
}

// formatDueDate formats the due date based on day range or if it's today
func formatDueDate(reminder Reminder) string {
	now := time.Now()
	currentDay := now.Day()

	// If start and end dates are the same, check if it's today
	if reminder.DayOfMonthStart == reminder.DayOfMonthEnd {
		if currentDay == reminder.DayOfMonthStart {
			return "Due Today"
		}
		return fmt.Sprintf("Due on %d", reminder.DayOfMonthStart)
	}

	// If it's a range and today falls within it
	if currentDay >= reminder.DayOfMonthStart && currentDay <= reminder.DayOfMonthEnd {
		return "Due Today"
	}

	// Return the range format
	return fmt.Sprintf("Due between %d-%d", reminder.DayOfMonthStart, reminder.DayOfMonthEnd)
}

func handleSummaryCommand(msg *tgbotapi.Message) {
	startTime := time.Now()
	log.Printf("📊 Starting daily summary command processing")

	defer func() {
		duration := time.Since(startTime)
		log.Printf("⏱️ Daily summary command completed in %d ms (%.3f seconds)",
			duration.Milliseconds(), duration.Seconds())
	}()

	respBody, err := apiCall("GET", "/api/summary/today", nil)
	if err != nil {
		errorMsg := fmt.Sprintf("Sorry, I couldn't fetch your daily summary: %s", err.Error())
		reply := tgbotapi.NewMessage(msg.Chat.ID, errorMsg)
		if _, sendErr := bot.Send(reply); sendErr != nil {
			log.Printf(ErrorSendMessage, sendErr)
		}
		return
	}

	var summaryResp SummaryResponse
	if err := json.Unmarshal(respBody, &summaryResp); err != nil {
		reply := tgbotapi.NewMessage(msg.Chat.ID, "❌ Error parsing daily summary response")
		if _, sendErr := bot.Send(reply); sendErr != nil {
			log.Printf(ErrorSendMessage, sendErr)
		}
		return
	}

	// Send the markdown response
	reply := tgbotapi.NewMessage(msg.Chat.ID, summaryResp.Markdown)
	reply.ParseMode = "Markdown"
	if _, err := bot.Send(reply); err != nil {
		log.Printf("❌ Failed to send daily summary to ChatID %d: %v", msg.Chat.ID, err)
	} else {
		log.Printf("✅ Daily summary sent successfully to ChatID: %d", msg.Chat.ID)
	}
}

func handleMonthCommand(msg *tgbotapi.Message) {
	startTime := time.Now()
	log.Printf("📈 Starting monthly summary command processing")

	defer func() {
		duration := time.Since(startTime)
		log.Printf("⏱️ Monthly summary command completed in %d ms (%.3f seconds)",
			duration.Milliseconds(), duration.Seconds())
	}()

	respBody, err := apiCall("GET", "/api/summary/month", nil)
	if err != nil {
		errorMsg := fmt.Sprintf("Sorry, I couldn't fetch your monthly summary: %s", err.Error())
		reply := tgbotapi.NewMessage(msg.Chat.ID, errorMsg)
		if _, sendErr := bot.Send(reply); sendErr != nil {
			log.Printf(ErrorSendMessage, sendErr)
		}
		return
	}

	var summaryResp SummaryResponse
	if err := json.Unmarshal(respBody, &summaryResp); err != nil {
		reply := tgbotapi.NewMessage(msg.Chat.ID, "❌ Error parsing monthly summary response")
		if _, sendErr := bot.Send(reply); sendErr != nil {
			log.Printf(ErrorSendMessage, sendErr)
		}
		return
	}

	// Send the markdown response
	reply := tgbotapi.NewMessage(msg.Chat.ID, summaryResp.Markdown)
	reply.ParseMode = "Markdown"
	if _, err := bot.Send(reply); err != nil {
		log.Printf("❌ Failed to send monthly summary to ChatID %d: %v", msg.Chat.ID, err)
	} else {
		log.Printf("✅ Monthly summary sent successfully to ChatID: %d", msg.Chat.ID)
	}
}

func handleQuickExpense(msg *tgbotapi.Message) {
	startTime := time.Now()
	text := strings.TrimSpace(msg.Text)
	log.Printf("🚀 Starting expense processing for ChatID: %d, Text: %s", msg.Chat.ID, text)

	defer func() {
		duration := time.Since(startTime)
		log.Printf("⏱️ Expense processing completed in %d ms (%.3f seconds) for ChatID: %d",
			duration.Milliseconds(), duration.Seconds(), msg.Chat.ID)
	}()

	// Parse expenses (single or batch)
	expenses, err := parseExpenses(text, msg)
	if err != nil {
		log.Printf("❌ Failed to parse expenses for ChatID %d: %v", msg.Chat.ID, err)
		reply := tgbotapi.NewMessage(msg.Chat.ID, "❌ "+err.Error())
		if _, sendErr := bot.Send(reply); sendErr != nil {
			log.Printf(ErrorSendMessage, sendErr)
		}
		return
	}

	log.Printf("📝 Parsed %d expenses for ChatID: %d", len(expenses), msg.Chat.ID)
	for i, expense := range expenses {
		log.Printf("💰 Expense %d: %s - %.2f", i+1, expense.Description, expense.Amount)
	}

	// Send to API as array
	log.Printf("🌐 Sending %d expenses to API for ChatID: %d", len(expenses), msg.Chat.ID)
	respBody, err := apiCall("POST", "/api/expenses/create-batch-from-bot", expenses)
	if err != nil {
		log.Printf("❌ API call failed for ChatID %d: %v", msg.Chat.ID, err)
		reply := tgbotapi.NewMessage(msg.Chat.ID, "❌ Error saving expenses: "+err.Error())
		if _, sendErr := bot.Send(reply); sendErr != nil {
			log.Printf(ErrorSendMessage, sendErr)
		}
		return
	}

	// Parse API response
	var apiResp struct {
		Success bool   `json:"success"`
		Message string `json:"message"`
		Error   string `json:"error"`
		Details string `json:"details"`
	}

	if err := json.Unmarshal(respBody, &apiResp); err != nil {
		log.Printf("❌ Failed to parse API response for ChatID %d: %v", msg.Chat.ID, err)
		reply := tgbotapi.NewMessage(msg.Chat.ID, "❌ Error parsing API response")
		if _, sendErr := bot.Send(reply); sendErr != nil {
			log.Printf(ErrorSendMessage, sendErr)
		}
		return
	}

	log.Printf("📊 API Response for ChatID %d - Success: %t, Message: %s, Error: %s",
		msg.Chat.ID, apiResp.Success, apiResp.Message, apiResp.Error)

	// Send success or error message based on API response
	if apiResp.Success {
		if len(expenses) == 1 {
			log.Printf("👍 Sending reaction for single expense to ChatID: %d", msg.Chat.ID)
			// Single expense - send reaction instead of message
			if err := sendReaction(msg.Chat.ID, msg.MessageID, "👍"); err != nil {
				log.Printf("❌ Failed to send reaction, falling back to message for ChatID %d: %v", msg.Chat.ID, err)
				// Fallback to text message if reaction fails
				successMsg := tgbotapi.NewMessage(msg.Chat.ID, "✅ Expense logged successfully!")
				if _, sendErr := bot.Send(successMsg); sendErr != nil {
					log.Printf(ErrorSendSuccess, sendErr)
				}
			} else {
				log.Printf("✅ Reaction sent successfully for ChatID: %d", msg.Chat.ID)
			}
		} else {
			// Multiple expenses - send text reply
			var successMsg string
			if apiResp.Message != "" {
				successMsg = "✅ " + apiResp.Message
			} else {
				successMsg = fmt.Sprintf("✅ %d expenses saved successfully", len(expenses))
			}

			log.Printf("✅ Sending success message for %d expenses to ChatID: %d", len(expenses), msg.Chat.ID)
			reply := tgbotapi.NewMessage(msg.Chat.ID, successMsg)
			if _, err := bot.Send(reply); err != nil {
				log.Printf(ErrorSendSuccess, err)
			} else {
				log.Printf("✅ Success message sent for ChatID: %d", msg.Chat.ID)
			}
		}
	} else {
		// Error response - always send text message
		errorMsg := "❌ API Error"
		if apiResp.Error != "" {
			errorMsg = "❌ " + apiResp.Error
		}
		if apiResp.Details != "" {
			errorMsg += "\nDetails: " + apiResp.Details
		}

		log.Printf("❌ Sending error message to ChatID %d: %s", msg.Chat.ID, errorMsg)
		reply := tgbotapi.NewMessage(msg.Chat.ID, errorMsg)
		if _, err := bot.Send(reply); err != nil {
			log.Printf(ErrorSendMessage, err)
		}
	}
}

func parseExpenses(text string, msg *tgbotapi.Message) ([]ExpenseInput, error) {
	lines := strings.Split(text, "\n")
	var expenses []ExpenseInput

	log.Printf("📊 Parsing %d lines of expense input for ChatID: %d", len(lines), msg.Chat.ID)

	for i, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			log.Printf("⏭️ Skipping empty line %d", i+1)
			continue // Skip empty lines
		}

		log.Printf("🔍 Parsing line %d: %s", i+1, line)
		amount, description, err := parseExpenseText(line)
		if err != nil {
			log.Printf("❌ Failed to parse line %d (%s): %v", i+1, line, err)
			return nil, fmt.Errorf("line %d: %s", i+1, err.Error())
		}

		expense := ExpenseInput{
			Description:    description,
			Amount:         amount,
			Date:           time.Now().Format("2006-01-02"),
			Source:         "bot",
			UserName:       getUserName(msg),
			TelegramChatID: strconv.FormatInt(msg.Chat.ID, 10),
		}

		if err := validateExpenseInput(expense); err != nil {
			log.Printf("❌ Validation failed for line %d: %v", i+1, err)
			return nil, fmt.Errorf("line %d: %s", i+1, err.Error())
		}

		log.Printf("✅ Parsed expense: %s - %.2f (User: %s)", description, amount, expense.UserName)
		expenses = append(expenses, expense)
	}

	if len(expenses) == 0 {
		log.Printf("❌ No valid expenses found in input")
		return nil, fmt.Errorf("no valid expenses found")
	}

	log.Printf("✅ Successfully parsed %d expenses", len(expenses))
	return expenses, nil
}

// getUserName gets the username for a chat ID from config or fallback to Telegram name
func getUserName(msg *tgbotapi.Message) string {
	chatID := strconv.FormatInt(msg.Chat.ID, 10)

	log.Printf("🔍 Getting username for ChatID: %s", chatID)

	// Check if we have a configured username for this chat ID
	if userName, exists := config.UserNames[chatID]; exists && userName != "" {
		log.Printf("✅ Found configured username for ChatID %s: %s", chatID, userName)
		return userName
	}

	// Fallback to Telegram username or first/last name
	if msg.From.UserName != "" {
		log.Printf("📱 Using Telegram username for ChatID %s: %s", chatID, msg.From.UserName)
		return msg.From.UserName
	}

	name := strings.TrimSpace(msg.From.FirstName + " " + msg.From.LastName)
	if name != "" {
		log.Printf("👤 Using full name for ChatID %s: %s", chatID, name)
		return name
	}

	// Last resort
	fallbackName := "User_" + chatID
	log.Printf("⚠️ Using fallback username for ChatID %s: %s", chatID, fallbackName)
	return fallbackName
}

func handleUnknownCommand(msg *tgbotapi.Message) {
	log.Printf("❓ Unknown command received from ChatID: %d, Text: %s", msg.Chat.ID, msg.Text)
	response := "I don't understand that command. Type /help for available commands."
	reply := tgbotapi.NewMessage(msg.Chat.ID, response)
	if _, err := bot.Send(reply); err != nil {
		log.Printf("❌ Failed to send unknown command message to ChatID %d: %v", msg.Chat.ID, err)
	} else {
		log.Printf("✅ Unknown command response sent to ChatID: %d", msg.Chat.ID)
	}
}

func parseExpenseText(text string) (float64, string, error) {
	// Clean up text (no currency symbols needed)
	text = strings.TrimSpace(text)
	log.Printf("🔍 Parsing expense text: %s", text)

	parts := strings.Fields(text)
	if len(parts) < 2 {
		log.Printf("❌ Invalid format - need description and amount")
		return 0, "", fmt.Errorf("invalid format - need description and amount")
	}

	var amounts []float64
	var descriptionParts []string

	// Separate amounts from description
	for _, part := range parts {
		if amount, err := strconv.ParseFloat(part, 64); err == nil && amount > 0 {
			amounts = append(amounts, amount)
			log.Printf("💰 Found amount: %.2f", amount)
		} else {
			descriptionParts = append(descriptionParts, part)
		}
	}

	if len(amounts) == 0 {
		log.Printf("❌ No valid amount found in: %s", text)
		return 0, "", fmt.Errorf("no valid amount found")
	}

	if len(descriptionParts) == 0 {
		log.Printf("❌ Missing description in: %s", text)
		return 0, "", fmt.Errorf("missing description")
	}

	// Sum all amounts
	var totalAmount float64
	for _, amount := range amounts {
		totalAmount += amount
	}

	description := strings.Join(descriptionParts, " ")
	log.Printf("✅ Parsed: %s - %.2f (total from %d amounts)", description, totalAmount, len(amounts))
	return totalAmount, description, nil
}

// validateExpenseInput validates expense input data
func validateExpenseInput(input ExpenseInput) error {
	if input.Amount <= 0 {
		return fmt.Errorf("amount must be positive")
	}
	if strings.TrimSpace(input.Description) == "" {
		return fmt.Errorf("description cannot be empty")
	}
	if strings.TrimSpace(input.TelegramChatID) == "" {
		return fmt.Errorf("telegram chat ID cannot be empty")
	}
	return nil
}

// loadConfig loads configuration from JSON env var or individual environment variables
func loadConfig() SpendWiseConfig {
	// Option 1: Try JSON in environment variable
	configJSON := os.Getenv("CONFIG_JSON")
	if configJSON != "" {
		log.Println("📋 Using JSON configuration from CONFIG_JSON environment variable")
		var secretConfig SecretConfig
		if err := json.Unmarshal([]byte(configJSON), &secretConfig); err != nil {
			log.Printf("❌ Failed to parse CONFIG_JSON: %v", err)
			log.Println("⚠️ Falling back to individual environment variables")
		} else {
			log.Println("✅ Configuration loaded from CONFIG_JSON environment variable")
			return convertSecretConfigToSpendWiseConfig(&secretConfig)
		}
	}

	// Option 2: Fallback to individual environment variables
	log.Println("🔧 Loading configuration from individual environment variables")
	return loadConfigFromEnvVars()
}

// convertSecretConfigToSpendWiseConfig converts SecretConfig to SpendWiseConfig
func convertSecretConfigToSpendWiseConfig(secretConfig *SecretConfig) SpendWiseConfig {
	// Convert SecretConfig to SpendWiseConfig
	allowedIDs := make(map[string]bool)
	for _, id := range secretConfig.AllowedIDs {
		allowedIDs[strings.TrimSpace(id)] = true
	}

	// Set defaults if not provided in secret
	apiUrl := secretConfig.APIUrl
	if apiUrl == "" {
		apiUrl = DefaultAPIURL
	}

	port := secretConfig.Port
	if port == "" {
		port = DefaultPort
	}

	// Validate required fields
	if secretConfig.BotToken == "" {
		log.Fatal("botToken is required in configuration")
	}
	if secretConfig.BotUrl == "" {
		log.Fatal("botUrl is required in configuration")
	}
	if secretConfig.APISecret == "" {
		log.Fatal("apiSecret is required in configuration")
	}

	log.Println("✅ Configuration loaded successfully")
	return SpendWiseConfig{
		BotToken:   secretConfig.BotToken,
		AllowedIDs: allowedIDs,
		APIUrl:     apiUrl,
		BotUrl:     secretConfig.BotUrl,
		APISecret:  secretConfig.APISecret,
		Port:       port,
		UserNames:  secretConfig.UserNames,
	}
}

// loadConfigFromEnvVars loads configuration from individual environment variables
func loadConfigFromEnvVars() SpendWiseConfig {
	// Load .env file if it exists
	if err := godotenv.Load(); err != nil {
		log.Println("No .env file found or error loading .env file:", err)
	}

	botToken := os.Getenv("BOT_TOKEN")
	if botToken == "" {
		log.Fatal("BOT_TOKEN environment variable is required")
	}

	apiUrl := os.Getenv("API_URL")
	if apiUrl == "" {
		apiUrl = DefaultAPIURL // default
	}

	botUrl := os.Getenv("BOT_URL")
	if botUrl == "" {
		log.Fatal("BOT_URL environment variable is required")
	}

	apiSecret := os.Getenv("API_SECRET")
	if apiSecret == "" {
		log.Fatal("API_SECRET environment variable is required")
	}

	port := os.Getenv("PORT")
	if port == "" {
		port = DefaultPort // default
	}

	// Parse allowed IDs
	allowedIDsStr := os.Getenv("ALLOWED_IDS")
	allowedIDs := make(map[string]bool)
	if allowedIDsStr != "" {
		ids := strings.Split(allowedIDsStr, ",")
		for _, id := range ids {
			allowedIDs[strings.TrimSpace(id)] = true
		}
	}

	// Parse username mappings: "chatID1:username1,chatID2:username2"
	userNamesStr := os.Getenv("USER_NAMES")
	userNames := make(map[string]string)
	if userNamesStr != "" {
		mappings := strings.Split(userNamesStr, ",")
		for _, mapping := range mappings {
			parts := strings.Split(mapping, ":")
			if len(parts) == 2 {
				chatID := strings.TrimSpace(parts[0])
				userName := strings.TrimSpace(parts[1])
				if chatID != "" && userName != "" {
					userNames[chatID] = userName
				}
			}
		}
	}

	log.Println("✅ Configuration loaded from environment variables")
	return SpendWiseConfig{
		BotToken:   botToken,
		AllowedIDs: allowedIDs,
		APIUrl:     apiUrl,
		BotUrl:     botUrl,
		APISecret:  apiSecret,
		Port:       port,
		UserNames:  userNames,
	}
}

// apiCall makes HTTP requests to the SpendWise API
func apiCall(method, endpoint string, body interface{}) ([]byte, error) {
	startTime := time.Now()
	log.Printf("🌐 Starting API call: %s %s", method, endpoint)

	defer func() {
		duration := time.Since(startTime)
		log.Printf("⏱️ API call completed in %d ms (%.3f seconds) - %s %s",
			duration.Milliseconds(), duration.Seconds(), method, endpoint)
	}()

	var reqBody []byte
	var err error

	if body != nil {
		reqBody, err = json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal request body: %v", err)
		}
	}

	url := config.APIUrl + endpoint
	req, err := http.NewRequest(method, url, bytes.NewBuffer(reqBody))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %v", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(HeaderAPISecret, config.APISecret)

	client := &http.Client{
		Timeout: 30 * time.Second,
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %v", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %v", err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		// Try to parse error response for better error messages
		var errorResp struct {
			Error   string `json:"error"`
			Details string `json:"details"`
		}

		if json.Unmarshal(respBody, &errorResp) == nil && errorResp.Error != "" {
			errorMsg := errorResp.Error
			if errorResp.Details != "" {
				errorMsg += ": " + errorResp.Details
			}
			return nil, fmt.Errorf("%s", errorMsg)
		}

		return nil, fmt.Errorf("API error (%d): %s", resp.StatusCode, string(respBody))
	}

	return respBody, nil
}

// sendReaction sends a reaction to a specific message
func sendReaction(chatID int64, messageID int, emoji string) error {
	startTime := time.Now()
	log.Printf("👍 Starting reaction send: %s to message %d", emoji, messageID)

	defer func() {
		duration := time.Since(startTime)
		log.Printf("⏱️ Reaction send completed in %d ms (%.3f seconds)",
			duration.Milliseconds(), duration.Seconds())
	}()

	// Create the reaction payload according to Telegram Bot API
	payload := map[string]interface{}{
		"chat_id":    chatID,
		"message_id": messageID,
		"reaction": []map[string]interface{}{
			{
				"type":  "emoji",
				"emoji": emoji,
			},
		},
	}

	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal reaction payload: %v", err)
	}

	// Create HTTP request to Telegram Bot API
	url := fmt.Sprintf("https://api.telegram.org/bot%s/setMessageReaction", config.BotToken)
	req, err := http.NewRequest("POST", url, bytes.NewBuffer(payloadBytes))
	if err != nil {
		return fmt.Errorf("failed to create reaction request: %v", err)
	}

	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{
		Timeout: 10 * time.Second,
	}

	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("reaction request failed: %v", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read reaction response: %v", err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("reaction API error (%d): %s", resp.StatusCode, string(respBody))
	}

	log.Printf("Reaction sent successfully: %s to message %d", emoji, messageID)
	return nil
}
