package utils

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

func ParseTime(input string) (time.Time, error) {
	now := time.Now()

	// Приводим к нижнему регистру для проверки
	lowerInput := strings.ToLower(input)

	if strings.Contains(lowerInput, "сейчас") {
		return now, nil
	}

	// Парсинг времени в формате HH:MM
	timeRegex := regexp.MustCompile(`(\d{1,2}):(\d{2})`)
	matches := timeRegex.FindStringSubmatch(input)
	if len(matches) == 3 {
		hour := parseInt(matches[1])
		minute := parseInt(matches[2])

		// Проверяем валидность времени
		if hour < 0 || hour > 23 || minute < 0 || minute > 59 {
			return now, fmt.Errorf("неверное время: %02d:%02d", hour, minute)
		}

		// Создаем время с текущей датой
		return time.Date(now.Year(), now.Month(), now.Day(), hour, minute, 0, 0, time.Local), nil
	}

	return now, fmt.Errorf("неверный формат времени. Используйте 'сейчас' или ЧЧ:MM")
}

func ParseName(input string) (string, string) {
	words := strings.Fields(input)
	if len(words) == 0 {
		return "", ""
	}
	if len(words) == 1 {
		return words[0], ""
	}
	return words[0], words[1]
}

func ContainsTime(input string) bool {
	timeRegex := regexp.MustCompile(`\d{1,2}:\d{2}`)
	return timeRegex.MatchString(input) || strings.Contains(strings.ToLower(input), "сейчас")
}

func parseInt(s string) int {
	var result int
	fmt.Sscanf(s, "%d", &result)
	return result
}

func ParseDate(dateStr string) (time.Time, error) {
	now := time.Now()

	switch strings.ToLower(dateStr) {
	case "сегодня":
		return now, nil
	case "вчера":
		return now.AddDate(0, 0, -1), nil
	default:
		// Пытаемся разные форматы дат
		formats := []string{"02.01.2006", "02.01.06", "2006-01-02", "02/01/2006"}
		for _, format := range formats {
			if t, err := time.Parse(format, dateStr); err == nil {
				return t, nil
			}
		}
		return time.Time{}, fmt.Errorf("неверный формат даты")
	}
}

func DownloadFile(url, filepath string) error {
	resp, err := http.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	out, err := os.Create(filepath)
	if err != nil {
		return err
	}
	defer out.Close()

	_, err = io.Copy(out, resp.Body)
	return err
}
func GetFileExtension(filename string) string {
	return strings.ToLower(filepath.Ext(filename))
}
func IsExcelFile(filename string) bool {
	ext := GetFileExtension(filename)
	return ext == ".xlsx" || ext == ".xls"
}
func RoundTimeToMinute(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), t.Day(),
		t.Hour(), t.Minute(), 0, 0, t.Location())
}
func GetMoscowTime() time.Time {
	// Пытаемся загрузить локацию Москвы
	location, err := time.LoadLocation("Europe/Moscow")
	if err != nil {
		// Если не удалось, используем UTC+3 (Московское время)
		location = time.FixedZone("MSK", 3*60*60)
	}
	return time.Now().In(location)
}
