package handlers

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
	"whereismychildren/utils"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

func (h *BotHandler) handleAddExcel(chatID int64, document *tgbotapi.Document) {
	// Проверка прав
	if !h.isAdmin(chatID) {
		h.sendError(chatID, "❌ У вас нет прав для выполнения этой команды")
		return
	}

	if document == nil {
		h.sendError(chatID, "Прикрепите Excel файл к команде /add_excel")
		return
	}

	// Проверяем, что файл Excel
	if !utils.IsExcelFile(document.FileName) {
		h.sendError(chatID, "❌ Файл должен быть в формате Excel (.xlsx или .xls)")
		return
	}

	// Получаем прямую ссылку на файл
	fileURL, err := h.bot.GetFileDirectURL(document.FileID)
	if err != nil {
		h.sendError(chatID, "❌ Ошибка получения файла: "+err.Error())
		return
	}

	// Создаем временный файл
	tmpDir := os.TempDir()
	tmpFile := filepath.Join(tmpDir, fmt.Sprintf("excel_%s.xlsx", document.FileID))
	defer os.Remove(tmpFile)

	// Скачиваем файл
	msg := tgbotapi.NewMessage(chatID, "📥 Скачиваю файл...")
	h.bot.Send(msg)

	if err := utils.DownloadFile(fileURL, tmpFile); err != nil {
		h.sendError(chatID, "❌ Ошибка скачивания файла: "+err.Error())
		return
	}

	// Обрабатываем Excel файл
	msg = tgbotapi.NewMessage(chatID, "📊 Обрабатываю данные...")
	h.bot.Send(msg)

	newSubordinates, err := h.excelProcessor.ProcessExcelFile(tmpFile)
	if err != nil {
		h.sendError(chatID, "❌ Ошибка обработки Excel: "+err.Error())
		return
	}

	// Формируем ответ
	if len(newSubordinates) == 0 {
		msg = tgbotapi.NewMessage(chatID, "✅ Новых подчиненных не найдено. Все уже добавлены.")
	} else {
		message := fmt.Sprintf("✅ Добавлено %d новых подчиненных:\n\n", len(newSubordinates))
		for i, sub := range newSubordinates {
			message += fmt.Sprintf("%d. %s %s %s\n", i+1, sub.LastName, sub.FirstName, sub.MiddleName)

			// Ограничиваем длину сообщения
			if len(message) > 3500 && i < len(newSubordinates)-1 {
				message += "\n... и другие"
				break
			}
		}

		msg = tgbotapi.NewMessage(chatID, message)
	}

	h.bot.Send(msg)

	// Показываем общий список
	h.showAllSubordinates(chatID)
}

func (h *BotHandler) showAllSubordinates(chatID int64) {
	subordinates, err := h.db.GetAllSubordinates()
	if err != nil {
		h.sendError(chatID, "❌ Ошибка получения списка подчиненных: "+err.Error())
		return
	}

	message := fmt.Sprintf("📋 Всего подчиненных: %d\n\n", len(subordinates))
	for i, sub := range subordinates {
		message += fmt.Sprintf("%d. %s %s %s\n", i+1, sub.LastName, sub.FirstName, sub.MiddleName)
		
		// Ограничиваем длину сообщения
		if len(message) > 3500 && i < len(subordinates)-1 {
			message += "\n... и другие"
			break
		}
	}

	msg := tgbotapi.NewMessage(chatID, message)
	h.bot.Send(msg)
}

func (h *BotHandler) handleStatistics(chatID int64) {
	// Реализация статистики
}
func (h *BotHandler) handleExcelExport(chatID int64) {
	// Проверка прав
	if !h.isAdmin(chatID) {
		h.sendError(chatID, "❌ У вас нет прав для выполнения этой команды")
		return
	}
	
	// Получаем все данные
	data, err := h.db.GetAllDataForExport()
	if err != nil {
		h.sendError(chatID, "Ошибка получения данных: "+err.Error())
		return
	}

	// Создаем Excel файл
	filepath, err := h.excelProcessor.ExportToExcel(data)
	if err != nil {
		h.sendError(chatID, "Ошибка создания Excel: "+err.Error())
		return
	}
	defer os.Remove(filepath) // Удаляем временный файл

	// Отправляем файл
	doc := tgbotapi.NewDocument(chatID, tgbotapi.FilePath(filepath))
	doc.Caption = "📊 Полная статистика"
	
	if _, err := h.bot.Send(doc); err != nil {
		h.sendError(chatID, "Ошибка отправки файла: "+err.Error())
	}
}
func (h *BotHandler) handleLeaveTimeInput(chatID int64, text string) {
	// Реализация ввода времени для ухода
	delete(h.userStates, chatID)

	userData, exists := h.userData[chatID]
	if !exists {
		h.sendError(chatID, "Данные сессии устарели")
		return
	}

	leaveTime, err := utils.ParseTime(text)
	if err != nil {
		h.sendError(chatID, "Неверный формат времени: "+err.Error())
		return
	}

	subordinateID := userData["subordinate_id"].(int)
	h.recordLeave(chatID, subordinateID, leaveTime)
	delete(h.userData, chatID)
}
func (h *BotHandler) handleUnplannedDetailsInput(chatID int64, text string) {
	// Реализация ввода описания для внеплановой деятельности
	delete(h.userStates, chatID)

	userData, exists := h.userData[chatID]
	if !exists {
		h.sendError(chatID, "Данные сессии устарели")
		return
	}

	if strings.TrimSpace(text) == "" {
		h.sendError(chatID, "Описание не может быть пустым")
		return
	}

	subordinateID := userData["subordinate_id"].(int)
	activityTime := userData["activity_time"].(time.Time)

	h.recordUnplannedActivity(chatID, subordinateID, activityTime, text)
	delete(h.userData, chatID)
}
