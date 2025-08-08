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
)

// Constants
const (
	CallbackPrefixMarkDone = "mark_done:"
	HeaderAPISecret        = "x-spendwise-secret"
	DefaultPort           = "8080"
	DefaultAPIURL         = "http://localhost:3000"
	ErrorSendMessage      = "Failed to send error message: %v"
	ErrorSendSuccess      = "Failed to send success message: %v"
)

// ---- Config Structures ----
type SpendWiseConfig struct {
	BotToken   string
	AllowedIDs map[string]bool
	APIUrl     string
	BotUrl     string
	APISecret  string
	Port       string
}

// ---- Data Models ----
type Reminder struct {
	ID             string   `json:"id"`
	Type           string   `json:"type"`
	SubType        string   `json:"subType"`
	CreatedAt      struct{
		Seconds     int64 `json:"seconds"`
		Nanoseconds int64 `json:"nanoseconds"`
	} `json:"createdAt"`
	PaidMonths     []string `json:"paidMonths"`
	DayOfMonthEnd  int      `json:"dayOfMonthEnd"`
	Amount         float64  `json:"amount"`
	Description    string   `json:"description"`
	DayOfMonthStart int     `json:"dayOfMonthStart"`
	UserID         string   `json:"userId"`
	MainType       string   `json:"mainType"`
	ActiveMonths   []string `json:"activeMonths"`
	DueDate        string   `json:"dueDate"`
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

var config SpendWiseConfig
var bot *tgbotapi.BotAPI

func main() {
	config = loadConfig()

	var err error
	bot, err = tgbotapi.NewBotAPI(config.BotToken)
	if err != nil {
		log.Fatal("Failed to start bot:", err)
	}
	bot.Debug = false
	log.Println("Bot initialized")

	// Set Telegram webhook
	webhookURL := config.BotUrl + "/webhook"
	_, err = bot.Request(tgbotapi.NewWebhook(webhookURL))
	if err != nil {
		log.Fatalf("Failed to set webhook: %v", err)
	}
	log.Printf("Webhook set to: %s", webhookURL)

	r := gin.Default()
	r.POST("/webhook", func(c *gin.Context) {
		var update tgbotapi.Update
		if err := c.BindJSON(&update); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid update"})
			return
		}
		handleUpdate(update)
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})

	r.POST("/internal/send-message", func(c *gin.Context) {
		if c.GetHeader(HeaderAPISecret) != config.APISecret {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
			return
		}
		var req struct {
			ChatID  int64                  `json:"chatId"`
			Message string                 `json:"message"`
			Options map[string]interface{} `json:"options"`
		}
		if err := c.BindJSON(&req); err != nil || req.ChatID == 0 || req.Message == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "missing fields"})
			return
		}
		msg := tgbotapi.NewMessage(req.ChatID, req.Message)
		if _, err := bot.Send(msg); err != nil {
			log.Printf("Failed to send message: %v", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to send message"})
			return
		}
		c.JSON(http.StatusOK, gin.H{"success": true})
	})

	r.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})

	log.Println("Listening on port", config.Port)
	r.Run(":" + config.Port)
}

func handleUpdate(update tgbotapi.Update) {
	if update.Message != nil {
		handleMessage(update.Message)
	} else if update.CallbackQuery != nil {
		handleCallbackQuery(update.CallbackQuery)
	}
}

func handleCallbackQuery(cb *tgbotapi.CallbackQuery) {
	chatID := cb.Message.Chat.ID
	if !config.AllowedIDs[strconv.FormatInt(chatID, 10)] {
		log.Println("Unauthorized callback query from chat ID:", chatID)
		return
	}

	data := cb.Data
	if !strings.HasPrefix(data, CallbackPrefixMarkDone) {
		bot.Request(tgbotapi.NewCallback(cb.ID, "Invalid action."))
		return
	}

	parts := strings.Split(data, ":")
	if len(parts) != 3 {
		bot.Request(tgbotapi.NewCallback(cb.ID, "Invalid format."))
		return
	}

	reminderID := parts[1]
	reminderType := parts[2]
	userID := strconv.FormatInt(chatID, 10)

	bot.Request(tgbotapi.NewCallback(cb.ID, "Processing..."))

	body := map[string]string{
		"reminderId":   reminderID,
		"reminderType": reminderType,
		"userId":       userID,
	}

	respBody, err := apiCall("POST", "/api/reminders/mark-as-done", body)
	if err != nil {
		if _, sendErr := bot.Send(tgbotapi.NewEditMessageText(cb.Message.Chat.ID, cb.Message.MessageID, "❌ Error: "+err.Error())); sendErr != nil {
			log.Printf("Failed to send error message: %v", sendErr)
		}
		return
	}

	var resp struct {
		Message string `json:"message"`
	}
	if err := json.Unmarshal(respBody, &resp); err != nil || resp.Message == "" {
		if _, sendErr := bot.Send(tgbotapi.NewEditMessageText(cb.Message.Chat.ID, cb.Message.MessageID, "✅ Marked as done.")); sendErr != nil {
			log.Printf(ErrorSendSuccess, sendErr)
		}
		return
	}

	msg := tgbotapi.NewEditMessageText(cb.Message.Chat.ID, cb.Message.MessageID, "✅ "+resp.Message)
	msg.ParseMode = "Markdown"
	if _, err := bot.Send(msg); err != nil {
		log.Printf("Failed to send callback response: %v", err)
	}
}

