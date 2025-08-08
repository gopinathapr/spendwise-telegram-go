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
- `USER_NAMES` - Username mappings: "chatID1:username1,chatID2:username2"

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

2. **Set environment variables:**
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


### Backend API
Request JSON Format: 
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
  },
  {
    "description": "Petrol for car",
    "amount": 2000,
    "date": "2024-07-29",
    "source": "bot",
    "userName": "Gopi",
    "telegramChatId": "6420106576"
  }
]

Structure: The payload is a JSON array [], where each element is an object {} representing a single expense.
description: The text description of the expense.
amount: A numeric value for the expense amount.
date: The date of the transaction in YYYY-MM-DD format.
source: For bot entries, this should always be "bot".
userName: The first name of the user from Telegram, used for display purposes.
telegramChatId: This is the most crucial field. The API uses this ID to look up the corresponding Firebase User ID from your telegram_user_mappings collection, ensuring the expense is logged for the correct user.
When making the API call, remember to include the x-spendwise-secret in the request headers for authorization.


____________
Response:
f the API successfully processes the batch of expenses, it will respond with an HTTP 200 OK status and a JSON body like this:

{
  "success": true,
  "message": "3 expenses added successfully."
}

The message field dynamically reports how many expenses from your payload were logged.

If something goes wrong, the response will indicate the problem.

If the x-spendwise-secret header is missing or incorrect, you will receive:

{
  "error": "Unauthorized."
}

If the JSON payload is not a valid array or is empty, the response will be:

{
  "error": "Invalid payload. Expected a non-empty array of expenses."
}

If an unexpected issue occurs while writing to the database, you'll get a more detailed error:

{
  "error": "Failed to create expenses.",
  "details": "A specific error message from the server would appear here."
}

In short, you can check for the success: true field in the response to confirm that the entire batch was added correctly.