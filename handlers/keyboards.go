package handlers

import (
	"fmt"

	"whereismychildren/database"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

func GetMainKeyboard() tgbotapi.ReplyKeyboardMarkup {
	return tgbotapi.NewReplyKeyboard(
		tgbotapi.NewKeyboardButtonRow(
			tgbotapi.NewKeyboardButton("Зафиксировать уход"),
			tgbotapi.NewKeyboardButton("Внеплановая деятельность"),
		),
		tgbotapi.NewKeyboardButtonRow(
			tgbotapi.NewKeyboardButton("Где подчинённые"),
			tgbotapi.NewKeyboardButton("Статистика"),
		),
	)
}

func CreateSubordinateSelectionKeyboard(subordinates []database.Subordinate) tgbotapi.InlineKeyboardMarkup {
	var rows [][]tgbotapi.InlineKeyboardButton

	// Создаем кнопки в две колонки для компактности
	for i := 0; i < len(subordinates); i += 2 {
		var row []tgbotapi.InlineKeyboardButton

		// Первая кнопка в ряду
		btn1 := tgbotapi.NewInlineKeyboardButtonData(
			fmt.Sprintf("%s %s", subordinates[i].LastName, subordinates[i].FirstName),
			fmt.Sprintf("select_sub_%d", subordinates[i].ID),
		)
		row = append(row, btn1)

		// Вторая кнопка в ряду (если есть)
		if i+1 < len(subordinates) {
			btn2 := tgbotapi.NewInlineKeyboardButtonData(
				fmt.Sprintf("%s %s", subordinates[i+1].LastName, subordinates[i+1].FirstName),
				fmt.Sprintf("select_sub_%d", subordinates[i+1].ID),
			)
			row = append(row, btn2)
		}

		rows = append(rows, row)
	}

	return tgbotapi.NewInlineKeyboardMarkup(rows...)
}