func handleMessage(msg *tgbotapi.Message) {
	chatID := msg.Chat.ID
	if !config.AllowedIDs[strconv.FormatInt(chatID, 10)] {
		log.Println("Unauthorized message from chat ID:", chatID)
		return
	}

	text := strings.TrimSpace(msg.Text)
	
	// Handle different commands
	switch {
	case strings.HasPrefix(text, "/start"):
		handleStartCommand(msg)
	case strings.HasPrefix(text, "/help"):
		handleHelpCommand(msg)
	case strings.HasPrefix(text, "/expense"):
		handleExpenseCommand(msg)
	case strings.HasPrefix(text, "/reminders"):
		handleRemindersCommand(msg)
	default:
		// Try to parse as expense if it contains amount
		if strings.Contains(text, "$") || strings.Contains(text, "€") || strings.Contains(text, "£") {
			handleQuickExpense(msg)
		} else {
			handleUnknownCommand(msg)
		}
	}
}

func handleStartCommand(msg *tgbotapi.Message) {
	response := "Welcome to SpendWise! 💰\n\n" +
		"Available commands:\n" +
		"/help - Show this help message\n" +
		"/expense - Add a new expense\n" +
		"/reminders - View your reminders\n\n" +
		"You can also quickly add expenses by typing: amount description\n" +
		"Example: $25 Coffee and breakfast"
	
	reply := tgbotapi.NewMessage(msg.Chat.ID, response)
	if _, err := bot.Send(reply); err != nil {
		log.Printf("Failed to send start message: %v", err)
	}
}

func handleHelpCommand(msg *tgbotapi.Message) {
	response := "SpendWise Bot Help 📖\n\n" +
		"Commands:\n" +
		"• /start - Welcome message\n" +
		"• /expense - Add a new expense\n" +
		"• /reminders - View your reminders\n\n" +
		"Quick expense format:\n" +
		"$amount description\n" +
		"Example: $15.50 Lunch at cafe"
	
	reply := tgbotapi.NewMessage(msg.Chat.ID, response)
	if _, err := bot.Send(reply); err != nil {
		log.Printf("Failed to send help message: %v", err)
	}
}

func handleExpenseCommand(msg *tgbotapi.Message) {
	response := "To add an expense, use the format:\n" +
		"$amount description\n\n" +
		"Examples:\n" +
		"• $25.99 Groceries\n" +
		"• $12 Coffee\n" +
		"• $150 Gas bill"
	
	reply := tgbotapi.NewMessage(msg.Chat.ID, response)
	if _, err := bot.Send(reply); err != nil {
		log.Printf("Failed to send expense help message: %v", err)
	}
}

func handleRemindersCommand(msg *tgbotapi.Message) {
	userID := strconv.FormatInt(msg.Chat.ID, 10)
	
	respBody, err := apiCall("GET", "/api/reminders?userId="+userID, nil)
	if err != nil {
		reply := tgbotapi.NewMessage(msg.Chat.ID, "❌ Error fetching reminders: "+err.Error())
		if _, sendErr := bot.Send(reply); sendErr != nil {
			log.Printf(ErrorSendMessage, sendErr)
		}
		return
	}
	
	var reminders []Reminder
	if err := json.Unmarshal(respBody, &reminders); err != nil {
		reply := tgbotapi.NewMessage(msg.Chat.ID, "❌ Error parsing reminders")
		if _, sendErr := bot.Send(reply); sendErr != nil {
			log.Printf(ErrorSendMessage, sendErr)
		}
		return
	}
	
	if len(reminders) == 0 {
		reply := tgbotapi.NewMessage(msg.Chat.ID, "No reminders found 📝")
		if _, err := bot.Send(reply); err != nil {
			log.Printf("Failed to send reminders message: %v", err)
		}
		return
	}
	
	response := "Your Reminders 📋\n\n"
	for _, reminder := range reminders {
		response += fmt.Sprintf("• %s - $%.2f\n  Due: %s\n\n", 
			reminder.Description, reminder.Amount, reminder.DueDate)
	}
	
	reply := tgbotapi.NewMessage(msg.Chat.ID, response)
	if _, err := bot.Send(reply); err != nil {
		log.Printf("Failed to send reminders list: %v", err)
	}
}

