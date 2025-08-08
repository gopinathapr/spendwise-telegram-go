# SpendWise Telegram Bot

A Telegram bot for expense tracking and reminder management.

## Environment Variables

Required:
- `BOT_TOKEN` - Telegram bot token
- `BOT_URL` - Public URL where the bot is hosted
- `API_SECRET` - Secret for API authentication
- `ALLOWED_IDS` - Comma-separated list of allowed Telegram chat IDs

Optional:
- `API_URL` - Backend API URL (default: http://localhost:3000)
- `PORT` - Server port (default: 8080)

## Commands

- `/start` - Welcome message
- `/help` - Show help
- `/expense` - Add expense help
- `/reminders` - View reminders
- `$amount description` - Quick expense entry

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

2. **Set environment variables:**
```bash
export BOT_TOKEN="your_telegram_bot_token_here"
export BOT_URL="https://your-domain.com"  # Or use ngrok for local testing
export API_SECRET="your_secret_key_here"
export ALLOWED_IDS="123456789,987654321"  # Your Telegram chat IDs
export API_URL="http://localhost:3000"    # Optional: your backend API
export PORT="8080"                        # Optional: server port
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
- Try quick expense: `$5.50 Coffee`
- Use `/help` for all commands
