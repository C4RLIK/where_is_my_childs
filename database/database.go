package database

import (
	"database/sql"
	"fmt"
	"log"
	"strconv"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

type DB struct {
	*sql.DB
}

func NewDB(dbPath string) (*DB, error) {
	db, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		return nil, err
	}

	if err := db.Ping(); err != nil {
		return nil, err
	}

	if err := InitDB(db); err != nil {
		return nil, err
	}

	return &DB{db}, nil
}

// Методы для работы с подчиненными
func (db *DB) AddSubordinate(sub Subordinate) (int, error) {
	result, err := db.Exec(
		"INSERT INTO subordinates (last_name, first_name, middle_name) VALUES (?, ?, ?)",
		sub.LastName, sub.FirstName, sub.MiddleName,
	)
	if err != nil {
		return 0, err
	}

	id, err := result.LastInsertId()
	return int(id), err
}

func (db *DB) GetSubordinateByID(id int) (Subordinate, error) {
	var sub Subordinate
	err := db.QueryRow(
		"SELECT id, last_name, first_name, middle_name FROM subordinates WHERE id = ?",
		id,
	).Scan(&sub.ID, &sub.LastName, &sub.FirstName, &sub.MiddleName)
	return sub, err
}

func (db *DB) FindSubordinatesByName(lastName, firstName string) ([]Subordinate, error) {
	log.Printf("Partial search: lastName='%s', firstName='%s'", lastName, firstName)

	query := `
		SELECT id, last_name, first_name, middle_name 
		FROM subordinates 
		WHERE LOWER(last_name) LIKE LOWER(?) OR LOWER(first_name) LIKE LOWER(?)
		ORDER BY last_name, first_name
	`
	likeTerm := "%" + lastName + "%"
	likeTerm2 := "%" + firstName + "%"

	rows, err := db.Query(query, likeTerm, likeTerm2)
	if err != nil {
		log.Printf("Error querying subordinates: %v", err)
		return nil, err
	}
	defer rows.Close()

	var subordinates []Subordinate
	for rows.Next() {
		var sub Subordinate
		if err := rows.Scan(&sub.ID, &sub.LastName, &sub.FirstName, &sub.MiddleName); err != nil {
			log.Printf("Error scanning subordinate: %v", err)
			continue
		}
		subordinates = append(subordinates, sub)
		log.Printf("Found subordinate: %s %s %s (ID: %d)",
			sub.LastName, sub.FirstName, sub.MiddleName, sub.ID)
	}

	log.Printf("Total subordinates found: %d", len(subordinates))
	return subordinates, nil
}

func (db *DB) GetAllSubordinates() ([]Subordinate, error) {
	rows, err := db.Query("SELECT id, last_name, first_name, middle_name FROM subordinates")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var subordinates []Subordinate
	for rows.Next() {
		var sub Subordinate
		if err := rows.Scan(&sub.ID, &sub.LastName, &sub.FirstName, &sub.MiddleName); err != nil {
			return nil, err
		}
		subordinates = append(subordinates, sub)
	}

	return subordinates, nil
}
func (db *DB) RemoveConflictingRecords(subordinateID int, date time.Time) error {
	dateStr := date.Format("2006-01-02")

	// Удаляем уход, если добавляем внеплановую деятельность
	_, err := db.Exec(`
        DELETE FROM leaves 
        WHERE subordinate_id = ? AND DATE(leave_time) = ?
    `, subordinateID, dateStr)

	if err != nil {
		return err
	}

	// Удаляем внеплановую деятельность, если добавляем уход
	_, err = db.Exec(`
        DELETE FROM unplanned_activities 
        WHERE subordinate_id = ? AND DATE(activity_time) = ?
    `, subordinateID, dateStr)

	return err
}

