
import express from 'express';
import { Telegraf, Markup } from 'telegraf';
import { formatInTimeZone } from 'date-fns-tz';
import { isSameDay } from 'date-fns';
import type { NotificationPayload, ExpenseInput } from './types';
import fs from 'fs';

const app = express();
app.use(express.json());

const timeZone = 'Asia/Kolkata';

// Define the structure of environment variables from secrets
interface SpendWiseSecrets {
    TELEGRAM_BOT_TOKEN: string;
    TELEGRAM_ALLOWED_USER_IDS: string;
    SPENDWISE_API_URL: string;
    SPENDWISE_BOT_URL: string;
    SPENDWISE_API_SECRET: string;
}

// Define the structure of the config the app will use
interface SpendWiseConfig {
    BOT_TOKEN: string;
    ALLOWED_USER_IDS: string[];
    NEXTJS_API_URL: string;
    API_SECRET: string;
    PORT?: string;
}

// Helper to get config from environment variables
function getConfig(): SpendWiseConfig {
    let secrets: SpendWiseSecrets;
    const secretsPath = process.env.SPENDWISE_SECRETS_PATH;

    try {
        if (secretsPath && fs.existsSync(secretsPath)) {
            const secretsFileContent = fs.readFileSync(secretsPath, 'utf8');
            secrets = JSON.parse(secretsFileContent) as SpendWiseSecrets;
            console.log("Loaded secrets from file path:", secretsPath);
        } else if (process.env.SPENDWISE_SECRETS) {
            secrets = JSON.parse(process.env.SPENDWISE_SECRETS) as SpendWiseSecrets;
            console.log("Loaded secrets from SPENDWISE_SECRETS environment variable.");
        } else {
            console.log("No secrets found, falling back to individual env vars for local dev.");
            secrets = {
                TELEGRAM_BOT_TOKEN: process.env.TELEGRAM_BOT_TOKEN || '',
                TELEGRAM_ALLOWED_USER_IDS: process.env.TELEGRAM_ALLOWED_USER_IDS || '',
                SPENDWISE_API_URL: process.env.SPENDWISE_API_URL || '',
                SPENDWISE_API_SECRET: process.env.SPENDWISE_API_SECRET || '',
                SPENDWISE_BOT_URL: process.env.SPENDWISE_BOT_URL || '',
            };
        }
    } catch (error) {
        console.error("Could not load or parse secrets:", error);
        throw new Error("Invalid secrets configuration.");
    }

    const botTokenToUse = secrets.TELEGRAM_BOT_TOKEN;
    if (!botTokenToUse) {
        throw new Error('TELEGRAM_BOT_TOKEN must be provided in secrets.');
    }

    const config: SpendWiseConfig = {
        BOT_TOKEN: botTokenToUse,
        ALLOWED_USER_IDS: secrets.TELEGRAM_ALLOWED_USER_IDS.split(',').map(id => id.trim()).filter(Boolean),
        NEXTJS_API_URL: secrets.SPENDWISE_API_URL, 
        API_SECRET: secrets.SPENDWISE_API_SECRET,
        PORT: process.env.PORT || '8080'
    };

    if (config.ALLOWED_USER_IDS.length === 0 || !config.NEXTJS_API_URL || !config.API_SECRET) {
        throw new Error('Required configuration (ALLOWED_USER_IDS, SPENDWISE_API_URL, SPENDWISE_API_SECRET) is missing.');
    }

    return config;
}

