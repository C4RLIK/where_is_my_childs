package config

import (
	"log"
	"os"
	"strconv"
	"strings"

	"github.com/joho/godotenv"
)

type Config struct {
	TelegramToken string
	DBPath        string
	AdminIDs      []int64
}

func Load() *Config {
	// Загружаем .env файл (если существует)
	if err := godotenv.Load(); err != nil {
		log.Println("No .env file found, using system environment variables")
	}

	adminIDs := parseAdminIDs(os.Getenv("ADMIN_IDS"))

	return &Config{
		TelegramToken: os.Getenv("TELEGRAM_TOKEN"),
		DBPath:        getEnv("DB_PATH", "bot.db"),
		AdminIDs:      adminIDs,
	}
}

func getEnv(key, defaultValue string) string {
	value := os.Getenv(key)
	if value == "" {
		return defaultValue
	}
	return value
}

func parseAdminIDs(adminIDsStr string) []int64 {
	if adminIDsStr == "" {
		return []int64{}
	}

	ids := strings.Split(adminIDsStr, ",")
	var adminIDs []int64

	for _, idStr := range ids {
		id, err := strconv.ParseInt(strings.TrimSpace(idStr), 10, 64)
		if err == nil {
			adminIDs = append(adminIDs, id)
		}
	}

	return adminIDs
}

// IsAdmin проверяет, является ли пользователь администратором
func (c *Config) IsAdmin(userID int64) bool {
	for _, adminID := range c.AdminIDs {
		if userID == adminID {
			return true
		}
	}
	return false
}