func handleQuickExpense(msg *tgbotapi.Message) {
	text := strings.TrimSpace(msg.Text)
	
	// Parse amount and description
	amount, description, err := parseExpenseText(text)
	if err != nil {
		reply := tgbotapi.NewMessage(msg.Chat.ID, "❌ Invalid format. Use: $amount description")
		if _, sendErr := bot.Send(reply); sendErr != nil {
			log.Printf(ErrorSendMessage, sendErr)
		}
		return
	}
	
	expense := ExpenseInput{
		Description:    description,
		Amount:         amount,
		Date:           time.Now().Format("2006-01-02"),
		Source:         "telegram",
		UserName:       msg.From.FirstName + " " + msg.From.LastName,
		TelegramChatID: strconv.FormatInt(msg.Chat.ID, 10),
	}
	
	// Validate expense input
	if err := validateExpenseInput(expense); err != nil {
		reply := tgbotapi.NewMessage(msg.Chat.ID, "❌ "+err.Error())
		if _, sendErr := bot.Send(reply); sendErr != nil {
			log.Printf(ErrorSendMessage, sendErr)
		}
		return
	}
	
	respBody, err := apiCall("POST", "/api/expenses", expense)
	if err != nil {
		reply := tgbotapi.NewMessage(msg.Chat.ID, "❌ Error saving expense: "+err.Error())
		if _, sendErr := bot.Send(reply); sendErr != nil {
			log.Printf(ErrorSendMessage, sendErr)
		}
		return
	}
	
	var resp struct {
		Message string `json:"message"`
		ID      string `json:"id"`
	}
	
	if err := json.Unmarshal(respBody, &resp); err == nil && resp.Message != "" {
		reply := tgbotapi.NewMessage(msg.Chat.ID, "✅ "+resp.Message)
		if _, err := bot.Send(reply); err != nil {
			log.Printf(ErrorSendSuccess, err)
		}
	} else {
		reply := tgbotapi.NewMessage(msg.Chat.ID, fmt.Sprintf("✅ Expense saved: $%.2f for %s", amount, description))
		if _, err := bot.Send(reply); err != nil {
			log.Printf(ErrorSendSuccess, err)
		}
	}
}

func handleUnknownCommand(msg *tgbotapi.Message) {
	response := "I don't understand that command. Type /help for available commands."
	reply := tgbotapi.NewMessage(msg.Chat.ID, response)
	if _, err := bot.Send(reply); err != nil {
		log.Printf("Failed to send unknown command message: %v", err)
	}
}

func parseExpenseText(text string) (float64, string, error) {
	// Remove currency symbols and find amount
	text = strings.ReplaceAll(text, "$", " ")
	text = strings.ReplaceAll(text, "€", " ")
	text = strings.ReplaceAll(text, "£", " ")
	
	parts := strings.Fields(text)
	if len(parts) < 2 {
		return 0, "", fmt.Errorf("invalid format")
	}
	
	var amount float64
	var description string
	var err error
	
	// Try to find the amount in the first few parts
	for i, part := range parts {
		if amount, err = strconv.ParseFloat(part, 64); err == nil {
			// Found amount, rest is description
			remaining := parts[i+1:]
			if len(remaining) == 0 {
				return 0, "", fmt.Errorf("missing description")
			}
			description = strings.Join(remaining, " ")
			break
		}
	}
	
	if amount <= 0 {
		return 0, "", fmt.Errorf("invalid amount")
	}
	
	return amount, description, nil
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

// loadConfig loads configuration from environment variables
func loadConfig() SpendWiseConfig {
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
	
	return SpendWiseConfig{
		BotToken:   botToken,
		AllowedIDs: allowedIDs,
		APIUrl:     apiUrl,
		BotUrl:     botUrl,
		APISecret:  apiSecret,
		Port:       port,
	}
}

// apiCall makes HTTP requests to the SpendWise API
func apiCall(method, endpoint string, body interface{}) ([]byte, error) {
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
		return nil, fmt.Errorf("API error (%d): %s", resp.StatusCode, string(respBody))
	}
	
	return respBody, nil
}