async function createBot() {
    const config = getConfig();
    const bot = new Telegraf(config.BOT_TOKEN);

    // A reusable helper function for all API calls to the Next.js backend
    async function apiCall<T>(endpoint: string, options: RequestInit = {}): Promise<T> {
        const response = await fetch(`${config.NEXTJS_API_URL}${endpoint}`, {
            ...options,
            headers: {
                'Content-Type': 'application/json',
                'x-spendwise-secret': config.API_SECRET,
                ...options.headers,
            },
        });

        if (!response.ok) {
            let errorBody: { error?: string } | undefined;
            try {
                const jsonResponse = await response.json() as unknown;
                if (typeof jsonResponse === 'object' && jsonResponse !== null) {
                    errorBody = jsonResponse as { error?: string };
                }
            } catch (e) {
                // Ignore JSON parsing errors for non-JSON responses
            }
            const errorMessage = errorBody?.error || `Next.js API responded with status ${response.status}`;
            throw new Error(errorMessage);
        }

        const contentType = response.headers.get("content-type");
        if (contentType && contentType.indexOf("application/json") !== -1) {
            return response.json() as T;
        } else {
            // Handle cases where API returns non-JSON success responses (e.g., just text)
            return response.text() as Promise<T>;
        }
    }

    // --- Authorization Middleware ---
    bot.use((ctx, next) => {
        const chatId = ctx.chat?.id;
        if (!chatId) {
            console.warn("Received update without a chat ID.", ctx);
            return;
        }
        if (config.ALLOWED_USER_IDS.includes(String(chatId))) {
            return next();
        }
        console.warn(`Unauthorized access from chat ID: ${chatId}. Ignoring message.`);
        return;
    });

    // --- Bot Commands & Handlers ---

    bot.start((ctx) => {
        return ctx.reply("Welcome to SpendWise Bot! Use /summary for today's expenses, or log expenses like 'Groceries 50'.");
    });

    bot.command('summary', async (ctx) => {
        const text = ctx.message.text.toLowerCase();
        const isMonthSummary = text.includes('month');
        const endpoint = isMonthSummary ? '/api/summary/month' : '/api/summary/today';

        try {
            const summary = await apiCall<{ markdown: string }>(endpoint);
            return ctx.replyWithMarkdown(summary.markdown);
        } catch (error: unknown) {
            console.error(`Error fetching summary from API:`, error);
            const errorMessage = error instanceof Error ? `Sorry, I couldn't fetch your summary: ${error.message}` : "Sorry, I couldn't fetch your summary. Please try again later.";
            return ctx.reply(errorMessage);
        }
    });

    bot.command('reminders', async (ctx) => {
        try {
            const responseData = await apiCall<NotificationPayload>('/api/reminders/get-payload');

            if (!responseData || !responseData.reminders || responseData.reminders.length === 0) {
                return ctx.reply("No reminders are due today or tomorrow. You're all caught up! 👍");
            }

            const reminders = responseData.reminders;
            const formatCurrency = (amount: number) => new Intl.NumberFormat("en-IN", { style: "currency", currency: "INR" }).format(amount);
            let telegramMessage = `*🔔 Active Reminders Due*\n\n`;

            const todayInIndiaStr = formatInTimeZone(new Date(), timeZone, 'yyyy-MM-dd');

            reminders.forEach(r => {
                let dueDateInfo = '';
                if (r.type === 'credit_card' && r.dueDate) {
                    const isDueToday = isSameDay(new Date(r.dueDate.replace(/-/g, '/')), new Date(todayInIndiaStr.replace(/-/g, '/')));
                    dueDateInfo = ` (Due ${isDueToday ? 'Today' : 'Tomorrow'})`;
                } else if (r.type === 'standard' && r.dayOfMonthStart) {
                    if (r.dayOfMonthStart === r.dayOfMonthEnd) {
                       dueDateInfo = ` (Due Today)`;
                    } else {
                        dueDateInfo = ` (Due between ${r.dayOfMonthStart}-${r.dayOfMonthEnd})`;
                    }
                }
                telegramMessage += `  • ${r.description} - ${formatCurrency(r.amount)}${dueDateInfo}\n`;
            });
            telegramMessage += `\n_Please check the app to take action._`;
            return ctx.replyWithMarkdown(telegramMessage);

        } catch (error: unknown) {
            console.error("Error in /reminders command handler:", error);
            const errorMessage = error instanceof Error ? `Sorry, I couldn't fetch your reminders: ${error.message}` : "Sorry, I couldn't fetch your reminders. Please try again later.";
            return ctx.reply(errorMessage);
        }
    });

    bot.on('callback_query', async (ctx) => {
        const callbackData = (ctx.callbackQuery as any).data;
        if (!callbackData || !callbackData.startsWith('mark_done:')) {
            return ctx.answerCbQuery("Invalid action.");
        }

        const [, reminderId, reminderType] = callbackData.split(':');
        const chatId = String(ctx.chat?.id);

        await ctx.answerCbQuery(`Processing...`);

        try {
            const result = await apiCall<{ error?: string; message?: string }>('/api/reminders/mark-as-done', {
                method: 'POST',
                body: JSON.stringify({ reminderId, reminderType, userId: chatId }),
            });
            await ctx.editMessageText(`✅ ${result.message}`, { parse_mode: 'Markdown' });
        } catch (error: unknown) {
            console.error("Error processing callback query:", error);
            const errorMessage = error instanceof Error ? `❌ Error: ${error.message}` : "Error processing callback query.";
            await ctx.editMessageText(errorMessage);
        }
        return;
    });

    bot.on('text', async (ctx) => {
        const messageText = ctx.message.text.trim();
        if (messageText.startsWith('/')) return;

        const fromFirstName = ctx.message.from.first_name || 'Telegram User';
        const chatId = String(ctx.chat.id);
        const dateStr = formatInTimeZone(new Date(), timeZone, 'yyyy-MM-dd');
        const lines = messageText.split('\n').filter(line => line.trim() !== '');
        const expensesToAdd: ExpenseInput[] = [];
        const failedLines: string[] = [];

        for (const line of lines) {
            const parsed = parseExpenseLine(line);
            if (parsed) {
                expensesToAdd.push({
                    description: parsed.description,
                    amount: parsed.amount,
                    date: dateStr,
                    source: 'bot',
                    userName: fromFirstName,
                    telegramChatId: chatId,
                });
            } else {
                failedLines.push(line);
            }
        }

        if (expensesToAdd.length > 0) {
            try {
                await apiCall<void>('/api/expenses/create-batch-from-bot', {
                    method: 'POST',
                    body: JSON.stringify(expensesToAdd)
                });
                if (lines.length === 1) return ctx.react('👍');
                let reply = `✅ Successfully logged ${expensesToAdd.length} expense(s).\n`;
                if (failedLines.length > 0) {
                    reply += `\n⚠️ Could not process ${failedLines.length} line(s):\n`;
                    failedLines.forEach(line => reply += `  - "${line}"\n`);
                }
                return ctx.reply(reply);
            } catch (error: unknown) {
                console.error("Error logging expense(s) from bot:", error);
                const replyMessage = error instanceof Error ? `Sorry, something went wrong: ${error.message}` : "Sorry, something went wrong while logging your expenses.";
                return ctx.reply(replyMessage);
            }
        }

        const replyMessage = failedLines.length > 0 ? `Sorry, I couldn't understand any of the lines. Please use the format 'Description Amount' for each line.` : "Sorry, I didn't understand that. Please use the format 'Description Amount'.";
        return ctx.reply(replyMessage);
    });

    return { bot, config };
}