// Методы для работы с уходами
func (db *DB) AddLeave(subordinateID int, leaveTime time.Time) error {
	today := leaveTime.Format("2006-01-02")
	roundedTime := time.Date(leaveTime.Year(), leaveTime.Month(), leaveTime.Day(),
		leaveTime.Hour(), leaveTime.Minute(), 0, 0, leaveTime.Location())

	log.Printf("Adding leave for subordinate %d at %s", subordinateID, roundedTime.Format("15:04"))

	// Проверяем вручную, есть ли уже запись на сегодня
	var existingID int
	err := db.QueryRow(`
		SELECT id FROM leaves 
		WHERE subordinate_id = ? AND DATE(leave_time) = ?
	`, subordinateID, today).Scan(&existingID)

	switch {
	case err == nil:
		log.Printf("Updating existing leave record %d", existingID)
		_, err = db.Exec(
			"UPDATE leaves SET leave_time = ? WHERE id = ?",
			leaveTime, existingID,
		)
		return err
	case err == sql.ErrNoRows:
		log.Printf("Creating new leave record")
		_, err = db.Exec(
			"INSERT INTO leaves (subordinate_id, leave_time) VALUES (?, ?)",
			subordinateID, roundedTime, // используем округленное время
		)
		return err
	default:
		return err
	}
}

func (db *DB) GetTodayLeaves() (map[int]time.Time, error) {
	today := time.Now().Format("2006-01-02")
	log.Printf("Getting leaves for today: %s", today)

	leaves := make(map[int]time.Time)

	rows, err := db.Query(`
        SELECT subordinate_id, leave_time 
        FROM leaves 
        WHERE DATE(leave_time) = ?
    `, today)
	if err != nil {
		log.Printf("Error querying leaves: %v", err)
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var subordinateID int
		var leaveTime time.Time
		if err := rows.Scan(&subordinateID, &leaveTime); err != nil {
			log.Printf("Error scanning leave row: %v", err)
			continue
		}
		// ОКРУГЛЯЕМ ВРЕМЯ ДО МИНУТ (убираем секунды и миллисекунды)
		roundedTime := time.Date(leaveTime.Year(), leaveTime.Month(), leaveTime.Day(),
			leaveTime.Hour(), leaveTime.Minute(), 0, 0, leaveTime.Location())
		leaves[subordinateID] = roundedTime
		log.Printf("Found leave - Subordinate ID: %d, Time: %s", subordinateID, roundedTime.Format("15:04"))
	}

	log.Printf("Total leaves found: %d", len(leaves))
	return leaves, nil
}

// Методы для работы с внеплановой деятельностью
func (db *DB) AddUnplannedActivity(subordinateID int, activityTime time.Time, description string) error {
	// Проверяем длину описания
	roundedTime := time.Date(activityTime.Year(), activityTime.Month(), activityTime.Day(),
		activityTime.Hour(), activityTime.Minute(), 0, 0, activityTime.Location())
	if len(description) > 1000 {
		description = description[:1000]
	}

	today := activityTime.Format("2006-01-02")

	// Проверяем вручную, есть ли уже запись на сегодня
	var existingID int
	err := db.QueryRow(`
		SELECT id FROM unplanned_activities 
		WHERE subordinate_id = ? AND DATE(activity_time) = ?
	`, subordinateID, today).Scan(&existingID)

	switch {
	case err == nil:
		// Обновляем существующую запись
		_, err = db.Exec(
			"UPDATE unplanned_activities SET activity_time = ?, description = ? WHERE id = ?",
			activityTime, description, existingID,
		)
		return err
	case err == sql.ErrNoRows:
		// Создаем новую запись
		_, err = db.Exec(
			"INSERT INTO unplanned_activities (subordinate_id, activity_time, description) VALUES (?, ?, ?)",
			subordinateID, roundedTime, description, // используем округленное время
		)
		return err
	default:
		return err
	}
}

func (db *DB) GetTodayUnplannedActivities() (map[int]UnplannedActivity, error) {
	today := time.Now().Format("2006-01-02")
	activities := make(map[int]UnplannedActivity)

	rows, err := db.Query(`
        SELECT subordinate_id, activity_time, description 
        FROM unplanned_activities 
        WHERE DATE(activity_time) = ?
    `, today)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var activity UnplannedActivity
		if err := rows.Scan(&activity.SubordinateID, &activity.ActivityTime, &activity.Description); err != nil {
			return nil, err
		}
		// ОКРУГЛЯЕМ ВРЕМЯ ДО МИНУТ
		roundedTime := time.Date(activity.ActivityTime.Year(), activity.ActivityTime.Month(), activity.ActivityTime.Day(),
			activity.ActivityTime.Hour(), activity.ActivityTime.Minute(), 0, 0, activity.ActivityTime.Location())
		activity.ActivityTime = roundedTime
		activities[activity.SubordinateID] = activity
	}

	return activities, nil
}

