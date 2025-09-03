package main

import (
	"log"
	"strings"

	"whereismychildren/config"
	"whereismychildren/database"
	"whereismychildren/handlers"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

func main() {
	// Загрузка конфигурации
	cfg := config.Load()
	if cfg.TelegramToken == "" {
		log.Fatal("TELEGRAM_TOKEN is required")
	}

	if len(cfg.AdminIDs) == 0 {
		log.Println("Warning: ADMIN_IDS not set, some commands will be unavailable")
	}

	// Инициализация базы данных
	db, err := database.NewDB(cfg.DBPath)
	if err != nil {
		log.Fatalf("Failed to initialize database: %v", err)
	}
	defer db.Close()

	// Инициализация бота
	bot, err := tgbotapi.NewBotAPI(cfg.TelegramToken)
	if err != nil {
		log.Fatalf("Failed to create bot: %v", err)
	}

	bot.Debug = true
	log.Printf("Authorized on account %s", bot.Self.UserName)

	// Инициализация обработчика с конфигом
	handler := handlers.NewBotHandler(bot, db, cfg)

	// Настройка обновлений
	u := tgbotapi.NewUpdate(0)
	u.Timeout = 60

	updates := bot.GetUpdatesChan(u)

	// Обработка сообщений
	for update := range updates {
		if update.CallbackQuery != nil {
			handler.HandleCallback(update)
			continue
		}

		if update.Message != nil {
			// Проверяем права для административных команд
			if isAdminCommand(update.Message.Text) && !cfg.IsAdmin(update.Message.From.ID) {
				msg := tgbotapi.NewMessage(update.Message.Chat.ID, "❌ У вас нет прав для выполнения этой команды")
				bot.Send(msg)
				continue
			}

			handler.HandleMessage(update)
		}
	}
}

// isAdminCommand проверяет, является ли команда административной
func isAdminCommand(text string) bool {
	if text == "" {
		return false
	}

	adminCommands := []string{
		"/add_excel",
		"/stat excel",
	}

	for _, cmd := range adminCommands {
		if strings.HasPrefix(text, cmd) {
			return true
		}
	}
	return false
}