function parseExpenseLine(line: string): { description: string, amount: number } | null {
    const text = line.trim();
    if (!text) return null;
    const parts = text.split(' ');
    if (parts.length < 2) return null;

    const potentialAmount = parseFloat(parts[parts.length - 1]);
    if (!isNaN(potentialAmount) && potentialAmount > 0) {
        const description = parts.slice(0, -1).join(' ');
        return { description, amount: potentialAmount };
    }
    return null;
}

// Start the server
async function startServer() {
    try {
        const { bot, config } = await createBot();
        
        // --- Internal API for Cloud Functions to send messages ---
        app.post('/internal/send-message', async (req, res) => {
            const suppliedSecret = req.headers['x-spendwise-secret'];
            if (suppliedSecret !== config.API_SECRET) {
                return res.status(401).json({ error: 'Unauthorized' });
            }

            const { chatId, message, options } = req.body;
            if (!chatId || !message) {
                return res.status(400).json({ error: 'Missing chatId or message' });
            }

            try {
                await bot.telegram.sendMessage(chatId, message, options);
                return res.status(200).json({ success: true });
            } catch (error: any) {
                console.error(`Failed to send message via internal API to ${chatId}:`, error);
                return res.status(500).json({ error: 'Failed to send message', details: error.message });
            }
        });


        app.get('/health', (req, res) => res.json({ status: 'ok', timestamp: new Date().toISOString() }));
        app.post('/webhook', (req, res) => bot.handleUpdate(req.body, res));

        const port = parseInt(config.PORT || '8080');
        app.listen(port, () => {
            console.log(`Server is running on port ${port}`);
            console.log(`Webhook endpoint available at: /webhook`);
            console.log(`Health check available at: /health`);
        });

        process.once('SIGINT', () => { bot.stop('SIGINT'); process.exit(0); });
        process.once('SIGTERM', () => { bot.stop('SIGTERM'); process.exit(0); });
    } catch (error: unknown) {
        console.error('Failed to start server:', error);
        process.exit(1);
    }
}

startServer();
