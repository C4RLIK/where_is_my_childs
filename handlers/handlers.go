package handlers

import (
	"fmt"
	"log"
	"regexp"
	"strconv"
	"strings"
	"time"

	"whereismychildren/config"
	"whereismychildren/database"
	"whereismychildren/excel"
	"whereismychildren/utils"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

type BotHandler struct {
	bot            *tgbotapi.BotAPI
	db             *database.DB
	excelProcessor *excel.ExcelProcessor
	userStates     map[int64]string
	userData       map[int64]map[string]interface{}
	config         *config.Config
}

func NewBotHandler(bot *tgbotapi.BotAPI, db *database.DB, cfg *config.Config) *BotHandler {
	return &BotHandler{
		bot:            bot,
		db:             db,
		excelProcessor: excel.NewExcelProcessor(db),
		userStates:     make(map[int64]string),
		userData:       make(map[int64]map[string]interface{}),
		config:         cfg,
	}
}

func (h *BotHandler) HandleMessage(update tgbotapi.Update) {
	if update.Message == nil {
		return
	}

	chatID := update.Message.Chat.ID
	text := update.Message.Text

	// Проверяем, есть ли документ с командой /add_excel
	if update.Message.Document != nil && update.Message.Caption != "" {
		if strings.HasPrefix(update.Message.Caption, "/add_excel") {
			if !h.isAdmin(chatID) {
				h.sendError(chatID, "❌ У вас нет прав для выполнения этой команды")
				return
			}
			h.handleAddExcel(chatID, update.Message.Document)
			return
		}
	}

	// Обрабатываем текстовые команды
	switch {
	case text == "/start":
		h.handleStart(chatID)
	case text == "Зафиксировать уход":
		h.handleRecordLeave(chatID)
	case text == "Внеплановая деятельность":
		h.handleUnplannedActivity(chatID) // ← обновленный вызов
	case text == "Где подчинённые":
		h.handleWhereSubordinates(chatID)
	case text == "Статистика":
		h.handleStatisticsMenu(chatID)
	case strings.HasPrefix(text, "/stat"):
		h.handleStatisticsCommand(chatID, text)
	case strings.HasPrefix(text, "/add_excel"):
		h.sendError(chatID, "❌ Прикрепите Excel файл к команде /add_excel")
	case h.userStates[chatID] == "waiting_activity_desc_input":
		h.processActivityDescriptionInput(chatID, text) // ← новый обработчик
	case h.userStates[chatID] == "waiting_leave_input":
		h.processLeaveInput(chatID, text)
	case h.userStates[chatID] == "waiting_unplanned_input":
		h.processUnplannedInput(chatID, text)
	case h.userStates[chatID] == "waiting_leave_time":
		h.handleLeaveTimeInput(chatID, text)
	case h.userStates[chatID] == "waiting_unplanned_details":
		h.handleUnplannedDetailsInput(chatID, text)
	default:
		h.handleFreeTextInput(chatID, text)
	}
}

func (h *BotHandler) HandleCallback(update tgbotapi.Update) {
	callback := update.CallbackQuery
	data := callback.Data
	chatID := callback.Message.Chat.ID

	switch {
	case strings.HasPrefix(data, "select_sub_"):
		subID, _ := strconv.Atoi(strings.TrimPrefix(data, "select_sub_"))
		h.handleSubordinateSelection(chatID, subID, callback.Message.MessageID)
	case data == "confirm_yes" || data == "confirm_no":
		h.handleConfirmation(chatID, data == "confirm_yes", callback.Message.MessageID)
	}
}

func (h *BotHandler) handleStart(chatID int64) {
	msg := tgbotapi.NewMessage(chatID, "Добро пожаловать! Используйте кнопки ниже для управления.")
	msg.ReplyMarkup = GetMainKeyboard()
	h.bot.Send(msg)
}

func (h *BotHandler) handleRecordLeave(chatID int64) {
	// Получаем всех подчиненных
	subordinates, err := h.db.GetAllSubordinates()
	if err != nil {
		h.sendError(chatID, "❌ Ошибка получения списка подчиненных: "+err.Error())
		return
	}

	if len(subordinates) == 0 {
		h.sendError(chatID, "❌ Нет добавленных подчиненных. Сначала добавьте их через Excel.")
		return
	}

	// Сортируем по алфавиту для удобства
	h.sortSubordinatesAlphabetically(subordinates)

	// Сохраняем состояние
	h.userStates[chatID] = "waiting_leave_selection"
	h.userData[chatID] = map[string]interface{}{
		"action":   "record_leave",
		"sub_list": subordinates,
	}

	// Отправляем сообщение с клавиатурой для выбора
	msg := tgbotapi.NewMessage(chatID, "👥 Выберите подчиненного, который уходит:")
	msg.ReplyMarkup = CreateSubordinateSelectionKeyboard(subordinates)
	h.bot.Send(msg)
}

func (h *BotHandler) handleUnplannedActivity(chatID int64) {
	// Получаем всех подчиненных
	subordinates, err := h.db.GetAllSubordinates()
	if err != nil {
		h.sendError(chatID, "❌ Ошибка получения списка подчиненных: "+err.Error())
		return
	}

	if len(subordinates) == 0 {
		h.sendError(chatID, "❌ Нет добавленных подчиненных. Сначала добавьте их через Excel.")
		return
	}

	// Сортируем по алфавиту для удобства
	h.sortSubordinatesAlphabetically(subordinates)

	// Сохраняем состояние для ввода описания после выбора сотрудника
	h.userStates[chatID] = "waiting_activity_description"
	h.userData[chatID] = map[string]interface{}{
		"action":   "unplanned_activity",
		"sub_list": subordinates,
	}

	// Отправляем сообщение с клавиатурой для выбора сотрудника
	msg := tgbotapi.NewMessage(chatID, "👥 Выберите сотрудника для внеплановой деятельности:")
	msg.ReplyMarkup = CreateSubordinateSelectionKeyboard(subordinates)
	h.bot.Send(msg)
}

func (h *BotHandler) handleWhereSubordinates(chatID int64) {
	subordinates, err := h.db.GetAllSubordinates()
	if err != nil {
		h.sendError(chatID, "❌ Ошибка получения данных: "+err.Error())
		return
	}

	// Сортируем по алфавиту (фамилия, имя)
	h.sortSubordinatesAlphabetically(subordinates)

	leaves, err := h.db.GetTodayLeaves()
	if err != nil {
		log.Printf("Error getting today leaves: %v", err)
		leaves = make(map[int]time.Time)
	}

	activities, err := h.db.GetTodayUnplannedActivities()
	if err != nil {
		log.Printf("Error getting today activities: %v", err)
		activities = make(map[int]database.UnplannedActivity)
	}

	// Логируем для отладки
	log.Printf("Total subordinates: %d", len(subordinates))
	log.Printf("Leaves found: %d", len(leaves))
	log.Printf("Activities found: %d", len(activities))

	for id, leave := range leaves {
		log.Printf("Leave - Subordinate ID: %d, Time: %s", id, leave.Format("15:04"))
	}

	message := "📊 **Статус подчиненных на сегодня:**\n\n"

	for _, sub := range subordinates {
		status := "📍 На месте"

		// СНАЧАЛА проверяем внеплановую деятельность (высший приоритет)
		if activity, exists := activities[sub.ID]; exists {
			// Обрезаем длинное описание
			shortDescription := activity.Description
			if len(shortDescription) > 50 {
				shortDescription = shortDescription[:47] + "..."
			}
			status = fmt.Sprintf("📋 Внеплановая (%s) - %s",
				activity.ActivityTime.Format("15:04"),
				shortDescription)
		} else if leaveTime, exists := leaves[sub.ID]; exists {
			// ЗАТЕМ проверяем уход (низший приоритет)
			status = fmt.Sprintf("🚪 Ушел в %s", leaveTime.Format("15:04"))
		}

		message += fmt.Sprintf("**%s %s %s** - %s\n",
			sub.LastName, sub.FirstName, sub.MiddleName, status)
	}

	// Добавляем статистику (исправляем подсчет)
	total := len(subordinates)

	// Учитываем, что один сотрудник может быть и в уходах и в деятельности
	// Но при отображении мы показываем только деятельность (высший приоритет)
	uniqueLeftCount := 0
	uniqueActivityCount := 0

	for _, sub := range subordinates {
		if _, exists := activities[sub.ID]; exists {
			uniqueActivityCount++
		} else if _, exists := leaves[sub.ID]; exists {
			uniqueLeftCount++
		}
	}

	presentCount := total - uniqueLeftCount - uniqueActivityCount

	message += fmt.Sprintf("\n📈 **Статистика:** Всего: %d, На месте: %d, Ушли: %d, Внеплановая: %d",
		total, presentCount, uniqueLeftCount, uniqueActivityCount)

	msg := tgbotapi.NewMessage(chatID, message)
	msg.ParseMode = "Markdown"
	h.bot.Send(msg)
}

// Сортировка подчиненных по алфавиту
func (h *BotHandler) sortSubordinatesAlphabetically(subordinates []database.Subordinate) {
	for i := 0; i < len(subordinates)-1; i++ {
		for j := i + 1; j < len(subordinates); j++ {
			// Сравниваем по фамилии, затем по имени
			if subordinates[i].LastName > subordinates[j].LastName ||
				(subordinates[i].LastName == subordinates[j].LastName &&
					subordinates[i].FirstName > subordinates[j].FirstName) {
				subordinates[i], subordinates[j] = subordinates[j], subordinates[i]
			}
		}
	}
}

func (h *BotHandler) handleStatisticsMenu(chatID int64) {
	msg := tgbotapi.NewMessage(chatID, "Выберите вариант статистики:\n"+
		"/stat сегодня - за сегодня\n"+
		"/stat вчера - за вчера\n"+
		"/stat ДД.ММ.ГГГГ - за конкретную дату\n"+
		"/stat excel - выгрузка в Excel")
	h.bot.Send(msg)
}

func (h *BotHandler) handleStatisticsCommand(chatID int64, text string) {
	parts := strings.Fields(text)
	if len(parts) < 2 {
		h.sendError(chatID, "Укажите период: /stat сегодня|вчера|ДД.ММ.ГГГГ|excel")
		return
	}

	period := parts[1]

	if period == "excel" {
		h.handleExcelExport(chatID)
		return
	}

	var targetDate time.Time
	now := time.Now()

	switch period {
	case "сегодня":
		targetDate = now
	case "вчера":
		targetDate = now.AddDate(0, 0, -1)
	default:
		// Пытаемся распарсить дату
		var err error
		targetDate, err = time.Parse("02.01.2006", period)
		if err != nil {
			targetDate, err = time.Parse("02.01.06", period)
			if err != nil {
				h.sendError(chatID, "Неверный формат даты. Используйте ДД.ММ.ГГГГ")
				return
			}
		}
	}

	h.showStatisticsForDate(chatID, targetDate)
}

func (h *BotHandler) showStatisticsForDate(chatID int64, date time.Time) {
	leaves, err := h.db.GetLeavesByDate(date)
	if err != nil {
		h.sendError(chatID, "Ошибка получения статистики: "+err.Error())
		return
	}

	message := fmt.Sprintf("📈 **Статистика за %s:**\n\n", date.Format("02.01.2006"))

	if len(leaves) == 0 {
		message += "Нет данных об уходах за этот день."
	} else {
		for _, item := range leaves {
			message += fmt.Sprintf("**%s %s %s** - ушел в %s\n",
				item.Subordinate.LastName, item.Subordinate.FirstName, item.Subordinate.MiddleName,
				item.LeaveTime.Format("15:04"))
		}
	}

	msg := tgbotapi.NewMessage(chatID, message)
	msg.ParseMode = "Markdown"
	h.bot.Send(msg)
}

func (h *BotHandler) processLeaveInput(chatID int64, text string) {
	// Убираем состояние
	delete(h.userStates, chatID)

	// Проверяем наличие "сейчас" или времени
	hasTime := utils.ContainsTime(text)
	hasNow := strings.Contains(strings.ToLower(text), "сейчас")

	if !hasTime && !hasNow {
		h.sendError(chatID, "❌ Неверный формат. Используйте 'сейчас' или время в формате ЧЧ:MM")
		return
	}

	// Определяем время ухода
	var leaveTime time.Time
	var err error

	if hasNow {
		leaveTime = time.Now()
		// Убираем "сейчас" из текста для поиска имени, НЕ меняя регистр фамилии
		text = strings.ReplaceAll(text, "сейчас", "")
		text = strings.ReplaceAll(text, "Сейчас", "")
		text = strings.ReplaceAll(text, "СЕЙЧАС", "")
	} else {
		leaveTime, err = utils.ParseTime(text)
		if err != nil {
			h.sendError(chatID, "❌ Ошибка парсинга времени: "+err.Error())
			return
		}
		// Убираем время из текста для поиска имени
		timeRegex := regexp.MustCompile(`\d{1,2}:\d{2}`)
		text = timeRegex.ReplaceAllString(text, "")
	}

	// Очищаем и проверяем текст
	cleanText := strings.TrimSpace(text)
	if cleanText == "" {
		h.sendError(chatID, "❌ Укажите фамилию сотрудника")
		return
	}

	log.Printf("Processing leave: '%s' at %s", cleanText, leaveTime.Format("15:04"))

	// Ищем сотрудника (одинаковая логика для "сейчас" и времени)
	subordinates, err := h.findExactSubordinate(cleanText, "")
	if err != nil {
		h.sendError(chatID, "❌ Ошибка поиска подчиненных: "+err.Error())
		return
	}

	if len(subordinates) == 0 {
		h.sendError(chatID, "❌ Сотрудник не найден")
		return
	}

	if len(subordinates) == 1 {
		// Если один подчиненный - сразу фиксируем
		h.recordLeave(chatID, subordinates[0].ID, leaveTime)
	} else {
		// Если несколько - сохраняем время и предлагаем выбрать
		action := "leave_time"
		if hasNow {
			action = "leave_now"
		}

		h.userData[chatID] = map[string]interface{}{
			"action":     action,
			"sub_list":   subordinates,
			"leave_time": leaveTime,
		}
		msg := tgbotapi.NewMessage(chatID, "Найдено несколько сотрудников. Выберите нужного:")
		msg.ReplyMarkup = CreateSubordinateSelectionKeyboard(subordinates)
		h.bot.Send(msg)
	}
}

func (h *BotHandler) processLeaveNow(chatID int64, text string) {
	// Убираем "сейчас" из текста, НЕ переводя весь текст в нижний регистр
	cleanText := strings.ReplaceAll(text, "сейчас", "")
	cleanText = strings.ReplaceAll(cleanText, "Сейчас", "") // на случай заглавной
	cleanText = strings.TrimSpace(cleanText)

	if cleanText == "" {
		h.sendError(chatID, "❌ Укажите фамилию или имя сотрудника")
		return
	}

	log.Printf("Searching for subordinate with: '%s'", cleanText)

	// Ищем точное совпадение (регистр уже правильный)
	subordinates, err := h.findExactSubordinate(cleanText, "")
	if err != nil {
		h.sendError(chatID, "❌ Ошибка поиска подчиненных: "+err.Error())
		return
	}

	if len(subordinates) == 0 {
		// Если не нашли, пробуем поискать по частичному совпадению
		subordinates, err = h.db.FindSubordinatesByName(cleanText, cleanText)
		if err != nil {
			h.sendError(chatID, "❌ Ошибка поиска подчиненных: "+err.Error())
			return
		}
	}

	if len(subordinates) == 0 {
		h.sendError(chatID, "❌ Сотрудник не найден")
		return
	}

	if len(subordinates) == 1 {
		// Если один подчиненный - сразу фиксируем
		h.recordLeave(chatID, subordinates[0].ID, time.Now())
	} else {
		// Если несколько - предлагаем выбрать
		h.userData[chatID] = map[string]interface{}{
			"action":   "leave_now",
			"sub_list": subordinates,
		}
		msg := tgbotapi.NewMessage(chatID, "Найдено несколько сотрудников. Выберите нужного:")
		msg.ReplyMarkup = CreateSubordinateSelectionKeyboard(subordinates)
		h.bot.Send(msg)
	}
}
func (h *BotHandler) findExactSubordinate(searchTerm1, searchTerm2 string) ([]database.Subordinate, error) {
	// Если оба термина пустые
	if searchTerm1 == "" && searchTerm2 == "" {
		return nil, fmt.Errorf("не указаны данные для поиска")
	}

	// Если только один термин - используем улучшенный поиск
	if searchTerm2 == "" {
		return h.db.FindSubordinatesBySingleTerm(searchTerm1)
	}

	// Если два термина - первый считаем фамилией, второй именем
	return h.db.FindSubordinatesByFullName(searchTerm1, searchTerm2)
}

func (h *BotHandler) processLeaveWithTime(chatID int64, text string) {
	// Парсим время
	leaveTime, err := utils.ParseTime(text)
	if err != nil {
		h.sendError(chatID, "❌ Ошибка парсинга времени: "+err.Error())
		return
	}

	// Извлекаем имя (убираем время из текста)
	timeRegex := regexp.MustCompile(`\d{1,2}:\d{2}`)
	cleanText := timeRegex.ReplaceAllString(text, "")
	cleanText = strings.TrimSpace(cleanText)

	if cleanText == "" {
		h.sendError(chatID, "❌ Укажите фамилию сотрудника")
		return
	}

	// Ищем точное совпадение
	subordinates, err := h.findExactSubordinate(cleanText, "")
	if err != nil {
		h.sendError(chatID, "❌ Ошибка поиска подчиненных: "+err.Error())
		return
	}

	if len(subordinates) == 0 {
		h.sendError(chatID, "❌ Сотрудник не найден")
		return
	}

	if len(subordinates) == 1 {
		// Если один подчиненный - сразу фиксируем
		h.recordLeave(chatID, subordinates[0].ID, leaveTime)
	} else {
		// Если несколько - сохраняем время и предлагаем выбрать
		h.userData[chatID] = map[string]interface{}{
			"action":     "leave_time",
			"sub_list":   subordinates,
			"leave_time": leaveTime,
		}
		msg := tgbotapi.NewMessage(chatID, "Найдено несколько сотрудников. Выберите нужного:")
		msg.ReplyMarkup = CreateSubordinateSelectionKeyboard(subordinates)
		h.bot.Send(msg)
	}
}

func (h *BotHandler) processUnplannedInput(chatID int64, text string) {
	// Убираем состояние
	delete(h.userStates, chatID)

	// Проверяем наличие времени или "сейчас"
	hasTime := utils.ContainsTime(text)
	hasNow := strings.Contains(strings.ToLower(text), "сейчас")

	if !hasTime && !hasNow {
		h.sendError(chatID, "Укажите время в формате ЧЧ:MM или 'сейчас'")
		return
	}

	// Определяем время деятельности
	var activityTime time.Time
	var err error
	var searchText string

	if hasNow {
		activityTime = time.Now()
		// Находим позицию "сейчас" в тексте
		nowIndex := strings.Index(strings.ToLower(text), "сейчас")
		if nowIndex == -1 {
			h.sendError(chatID, "Ошибка обработки команды")
			return
		}
		// Берем текст после "сейчас" как описание
		descriptionStart := nowIndex + len("сейчас")
		searchText = strings.TrimSpace(text[:nowIndex])           // фамилия до "сейчас"
		description := strings.TrimSpace(text[descriptionStart:]) // описание после "сейчас"

		if searchText == "" {
			h.sendError(chatID, "Укажите фамилию сотрудника")
			return
		}

		if description == "" {
			h.sendError(chatID, "Укажите описание деятельности")
			return
		}

		// Проверяем длину описания
		if len(description) > 1000 {
			description = description[:1000]
		}

		// Ищем подчиненных и фиксируем деятельность
		h.processUnplannedActivity(chatID, searchText, activityTime, description)
		return

	} else {
		// Обработка с указанием времени
		activityTime, err = utils.ParseTime(text)
		if err != nil {
			h.sendError(chatID, "Ошибка парсинга времени: "+err.Error())
			return
		}

		// Находим время в тексте
		timeRegex := regexp.MustCompile(`\d{1,2}:\d{2}`)
		timeMatch := timeRegex.FindStringIndex(text)
		if timeMatch == nil {
			h.sendError(chatID, "Не удалось найти время в тексте")
			return
		}

		// Берем текст до времени как фамилия, после времени как описание
		searchText = strings.TrimSpace(text[:timeMatch[0]])   // фамилия до времени
		description := strings.TrimSpace(text[timeMatch[1]:]) // описание после времени

		if searchText == "" {
			h.sendError(chatID, "Укажите фамилию сотрудника")
			return
		}

		if description == "" {
			h.sendError(chatID, "Укажите описание деятельности")
			return
		}

		// Проверяем длину описания
		if len(description) > 1000 {
			description = description[:1000]
		}

		// Ищем подчиненных и фиксируем деятельность
		h.processUnplannedActivity(chatID, searchText, activityTime, description)
		return
	}
}
func (h *BotHandler) processUnplannedActivity(chatID int64, searchText string, activityTime time.Time, description string) {
	// Извлекаем только фамилию (первое слово)
	parts := strings.Fields(searchText)
	if len(parts) == 0 {
		h.sendError(chatID, "Укажите фамилию сотрудника")
		return
	}

	lastName := parts[0]

	// Ищем подчиненных по ТОЧНОЙ фамилии
	subordinates, err := h.findExactSubordinate(lastName, "")
	if err != nil {
		h.sendError(chatID, "Ошибка поиска подчиненных: "+err.Error())
		return
	}

	if len(subordinates) == 0 {
		// Если не нашли по точному совпадению, пробуем частичный поиск
		subordinates, err = h.db.FindSubordinatesByName(lastName, "")
		if err != nil {
			h.sendError(chatID, "Ошибка поиска подчиненных: "+err.Error())
			return
		}
	}

	if len(subordinates) == 0 {
		h.sendError(chatID, "Сотрудник не найден")
		return
	}

	if len(subordinates) == 1 {
		// Если один подчиненный - сразу фиксируем
		h.recordUnplannedActivity(chatID, subordinates[0].ID, activityTime, description)
	} else {
		// Если несколько - сохраняем данные и предлагаем выбрать
		h.userData[chatID] = map[string]interface{}{
			"action":        "unplanned",
			"sub_list":      subordinates,
			"activity_time": activityTime,
			"description":   description,
		}
		msg := tgbotapi.NewMessage(chatID, "Найдено несколько сотрудников. Выберите нужного:")
		msg.ReplyMarkup = CreateSubordinateSelectionKeyboard(subordinates)
		h.bot.Send(msg)
	}
}
func (h *BotHandler) recordLeave(chatID int64, subordinateID int, leaveTime time.Time) {
	log.Printf("Recording leave for subordinate %d at %s", subordinateID, leaveTime.Format("15:04"))

	// УДАЛЯЕМ КОНФЛИКТУЮЩИЕ ЗАПИСИ
	err := h.db.RemoveConflictingRecords(subordinateID, leaveTime)
	if err != nil {
		log.Printf("Error removing conflicting records: %v", err)
		h.sendError(chatID, "❌ Ошибка обработки записи: "+err.Error())
		return
	}

	// Добавляем новую запись ухода
	err = h.db.AddLeave(subordinateID, leaveTime)
	if err != nil {
		log.Printf("Error adding leave: %v", err)
		h.sendError(chatID, "❌ Ошибка записи ухода: "+err.Error())
		return
	}

	sub, _ := h.db.GetSubordinateByID(subordinateID)
	msg := tgbotapi.NewMessage(chatID, fmt.Sprintf(
		"✅ %s %s ушёл в %s (МСК)",
		sub.LastName, sub.FirstName, leaveTime.Format("15:04")))
	h.bot.Send(msg)
}

func (h *BotHandler) recordUnplannedActivity(chatID int64, subordinateID int, activityTime time.Time, description string) {
	// УДАЛЯЕМ КОНФЛИКТУЮЩИЕ ЗАПИСИ
	err := h.db.RemoveConflictingRecords(subordinateID, activityTime)
	if err != nil {
		h.sendError(chatID, "Ошибка обработки записи: "+err.Error())
		return
	}

	// Добавляем новую запись деятельности
	err = h.db.AddUnplannedActivity(subordinateID, activityTime, description)
	if err != nil {
		h.sendError(chatID, "Ошибка записи деятельности: "+err.Error())
		return
	}

	sub, _ := h.db.GetSubordinateByID(subordinateID)
	msg := tgbotapi.NewMessage(chatID, fmt.Sprintf(
		"✅ Для %s %s зафиксирована деятельность в %s: %s",
		sub.LastName, sub.FirstName, activityTime.Format("15:04"), description))
	h.bot.Send(msg)
}

func (h *BotHandler) getExistingLeaveToday(subordinateID int) (time.Time, error) {
	leaves, err := h.db.GetTodayLeaves()
	if err != nil {
		return time.Time{}, err
	}

	if leaveTime, exists := leaves[subordinateID]; exists {
		return leaveTime, nil
	}

	return time.Time{}, nil
}

func (h *BotHandler) getExistingActivityToday(subordinateID int) (*database.UnplannedActivity, error) {
	activities, err := h.db.GetTodayUnplannedActivities()
	if err != nil {
		return nil, err
	}

	if activity, exists := activities[subordinateID]; exists {
		return &activity, nil
	}

	return nil, nil
}

func (h *BotHandler) handleSubordinateSelection(chatID int64, subID int, messageID int) {
	// Удаляем сообщение с кнопками
	deleteMsg := tgbotapi.NewDeleteMessage(chatID, messageID)
	h.bot.Send(deleteMsg)

	// Проверяем состояние пользователя
	userState, exists := h.userStates[chatID]
	// Убираем лишнюю переменную userData, так как она не используется в этом коде
	_, dataExists := h.userData[chatID]

	if !exists || !dataExists {
		h.sendError(chatID, "❌ Данные сессии устарели")
		return
	}

	// Получаем московское время
	mskTime := utils.GetMoscowTime()

	switch userState {
	case "waiting_leave_selection":
		// Для ухода - сразу фиксируем
		h.recordLeave(chatID, subID, mskTime)
		delete(h.userStates, chatID)
		delete(h.userData, chatID)

	case "waiting_activity_description":
		// Для внеплановой деятельности - запрашиваем описание
		h.userData[chatID] = map[string]interface{}{
			"subordinate_id": subID,
			"activity_time":  mskTime,
		}
		h.userStates[chatID] = "waiting_activity_desc_input"

		msg := tgbotapi.NewMessage(chatID, "📝 Введите описание внеплановой деятельности:")
		h.bot.Send(msg)

	default:
		h.sendError(chatID, "❌ Неизвестное состояние")
		delete(h.userStates, chatID)
		delete(h.userData, chatID)
	}
}

func (h *BotHandler) handleConfirmation(chatID int64, confirmed bool, messageID int) {
	// Удаляем сообщение с кнопками
	deleteMsg := tgbotapi.NewDeleteMessage(chatID, messageID)
	h.bot.Send(deleteMsg)

	if !confirmed {
		msg := tgbotapi.NewMessage(chatID, "❌ Действие отменено")
		h.bot.Send(msg)
		delete(h.userData, chatID)
		return
	}

	userData, exists := h.userData[chatID]
	if !exists {
		h.sendError(chatID, "Данные сессии устарели")
		return
	}

	action := userData["action"].(string)
	subordinateID := userData["subordinate_id"].(int)

	switch action {
	case "confirm_leave":
		leaveTime := userData["leave_time"].(time.Time)
		err := h.db.AddLeave(subordinateID, leaveTime)
		if err != nil {
			h.sendError(chatID, "Ошибка обновления ухода: "+err.Error())
			return
		}
		sub, _ := h.db.GetSubordinateByID(subordinateID)
		msg := tgbotapi.NewMessage(chatID, fmt.Sprintf(
			"✅ Уход для %s %s обновлен на %s",
			sub.LastName, sub.FirstName, leaveTime.Format("15:04")))
		h.bot.Send(msg)

	case "confirm_unplanned":
		activityTime := userData["activity_time"].(time.Time)
		description := userData["description"].(string)
		err := h.db.AddUnplannedActivity(subordinateID, activityTime, description)
		if err != nil {
			h.sendError(chatID, "Ошибка обновления деятельности: "+err.Error())
			return
		}
		sub, _ := h.db.GetSubordinateByID(subordinateID)
		msg := tgbotapi.NewMessage(chatID, fmt.Sprintf(
			"✅ Деятельность для %s %s обновлена: %s - %s",
			sub.LastName, sub.FirstName, activityTime.Format("15:04"), description))
		h.bot.Send(msg)
	}

	delete(h.userData, chatID)
}

func (h *BotHandler) handleFreeTextInput(chatID int64, text string) {
	// Автоматическое определение типа команды
	if strings.Contains(text, "сейчас") || utils.ContainsTime(text) {
		h.processLeaveInput(chatID, text)
	} else {
		msg := tgbotapi.NewMessage(chatID, "Не понимаю команду. Используйте кнопки или стандартные форматы.")
		h.bot.Send(msg)
	}
}

func (h *BotHandler) sendError(chatID int64, message string) {
	msg := tgbotapi.NewMessage(chatID, "❌ "+message)
	h.bot.Send(msg)
}

func truncateString(s string, maxLength int) string {
	if len(s) <= maxLength {
		return s
	}
	return s[:maxLength-3] + "..."
}
func (h *BotHandler) isAdmin(chatID int64) bool {
	return h.config.IsAdmin(chatID)
}

func (h *BotHandler) checkAdmin(chatID int64) bool {
	if !h.isAdmin(chatID) {
		h.sendError(chatID, "❌ У вас нет прав для выполнения этой команды")
		return false
	}
	return true
}
func normalizeSearchTerm(term string) string {
	if term == "" {
		return ""
	}

	// Приводим к нижнему регистру
	lower := strings.ToLower(term)

	// Первую букву делаем заглавной
	if len(lower) > 0 {
		return strings.ToUpper(lower[:1]) + lower[1:]
	}

	return lower
}
func (h *BotHandler) processActivityDescriptionInput(chatID int64, text string) {
	// Проверяем состояние
	if h.userStates[chatID] != "waiting_activity_desc_input" {
		h.sendError(chatID, "❌ Неверное состояние")
		return
	}

	userData, exists := h.userData[chatID]
	if !exists {
		h.sendError(chatID, "❌ Данные сессии устарели")
		delete(h.userStates, chatID)
		return
	}

	subordinateID, ok1 := userData["subordinate_id"].(int)
	activityTime, ok2 := userData["activity_time"].(time.Time)

	if !ok1 || !ok2 {
		h.sendError(chatID, "❌ Ошибка данных сессии")
		delete(h.userStates, chatID)
		delete(h.userData, chatID)
		return
	}

	// Проверяем, что описание не пустое
	description := strings.TrimSpace(text)
	if description == "" {
		h.sendError(chatID, "❌ Описание не может быть пустым")
		return
	}

	// Ограничиваем длину описания
	if len(description) > 1000 {
		description = description[:1000]
	}

	// Фиксируем внеплановую деятельность
	h.recordUnplannedActivity(chatID, subordinateID, activityTime, description)

	// Очищаем состояние
	delete(h.userStates, chatID)
	delete(h.userData, chatID)
}
