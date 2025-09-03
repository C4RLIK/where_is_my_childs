package database

import (
	"database/sql"
	"log"
)

func InitDB(db *sql.DB) error {
	// Таблица подчиненных
	_, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS subordinates (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			last_name TEXT NOT NULL,
			first_name TEXT NOT NULL,
			middle_name TEXT,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			UNIQUE(last_name, first_name, middle_name)
		)
	`)
	if err != nil {
		return err
	}

	// Таблица уходов
	_, err = db.Exec(`
		CREATE TABLE IF NOT EXISTS leaves (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			subordinate_id INTEGER NOT NULL,
			leave_time DATETIME NOT NULL,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			FOREIGN KEY (subordinate_id) REFERENCES subordinates (id)
		)
	`)
	if err != nil {
		return err
	}

	// Создаем индекс для быстрого поиска уходов по дате и подчиненному
	_, err = db.Exec(`
		CREATE INDEX IF NOT EXISTS idx_leaves_subordinate_date 
		ON leaves (subordinate_id, DATE(leave_time))
	`)
	if err != nil {
		return err
	}

	// Таблица внеплановой деятельности
	_, err = db.Exec(`
		CREATE TABLE IF NOT EXISTS unplanned_activities (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			subordinate_id INTEGER NOT NULL,
			activity_time DATETIME NOT NULL,
			description TEXT NOT NULL,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			FOREIGN KEY (subordinate_id) REFERENCES subordinates (id)
		)
	`)
	if err != nil {
		return err
	}

	// Создаем индекс для быстрого поиска деятельности по дате и подчиненному
	_, err = db.Exec(`
		CREATE INDEX IF NOT EXISTS idx_activities_subordinate_date 
		ON unplanned_activities (subordinate_id, DATE(activity_time))
	`)
	if err != nil {
		return err
	}

	log.Println("Database initialized successfully")
	return nil
}