// Методы для статистики
func (db *DB) GetLeavesByDate(date time.Time) ([]struct {
	Subordinate Subordinate
	LeaveTime   time.Time
}, error) {
	var result []struct {
		Subordinate Subordinate
		LeaveTime   time.Time
	}

	dateStr := date.Format("2006-01-02")

	rows, err := db.Query(`
		SELECT s.id, s.last_name, s.first_name, s.middle_name, l.leave_time
		FROM leaves l
		JOIN subordinates s ON l.subordinate_id = s.id
		WHERE DATE(l.leave_time) = ?
		ORDER BY l.leave_time
	`, dateStr)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var item struct {
			Subordinate Subordinate
			LeaveTime   time.Time
		}
		if err := rows.Scan(
			&item.Subordinate.ID,
			&item.Subordinate.LastName,
			&item.Subordinate.FirstName,
			&item.Subordinate.MiddleName,
			&item.LeaveTime,
		); err != nil {
			return nil, err
		}
		result = append(result, item)
	}

	return result, nil
}

func (db *DB) GetAllDataForExport() ([]struct {
	Subordinate  Subordinate
	LeaveTime    *time.Time
	ActivityTime *time.Time
	ActivityDesc *string
	Date         time.Time
}, error) {
	var result []struct {
		Subordinate  Subordinate
		LeaveTime    *time.Time
		ActivityTime *time.Time
		ActivityDesc *string
		Date         time.Time
	}

	// Получаем все уходы
	leaveRows, err := db.Query(`
		SELECT s.id, s.last_name, s.first_name, s.middle_name, 
		       l.leave_time, DATE(l.leave_time) as date
		FROM leaves l
		JOIN subordinates s ON l.subordinate_id = s.id
		ORDER BY date, s.last_name, s.first_name
	`)
	if err != nil {
		return nil, err
	}
	defer leaveRows.Close()

	// Собираем данные в мапу для удобства
	dataMap := make(map[string]map[time.Time]struct {
		LeaveTime    *time.Time
		ActivityTime *time.Time
		ActivityDesc *string
	})

	for leaveRows.Next() {
		var sub Subordinate
		var leaveTime time.Time
		var date time.Time

		if err := leaveRows.Scan(
			&sub.ID, &sub.LastName, &sub.FirstName, &sub.MiddleName,
			&leaveTime, &date,
		); err != nil {
			return nil, err
		}

		key := fmt.Sprintf("%d", sub.ID)
		if dataMap[key] == nil {
			dataMap[key] = make(map[time.Time]struct {
				LeaveTime    *time.Time
				ActivityTime *time.Time
				ActivityDesc *string
			})
		}

		entry := dataMap[key][date]
		entry.LeaveTime = &leaveTime
		dataMap[key][date] = entry
	}

	// Получаем все внеплановые деятельности
	activityRows, err := db.Query(`
		SELECT s.id, s.last_name, s.first_name, s.middle_name, 
		       u.activity_time, u.description, DATE(u.activity_time) as date
		FROM unplanned_activities u
		JOIN subordinates s ON u.subordinate_id = s.id
		ORDER BY date, s.last_name, s.first_name
	`)
	if err != nil {
		return nil, err
	}
	defer activityRows.Close()

	for activityRows.Next() {
		var sub Subordinate
		var activityTime time.Time
		var description string
		var date time.Time

		if err := activityRows.Scan(
			&sub.ID, &sub.LastName, &sub.FirstName, &sub.MiddleName,
			&activityTime, &description, &date,
		); err != nil {
			return nil, err
		}

		key := fmt.Sprintf("%d", sub.ID)
		if dataMap[key] == nil {
			dataMap[key] = make(map[time.Time]struct {
				LeaveTime    *time.Time
				ActivityTime *time.Time
				ActivityDesc *string
			})
		}

		entry := dataMap[key][date]
		entry.ActivityTime = &activityTime
		entry.ActivityDesc = &description
		dataMap[key][date] = entry
	}

	// Преобразуем мапу в слайс
	for subID, dates := range dataMap {
		id, _ := strconv.Atoi(subID)
		sub, _ := db.GetSubordinateByID(id)

		for date, data := range dates {
			result = append(result, struct {
				Subordinate  Subordinate
				LeaveTime    *time.Time
				ActivityTime *time.Time
				ActivityDesc *string
				Date         time.Time
			}{
				Subordinate:  sub,
				LeaveTime:    data.LeaveTime,
				ActivityTime: data.ActivityTime,
				ActivityDesc: data.ActivityDesc,
				Date:         date,
			})
		}
	}

	return result, nil
}
func (db *DB) FindSubordinatesByExactName(lastName, firstName string) ([]Subordinate, error) {
	log.Printf("Exact search: lastName='%s', firstName='%s'", lastName, firstName)

	query := `
		SELECT id, last_name, first_name, middle_name 
		FROM subordinates 
		WHERE LOWER(last_name) = LOWER(?) OR LOWER(first_name) = LOWER(?)
		ORDER BY last_name, first_name
	`
	rows, err := db.Query(query, lastName, firstName)
	if err != nil {
		log.Printf("Error querying exact subordinates: %v", err)
		return nil, err
	}
	defer rows.Close()

	var subordinates []Subordinate
	for rows.Next() {
		var sub Subordinate
		if err := rows.Scan(&sub.ID, &sub.LastName, &sub.FirstName, &sub.MiddleName); err != nil {
			log.Printf("Error scanning exact subordinate: %v", err)
			continue
		}
		subordinates = append(subordinates, sub)
		log.Printf("Found exact subordinate: %s %s %s (ID: %d)",
			sub.LastName, sub.FirstName, sub.MiddleName, sub.ID)
	}

	log.Printf("Total exact subordinates found: %d", len(subordinates))
	return subordinates, nil
}
func (db *DB) FindSubordinatesByFullName(lastName, firstName string) ([]Subordinate, error) {
	log.Printf("Full name search: lastName='%s', firstName='%s'", lastName, firstName)

	query := `
		SELECT id, last_name, first_name, middle_name 
		FROM subordinates 
		WHERE (LOWER(last_name) = LOWER(?) AND LOWER(first_name) = LOWER(?))
		   OR (LOWER(last_name) = LOWER(?) AND LOWER(first_name) LIKE LOWER(?))
		   OR (LOWER(last_name) LIKE LOWER(?) AND LOWER(first_name) = LOWER(?))
		ORDER BY last_name, first_name
	`
	likeLastName := "%" + lastName + "%"
	likeFirstName := "%" + firstName + "%"

	rows, err := db.Query(query,
		lastName, firstName,
		lastName, likeFirstName,
		likeLastName, firstName,
	)
	if err != nil {
		log.Printf("Error querying full name: %v", err)
		return nil, err
	}
	defer rows.Close()

	var subordinates []Subordinate
	for rows.Next() {
		var sub Subordinate
		if err := rows.Scan(&sub.ID, &sub.LastName, &sub.FirstName, &sub.MiddleName); err != nil {
			log.Printf("Error scanning full name subordinate: %v", err)
			continue
		}
		subordinates = append(subordinates, sub)
		log.Printf("Found full name subordinate: %s %s %s (ID: %d)",
			sub.LastName, sub.FirstName, sub.MiddleName, sub.ID)
	}

	log.Printf("Total full name subordinates found: %d", len(subordinates))
	return subordinates, nil
}
func (db *DB) FindSubordinatesBySingleTerm(term string) ([]Subordinate, error) {
	log.Printf("Single term search: '%s'", term)

	query := `
		SELECT id, last_name, first_name, middle_name 
		FROM subordinates 
		WHERE LOWER(last_name) = LOWER(?) OR LOWER(first_name) = LOWER(?)
		ORDER BY last_name, first_name
	`
	rows, err := db.Query(query, term, term)
	if err != nil {
		log.Printf("Error querying single term: %v", err)
		return nil, err
	}
	defer rows.Close()

	var subordinates []Subordinate
	for rows.Next() {
		var sub Subordinate
		if err := rows.Scan(&sub.ID, &sub.LastName, &sub.FirstName, &sub.MiddleName); err != nil {
			log.Printf("Error scanning single term subordinate: %v", err)
			continue
		}
		subordinates = append(subordinates, sub)
		log.Printf("Found single term subordinate: %s %s %s (ID: %d)",
			sub.LastName, sub.FirstName, sub.MiddleName, sub.ID)
	}

	log.Printf("Total single term subordinates found: %d", len(subordinates))
	return subordinates, nil
}
