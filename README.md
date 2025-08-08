# 💰 SpendWise Telegram Bot

A powerful Telegram bot for expense tracking and reminder management, built with Go. Track your daily expenses, get spending summaries, and manage reminders through simple chat commands.

## ✨ Features

- 📝 **Quick Expense Logging** - Multiple input formats supported
- 📊 **Daily & Monthly Summaries** - Get detailed spending reports
- 🔔 **Smart Reminders** - Stay on top of your bills and payments
- 🔒 **Secure Authentication** - API secret protection and user access control
- 🎯 **Batch Processing** - Add multiple expenses at once
- 🌍 **Indian Currency Support** - ₹ formatting with proper comma separation

## 🤖 Bot Commands

| Command | Description | Example |
|---------|-------------|---------|
| `/start` | Welcome message and quick help | - |
| `/help` | Detailed help and usage guide | - |
| `/expense` | Get help for expense logging formats | - |
| `/summary` | View today's expense summary | - |
| `/month` | View current month's summary | - |
| `/reminders` | View pending reminders | - |

### 💸 Expense Input Formats

The bot supports multiple flexible formats for logging expenses:

#### Single Expense Formats
```
Coffee 5.50
5.50 Coffee
Lunch at restaurant 25.75
150 Grocery shopping
```

#### Multiple Amounts (Auto-summed)
```
Coffee 5 10 15    // Total: ₹30.00
```

#### Batch Processing (Multiple Lines)
```
Coffee 5.50
Lunch 25.75
Gas bill 150
Bus ticket 8.50
```

## 🚀 Quick Start

