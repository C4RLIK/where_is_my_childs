package excel

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"whereismychildren/database"

	"github.com/tealeg/xlsx"
	"github.com/xuri/excelize/v2"
)

type ExcelProcessor struct {
	db *database.DB
}

func NewExcelProcessor(db *database.DB) *ExcelProcessor {
	return &ExcelProcessor{db: db}
}

func (ep *ExcelProcessor) ProcessExcelFile(filePath string) ([]database.Subordinate, error) {
	f, err := excelize.OpenFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to open excel file: %v", err)
	}
	defer f.Close()

	// Получаем имя первого листа
	sheets := f.GetSheetList()
	if len(sheets) == 0 {
		return nil, fmt.Errorf("no sheets found in excel file")
	}

	sheetName := sheets[0]
	rows, err := f.GetRows(sheetName)
	if err != nil {
		return nil, fmt.Errorf("failed to get rows: %v", err)
	}

	var newSubordinates []database.Subordinate
	existingSubs, err := ep.db.GetAllSubordinates()
	if err != nil {
		return nil, err
	}

	// Создаем мапу для быстрой проверки существующих подчиненных
	existingMap := make(map[string]bool)
	for _, sub := range existingSubs {
		key := fmt.Sprintf("%s|%s|%s", sub.LastName, sub.FirstName, sub.MiddleName)
		existingMap[key] = true
	}

	for rowIndex, row := range rows {
		// Пропускаем заголовок (первую строку)
		if rowIndex == 0 {
			continue
		}

		// Проверяем, что в строке достаточно данных
		if len(row) < 3 {
			log.Printf("Skipping row %d: not enough columns", rowIndex+1)
			continue
		}

		lastName := strings.TrimSpace(row[0])
		firstName := strings.TrimSpace(row[1])
		middleName := strings.TrimSpace(row[2])

		// Пропускаем пустые строки
		if lastName == "" && firstName == "" {
			continue
		}

		// Проверяем, существует ли уже такой подчиненный
		key := fmt.Sprintf("%s|%s|%s", lastName, firstName, middleName)
		if existingMap[key] {
			log.Printf("Subordinate already exists: %s %s %s", lastName, firstName, middleName)
			continue
		}

		sub := database.Subordinate{
			LastName:   lastName,
			FirstName:  firstName,
			MiddleName: middleName,
		}

		if _, err := ep.db.AddSubordinate(sub); err != nil {
			log.Printf("Failed to add subordinate %s: %v", key, err)
			continue
		}

		newSubordinates = append(newSubordinates, sub)
		existingMap[key] = true // Добавляем в мапу, чтобы избежать дубликатов в этом файле
	}

	return newSubordinates, nil
}
func (ep *ExcelProcessor) ExportToExcel(data []struct {
	Subordinate  database.Subordinate
	LeaveTime    *time.Time
	ActivityTime *time.Time
	ActivityDesc *string
	Date         time.Time
}) (string, error) {
	file := xlsx.NewFile()
	sheet, err := file.AddSheet("Статистика")
	if err != nil {
		return "", err
	}

	// Заголовки
	headerRow := sheet.AddRow()
	headers := []string{"Дата", "Фамилия", "Имя", "Отчество", "Время ухода", "Время деятельности", "Описание деятельности"}
	for _, header := range headers {
		cell := headerRow.AddCell()
		cell.Value = header
	}

	// Данные
	for _, item := range data {
		row := sheet.AddRow()

		// Дата
		dateCell := row.AddCell()
		dateCell.Value = item.Date.Format("02.01.2006")

		// ФИО
		row.AddCell().Value = item.Subordinate.LastName
		row.AddCell().Value = item.Subordinate.FirstName
		row.AddCell().Value = item.Subordinate.MiddleName

		// Время ухода
		leaveCell := row.AddCell()
		if item.LeaveTime != nil {
			leaveCell.Value = item.LeaveTime.Format("15:04")
		}

		// Время деятельности
		activityCell := row.AddCell()
		if item.ActivityTime != nil {
			activityCell.Value = item.ActivityTime.Format("15:04")
		}

		// Описание
		descCell := row.AddCell()
		if item.ActivityDesc != nil {
			descCell.Value = *item.ActivityDesc
		}
	}

	// Сохраняем файл
	filename := fmt.Sprintf("statistics_export_%s.xlsx", time.Now().Format("20060102_150405"))
	filepath := filepath.Join(os.TempDir(), filename)

	if err := file.Save(filepath); err != nil {
		return "", err
	}

	return filepath, nil
}
func (ep *ExcelProcessor) DownloadAndProcessExcel(fileURL, fileID string) ([]database.Subordinate, error) {
	// Создаем временный файл
	tmpFile := filepath.Join(os.TempDir(), fmt.Sprintf("excel_%s.xlsx", fileID))
	defer os.Remove(tmpFile)

	// Скачиваем файл (упрощенная версия)
	// В реальном приложении нужно использовать http.Get для скачивания по fileURL
	// Здесь предполагается, что файл уже доступен по fileURL

	// Для демонстрации просто создаем пустой файл
	file, err := os.Create(tmpFile)
	if err != nil {
		return nil, err
	}
	file.Close()

	// Обрабатываем файл
	return ep.ProcessExcelFile(tmpFile)
}