### Prerequisites
- Go 1.19 or higher
- Telegram Bot Token (from [@BotFather](https://t.me/botfather))
- SpendWise Backend API

### Installation

1. **Clone the repository**
```bash
git clone https://github.com/gopinathapr/spendwise-telegram-go.git
cd spendwise-telegram-go
```

2. **Install dependencies**
```bash
go mod tidy
```

3. **Configure environment variables**

Create a `.env` file:
```bash
cp .env.example .env
```

Edit `.env` with your configuration:
```env
# Telegram Bot Configuration
BOT_TOKEN=your_telegram_bot_token_here
BOT_URL=https://your-domain.com

# API Configuration
API_URL=http://localhost:3000
API_SECRET=your_api_secret_here

# Server Configuration
PORT=8080

# Access Control - comma-separated chat IDs
ALLOWED_IDS=123456789,987654321

# Username Mappings - chatID:username pairs, comma-separated
USER_NAMES=123456789:john_doe,987654321:jane_smith
```

4. **Get your Telegram Chat ID**
   - Message your bot anything
   - Check the logs - it will show unauthorized attempts with your chat ID
   - Add your chat ID to `ALLOWED_IDS`

5. **Run the bot**
```bash
go run main.go
```

### 🌐 Local Development with ngrok

For testing webhooks locally:
```bash
# Install ngrok: https://ngrok.com/
grok http 8080

# Copy the HTTPS URL and set it as BOT_URL
export BOT_URL="https://abc123.ngrok.io"
```

## 📋 Environment Variables

### Required Variables
- `BOT_TOKEN` - Telegram bot token from BotFather
- `BOT_URL` - Public URL where the bot is hosted (for webhooks)
- `API_SECRET` - Secret for API authentication
- `ALLOWED_IDS` - Comma-separated list of allowed Telegram chat IDs

### Optional Variables
- `API_URL` - Backend API URL (default: `http://localhost:3000`)
- `PORT` - Server port (default: `8080`)
- `USER_NAMES` - Username mappings: `"chatID1:username1,chatID2:username2"`

## 🔌 API Integration

### Expense Creation Endpoint
`POST /api/expenses`

**Request Headers:**
```http
Content-Type: application/json
x-spendwise-secret: your_api_secret_here
```

**Request Body:**
```json
[
  {
    "description": "Groceries from More",
    "amount": 1250,
    "date": "2024-07-29",
    "source": "bot",
    "userName": "Gopi",
    "telegramChatId": "6420106576"
  },
  {
    "description": "Coffee with friends",
    "amount": 400,
    "date": "2024-07-29",
    "source": "bot",
    "userName": "Gopi",
    "telegramChatId": "6420106576"
  }
]
```

**Success Response (200 OK):**
```json
{
  "success": true,
  "message": "2 expenses added successfully."
}
```

**Error Responses:**
```json
// Unauthorized
{
  "error": "Unauthorized."
}

// Invalid payload
{
  "error": "Invalid payload. Expected a non-empty array of expenses."
}

// Server error
{
  "error": "Failed to create expenses.",
  "details": "Specific error message from the server"
}
```

### Summary Endpoints

#### Daily Summary
`GET /api/summary/today`

**Response:**
```json
{
  "markdown": "📊 **Today's Summary - Aug 8, 2025**\n\n💰 **Total Spent:** ₹1,650.00\n\n📝 **Expenses:**\n• Coffee - ₹5.50\n• Lunch - ₹25.75\n• Groceries - ₹1,250.00\n• Bus ticket - ₹8.50"
}
```

#### Monthly Summary
`GET /api/summary/month`

**Response:**
```json
{
  "markdown": "📊 **August 2025 Summary**\n\n💰 **Total Spent:** ₹45,230.00\n\n📈 **Daily Average:** ₹1,507.67\n\n🏆 **Top Categories:**\n• Groceries: ₹15,420.00\n• Transportation: ₹8,950.00\n• Food & Dining: ₹12,200.00"
}
```

### Reminders Endpoint
`GET /api/reminders/get-payload`

**Response:**
```json
{
  "fcmTokens": [],
  "telegramUserIds": [
    "6420106576",
    "7004080768"
  ],
  "reminders": [
    {
      "id": "SKe7V4zOBc3fMDRFUBOQ",
      "activeMonths": [],
      "description": "Power Bill",
      "amount": 850,
      "subType": "Power Bill",
      "paidMonths": [],
      "userId": "41mTq5vTQyczGcE7YxueuSojYrv2",
      "dayOfMonthStart": 6,
      "mainType": "Family",
      "createdAt": {
        "seconds": 1754037196,
        "nanoseconds": 596000000
      },
      "dayOfMonthEnd": 15,
      "type": "standard"
    }
  ]
}
```

### Mark Reminder as Done
`POST /api/reminders/mark-as-done`

**Request:**
```json
{
  "reminderId": "SKe7V4zOBc3fMDRFUBOQ",
  "reminderType": "standard"
}
```

## 📱 Usage Examples

### Adding Expenses
```
User: Coffee 5.50
Bot: ✅ Expense logged successfully!

User: 25.75 Lunch at restaurant
Bot: ✅ Expense logged successfully!

User: Coffee 5 10 15
Bot: ✅ Expense logged successfully!

User: Coffee 5.50
      Lunch 25.75
      Gas bill 150
Bot: ✅ 3 expenses saved successfully
```

### Getting Summaries
```
User: /summary
Bot: 📊 **Today's Summary - Aug 8, 2025**
     💰 **Total Spent:** ₹1,650.00
     ...

User: /month
Bot: 📊 **August 2025 Summary**
     💰 **Total Spent:** ₹45,230.00
     ...
```

### Viewing Reminders
```
User: /reminders
Bot: 🔔 Daily Reminders

     • Power Bill - ₹850.00 (Due between 6-15)
     • Test reminder - ₹1,000.00 (Due Today)

     Please check the app to take action.
```

## 🏗️ Architecture

```
┌─────────────────┐    ┌──────────────────┐    ┌─────────────────┐
│   Telegram Bot  │────│  Go Bot Server   │────│  SpendWise API  │
│   (Webhook)     │    │  (This Project)  │    │   (Backend)     │
└─────────────────┘    └──────────────────┘    └─────────────────┘
```

The bot receives webhook updates from Telegram, processes commands and expense inputs, then communicates with the SpendWise backend API to store data and retrieve summaries.

## 🔒 Security Features

- **API Secret Authentication** - All API requests include `x-spendwise-secret` header
- **User Access Control** - Only allowed chat IDs can use the bot
- **Input Validation** - Expense amounts and formats are validated
- **Error Handling** - Graceful error responses for invalid inputs

## 🛠️ Development

### Project Structure
```
spendwise-telegram-go/
├── main.go              # Main bot application
├── go.mod               # Go modules
├── go.sum               # Dependencies checksum
├── .env.example         # Environment variables template
├── Dockerfile           # Docker configuration
└── README.md            # This file
```

### Building
```bash
# Build binary
go build -o spendwise-bot .

# Run binary
./spendwise-bot
```

### Testing
```bash
# Test compilation
go build -o /dev/null .

# Run with verbose logging
go run main.go
```

## 📄 License

This project is licensed under the MIT License.

## 🤝 Contributing

1. Fork the repository
2. Create a feature branch (`git checkout -b feature/amazing-feature`)
3. Commit your changes (`git commit -m 'Add amazing feature'`)
4. Push to the branch (`git push origin feature/amazing-feature`)
5. Open a Pull Request

## 📞 Support

If you encounter any issues or have questions, please open an issue on GitHub.

---

**Made with ❤️ for better expense tracking**

## Commands

- `/start` - Welcome message
- `/help` - Show help
- `/expense` - Add expense help
- `/reminders` - View reminders
- Quick expense formats:
  - `description amount` (e.g., `Coffee 5.50`)
  - `amount description` (e.g., `5.50 Coffee`)
  - `description multiple amounts` (e.g., `Coffee 5 10 15` = 30 total)
  - Batch: multiple lines supported

## Running Locally

### Prerequisites
1. Install Go (1.19+): https://golang.org/dl/
2. Create a Telegram bot:
   - Message [@BotFather](https://t.me/botfather) on Telegram
   - Use `/newbot` command and follow instructions
   - Save the bot token

### Setup Steps

1. **Clone and install dependencies:**
```bash
cd /Users/goramana/Desktop/PRG/Personal/spendwise-telegram-go
go mod tidy
```

2. **Configure environment variables:**

   **Option A: Using .env file (recommended for local development):**
   ```bash
   cp .env.example .env
   # Edit .env file with your actual values
   ```

   **Option B: Export environment variables:**
   ```bash
   export BOT_TOKEN="your_telegram_bot_token_here"
   export BOT_URL="https://your-domain.com"  # Or use ngrok for local testing
   export API_SECRET="your_secret_key_here"
   export ALLOWED_IDS="123456789,987654321"  # Your Telegram chat IDs
   export API_URL="http://localhost:3000"    # Optional: your backend API
   export PORT="8080"                        # Optional: server port
   export USER_NAMES="123456789:John,987654321:Jane"  # Optional: username mappings
   ```

3. **For local testing with ngrok:**
```bash
# Install ngrok: https://ngrok.com/
ngrok http 8080
# Copy the HTTPS URL and set it as BOT_URL
export BOT_URL="https://abc123.ngrok.io"
```

4. **Get your Telegram Chat ID:**
   - Message your bot anything
   - Check the logs when you run the bot - it will show unauthorized attempts with your chat ID
   - Add your chat ID to ALLOWED_IDS

5. **Run the bot:**
```bash
go run main.go
```

### Testing
- Send `/start` to your bot on Telegram
- Try different expense formats:
  - `Coffee 5.50` (single amount)
  - `5.50 Coffee` (amount first)
  - `Coffee 5 10 15` (multiple amounts = 30 total)
  - Batch with multiple lines
- Use `/help` for all commands
